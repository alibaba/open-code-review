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

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func TestResultRootMatchesCLIOperation(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"review walks to git root", []string{"review"}, canonical},
		{"scan stays in cwd", []string{"scan"}, filepath.Join(canonical, "sub")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resultRoot(context.Background(), sub, tc.args)
			if err != nil || got != tc.want {
				t.Fatalf("root = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, args := range [][]string{nil, {"unknown"}} {
		if _, err := resultRoot(context.Background(), root, args); err == nil {
			t.Fatalf("expected invalid root for %v", args)
		}
	}
	if _, err := resultRoot(context.Background(), ".", []string{"scan"}); err == nil {
		t.Fatal("relative cwd accepted")
	}
	if _, err := resultRoot(context.Background(), t.TempDir(), []string{"review"}); err == nil {
		t.Fatal("nonrepository review accepted")
	}
}

func testGitDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func TestFindingLocationBoundaries(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.go")
	if err := os.WriteFile(file, []byte("one\ntwo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("outside\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(root, "inside.go")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		path       string
		start, end int
		valid      bool
	}{
		{"line range", "file.go", 1, 2, true},
		{"absolute inside", file, 2, 2, true},
		{"file level", "file.go", 0, 0, true},
		{"internal symlink", "inside.go", 1, 1, true},
		{"symlink escape", "escape.go", 1, 1, false},
		{"absolute outside", outside, 1, 1, false},
		{"traversal", "../file.go", 1, 1, false},
		{"embedded traversal", "x/../file.go", 1, 1, false},
		{"missing", "missing.go", 1, 1, false},
		{"directory", ".", 0, 0, false},
		{"uri", "file:///file.go", 1, 1, false},
		{"windows path", "C:\\file.go", 1, 1, false},
		{"empty", "", 0, 0, false},
		{"newline", "file.go\n", 1, 1, false},
		{"beyond eof", "file.go", 3, 3, false},
		{"end beyond eof", "file.go", 1, 3, false},
		{"reversed", "file.go", 2, 1, false},
		{"negative", "file.go", -1, -1, false},
		{"missing end", "file.go", 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := findingLocation(root, contract.Comment{Path: tc.path, StartLine: tc.start, EndLine: tc.end})
			if (got != nil) != tc.valid {
				t.Fatalf("location = %#v; valid = %v", got, tc.valid)
			}
			if got != nil {
				if !filepath.IsAbs(got.Path) {
					t.Fatal("location not absolute")
				}
				if tc.start == 0 && got.Line != nil {
					t.Fatal("invented file-level line")
				}
				if tc.start > 0 && (got.Line == nil || *got.Line != tc.start) {
					t.Fatal("incorrect line")
				}
			}
		})
	}
}

func TestHasLineBounds(t *testing.T) {
	for _, tc := range []struct {
		text string
		line int
		want bool
	}{
		{"", 1, false},
		{"one", 1, true},
		{"one\n", 2, false},
		{"one\ntwo", 2, true},
		{"\n\n", 2, true},
		{strings.Repeat("x", 32768) + "\nend", 2, true},
		{strings.Repeat("x", 32768), 2, false},
	} {
		if got := hasLine(strings.NewReader(tc.text), tc.line); got != tc.want {
			t.Fatalf("line %d, input length %d: got %v, want %v", tc.line, len(tc.text), got, tc.want)
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
			for _, want := range []string{"First unique finding", "Second unique finding", "### medium", "### low", "```go\nfixed()\n```", "[a.go:2](<file://", "#L2"} {
				if strings.Count(body.String(), want) != 1 {
					t.Errorf("expected %q exactly once in result: %s", want, body.String())
				}
			}
		})
	}
}

func TestFormattedFindingsKeepSafeNavigationAndText(t *testing.T) {
	root := testGitDir(t)
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("first\nlast"), 0600); err != nil {
		t.Fatal(err)
	}
	result := &orchestrator.Result{Review: &contract.ReviewResult{Comments: []contract.Comment{
		{Path: "a.go", StartLine: 2, EndLine: 2, Severity: "critical", Content: "broken", SuggestionCode: "fixed"},
		{Path: "a.go", Content: "file finding"},
		{Path: "../outside", Content: "unlocatable"},
	}}}
	text := formatResultAtRoot(result, root)
	if !strings.Contains(text, "```go\nfixed\n```") || !strings.Contains(text, "#L2") {
		t.Fatalf("finding lost formatted source or navigation: %s", text)
	}
	if !strings.Contains(text, "[a.go:2](<file://") || strings.Contains(text, "[../outside](<file://") {
		t.Fatalf("unsafe or missing final navigation: %s", text)
	}
	if !strings.Contains(formatResult(result), "unlocatable") {
		t.Fatal("unsafe finding text lost")
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
