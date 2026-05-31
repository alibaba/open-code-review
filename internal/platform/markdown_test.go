package platform

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func TestAppendOrReplaceManagedBlock_AppendsAfterExistingDescription(t *testing.T) {
	got := AppendOrReplaceManagedBlock("User description", "Generated summary")

	if !strings.Contains(got, "User description") {
		t.Fatalf("original description lost: %q", got)
	}
	if !strings.Contains(got, "\n\n---\n") {
		t.Fatalf("expected separator after original description, got %q", got)
	}
	if !strings.Contains(got, PRSummaryStartMarker) || !strings.Contains(got, PRSummaryEndMarker) {
		t.Fatalf("missing summary markers: %q", got)
	}
	if !strings.Contains(got, "Generated summary") {
		t.Fatalf("summary content missing: %q", got)
	}
}

func TestAppendOrReplaceManagedBlock_ReplacesExistingBlock(t *testing.T) {
	input := "User\n\n---\n" + PRSummaryStartMarker + "\nold content\n" + PRSummaryEndMarker
	got := AppendOrReplaceManagedBlock(input, "new content")

	if strings.Contains(got, "old content") {
		t.Fatalf("old summary remained: %q", got)
	}
	if !strings.Contains(got, "new content") {
		t.Fatalf("new summary missing: %q", got)
	}
	if strings.Count(got, PRSummaryStartMarker) != 1 {
		t.Fatalf("expected exactly one managed block, got %d occurrences in: %q", strings.Count(got, PRSummaryStartMarker), got)
	}
	if strings.Count(got, PRSummaryEndMarker) != 1 {
		t.Fatalf("expected exactly one end marker, got %d occurrences in: %q", strings.Count(got, PRSummaryEndMarker), got)
	}
	if !strings.Contains(got, "User") {
		t.Fatalf("user text outside markers was deleted: %q", got)
	}
}

func TestAppendOrReplaceManagedBlock_EmptyDescription(t *testing.T) {
	got := AppendOrReplaceManagedBlock("", "summary only")

	if !strings.Contains(got, PRSummaryStartMarker) || !strings.Contains(got, PRSummaryEndMarker) {
		t.Fatalf("missing markers: %q", got)
	}
	if !strings.Contains(got, "summary only") {
		t.Fatalf("summary content missing: %q", got)
	}
	if strings.Contains(got, "---") {
		t.Fatalf("unexpected separator for empty description: %q", got)
	}
}

func TestAppendOrReplaceManagedBlock_PreservesUserTextOutsideMarkers(t *testing.T) {
	original := "Title\n\n---\n" + PRSummaryStartMarker + "\nold\n" + PRSummaryEndMarker + "\n\nTrailing user text"
	got := AppendOrReplaceManagedBlock(original, "updated")

	if !strings.Contains(got, "Title") {
		t.Fatalf("leading user text lost: %q", got)
	}
	if !strings.Contains(got, "Trailing user text") {
		t.Fatalf("trailing user text lost: %q", got)
	}
	if strings.Contains(got, "old") {
		t.Fatalf("old managed content remained: %q", got)
	}
}

func TestAppendOrReplaceManagedBlock_TrailingTextNotGluedToMarker(t *testing.T) {
	original := "Title\n\n---\n" + PRSummaryStartMarker + "\nold\n" + PRSummaryEndMarker + "\n\nTrailing user text"
	got := AppendOrReplaceManagedBlock(original, "updated")

	wantSuffix := PRSummaryEndMarker + "\n\nTrailing user text"
	if !strings.Contains(got, wantSuffix) {
		t.Fatalf("trailing text glued to end marker (missing %q): %q", wantSuffix, got)
	}
}

// --- RenderInlineComment tests ---

func TestRenderInlineComment_ContainsBadge(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{Content: "check this"})

	if !strings.Contains(body, "OpenCodeReview") {
		t.Fatalf("missing OpenCodeReview badge: %q", body)
	}
	if !strings.Contains(body, "img.shields.io/badge/OpenCodeReview") {
		t.Fatalf("missing shields.io badge URL: %q", body)
	}
}

func TestRenderInlineComment_ContainsInlineMarker(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{Content: "check this line"})

	if !strings.Contains(body, InlineMarker) {
		t.Fatalf("missing inline marker: %q", body)
	}
	if !strings.Contains(body, "check this line") {
		t.Fatalf("comment content missing: %q", body)
	}
}

