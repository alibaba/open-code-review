// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"encoding/json"
	"errors"
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
	for _, want := range []string{"Partial", "Critical", "bug", "3 files processed", "1 finding", "fixed()", "failed.go", "provider stalled"} {
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
	uri := fileURLString(location.Path, location.Line)
	for _, want := range []string{"## OCR review", "\n\n### 1. Critical · ", "[a \\[test\\]\\(one\\)\\#.go:1-2](<" + uri + ">)", "\n\nExplanation.\n\n", "**Existing code:**\n\n````go\n// ```\nold()\n````", "**Suggested code (not applied):**\n\n```go\nnew()\n```"} {
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
		writeFinding(&b, 1, contract.Comment{Path: path, Content: "Kept"}, nil)
		if strings.Contains(b.String(), "](https:") || strings.Contains(b.String(), "file://") || strings.Contains(b.String(), "\n# title") || !strings.Contains(b.String(), "Kept") {
			t.Fatalf("unsafe or missing fallback: %q", b.String())
		}
	}
	for _, tc := range []struct{ path, language string }{{"a.go", "go"}, {"a.py", "python"}, {"a.ts", "typescript"}, {"a.unknown", ""}} {
		var b strings.Builder
		writeFinding(&b, 1, contract.Comment{Path: tc.path, ExistingCode: "example"}, nil)
		if !strings.Contains(b.String(), "```"+tc.language+"\nexample\n```") {
			t.Fatalf("language for %s: %q", tc.path, b.String())
		}
	}
}

func TestReportStatusAndMissingUsage(t *testing.T) {
	for _, tc := range []struct{ status, label string }{
		{"success", "Complete"}, {"complete", "Complete"}, {"partial", "Partial"},
		{"failed", "Failed"}, {"skipped", "Skipped"},
		{"completed_with_warnings", "Completed with warnings"}, {"completed_with_errors", "Completed with errors"},
		{"future_status", "future\\_status"}, {"", "Status unavailable"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			for _, scan := range []bool{false, true} {
				result := &orchestrator.Result{Review: &contract.ReviewResult{Status: tc.status, Message: "Result detail"}}
				operation := "review"
				if scan {
					result = &orchestrator.Result{Scan: &contract.ScanResult{Status: tc.status, Message: "Result detail"}}
					operation = "scan"
				}
				got := formatResult(result)
				if !strings.HasPrefix(got, "## OCR "+operation+" · "+tc.label+"\n\n") || !strings.Contains(got, "Result detail") || !strings.Contains(got, "0 findings") {
					t.Fatalf("lost status or result: %s", got)
				}
				for _, invented := range []string{"Status:", "Completion:", "Usage:", "files reviewed", "No issues", "No findings"} {
					if strings.Contains(got, invented) {
						t.Fatalf("unexpected %q: %s", invented, got)
					}
				}
			}
		})
	}
	for _, result := range []*orchestrator.Result{nil, {}} {
		if got := formatResult(result); got != "OCR returned no result." {
			t.Fatalf("missing result: %s", got)
		}
	}
}

