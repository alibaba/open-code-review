package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/agent"
	"github.com/open-code-review/open-code-review/internal/model"
)

func TestHasSubtaskErrors(t *testing.T) {
	tests := []struct {
		name     string
		warnings []agent.AgentWarning
		want     bool
	}{
		{"nil warnings", nil, false},
		{"empty", []agent.AgentWarning{}, false},
		{"no subtask errors", []agent.AgentWarning{{Type: "other", Message: "msg"}}, false},
		{"has subtask error", []agent.AgentWarning{{Type: "subtask_error", Message: "fail"}}, true},
		{"mixed", []agent.AgentWarning{{Type: "warn"}, {Type: "subtask_error"}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasSubtaskErrors(tc.warnings)
			if got != tc.want {
				t.Errorf("hasSubtaskErrors() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWrapByRunes(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		maxW  int
		lines int
	}{
		{"empty", "", 80, 0},
		{"short line", "hello", 80, 1},
		{"exact width", strings.Repeat("a", 10), 10, 1},
		{"wraps long line", strings.Repeat("word ", 25), 20, 7},
		{"respects newlines", "line1\nline2\nline3", 80, 3},
		{"wrap with newlines", "short\n" + strings.Repeat("x", 50), 20, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapByRunes(tc.text, tc.maxW)
			if len(got) != tc.lines {
				t.Errorf("wrapByRunes() got %d lines, want %d\nlines: %v", len(got), tc.lines, got)
			}
		})
	}
}

func TestWrapSingleRuneLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		maxW int
		min  int
	}{
		{"short line unchanged", "hello", 100, 1},
		{"wraps at space", "hello world foo bar baz", 12, 2},
		{"no space to wrap", strings.Repeat("x", 30), 10, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapSingleRuneLine(tc.line, tc.maxW)
			if len(got) < tc.min {
				t.Errorf("got %d lines, want at least %d", len(got), tc.min)
			}
		})
	}
}

func TestRuneWrapCut(t *testing.T) {
	// Short line returns full length
	runes := []rune("short")
	cut := runeWrapCut(runes, 100)
	if cut != len(runes) {
		t.Errorf("expected %d, got %d", len(runes), cut)
	}

	// Cuts at space
	runes = []rune("hello world test")
	cut = runeWrapCut(runes, 11)
	if runes[cut] != ' ' && cut != 11 {
		t.Errorf("expected cut at space boundary, got %d (char=%c)", cut, runes[cut])
	}
}

func TestVisibleRunesLen(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"\x01\x02\x03", 0},
		{"a\x01b", 2},
		{"\x7f", 0},
	}
	for _, tc := range tests {
		got := visibleRunesLen([]rune(tc.input))
		if got != tc.want {
			t.Errorf("visibleRunesLen(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestSplitToLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a\nb\nc", 3},
		{"a\nb\nc\n", 3},
		{"single", 1},
		{"crlf\r\nline", 2},
		{"", 0},
	}
	for _, tc := range tests {
		got := splitToLines(tc.input)
		if len(got) != tc.want {
			t.Errorf("splitToLines(%q) = %d lines, want %d", tc.input, len(got), tc.want)
		}
	}
}