func TestRenderInlineComment_ContainsLocation(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:      "main.go",
		Content:   "issue here",
		StartLine: 10,
		EndLine:   12,
	})

	if !strings.Contains(body, "**Location:**") {
		t.Fatalf("missing location label: %q", body)
	}
	if !strings.Contains(body, "main.go:10-12") {
		t.Fatalf("expected range location, got: %q", body)
	}
}

func TestRenderInlineComment_LocationSingleLine(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:      "main.go",
		Content:   "issue",
		StartLine: 5,
		EndLine:   5,
	})

	if !strings.Contains(body, "main.go:5`") {
		t.Fatalf("expected single-line location, got: %q", body)
	}
}

func TestRenderInlineComment_LocationPathOnly(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:    "main.go",
		Content: "issue",
	})

	if !strings.Contains(body, "main.go`") {
		t.Fatalf("expected path-only location, got: %q", body)
	}
	if strings.Contains(body, "main.go:") {
		t.Fatalf("unexpected line number in path-only location: %q", body)
	}
}

func TestRenderInlineComment_SuggestionBlockWhenPresent(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:           "main.go",
		Content:        "use better pattern",
		SuggestionCode: "fmt.Println(x)",
	})

	if !strings.Contains(body, "**Suggested change:**") {
		t.Fatalf("missing suggested change label: %q", body)
	}
	if !strings.Contains(body, "```go") {
		t.Fatalf("missing language-aware code fence for .go file: %q", body)
	}
	if !strings.Contains(body, "fmt.Println(x)") {
		t.Fatalf("missing suggestion code content: %q", body)
	}
}

func TestRenderInlineComment_NoSuggestionBlockWhenEmpty(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Content:        "issue",
		SuggestionCode: "",
	})

	if strings.Contains(body, "Suggested change") {
		t.Fatalf("suggestion block should be absent when SuggestionCode is empty: %q", body)
	}
}

func TestRenderInlineComment_DetailsWhenExistingCodePresent(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:         "main.go",
		Content:      "issue",
		ExistingCode: "old code here",
		StartLine:    10,
		EndLine:      12,
	})

	if !strings.Contains(body, "<details>") {
		t.Fatalf("missing details block: %q", body)
	}
	if !strings.Contains(body, "Review context") {
		t.Fatalf("missing review context summary: %q", body)
	}
	if !strings.Contains(body, "old code here") {
		t.Fatalf("missing existing code: %q", body)
	}
	if !strings.Contains(body, "main.go") {
		t.Fatalf("missing file path in details: %q", body)
	}
}

func TestRenderInlineComment_DetailsWhenThinkingPresent(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Content:  "issue",
		Thinking: "reasoning about the problem",
	})

	if !strings.Contains(body, "<details>") {
		t.Fatalf("missing details block when Thinking is present: %q", body)
	}
	if !strings.Contains(body, "reasoning about the problem") {
		t.Fatalf("Thinking content missing from details block: %q", body)
	}
	if !strings.Contains(body, "Reviewer notes:") {
		t.Fatalf("missing 'Reviewer notes:' label for Thinking content: %q", body)
	}
}

func TestRenderInlineComment_ExistingCodeInFencedBlock(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:         "main.go",
		Content:      "issue",
		ExistingCode: "return err",
		StartLine:    10,
	})

	if !strings.Contains(body, "```go") {
		t.Fatalf("ExistingCode should be in fenced code block with language from path: %q", body)
	}
	if !strings.Contains(body, "return err") {
		t.Fatalf("ExistingCode content missing: %q", body)
	}
}

func TestRenderInlineComment_ThinkingInDetailsBlock(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:         "main.go",
		Content:      "issue",
		ExistingCode: "return err",
		Thinking:     "The error should be wrapped.",
		StartLine:    10,
	})

	if !strings.Contains(body, "<details>") {
		t.Fatalf("missing details block: %q", body)
	}
	if !strings.Contains(body, "The error should be wrapped.") {
		t.Fatalf("Thinking content missing from details block: %q", body)
	}
	if !strings.Contains(body, "Reviewer notes:") {
		t.Fatalf("missing 'Reviewer notes:' label: %q", body)
	}
	if !strings.Contains(body, "```go") {
		t.Fatalf("ExistingCode should use language-aware fence for .go file: %q", body)
	}
}

