// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

type waitingParser struct{ ready chan struct{} }

func TestTurnDeadlineIncludesParsing(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{}, intent.NewParser(waitingParser{make(chan struct{})}, nil))
	a.TurnTimeout = 20 * time.Millisecond
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	r, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("review please")}})
	if err != nil {
		t.Fatal(err)
	}
	if r.StopReason != acp.StopReasonEndTurn || r.Meta["ocr"].(map[string]any)["kind"] != "timed_out" {
		t.Fatalf("timeout response: %+v", r)
	}
}

func TestInterruptedCloseRemainsTrackedByShutdown(t *testing.T) {
	r := cleanupRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	a := NewAgent("ocr", r)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	p := acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}}
	go func() { _, _ = a.Prompt(context.Background(), p) }()
	<-r.ready
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.CloseSession(ctx, acp.CloseSessionRequest{SessionId: s.SessionId}); !errors.Is(err, context.Canceled) {
		t.Fatalf("close: %v", err)
	}
	if _, err := a.Prompt(context.Background(), p); err == nil {
		t.Fatal("closing session accepted work")
	}
	if err := a.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown lost running task: %v", err)
	}
	close(r.release)
	cleanup, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := a.Shutdown(cleanup); err != nil {
		t.Fatal(err)
	}
}

func (p waitingParser) CallTool(ctx context.Context, _ intent.LLMRequest) (intent.ToolCall, error) {
	close(p.ready)
	<-ctx.Done()
	return intent.ToolCall{}, ctx.Err()
}

func TestShutdownDeadlineAndCloseWait(t *testing.T) {
	r := cleanupRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	a := NewAgent("ocr", r)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	go func() {
		_, _ = a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	}()
	<-r.ready
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown: %v", err)
	}
	<-r.cancelled
	closed := make(chan error, 1)
	go func() {
		_, err := a.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: s.SessionId})
		closed <- err
	}()
	close(r.release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
}

func TestCancelDuringParsing(t *testing.T) {
	ready := make(chan struct{})
	a := NewAgent("ocr", fakeRunner{}, intent.NewParser(waitingParser{ready}, nil))
	s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan acp.PromptResponse, 1)
	go func() {
		r, _ := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("review please")}})
		result <- r
	}()
	<-ready
	if err := a.Cancel(context.Background(), acp.CancelNotification{SessionId: s.SessionId}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-result:
		if r.StopReason != acp.StopReasonCancelled {
			t.Fatalf("stop: %s", r.StopReason)
		}
	case <-time.After(time.Second):
		t.Fatal("parser did not cancel")
	}
}

func TestSessionIDsSurviveDeletion(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	one, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	two, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	original := a.sessions[two.SessionId]
	if _, err := a.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: one.SessionId}); err != nil {
		t.Fatal(err)
	}
	three, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	if three.SessionId == two.SessionId || a.sessions[two.SessionId] != original {
		t.Fatal("existing session overwritten")
	}
}

func TestCancelClearsIdleClarification(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	_, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review --from main")}})
	if err != nil {
		t.Fatal(err)
	}
	if a.sessions[s.SessionId].state.Pending() == nil {
		t.Fatal("expected pending clarification")
	}
	_ = a.Cancel(context.Background(), acp.CancelNotification{SessionId: s.SessionId})
	if a.sessions[s.SessionId].state.Pending() != nil {
		t.Fatal("cancel retained clarification")
	}
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

func TestShutdownWaitsForCleanup(t *testing.T) {
	r := cleanupRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	a := NewAgent("ocr", r)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	go func() {
		_, _ = a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	}()
	<-r.ready
	finished := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { finished <- a.Shutdown(ctx) }()
	<-r.cancelled
	select {
	case <-finished:
		t.Fatal("shutdown returned before cleanup")
	default:
	}
	close(r.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if _, err := a.NewSession(ctx, acp.NewSessionRequest{Cwd: testGitDir(t)}); err == nil {
		t.Fatal("shutdown accepted new session")
	}
}
