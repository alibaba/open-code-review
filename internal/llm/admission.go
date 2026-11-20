// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// admissionScopeKey marks a context as belonging to an `ocr review` run. The
// admission gate admits only requests whose context carries this scope, which
// `context.WithoutCancel` children inherit along with every other value.
// Retry-report identity (RequestMeta) is deliberately NOT consulted here:
// admission and observability are separate contracts, and the review grace
// round carries no RequestMeta at all. `ocr scan` and `ocr llm test` never
// set the scope, so they stay ungated by construction.
type admissionScopeKey struct{}

// ContextWithAdmissionScope marks ctx (and everything derived from it) as
// gated by the provider admission middleware.
func ContextWithAdmissionScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, admissionScopeKey{}, true)
}

// HasAdmissionScope reports whether ctx belongs to a gated `ocr review` run.
// It is the read-side counterpart of ContextWithAdmissionScope and, like
// RequestMetaFromContext, exists so packages downstream of the client — chiefly
// internal/llmloop — can assert in their own tests that a request reached the
// client still carrying the review scope, and that scan's carry none. It is
// read-only and stays inside internal/.
func HasAdmissionScope(ctx context.Context) bool {
	v, ok := ctx.Value(admissionScopeKey{}).(bool)
	return ok && v
}

func hasAdmissionScope(ctx context.Context) bool {
	return HasAdmissionScope(ctx)
}

// admissionGate bounds the number of simultaneous in-flight provider attempts
// — attempts whose transport has not returned or whose response body is still
// open. It follows the internal/gitcmd.Runner semaphore idiom.
type admissionGate struct {
	sem chan struct{}
}

// newAdmissionGate returns nil for limit <= 0: a disabled gate mounts no
// middleware at all, keeping the ungated path byte-identical to before.
func newAdmissionGate(limit int) *admissionGate {
	if limit <= 0 {
		return nil
	}
	return &admissionGate{sem: make(chan struct{}, limit)}
}

func (g *admissionGate) acquire(ctx context.Context) error {
	select {
	case g.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *admissionGate) release() { <-g.sem }

// inFlight reports the number of held permits. It exists as a leak canary for
// tests: every scenario must drain back to zero.
func (g *admissionGate) inFlight() int { return len(g.sem) }

// newAdmissionMiddleware builds the admission hook in the shared middleware
// shape.
//
// Mount it OUTSIDE the retry observer (appended before it — options wrap the
// previous, so the earlier append is the outer layer): the permit is then
// acquired before the observer's startedAt timestamp, which keeps gate queue
// time out of the retry report's duration_to_headers_ms (that field measures
// transport time only). Queue time may legitimately show up in the measured
// gap between two attempts (observed_backoff_ms); see RecordAttempt.
//
// Per real SDK attempt (the SDK invokes middleware once per attempt inside
// its retry loop): acquire one permit with the request context; a cancelled
// acquisition returns without calling the transport and leaves no waiter or
// permit behind. On a transport error or a body-less response the permit is
// released immediately; otherwise ownership transfers to an idempotent body
// wrapper released at EOF or Close — so the SDK closing a retryable response
// before backoff frees the permit for the wait, and a streaming body holds
// its permit until the stream ends.
func newAdmissionMiddleware(gate *admissionGate) retryObserver {
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if gate == nil || !hasAdmissionScope(req.Context()) {
			return next(req)
		}
		if err := gate.acquire(req.Context()); err != nil {
			return nil, err
		}
		var once sync.Once
		release := func() { once.Do(gate.release) }
		handedOff := false
		// A panic between acquisition and the body handoff would otherwise
		// leak the permit for the rest of the process: the clients re-raise
		// panics at their boundary, which unwinds through here.
		defer func() {
			if !handedOff {
				release()
			}
		}()
		res, err := next(req)
		if err != nil || res == nil || res.Body == nil {
			release()
			return res, err
		}
		res.Body = newReleasingBody(res.Body, release)
		handedOff = true
		return res, nil
	}
}

// releasingBody releases one admission permit exactly once, at the first of
// body EOF, any read error, or Close. Reads, read errors, and the underlying
// Close error all pass through unchanged.
type releasingBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func newReleasingBody(body io.ReadCloser, release func()) *releasingBody {
	return &releasingBody{ReadCloser: body, release: release}
}

func (b *releasingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		// EOF ends the body; any other read error terminal-errors it. Either
		// way nothing more can be waited for, so the permit frees now and a
		// later Close is a no-op via the once.
		b.once.Do(b.release)
	}
	return n, err
}

func (b *releasingBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}
