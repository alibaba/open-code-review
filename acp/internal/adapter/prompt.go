// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func (a *Agent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	s, task, e := a.beginTurn(ctx, p.SessionId)
	if e != nil {
		return acp.PromptResponse{}, e
	}
	defer a.endTurn(task, p.SessionId, s)
	text, paths, e := promptContent(s.cwd, p.Prompt)
	if e != nil {
		s.state.Clear()
		return a.sendRejection(task, p.SessionId, e.Error())
	}
	if len(paths) > 0 && strings.TrimSpace(text) == "" {
		s.state.Clear()
		return a.sendTerminal(task, p.SessionId, "Send /scan with these resources to scan the selected files.", acp.StopReasonEndTurn)
	}
	r, e := a.parser.WithRepo(intent.GitRepo{Dir: s.cwd}).Parse(task, text, s.state)
	if task.Err() != nil {
		if errors.Is(task.Err(), context.DeadlineExceeded) {
			return a.reportTimeout(ctx, p.SessionId)
		}
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if e != nil {
		return a.sendRejection(task, p.SessionId, "Invalid scan or review options: "+e.Error())
	}
	if r.Kind == intent.KindClarify {
		return a.sendTerminal(task, p.SessionId, r.Clarify.Question, acp.StopReasonEndTurn)
	}
	if r.Kind == intent.KindReject {
		return a.sendRejection(task, p.SessionId, r.Reject.Reason+" "+r.Reject.Hint)
	}
	if len(paths) > 0 {
		if r.Scan == nil || len(r.Scan.Paths) > 0 {
			return a.sendRejection(task, p.SessionId, "Resource links select files only with /scan without --path. Remove the links for repository review or explicit --path scans.")
		}
		r.Scan.Paths = paths
	}
	var args []string
	if r.Review != nil {
		args, e = contract.BuildReviewArgs(r.Review)
	} else {
		args, e = contract.BuildScanArgs(r.Scan)
	}
	if e != nil {
		return a.sendRejection(task, p.SessionId, "Invalid scan or review options: "+e.Error())
	}
	root, e := resultRoot(task, s.cwd, args)
	if e != nil {
		if task.Err() != nil {
			return a.finishSend(task, p.SessionId, task.Err(), acp.StopReasonEndTurn)
		}
		return a.sendRejection(task, p.SessionId, "Cannot determine the OCR target directory. Review requires a Git repository; check the session directory or use /scan for a non-Git directory.")
	}
	// Output failures stop OCR without marking the whole turn as user-cancelled.
	runTask, stopRunner := context.WithCancel(task)
	defer stopRunner()
	o, progressErr := a.collectProgress(runTask, stopRunner, p.SessionId, orchestrator.Request{CWD: s.cwd, Args: args})
	if o.Kind == orchestrator.OutcomeTimedOut || errors.Is(task.Err(), context.DeadlineExceeded) {
		return a.reportTimeout(ctx, p.SessionId)
	}
	if progressErr != nil {
		return a.finishSend(task, p.SessionId, progressErr, acp.StopReasonEndTurn)
	}
	if o.Kind == orchestrator.OutcomeCancelled || task.Err() != nil {
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if o.Result != nil {
		text := formatResultAtRoot(o.Result, root)
		if o.Err != nil {
			text += "\nOCR failed: " + o.Err.Error()
		}
		resp, sendErr := a.sendTerminal(task, p.SessionId, text, acp.StopReasonEndTurn)
		if sendErr != nil {
			return resp, sendErr
		}
		if o.Err != nil {
			return acp.PromptResponse{}, o.Err
		}
		return resp, nil
	}
	if o.Err != nil {
		return acp.PromptResponse{}, o.Err
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

// Application-level rejections are completed guidance, not model refusals.
// Clients such as Zed discard the turn's messages on the refusal stop reason.
func (a *Agent) sendRejection(ctx context.Context, id acp.SessionId, text string) (acp.PromptResponse, error) {
	response, err := a.sendTerminal(ctx, id, text, acp.StopReasonEndTurn)
	if err == nil && response.StopReason == acp.StopReasonEndTurn && response.Meta == nil && ctx.Err() == nil {
		response.Meta = map[string]any{"ocr": map[string]any{"kind": "rejected"}}
	}
	return response, err
}

func (a *Agent) sendTerminal(ctx context.Context, id acp.SessionId, text string, reason acp.StopReason) (acp.PromptResponse, error) {
	err := a.update(ctx, id, text)
	return a.finishSend(ctx, id, err, reason)
}

func (a *Agent) finishSend(ctx context.Context, id acp.SessionId, err error, reason acp.StopReason) (acp.PromptResponse, error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return a.reportTimeout(ctx, id)
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}
	if err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: reason}, nil
}

// ACP v1 has no timeout stop reason. Preserve the machine-readable cause in
// metadata instead of using max_turn_requests, which means a different limit.
func timeoutResponse() acp.PromptResponse {
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn, Meta: map[string]any{"ocr": map[string]any{"kind": "timed_out", "retryable": true}}}
}

func (a *Agent) reportTimeout(ctx context.Context, id acp.SessionId) (acp.PromptResponse, error) {
	// The task deadline has expired, but its connection may still be usable.
	notice, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	// A best-effort notice must not replace the machine-readable turn outcome.
	// A broken transport still closes normally and may prevent response delivery.
	_ = a.update(notice, id, "OCR timed out before this turn completed. Retry or increase --turn-timeout.")
	return timeoutResponse(), nil
}
