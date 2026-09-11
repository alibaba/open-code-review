// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// parseNatural turns free text into an intent through one LLM tool call. There
// is no deterministic fallback: a failed call becomes a clarification or a
// rejection, never a keyword match.
func (p *Parser) parseNatural(ctx context.Context, text string, st *State) (Result, error) {
	if p.llm == nil {
		return RejectResult("natural-language parsing is unavailable because no parsing LLM is configured",
			"Use /review or /scan."), nil
	}

	req := LLMRequest{
		System: systemPrompt,
		User:   buildUserPrompt(text, st),
		Tool:   SubmitIntentTool(),
	}

	callCtx := ctx
	if p.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	call, err := p.llm.CallTool(callCtx, req)
	if err != nil {
		return RejectResult("could not parse the request with the LLM: "+err.Error(),
			"Use /review or /scan, or try again."), nil
	}
	if call.Name != submitIntentToolName {
		return RejectResult("the LLM returned an unexpected tool: "+call.Name,
			"Try again, or use /review or /scan."), nil
	}

	var raw rawIntent
	dec := json.NewDecoder(bytes.NewReader(call.Arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return RejectResult("the LLM returned a malformed intent: "+err.Error(),
			"Try again, or use /review or /scan."), nil
	}
	if err := ensureEOF(dec); err != nil {
		return RejectResult("the LLM returned trailing data after the intent: "+err.Error(),
			"Try again, or use /review or /scan."), nil
	}

	return p.applyRaw(ctx, st, &raw)
}

func ensureEOF(dec *json.Decoder) error {
	var extra json.RawMessage
	err := dec.Decode(&extra)
	if err == nil {
		return errors.New("extra JSON value")
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (p *Parser) applyRaw(ctx context.Context, st *State, raw *rawIntent) (Result, error) {
	switch raw.Action {
	case actionReview:
		if raw.Review == nil {
			p.clearPending(st)
			return RejectResult("the LLM returned action review without a review object",
				"Try again, or use /review."), nil
		}
		s := &pendingSlots{action: actionReview, extra: append([]string(nil), raw.Extra...)}
		switch raw.Review.Type {
		case reviewWorkspace:
			s.reviewType = reviewWorkspace
		case reviewRange:
			s.reviewType = reviewRange
			s.from, s.to = raw.Review.From, raw.Review.To
		case reviewCommit:
			s.reviewType = reviewCommit
			s.commit = raw.Review.Commit
		default:
			p.clearPending(st)
			return RejectResult("the LLM returned an unknown review type: "+raw.Review.Type,
				"Try again, or use /review."), nil
		}
		return p.finalizeReview(ctx, st, s)

	case actionScan:
		if raw.Scan == nil {
			p.clearPending(st)
			return RejectResult("the LLM returned action scan without a scan object",
				"Try again, or use /scan."), nil
		}
		s := &pendingSlots{
			action: actionScan,
			paths:  append([]string(nil), raw.Scan.Paths...),
			extra:  append([]string(nil), raw.Extra...),
		}
		return p.finalizeScan(ctx, st, s)

	case actionClarify:
		if strings.TrimSpace(raw.Question) == "" {
			p.clearPending(st)
			return RejectResult("the LLM returned clarify without a question",
				"Try again, or use /review or /scan."), nil
		}
		if partial := slotsFromRaw(raw); partial != nil {
			p.storePending(st, partial, raw.Missing)
		}
		return ClarifyResult(raw.Question, raw.Missing...), nil

	case actionReject:
		if strings.TrimSpace(raw.Reason) == "" {
			p.clearPending(st)
			return RejectResult("the LLM returned reject without a reason",
				"Try again, or use /review or /scan."), nil
		}
		p.clearPending(st)
		return RejectResult(raw.Reason, raw.Hint), nil

	default:
		p.clearPending(st)
		return RejectResult("the LLM returned an unknown action: "+raw.Action,
			"Try again, or use /review or /scan."), nil
	}
}

// slotsFromRaw preserves whatever partial slots a clarify call carried so the
// next turn can merge into them.
func slotsFromRaw(raw *rawIntent) *pendingSlots {
	if raw.Review != nil {
		s := &pendingSlots{action: actionReview, extra: append([]string(nil), raw.Extra...)}
		switch raw.Review.Type {
		case reviewRange:
			s.reviewType, s.from, s.to = reviewRange, raw.Review.From, raw.Review.To
		case reviewCommit:
			s.reviewType, s.commit = reviewCommit, raw.Review.Commit
		default:
			s.reviewType = reviewWorkspace
		}
		return s
	}
	if raw.Scan != nil {
		return &pendingSlots{
			action: actionScan,
			paths:  append([]string(nil), raw.Scan.Paths...),
			extra:  append([]string(nil), raw.Extra...),
		}
	}
	return nil
}