func TestBuildDiffLines(t *testing.T) {
	t.Run("empty suggestion returns nil", func(t *testing.T) {
		c := model.LlmComment{ExistingCode: "old", SuggestionCode: ""}
		got := buildDiffLines(c)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("empty existing returns nil", func(t *testing.T) {
		c := model.LlmComment{ExistingCode: "", SuggestionCode: "new"}
		got := buildDiffLines(c)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("diff computed", func(t *testing.T) {
		c := model.LlmComment{
			ExistingCode:   "line1\nline2\n",
			SuggestionCode: "line1\nmodified\n",
		}
		got := buildDiffLines(c)
		if len(got) == 0 {
			t.Error("expected non-empty diff lines")
		}
	})
}

func TestOutputJSONWithWarnings_NoCommentsSubtaskError(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	warnings := []agent.AgentWarning{{Type: "subtask_error", File: "x.go", Message: "fail"}}
	err := outputJSONWithWarnings(nil, warnings, 1, 10, 5, 15, 0, 0, time.Second, "", nil, "abc123trace", nil, "")
	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "completed_with_errors" {
		t.Errorf("status = %q, want completed_with_errors", out.Status)
	}
	if !strings.Contains(out.Message, "errors") {
		t.Errorf("message = %q, expected to mention errors", out.Message)
	}
	if out.TraceID != "abc123trace" {
		t.Errorf("trace_id = %q, want abc123trace", out.TraceID)
	}
}

func TestStatusBadge(t *testing.T) {
	tests := []struct {
		status string
		substr string
	}{
		{"added", "[A]"},
		{"modified", "[M]"},
		{"deleted", "[D]"},
		{"renamed", "[R]"},
		{"binary", "[B]"},
		{"scan", "[S]"},
		{"unknown", "[?]"},
	}
	for _, tc := range tests {
		got := statusBadge(tc.status)
		if !strings.Contains(got, tc.substr) {
			t.Errorf("statusBadge(%q) = %q, expected to contain %q", tc.status, got, tc.substr)
		}
	}
}

func TestOutputJSON(t *testing.T) {
	// Redirect stdout to capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix bug", StartLine: 1, EndLine: 5},
	}
	err := outputJSON(comments)

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputJSON error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Status != "success" {
		t.Errorf("status = %q, want success", out.Status)
	}
	if len(out.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(out.Comments))
	}
}

func TestOutputJSON_NoComments(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputJSON(nil)

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("outputJSON error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message when no comments")
	}
}

func TestOutputJSONWithWarnings(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	comments := []model.LlmComment{{Path: "b.go", Content: "test"}}
	warnings := []agent.AgentWarning{{Type: "subtask_error", File: "c.go", Message: "failed"}}
	err := outputJSONWithWarnings(comments, warnings, 5, 100, 50, 150, 10, 5, 3*time.Second, "summary", map[string]int64{"file_read": 3}, "trace-xyz-789", nil, "")
	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "completed_with_errors" {
		t.Errorf("status = %q, want completed_with_errors", out.Status)
	}
	if out.Summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if out.Summary.FilesReviewed != 5 {
		t.Errorf("FilesReviewed = %d, want 5", out.Summary.FilesReviewed)
	}
	if out.ToolCalls == nil || out.ToolCalls.Total != 3 {
		t.Errorf("ToolCalls.Total = %v", out.ToolCalls)
	}
	if out.TraceID != "trace-xyz-789" {
		t.Errorf("trace_id = %q, want trace-xyz-789", out.TraceID)
	}
}

func TestOutputJSONWithWarnings_NoCommentsNoErrors(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	warnings := []agent.AgentWarning{{Type: "warning", Message: "something"}}
	err := outputJSONWithWarnings(nil, warnings, 2, 50, 20, 70, 0, 0, time.Second, "", nil, "", nil, "")
	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "completed_with_warnings" {
		t.Errorf("status = %q, want completed_with_warnings", out.Status)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestOutputJSONNoFiles(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputJSONNoFiles("test-trace-id-456")

	_ = w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Status != "skipped" {
		t.Errorf("status = %q, want skipped", out.Status)
	}
	if out.TraceID != "test-trace-id-456" {
		t.Errorf("trace_id = %q, want test-trace-id-456", out.TraceID)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestOutputText_NoComments(t *testing.T) {
	got := captureStdout(t, func() {
		outputText(nil)
	})
	if !strings.Contains(got, "Looks good to me") {
		t.Errorf("expected 'Looks good to me', got %q", got)
	}
}

func TestOutputText_WithComments(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "main.go", StartLine: 10, EndLine: 15, Content: "potential nil dereference"},
	}
	got := captureStdout(t, func() {
		outputText(comments)
	})
	if !strings.Contains(got, "main.go") {
		t.Errorf("expected path in output, got %q", got)
	}
	if !strings.Contains(got, "potential nil dereference") {
		t.Errorf("expected comment content in output, got %q", got)
	}
}

func TestOutputTextWithWarnings_NoCommentsNoErrors(t *testing.T) {
	warnings := []agent.AgentWarning{{Type: "warning", File: "x.go", Message: "slow"}}
	got := captureStdout(t, func() {
		outputTextWithWarnings(nil, warnings)
	})
	if !strings.Contains(got, "Looks good to me") {
		t.Errorf("expected 'Looks good to me', got %q", got)
	}
}

func TestOutputTextWithWarnings_NoCommentsWithSubtaskError(t *testing.T) {
	warnings := []agent.AgentWarning{{Type: "subtask_error", File: "y.go", Message: "failed"}}
	got := captureStdout(t, func() {
		outputTextWithWarnings(nil, warnings)
	})
	if !strings.Contains(got, "could not be reviewed") {
		t.Errorf("expected subtask error message, got %q", got)
	}
}

func TestOutputTextWithWarnings_WithComments(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", StartLine: 1, EndLine: 3, Content: "fix this"},
	}
	warnings := []agent.AgentWarning{{Type: "info", File: "b.go", Message: "note"}}
	got := captureStdout(t, func() {
		outputTextWithWarnings(comments, warnings)
	})
	if !strings.Contains(got, "a.go") {
		t.Errorf("expected comment path, got %q", got)
	}
	if !strings.Contains(got, "fix this") {
		t.Errorf("expected comment content, got %q", got)
	}
}

func TestRenderComment_EmptyContentNoDiff(t *testing.T) {
	got := captureStdout(t, func() {
		renderComment(model.LlmComment{Path: "skip.go", StartLine: 1, EndLine: 1, Content: "", ExistingCode: "", SuggestionCode: ""})
	})
	if got != "" {
		t.Errorf("expected empty output for empty comment, got %q", got)
	}
}

func TestRenderComment_ContentOnly(t *testing.T) {
	got := captureStdout(t, func() {
		renderComment(model.LlmComment{Path: "file.go", StartLine: 5, EndLine: 10, Content: "consider renaming"})
	})
	if !strings.Contains(got, "file.go:5-10") {
		t.Errorf("expected path:line range, got %q", got)
	}
	if !strings.Contains(got, "consider renaming") {
		t.Errorf("expected content, got %q", got)
	}
}

func TestRenderComment_WithDiff(t *testing.T) {
	got := captureStdout(t, func() {
		renderComment(model.LlmComment{
			Path:           "diff.go",
			StartLine:      1,
			EndLine:        2,
			Content:        "rename var",
			ExistingCode:   "old := 1\n",
			SuggestionCode: "new := 1\n",
		})
	})
	if !strings.Contains(got, "diff.go:1-2") {
		t.Errorf("expected path:line range, got %q", got)
	}
	if !strings.Contains(got, "rename var") {
		t.Errorf("expected content, got %q", got)
	}
}

func TestPrintDiffLine(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		content string
	}{
		{"added", "+", "new line"},
		{"deleted", "-", "old line"},
		{"context", " ", "context line"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := captureStdout(t, func() {
				printDiffLine(tc.prefix, tc.content, "\033[92m", "\033[48;2;0;60;0m")
			})
			if !strings.Contains(got, tc.prefix) {
				t.Errorf("expected prefix %q in output, got %q", tc.prefix, got)
			}
			if !strings.Contains(got, tc.content) {
				t.Errorf("expected content %q in output, got %q", tc.content, got)
			}
		})
	}
}

