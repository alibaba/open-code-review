// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func TestRejectedGuidancePreservesTerminalFailures(t *testing.T) {
	a := NewAgent("ocr", &captureRunner{})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := a.sendRejection(cancelled, "s", "Use /review.")
	if err != nil || resp.StopReason != acp.StopReasonCancelled || resp.Meta != nil {
		t.Fatalf("cancellation overwritten: %+v, %v", resp, err)
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	resp, err = a.sendRejection(expired, "s", "Use /review.")
	ocr, _ := resp.Meta["ocr"].(map[string]any)
	if err != nil || ocr["kind"] != "timed_out" {
		t.Fatalf("timeout overwritten: %+v, %v", resp, err)
	}
	sink := &discoveryRecorder{err: context.Canceled}
	a.SetAgentConnection(sink)
	resp, err = a.sendRejection(context.Background(), "s", "Use /review.")
	if err != nil || resp.StopReason != acp.StopReasonCancelled || resp.Meta != nil {
		t.Fatalf("transport cancellation overwritten: %+v, %v", resp, err)
	}
	sink.err = errors.New("disconnected")
	if _, err = a.sendRejection(context.Background(), "s", "Use /review."); !errors.Is(err, sink.err) {
		t.Fatalf("send error lost: %v", err)
	}
}

type noticeRecorder struct {
	text    string
	expired bool
}

func (r *noticeRecorder) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	r.expired = ctx.Err() != nil
	if n.Update.AgentMessageChunk != nil {
		r.text += n.Update.AgentMessageChunk.Content.Text.Text
	}
	return nil
}

func TestTimeoutHasVisibleNotice(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{}, intent.NewParser(waitingParser{make(chan struct{})}, nil))
	a.TurnTimeout = 10 * time.Millisecond
	recorder := &noticeRecorder{}
	a.SetAgentConnection(recorder)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	_, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("review please")}})
	if err != nil {
		t.Fatal(err)
	}
	if recorder.expired || !strings.Contains(recorder.text, "timed out") || !strings.Contains(recorder.text, "Retry") {
		t.Fatalf("notice: %+v", recorder)
	}
}

func TestPartialResultRetainsDiagnosticsAndSuggestions(t *testing.T) {
	r := &orchestrator.Result{Review: &contract.ReviewResult{Status: "partial", Message: "Incomplete review", Summary: &contract.Summary{FilesReviewed: 3, Comments: 1}, Comments: []contract.Comment{{Path: "a.go", Severity: "critical", Category: "bug", Content: "unsafe", SuggestionCode: "fixed()"}}, Manifest: &contract.Manifest{TerminalState: "partial", Coverage: &contract.Coverage{Failed: []contract.CoverageItem{{Path: "failed.go", Classification: "timeout", Reason: "provider stalled"}}}}}}
	got := formatResult(r)
	for _, want := range []string{"partial", "critical", "bug", "Files reviewed: 3", "findings: 1", "fixed()", "failed.go", "provider stalled"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestSessionRejectsUnknownCommit(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	recorder := &noticeRecorder{}
	a.SetAgentConnection(recorder)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	r, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review --commit missing-ref")}})
	if err != nil {
		t.Fatal(err)
	}
	if r.StopReason != acp.StopReasonEndTurn || !strings.Contains(recorder.text, "could not resolve that ref") || a.sessions[s.SessionId].state.Pending() == nil {
		t.Fatalf("invalid repository ref accepted: %+v", r)
	}
}

func TestFindingMarkdownFormatting(t *testing.T) {
	root := t.TempDir()
	path := "a [test](one)#.go"
	if err := os.WriteFile(filepath.Join(root, path), []byte("first\nsecond\n"), 0600); err != nil {
		t.Fatal(err)
	}
	comment := contract.Comment{Path: path, StartLine: 1, EndLine: 2, Severity: "critical", Category: "bug", Content: "Explanation.", ExistingCode: "// ```\nold()", SuggestionCode: "new()"}
	result := &orchestrator.Result{Review: &contract.ReviewResult{Status: "partial", Comments: []contract.Comment{comment}}}
	got := formatResultAtRoot(result, root)
	location := findingLocation(root, comment)
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(location.Path), Fragment: "L1"}).String()
	for _, want := range []string{"## OCR review", "\n\n---\n\n### critical · bug\n\n**Location:**", "[a \\[test\\]\\(one\\)\\#.go:1-2](<" + uri + ">)", "\n\nExplanation.\n\n", "**Existing code:**\n\n````go\n// ```\nold()\n````", "**Suggested code:**\n\n```go\nnew()\n```"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(formatResult(result), "file://") {
		t.Fatal("no-root formatting must not invent navigation")
	}
}

func TestFindingMarkdownFallbackAndLanguages(t *testing.T) {
	for _, path := range []string{"../outside.go", "missing.go", "[link](https://example.com)", "bad\n# title"} {
		var b strings.Builder
		writeFinding(&b, contract.Comment{Path: path, Content: "Kept"}, nil)
		if strings.Contains(b.String(), "](https:") || strings.Contains(b.String(), "file://") || strings.Contains(b.String(), "\n# title") || !strings.Contains(b.String(), "Kept") {
			t.Fatalf("unsafe or missing fallback: %q", b.String())
		}
	}
	for _, tc := range []struct{ path, language string }{{"a.go", "go"}, {"a.py", "python"}, {"a.ts", "typescript"}, {"a.unknown", ""}} {
		var b strings.Builder
		writeFinding(&b, contract.Comment{Path: tc.path, ExistingCode: "example"}, nil)
		if !strings.Contains(b.String(), "```"+tc.language+"\nexample\n```") {
			t.Fatalf("language for %s: %q", tc.path, b.String())
		}
	}
}