func TestRenderInlineComment_NoDetailsWhenBothEmpty(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Content:      "issue",
		ExistingCode: "",
		Thinking:     "",
	})

	if strings.Contains(body, "<details>") {
		t.Fatalf("details block should be absent when ExistingCode and Thinking are both empty: %q", body)
	}
}

func TestRenderInlineComment_ContainsFooter(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{Content: "issue"})

	if !strings.Contains(body, "Generated by OpenCodeReview") {
		t.Fatalf("missing footer: %q", body)
	}
}

func TestRenderInlineComment_FullExample(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:           "pkg/handler.go",
		Content:        "This error is not handled properly.\nConsider wrapping it.",
		SuggestionCode: "if err != nil {\n    return fmt.Errorf(\"wrap: %w\", err)\n}",
		ExistingCode:   "if err != nil {\n    return err\n}",
		StartLine:      42,
		EndLine:        44,
		Thinking:       "The error should be wrapped for context.",
	})

	checks := []string{
		"OpenCodeReview",
		"img.shields.io",
		"This error is not handled properly.",
		"**Location:** `pkg/handler.go:42-44`",
		"**Suggested change:**",
		"```go",
		"fmt.Errorf(\"wrap: %w\", err)",
		"<details>",
		"Review context",
		"File: pkg/handler.go",
		"Lines: 42-44",
		"Existing code:",
		"return err",
		"Reviewer notes:",
		"The error should be wrapped for context.",
		"Generated by OpenCodeReview",
		InlineMarker,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in full inline comment", want)
		}
	}
}

// --- RenderSummaryComment tests ---

func TestRenderSummaryComment_ContainsBadge(t *testing.T) {
	body := RenderSummaryComment(nil, nil)

	if !strings.Contains(body, "OpenCodeReview") {
		t.Fatalf("missing badge: %q", body)
	}
	if !strings.Contains(body, "img.shields.io/badge/OpenCodeReview") {
		t.Fatalf("missing shields.io badge URL: %q", body)
	}
}

func TestRenderSummaryComment_ContainsSummaryMarker(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix this", StartLine: 1},
	}
	body := RenderSummaryComment(comments, nil)

	if !strings.Contains(body, SummaryMarker) {
		t.Fatalf("missing summary marker: %q", body)
	}
}

func TestRenderSummaryComment_ShowsReviewCompletedAndCount(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix this", StartLine: 1},
	}
	body := RenderSummaryComment(comments, nil)

	if !strings.Contains(body, "Review completed") {
		t.Fatalf("expected 'Review completed': %q", body)
	}
	if !strings.Contains(body, "Review comments: 1") {
		t.Fatalf("expected 'Review comments: 1': %q", body)
	}
}

func TestRenderSummaryComment_ShowsWarningCount(t *testing.T) {
	warnings := []PublishWarning{
		{Type: "inline_failed", Path: "a.go", Message: "line not in diff"},
	}
	body := RenderSummaryComment(nil, warnings)

	if !strings.Contains(body, "Inline comments not published: 1") {
		t.Fatalf("expected 'Inline comments not published: 1': %q", body)
	}
	if strings.Contains(body, "Publish warnings:") {
		t.Fatalf("should NOT contain old 'Publish warnings:' wording: %q", body)
	}
}

func TestRenderSummaryComment_ZeroCommentsShowsLooksGood(t *testing.T) {
	body := RenderSummaryComment(nil, nil)

	if !strings.Contains(body, "No comments generated") {
		t.Fatalf("expected 'no comments' message: %q", body)
	}
	if !strings.Contains(body, "Looks good to me") {
		t.Fatalf("expected 'looks good to me': %q", body)
	}
	if !strings.Contains(body, SummaryMarker) {
		t.Fatalf("missing summary marker: %q", body)
	}
}

func TestRenderSummaryComment_FindingsList(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix error handling", StartLine: 10},
		{Path: "b.go", Content: "missing nil check", StartLine: 20},
	}
	body := RenderSummaryComment(comments, nil)

	if !strings.Contains(body, "### Findings") {
		t.Fatalf("missing Findings section: %q", body)
	}
	if !strings.Contains(body, "a.go:10") {
		t.Fatalf("missing first finding location: %q", body)
	}
	if !strings.Contains(body, "fix error handling") {
		t.Fatalf("missing first finding content: %q", body)
	}
	if !strings.Contains(body, "b.go:20") {
		t.Fatalf("missing second finding location: %q", body)
	}
}

