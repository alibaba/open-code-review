// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package presentation

import (
	"context"
	"fmt"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	e := make(chan orchestrator.Event)
	close(e)
	o := make(chan orchestrator.Outcome, 1)
	o <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted, Result: &orchestrator.Result{Review: &contract.ReviewResult{Message: "done"}}}
	close(o)
	return e, o
}

type errorRunner struct{ kind orchestrator.OutcomeKind }

func (r errorRunner) Run(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	e := make(chan orchestrator.Event, 1)
	e <- orchestrator.Event{Kind: orchestrator.EventWarning, Message: "warn"}
	close(e)
	o := make(chan orchestrator.Outcome, 1)
	o <- orchestrator.Outcome{Kind: r.kind, Err: fmt.Errorf("failure")}
	close(o)
	return e, o
}

type captureRunner struct{ requests []orchestrator.Request }

func (r *captureRunner) Run(ctx context.Context, request orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	r.requests = append(r.requests, request)
	return fakeRunner{}.Run(ctx, request)
}

type cleanupRunner struct{ ready, cancelled, release chan struct{} }

func (r cleanupRunner) Run(ctx context.Context, _ orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	events := make(chan orchestrator.Event)
	outcomes := make(chan orchestrator.Outcome, 1)
	go func() {
		close(r.ready)
		<-ctx.Done()
		close(r.cancelled)
		<-r.release
		close(events)
		outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCancelled}
		close(outcomes)
	}()
	return events, outcomes
}

type progressRunner func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome)

func (r progressRunner) Run(ctx context.Context, req orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	return r(ctx, req)
}

type progressSink func(context.Context, acp.SessionNotification) error

func notifySink(s progressSink) func(context.Context, acp.SessionId, acp.SessionUpdate) error {
	return func(ctx context.Context, id acp.SessionId, update acp.SessionUpdate) error {
		return s(ctx, acp.SessionNotification{SessionId: id, Update: update})
	}
}

func formatReport(result *orchestrator.Result) string {
	return FormatResult(result, "")
}