func TestReportSummarySeverityOrderAndFooter(t *testing.T) {
	got := formatResult(&orchestrator.Result{Review: &contract.ReviewResult{
		Status: "complete", Summary: &contract.Summary{FilesReviewed: 12, Comments: 7, TotalTokens: 18420, Elapsed: "42s", BudgetExceeded: true},
		Comments: []contract.Comment{{Severity: "low"}, {Severity: "high"}, {Severity: "critical"}, {Severity: "medium"}, {Severity: "high"}, {Severity: "future"}, {}},
	}})
	for _, want := range []string{"12 files reviewed · 7 findings", "Critical 1 · High 2 · Medium 1 · Low 1 · future 1 · Unspecified 1", "**Token budget exceeded.**", "### 7. Unspecified", "Usage: 18420 tokens · 42s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
	if !strings.HasSuffix(got, "Usage: 18420 tokens · 42s") {
		t.Fatalf("usage is not footer: %s", got)
	}
	for _, tc := range []struct {
		summary contract.Summary
		footer  string
	}{
		{contract.Summary{TotalTokens: 2}, "Usage: 2 tokens"},
		{contract.Summary{Elapsed: "1s"}, "Usage: 1s"},
		{contract.Summary{}, ""},
	} {
		got := formatResult(&orchestrator.Result{Scan: &contract.ScanResult{Status: "success", Summary: &tc.summary}})
		if tc.footer == "" && strings.Contains(got, "Usage:") || tc.footer != "" && !strings.HasSuffix(got, tc.footer) {
			t.Fatalf("optional footer: %s", got)
		}
	}
}

func TestReportRetainsManifestDiagnostics(t *testing.T) {
	for _, status := range []string{"partial", "failed"} {
		got := formatResult(&orchestrator.Result{Review: &contract.ReviewResult{Status: status, Manifest: &contract.Manifest{TerminalState: status,
			RunFailure: &contract.RunFailure{Classification: "provider_error", Reason: "Provider unavailable"},
			Coverage:   &contract.Coverage{Failed: []contract.CoverageItem{{Path: "failed.go", Classification: "timeout", Reason: "Deadline exceeded"}}},
		}}})
		for _, want := range []string{"provider_error", "Provider unavailable", "failed.go", "timeout", "Deadline exceeded"} {
			if !strings.Contains(got, want) {
				t.Fatalf("lost diagnostic %q: %s", want, got)
			}
		}
		if strings.Contains(got, "Completion:") || strings.Contains(got, "Manifest status:") {
			t.Fatalf("duplicate status: %s", got)
		}
	}
	got := formatResult(&orchestrator.Result{Review: &contract.ReviewResult{Status: "success", Manifest: &contract.Manifest{TerminalState: "partial"}}})
	if !strings.HasPrefix(got, "## OCR review · Partial") || !strings.Contains(got, "Reported status: Complete") {
		t.Fatalf("conflicting status dropped: %s", got)
	}
}

func TestReportHeadingEscapesMetadata(t *testing.T) {
	got := formatResult(&orchestrator.Result{Scan: &contract.ScanResult{Status: "future\n# heading", Comments: []contract.Comment{{Severity: "[urgent](https://example.com)\n# heading", Category: "**category**", Path: "bad\n# heading", Content: "Body"}}}})
	if strings.Contains(got, "\n# heading") || strings.Contains(got, "](https://") || strings.Contains(got, "**category**") {
		t.Fatalf("unsafe headings: %s", got)
	}
	if !strings.Contains(got, "Body") {
		t.Fatalf("lost content: %s", got)
	}
}

func TestReportCoverageUsesManifest(t *testing.T) {
	for _, tc := range []struct{ name, coverage, want string }{
		{"partial", `{"selected":[{},{},{},{}],"completed":[{},{},{}],"failed":[{"path":"server.go","classification":"budget","reason":"round limit"}]}`, "3/4 files completed · 1 failed · 0 findings"},
		{"resumed", `{"selected":[{},{},{},{}],"completed":[{}],"reused":[{}],"failed":[{}],"waived":[{}]}`, "1/4 files completed · 1 reused · 1 failed · 1 waived · 0 findings"},
		{"empty", `{"selected":[],"completed":[],"failed":[]}`, "0/0 files completed · 0 findings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var review contract.ReviewResult
			if err := json.Unmarshal([]byte(`{"status":"partial","summary":{"files_reviewed":4},"manifest":{"terminal_state":"partial","coverage":`+tc.coverage+`}}`), &review); err != nil {
				t.Fatal(err)
			}
			got := formatResult(&orchestrator.Result{Review: &review})
			if !strings.Contains(got, tc.want) || strings.Contains(got, "files reviewed") {
				t.Fatalf("misleading coverage: %s", got)
			}
		})
	}
}

func TestReportFindingsMatchRenderedList(t *testing.T) {
	for _, count := range []int64{0, 5} {
		got := formatResult(&orchestrator.Result{Review: &contract.ReviewResult{Status: "complete", Summary: &contract.Summary{Comments: count}, Comments: []contract.Comment{{Severity: "low", Content: "Actual finding"}}}})
		if !strings.Contains(got, "1 finding\n\nLow 1") || !strings.Contains(got, "### 1. Low") {
			t.Fatalf("inconsistent findings: %s", got)
		}
	}
	got := formatResult(&orchestrator.Result{Scan: &contract.ScanResult{Status: "completed_with_warnings", Summary: &contract.Summary{BudgetExceeded: true}}})
	if strings.Contains(got, "Review coverage") || !strings.Contains(got, "Coverage may be incomplete") {
		t.Fatalf("wrong operation: %s", got)
	}
}