func TestRenderSummaryComment_WarningsList(t *testing.T) {
	warnings := []PublishWarning{
		{Type: "inline_failed", Path: "a.go", Message: "line not in diff"},
		{Type: "position_error", Message: "bad position"},
	}
	body := RenderSummaryComment(nil, warnings)

	// Warnings are now folded under <details> with diagnostics label.
	if !strings.Contains(body, "<details>") {
		t.Fatalf("missing <details> block for warnings: %q", body)
	}
	if !strings.Contains(body, "Publish diagnostics") {
		t.Fatalf("missing 'Publish diagnostics' label: %q", body)
	}
	if !strings.Contains(body, "[inline_failed] a.go") || !strings.Contains(body, "line not in diff") {
		t.Fatalf("missing first warning: %q", body)
	}
	if !strings.Contains(body, "[position_error]") || !strings.Contains(body, "bad position") {
		t.Fatalf("missing second warning: %q", body)
	}
}

// --- RenderRequestChangesComment tests ---

func TestRenderRequestChangesComment_ContainsCriticalBadge(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "main.go", Content: "critical bug", StartLine: 42},
	}
	body := RenderRequestChangesComment(comments)

	if !strings.Contains(body, "img.shields.io/badge/OpenCodeReview-AI%20Review-critical") {
		t.Fatalf("missing critical badge: %q", body)
	}
}

func TestRenderRequestChangesComment_ContainsRequestChangesMarker(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "main.go", Content: "critical bug\nwith details", StartLine: 42},
	}
	body := RenderRequestChangesComment(comments)

	if !strings.Contains(body, RequestChangesMarker) {
		t.Fatalf("missing request-changes marker: %q", body)
	}
	if !strings.Contains(body, "main.go:42") {
		t.Fatalf("expected file:line reference: %q", body)
	}
	if !strings.Contains(body, "critical bug") {
		t.Fatalf("expected first line of comment: %q", body)
	}
}

func TestRenderRequestChangesComment_ContainsHeader(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "main.go", Content: "bug", StartLine: 1},
	}
	body := RenderRequestChangesComment(comments)

	if !strings.Contains(body, "## OpenCodeReview found critical issues") {
		t.Fatalf("missing header: %q", body)
	}
}

func TestRenderInlineComment_TrimsContentWhitespace(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{Content: "  spaced  "})

	if !strings.Contains(body, "spaced") {
		t.Fatalf("content not trimmed: %q", body)
	}
}

// --- codeFenceLanguage tests ---

func TestCodeFenceLanguage_GoFile(t *testing.T) {
	if got := codeFenceLanguage("main.go"); got != "go" {
		t.Fatalf("expected go, got %q", got)
	}
}

func TestCodeFenceLanguage_TypeScriptFile(t *testing.T) {
	if got := codeFenceLanguage("src/app.ts"); got != "typescript" {
		t.Fatalf("expected typescript, got %q", got)
	}
}

func TestCodeFenceLanguage_PythonFile(t *testing.T) {
	if got := codeFenceLanguage("lib/util.py"); got != "python" {
		t.Fatalf("expected python, got %q", got)
	}
}

func TestCodeFenceLanguage_JSONFile(t *testing.T) {
	if got := codeFenceLanguage("config.json"); got != "json" {
		t.Fatalf("expected json, got %q", got)
	}
}

func TestCodeFenceLanguage_UnknownExtension(t *testing.T) {
	if got := codeFenceLanguage("file.xyz"); got != "text" {
		t.Fatalf("expected text for unknown extension, got %q", got)
	}
}

func TestCodeFenceLanguage_NoExtension(t *testing.T) {
	if got := codeFenceLanguage("Makefile"); got != "text" {
		t.Fatalf("expected text for no extension, got %q", got)
	}
}

func TestCodeFenceLanguage_EmptyPath(t *testing.T) {
	if got := codeFenceLanguage(""); got != "text" {
		t.Fatalf("expected text for empty path, got %q", got)
	}
}

func TestCodeFenceLanguage_UpperCaseExtension(t *testing.T) {
	if got := codeFenceLanguage("MAIN.GO"); got != "go" {
		t.Fatalf("expected go for MAIN.GO, got %q", got)
	}
}