func TestOutputPreviewText_NoFiles(t *testing.T) {
	p := &agent.DiffPreview{TotalFiles: 0}
	got := captureStdout(t, func() {
		outputPreviewText(p)
	})
	if !strings.Contains(got, "No files changed") {
		t.Errorf("expected 'No files changed', got %q", got)
	}
}

func TestOutputPreviewText_WithReviewableFiles(t *testing.T) {
	p := &agent.DiffPreview{
		Entries: []agent.DiffPreviewEntry{
			{Path: "main.go", Status: "modified", Insertions: 10, Deletions: 3, WillReview: true},
			{Path: "util.go", Status: "added", Insertions: 20, Deletions: 0, WillReview: true},
		},
		TotalInsertions: 30,
		TotalDeletions:  3,
		TotalFiles:      2,
		ReviewableCount: 2,
		ExcludedCount:   0,
	}
	got := captureStdout(t, func() {
		outputPreviewText(p)
	})
	if !strings.Contains(got, "2 file(s) changed") {
		t.Errorf("expected file count, got %q", got)
	}
	if !strings.Contains(got, "Will review (2)") {
		t.Errorf("expected 'Will review' section, got %q", got)
	}
	if !strings.Contains(got, "main.go") || !strings.Contains(got, "util.go") {
		t.Errorf("expected file paths, got %q", got)
	}
}

func TestOutputPreviewText_WithExcludedFiles(t *testing.T) {
	p := &agent.DiffPreview{
		Entries: []agent.DiffPreviewEntry{
			{Path: "src.go", Status: "modified", Insertions: 5, Deletions: 1, WillReview: true},
			{Path: "vendor/lib.go", Status: "added", Insertions: 100, Deletions: 0, WillReview: false, ExcludeReason: model.ExcludeDefaultPath},
		},
		TotalInsertions: 105,
		TotalDeletions:  1,
		TotalFiles:      2,
		ReviewableCount: 1,
		ExcludedCount:   1,
	}
	got := captureStdout(t, func() {
		outputPreviewText(p)
	})
	if !strings.Contains(got, "Will review (1)") {
		t.Errorf("expected 'Will review (1)', got %q", got)
	}
	if !strings.Contains(got, "Excluded from review (1)") {
		t.Errorf("expected 'Excluded from review (1)', got %q", got)
	}
	if !strings.Contains(got, "vendor/lib.go") {
		t.Errorf("expected excluded file path, got %q", got)
	}
	if !strings.Contains(got, "default_path") {
		t.Errorf("expected exclude reason, got %q", got)
	}
}
