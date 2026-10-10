// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package presentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/adapter/workspace"
	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
)

func TestPartialResultRetainsDiagnosticsAndSuggestions(t *testing.T) {
	r := &orchestrator.Result{Review: &contract.ReviewResult{Status: "partial", Message: "Incomplete review", Summary: &contract.Summary{FilesReviewed: 3, Comments: 1}, Comments: []contract.Comment{{Path: "a.go", Severity: "critical", Category: "bug", Content: "unsafe", SuggestionCode: "fixed()"}}, Manifest: &contract.Manifest{TerminalState: "partial", Coverage: &contract.Coverage{Failed: []contract.CoverageItem{{Path: "failed.go", Classification: "timeout", Reason: "provider stalled"}}}}}}
	got := formatReport(r)
	for _, want := range []string{"Partial", "Critical", "bug", "3 files processed", "1 finding", "fixed()", "failed.go", "provider stalled"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
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
	got := FormatResult(result, root)
	location := workspace.ResolveLocation(root, comment.Path, comment.StartLine, comment.EndLine)
	uri := workspace.FileURLString(location.Path, location.Line)
	for _, want := range []string{"## OCR review", "\n\n### 1. Critical · ", "[a \\[test\\]\\(one\\)\\#.go:1-2](<" + uri + ">)", "\n\nExplanation.\n\n", "**Existing code:**\n\n````go\n// ```\nold()\n````", "**Suggested code (not applied):**\n\n```go\nnew()\n```"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(formatReport(result), "file://") {
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
				got := formatReport(result)
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
		if got := formatReport(result); got != "OCR returned no result." {
			t.Fatalf("missing result: %s", got)
		}
	}
}

func TestReportSummarySeverityOrderAndFooter(t *testing.T) {
	got := formatReport(&orchestrator.Result{Review: &contract.ReviewResult{
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
		got := formatReport(&orchestrator.Result{Scan: &contract.ScanResult{Status: "success", Summary: &tc.summary}})
		if tc.footer == "" && strings.Contains(got, "Usage:") || tc.footer != "" && !strings.HasSuffix(got, tc.footer) {
			t.Fatalf("optional footer: %s", got)
		}
	}
}

func TestReportRetainsManifestDiagnostics(t *testing.T) {
	for _, status := range []string{"partial", "failed"} {
		got := formatReport(&orchestrator.Result{Review: &contract.ReviewResult{Status: status, Manifest: &contract.Manifest{TerminalState: status,
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
	got := formatReport(&orchestrator.Result{Review: &contract.ReviewResult{Status: "success", Manifest: &contract.Manifest{TerminalState: "partial"}}})
	if !strings.HasPrefix(got, "## OCR review · Partial") || !strings.Contains(got, "Reported status: Complete") {
		t.Fatalf("conflicting status dropped: %s", got)
	}
}

func TestReportHeadingEscapesMetadata(t *testing.T) {
	got := formatReport(&orchestrator.Result{Scan: &contract.ScanResult{Status: "future\n# heading", Comments: []contract.Comment{{Severity: "[urgent](https://example.com)\n# heading", Category: "**category**", Path: "bad\n# heading", Content: "Body"}}}})
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
			got := formatReport(&orchestrator.Result{Review: &review})
			if !strings.Contains(got, tc.want) || strings.Contains(got, "files reviewed") {
				t.Fatalf("misleading coverage: %s", got)
			}
		})
	}
}

func TestReportFindingsMatchRenderedList(t *testing.T) {
	for _, count := range []int64{0, 5} {
		got := formatReport(&orchestrator.Result{Review: &contract.ReviewResult{Status: "complete", Summary: &contract.Summary{Comments: count}, Comments: []contract.Comment{{Severity: "low", Content: "Actual finding"}}}})
		if !strings.Contains(got, "1 finding\n\nLow 1") || !strings.Contains(got, "### 1. Low") {
			t.Fatalf("inconsistent findings: %s", got)
		}
	}
	got := formatReport(&orchestrator.Result{Scan: &contract.ScanResult{Status: "completed_with_warnings", Summary: &contract.Summary{BudgetExceeded: true}}})
	if strings.Contains(got, "Review coverage") || !strings.Contains(got, "Coverage may be incomplete") {
		t.Fatalf("wrong operation: %s", got)
	}
}
func TestFormatResult(t *testing.T) {
	got := formatReport(&orchestrator.Result{Review: &contract.ReviewResult{Message: "summary", Comments: []contract.Comment{{Path: "x.go", StartLine: 2, EndLine: 3, Content: "issue"}}}})
	if got == "" {
		t.Fatal("empty result")
	}
}

func TestFormatResultVariants(t *testing.T) {
	if formatReport(nil) != "OCR returned no result." {
		t.Fatal("nil result")
	}
	if formatReport(&orchestrator.Result{Scan: &contract.ScanResult{Message: "scan", Comments: []contract.Comment{{Path: "a", Content: "x"}}}}) == "" {
		t.Fatal("scan result")
	}
}

func TestFormattedFindingsKeepSafeNavigationAndText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("first\nlast"), 0600); err != nil {
		t.Fatal(err)
	}
	result := &orchestrator.Result{Review: &contract.ReviewResult{Comments: []contract.Comment{
		{Path: "a.go", StartLine: 2, EndLine: 2, Severity: "critical", Content: "broken", SuggestionCode: "fixed"},
		{Path: "a.go", Content: "file finding"},
		{Path: "../outside", Content: "unlocatable"},
	}}}
	text := FormatResult(result, root)
	if !strings.Contains(text, "```go\nfixed\n```") || !strings.Contains(text, "#L2") {
		t.Fatalf("finding lost formatted source or navigation: %s", text)
	}
	if !strings.Contains(text, "[a.go:2](<file://") || strings.Contains(text, "[../outside](<file://") {
		t.Fatalf("unsafe or missing final navigation: %s", text)
	}
	if !strings.Contains(formatReport(result), "unlocatable") {
		t.Fatal("unsafe finding text lost")
	}
}
