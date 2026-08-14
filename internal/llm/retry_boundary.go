// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/openai/openai-go/v3/packages/ssestream"
)

// errRequestPanicked finalizes a logical request whose client call panicked.
//
// The panic itself is re-raised unchanged; this sentinel exists only so the
// request has a recorded outcome. Without it the entry stays unfinalized and
// Freeze fails for the whole run: agent.go recovers per file and keeps going, so
// one panicking file would erase every other file's retry records.
var errRequestPanicked = errors.New("llm request panicked")

// streamIntegrityError is a stream-completeness failure detected by OCR itself
// rather than reported by the transport or the SDK.
//
// It exists because classification may only read HTTP status and Go error types,
// never message text. The three conditions it carries used to be bare
// fmt.Errorf values, which were indistinguishable from any other error at the
// client boundary. The rendered messages are unchanged.
type streamIntegrityError struct{ reason string }

func (e *streamIntegrityError) Error() string { return "OpenAI streaming response " + e.reason }

func (e *streamIntegrityError) Unwrap() error { return io.ErrUnexpectedEOF }

// classifyBoundaryError classifies an error that only surfaced after the SDK
// retry loop finished, and reports whether it recognized it at all.
//
// The false return is the point: an unrecognized error is left alone rather than
// bucketed as unknown, because the only remaining way to tell errors apart here
// would be their message text. A missing correction leaves the attempt as the
// observed HTTP 200, which is at least a fact; a guessed one is not.
//
// Both JSON error types are matched with errors.As because the SDKs wrap them
// ("error parsing response json: %w"). Decoding happens after the retry loop in
// both SDKs, so this path never coincides with an SDK retry.
func classifyBoundaryError(err error) (ErrorClass, FailurePhase, bool) {
	switch {
	case err == nil:
		return "", "", false
	case errors.Is(err, context.Canceled):
		return ErrorClassCancelled, FailurePhaseContext, true
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorClassTimeout, FailurePhaseContext, true
	case errors.Is(err, io.ErrUnexpectedEOF):
		return ErrorClassNetwork, FailurePhaseResponseDecode, true
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return ErrorClassUnknown, FailurePhaseResponseDecode, true
	}
	return "", "", false
}

// classifyStreamError classifies a failure that surfaced while consuming an
// already-established SSE stream. Unlike classifyBoundaryError it always returns
// a classification, because the caller already knows the failure came from the
// stream.
//
// Context errors keep FailurePhaseContext: cancelling mid-stream fails at the
// context, not in the stream, and the class and phase are independent enough
// that no cancelled/stream pairing is needed.
//
// Anything else is unknown/stream rather than the network/transport default that
// classifyAttempt would give. That default is sound for an opaque error at the
// transport phase, where the network stack is the overwhelmingly likely source;
// after HTTP 200 the same error could just as easily come from decoding or from
// the provider, so calling it network would be an unverified claim about the
// transport.
func classifyStreamError(err error) (ErrorClass, FailurePhase) {
	var integrityErr *streamIntegrityError
	var streamErr *ssestream.StreamError
	switch {
	case errors.As(err, &integrityErr), errors.As(err, &streamErr):
		return ErrorClassProvider, FailurePhaseStream
	case errors.Is(err, context.Canceled):
		return ErrorClassCancelled, FailurePhaseContext
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorClassTimeout, FailurePhaseContext
	}
	return ErrorClassUnknown, FailurePhaseStream
}

// reviseAttempt applies a client-boundary correction to the last attempt of the
// logical request identified by ctx.
//
// Requests without identity (scan, llm test) and a nil collector are no-ops, so
// no call site needs to check either. The correction is idempotent by way of
// ReviseLastAttempt's precondition: once the last attempt is an error, every
// later correction does nothing, which is what lets the inline stream and
// Responses corrections coexist with the generic one in the boundary defer.
func reviseAttempt(ctx context.Context, collector *RetryCollector, class ErrorClass, phase FailurePhase) {
	if collector == nil {
		return
	}
	meta, ok := RequestMetaFromContext(ctx)
	if !ok {
		return
	}
	collector.ReviseLastAttempt(meta, class, phase)
}

// finalizeRequest is the shared body of the client-boundary defer: correct the
// last attempt if the error only became visible after the SDK returned, then
// decide the request-level outcome.
//
// The order is load-bearing. Finalizing first would make the correction a
// "revised after Finalize" violation, which fails Freeze and drops the entire
// run's report — strictly worse than not correcting at all.
//
// parentCancelled reads the context the business layer passed in, and only
// context.Canceled counts: the per-attempt deadline from WithRequestTimeout must
// surface as failed, since attempt-level timeout already expresses it and
// conflating it with a user abort would misreport why the run stopped.
//
// Callers must invoke this exactly once per logical request. A second call is
// recorded as a violation, which is why only CompletionsWithCtx defers it and
// completionsStreaming must not defer its own.
func finalizeRequest(ctx context.Context, collector *RetryCollector, reqErr error) {
	if collector == nil {
		return
	}
	meta, ok := RequestMetaFromContext(ctx)
	if !ok {
		return
	}
	if class, phase, recognized := classifyBoundaryError(reqErr); recognized {
		collector.ReviseLastAttempt(meta, class, phase)
	}
	collector.Finalize(meta, reqErr, errors.Is(ctx.Err(), context.Canceled))
}
