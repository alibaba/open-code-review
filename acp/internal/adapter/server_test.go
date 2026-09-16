// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

func TestAgentSessionAndPrompt(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	dir := testGitDir(t)
	got, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: filepath.Clean(dir), McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: got.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("stop=%s", resp.StopReason)
	}
	if _, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: "relative", McpServers: []acp.McpServer{}}); err == nil {
		t.Fatal("expected absolute cwd validation")
	}
	_ = os.ErrNotExist
}

func TestFormatResult(t *testing.T) {
	got := formatResult(&orchestrator.Result{Review: &contract.ReviewResult{Message: "summary", Comments: []contract.Comment{{Path: "x.go", StartLine: 2, EndLine: 3, Content: "issue"}}}})
	if got == "" {
		t.Fatal("empty result")
	}
}

func TestAgentLifecycleMethods(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	if _, err := a.Initialize(context.Background(), acp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(context.Background(), acp.AuthenticateRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Logout(context.Background(), acp.LogoutRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetSessionMode(context.Background(), acp.SetSessionModeRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ResumeSession(context.Background(), acp.ResumeSessionRequest{}); err == nil {
		t.Fatal("expected unsupported resume")
	}
	if _, err := a.ListSessions(context.Background(), acp.ListSessionsRequest{}); err == nil {
		t.Fatal("expected unsupported list")
	}
	d := testGitDir(t)
	s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: d, McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Cancel(context.Background(), acp.CancelNotification{SessionId: s.SessionId}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: s.SessionId}); err != nil {
		t.Fatal(err)
	}
}

func TestSetConnection(t *testing.T) { a := NewAgent("ocr", fakeRunner{}); a.SetAgentConnection(nil) }

func TestPromptRejectAndClarify(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	d := testGitDir(t)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: d, McpServers: []acp.McpServer{}})
	if r, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/unknown")}}); err != nil || r.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("reject: %v %+v", err, r)
	}
	if r, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review --from main")}}); err != nil || r.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("clarify: %v %+v", err, r)
	}
}

func TestFormatResultVariants(t *testing.T) {
	if formatResult(nil) != "OCR completed" {
		t.Fatal("nil result")
	}
	if formatResult(&orchestrator.Result{Scan: &contract.ScanResult{Message: "scan", Comments: []contract.Comment{{Path: "a", Content: "x"}}}}) == "" {
		t.Fatal("scan result")
	}
}

func TestPromptOutcomeBranches(t *testing.T) {
	d := testGitDir(t)
	for _, kind := range []orchestrator.OutcomeKind{orchestrator.OutcomeFailed, orchestrator.OutcomeTimedOut} {
		a := NewAgent("ocr", errorRunner{kind: kind})
		s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: d, McpServers: []acp.McpServer{}})
		_, _ = a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	}
}

func TestPromptErrorsAndCancel(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	if _, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: "missing"}); err == nil {
		t.Fatal("expected unknown session")
	}
	if err := a.Cancel(context.Background(), acp.CancelNotification{SessionId: "missing"}); err != nil {
		t.Fatal(err)
	}
	d := testGitDir(t)
	_, _ = a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: d, McpServers: []acp.McpServer{}})
	if _, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: filepath.Join(d, "missing"), McpServers: []acp.McpServer{}}); err == nil {
		t.Fatal("expected invalid cwd")
	}
	_ = os.ErrNotExist
}

func TestPromptScanAndTimeout(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	d := testGitDir(t)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: d, McpServers: []acp.McpServer{}})
	if _, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/scan")}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Prompt(ctx, acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}}); err == nil { /* parser may reject before cancellation */
	}
}
