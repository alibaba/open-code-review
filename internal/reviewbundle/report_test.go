package reviewbundle

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportMarkdownIsStableAndPriorityOrdered(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 2},
		Comments: []ReviewComment{
			{
				Path: "main.go", StartLine: 8, EndLine: 8,
				Priority: "low", Category: "test", Title: "Later",
				Content: "low content", Recommendation: "add test", Confidence: 0.6,
			},
			{
				Path: "main.go", StartLine: 3, EndLine: 4,
				Priority: "high", Category: "bug", Title: "First",
				Content: "high content", Recommendation: "fix it", Confidence: 0.95,
			},
		},
	}

	report, err := RenderReport(bundle, comments, ReportOptions{Format: "markdown"})
	if err != nil {
		t.Fatalf("RenderReport() error = %v", err)
	}
	text := string(report)
	if strings.Index(text, "[HIGH]") > strings.Index(text, "[LOW]") {
		t.Fatalf("report is not priority ordered:\n%s", text)
	}
	if !strings.Contains(text, "`main.go:3-4`") || !strings.Contains(text, "Validation: not supplied") {
		t.Fatalf("report missing evidence metadata:\n%s", text)
	}
}

func TestReportJSONPreservesCommentsProtocol(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{},
		Comments:      []ReviewComment{},
	}
	report, err := RenderReport(bundle, comments, ReportOptions{Format: "json"})
	if err != nil {
		t.Fatalf("RenderReport() error = %v", err)
	}
	var decoded Comments
	if err := json.Unmarshal(report, &decoded); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if decoded.BundleID != comments.BundleID || decoded.Comments == nil {
		t.Fatalf("decoded report = %+v", decoded)
	}
}

func TestReportIncludesValidationFailures(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{},
		Comments:      []ReviewComment{},
	}
	validation := &ValidationResult{
		SchemaVersion: "codex-review-validation/v1",
		BundleID:      bundle.BundleID,
		Valid:         false,
		Errors: []ValidationNotice{{
			Code: "stale_bundle", Message: "target changed",
		}},
		Warnings: []ValidationNotice{{
			Code: "outside_changed_hunk", Message: "line is outside changed hunk",
		}},
	}
	report, err := RenderReport(
		bundle,
		comments,
		ReportOptions{Format: "text", Validation: validation},
	)
	if err != nil {
		t.Fatalf("RenderReport() error = %v", err)
	}
	if !strings.Contains(string(report), "INVALID") ||
		!strings.Contains(string(report), "stale_bundle") ||
		!strings.Contains(string(report), "outside_changed_hunk") {
		t.Fatalf("report missing validation failure:\n%s", report)
	}
}

func TestReportEscapesBackticksAndTextWarnings(t *testing.T) {
	bundle := validationBundle()
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 1},
		Comments: []ReviewComment{{
			Path: "main`weird.go", StartLine: 1, EndLine: 1,
			Priority: "high", Category: "bug", Title: "Fence",
			Content: "content", Recommendation: "fix", Confidence: 1,
			ExistingCode: "````\ncode",
		}},
		Warnings: []ProtocolNotice{{Code: "partial", Message: "some scope skipped"}},
	}
	markdown, err := RenderReport(bundle, comments, ReportOptions{Format: "markdown"})
	if err != nil {
		t.Fatalf("RenderReport(markdown) error = %v", err)
	}
	if !strings.Contains(string(markdown), "`` main`weird.go:1 ``") ||
		!strings.Contains(string(markdown), "`````") {
		t.Fatalf("markdown report did not escape code spans/fences:\n%s", markdown)
	}
	text, err := RenderReport(bundle, comments, ReportOptions{Format: "text"})
	if err != nil {
		t.Fatalf("RenderReport(text) error = %v", err)
	}
	if !strings.Contains(string(text), "WARNING partial") {
		t.Fatalf("text report missing warnings:\n%s", text)
	}
}
