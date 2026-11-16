// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/adapter/presentation"
	"github.com/alibaba/open-code-review/acp/internal/adapter/workspace"
	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func testGitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func unreadableLocalFile(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "unreadable.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Skipf("cannot remove file read permissions: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0600); err != nil {
			t.Errorf("restore file permissions: %v", err)
		}
	})
	f, err := os.Open(path)
	if err == nil {
		f.Close()
		t.Skip("current process can read files without read permission")
	}
	if !os.IsPermission(err) {
		t.Fatalf("expected permission denial, got %v", err)
	}
	return root, path
}

func TestUnreadableFindingKeepsTextWithoutNavigation(t *testing.T) {
	root, path := unreadableLocalFile(t)
	for _, line := range []int{0, 1} {
		comment := contract.Comment{Path: path, StartLine: line, EndLine: line, Content: "Unreadable file finding"}
		if location := workspace.ResolveLocation(root, comment.Path, comment.StartLine, comment.EndLine); location != nil {
			t.Fatalf("unreadable file has location: %+v", location)
		}
		result := &orchestrator.Result{Review: &contract.ReviewResult{Comments: []contract.Comment{comment}}}
		text := presentation.FormatResult(result, root)
		if !strings.Contains(text, comment.Content) || strings.Contains(text, "(<file://") {
			t.Fatalf("unreadable finding lost text or retained navigation: %s", text)
		}
	}
}

type findingRecorder struct {
	updates []acp.SessionNotification
	err     error
}

func (r *findingRecorder) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	r.updates = append(r.updates, n)
	return r.err
}

func TestPromptFindingsAppearOnlyInFinalMessage(t *testing.T) {
	for _, command := range []string{"/review", "/scan"} {
		t.Run(command, func(t *testing.T) {
			root := testGitDir(t)
			if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("first\nlast\n"), 0600); err != nil {
				t.Fatal(err)
			}
			comments := []contract.Comment{
				{Path: "a.go", StartLine: 2, EndLine: 2, Severity: "medium", Category: "security", Content: "First unique finding", SuggestionCode: "fixed()"},
				{Path: "a.go", StartLine: 1, EndLine: 1, Severity: "low", Category: "bug", Content: "Second unique finding"},
			}
			result := &orchestrator.Result{Review: &contract.ReviewResult{Status: "success", Comments: comments}}
			if command == "/scan" {
				result = &orchestrator.Result{Scan: &contract.ScanResult{Status: "success", Comments: comments}}
			}
			runner := progressRunner(func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
				events := make(chan orchestrator.Event)
				close(events)
				outcomes := make(chan orchestrator.Outcome, 1)
				outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted, Result: result}
				close(outcomes)
				return events, outcomes
			})
			agent := NewAgent("ocr", runner)
			recorder := &findingRecorder{}
			agent.SetAgentConnection(recorder)
			session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: root})
			if err != nil {
				t.Fatal(err)
			}
			response, err := agent.Prompt(context.Background(), acp.PromptRequest{SessionId: session.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock(command)}})
			if err != nil || response.StopReason != acp.StopReasonEndTurn {
				t.Fatalf("prompt response = %+v, %v", response, err)
			}
			var body strings.Builder
			progress, completed := 0, 0
			for _, notice := range recorder.updates {
				if call := notice.Update.ToolCall; call != nil {
					if call.Kind != acp.ToolKindOther || len(call.Locations) != 0 {
						t.Errorf("unexpected finding/navigation tool card: %+v", call)
					}
					progress++
				}
				if update := notice.Update.ToolCallUpdate; update != nil && update.Status != nil && *update.Status == acp.ToolCallStatusCompleted {
					completed++
				}
				if message := notice.Update.AgentMessageChunk; message != nil {
					body.WriteString(message.Content.Text.Text)
				}
			}
			if progress != 1 || completed != 1 {
				t.Errorf("want only one completed progress card, got starts=%d completed=%d", progress, completed)
			}
			for _, want := range []string{"First unique finding", "Second unique finding", "### 1. Medium", "### 2. Low", "```go\nfixed()\n```", "[a.go:2](<file://", "#L2"} {
				if strings.Count(body.String(), want) != 1 {
					t.Errorf("expected %q exactly once in result: %s", want, body.String())
				}
			}
		})
	}
}

func TestReviewRejectsNonRepositoryBeforeRunner(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &noticeRecorder{}
	a.SetAgentConnection(recorder)
	response, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	if err != nil || response.StopReason != acp.StopReasonEndTurn || !strings.Contains(recorder.text, "Git repository") {
		t.Fatalf("response = %+v, %v, %q", response, err, recorder.text)
	}
	response, err = a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/scan")}})
	if err != nil || response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("nonrepository scan = %+v, %v", response, err)
	}
}