func TestCodeFenceLanguage_UpperCaseRExtension(t *testing.T) {
	if got := codeFenceLanguage("analysis.R"); got != "r" {
		t.Fatalf("expected r for analysis.R, got %q", got)
	}
}

// --- Fix A: language-aware code fences in rendered comments ---

func TestRenderInlineComment_ExistingCodeUsesLanguageFence(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:         "handler.go",
		Content:      "issue",
		ExistingCode: "return err",
		StartLine:    10,
	})

	if !strings.Contains(body, "```go") {
		t.Fatalf("ExistingCode should use ```go fence for .go file, got: %q", body)
	}
	if strings.Contains(body, "```text") {
		t.Fatalf("ExistingCode should NOT use ```text fence when path has known extension: %q", body)
	}
}

func TestRenderInlineComment_SuggestionCodeUsesLanguageFence(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Path:           "handler.go",
		Content:        "issue",
		SuggestionCode: "return fmt.Errorf(\"wrap: %w\", err)",
		StartLine:      10,
	})

	if !strings.Contains(body, "```go") {
		t.Fatalf("SuggestionCode should use ```go fence for .go file, got: %q", body)
	}
}

// --- Fix B: no GitLab `suggestion` fence ---

func TestRenderInlineComment_NoSuggestionFence(t *testing.T) {
	body := RenderInlineComment(model.LlmComment{
		Content:        "use better pattern",
		SuggestionCode: "fmt.Println(x)",
	})

	if strings.Contains(body, "```suggestion") {
		t.Fatalf("should NOT use ```suggestion fence (GitLab-specific), got: %q", body)
	}
	if !strings.Contains(body, "**Suggested change:**") {
		t.Fatalf("should still have 'Suggested change' label: %q", body)
	}
}

// --- Fix D: summary diagnostics folding ---

func TestRenderSummaryComment_WarningsFoldedInDetails(t *testing.T) {
	warnings := []PublishWarning{
		{Type: "inline_failed", Path: "a.go", Message: "line not in diff"},
		{Type: "position_error", Message: "bad position"},
	}
	body := RenderSummaryComment(nil, warnings)

	if !strings.Contains(body, "<details>") {
		t.Fatalf("warnings should be folded in <details> block: %q", body)
	}
	if !strings.Contains(body, "Publish diagnostics") {
		t.Fatalf("missing 'Publish diagnostics' summary label: %q", body)
	}
	if !strings.Contains(body, "line not in diff") {
		t.Fatalf("warning detail should be inside details block: %q", body)
	}
}

func TestRenderSummaryComment_WarningCountInMainBody(t *testing.T) {
	warnings := []PublishWarning{
		{Type: "inline_failed", Path: "a.go", Message: "line not in diff"},
		{Type: "position_error", Message: "bad position"},
	}
	body := RenderSummaryComment(nil, warnings)

	if !strings.Contains(body, "Inline comments not published: 2") {
		t.Fatalf("main body should show 'Inline comments not published: 2': %q", body)
	}
	if strings.Contains(body, "Publish warnings:") {
		t.Fatalf("should NOT contain old 'Publish warnings:' wording: %q", body)
	}
}

func TestRenderSummaryComment_NoDetailsBlockWhenNoWarnings(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix", StartLine: 1},
	}
	body := RenderSummaryComment(comments, nil)

	if strings.Contains(body, "<details>") {
		t.Fatalf("no <details> block expected when no warnings: %q", body)
	}
}

// --- formatLocation tests ---

func TestFormatLocation_Range(t *testing.T) {
	got := formatLocation("main.go", 10, 15)
	if got != "main.go:10-15" {
		t.Fatalf("expected main.go:10-15, got %q", got)
	}
}

func TestFormatLocation_SingleLine(t *testing.T) {
	got := formatLocation("main.go", 5, 5)
	if got != "main.go:5" {
		t.Fatalf("expected main.go:5, got %q", got)
	}
}

func TestFormatLocation_EndBeforeStart(t *testing.T) {
	got := formatLocation("main.go", 10, 5)
	if got != "main.go:10" {
		t.Fatalf("expected main.go:10 (single line), got %q", got)
	}
}

func TestFormatLocation_NoLine(t *testing.T) {
	got := formatLocation("main.go", 0, 0)
	if got != "main.go" {
		t.Fatalf("expected main.go, got %q", got)
	}
}

func TestFormatLocation_EmptyPath(t *testing.T) {
	got := formatLocation("", 1, 5)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
