package reviewbundle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
)

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestReadLimitedRejectsOversizedAndReadErrors(t *testing.T) {
	if _, err := readLimited(strings.NewReader(strings.Repeat("x", MaxProtocolDocumentBytes+1))); err == nil {
		t.Fatal("readLimited() error = nil, want size limit error")
	}
	if _, err := readLimited(failingReader{}); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("readLimited(failingReader) error = %v, want read failure", err)
	}
}

func TestFilterOversizedInputsReportsSkippedItems(t *testing.T) {
	keptDiffs, warnings := filterOversizedDiffs([]model.Diff{
		{OldPath: "old.go", Diff: "small"},
		{NewPath: "large.go", Diff: "large"},
	}, 1)
	if len(keptDiffs) != 0 || len(warnings) != 2 {
		t.Fatalf("diff filter kept=%d warnings=%+v, want all skipped", len(keptDiffs), warnings)
	}
	if warnings[0].Code != "oversized_diff" || !strings.Contains(warnings[0].Message, "old.go") {
		t.Fatalf("first warning = %+v, want old path fallback", warnings[0])
	}

	keptScans, skipped := filterOversizedScanItems([]model.ScanItem{
		{Path: "a.go", Content: "content"},
	}, 1)
	if len(keptScans) != 0 || len(skipped) != 1 ||
		skipped[0].Reason != "oversized_scan" ||
		skipped[0].EstimatedTokens <= 0 {
		t.Fatalf("scan filter kept=%d skipped=%+v", len(keptScans), skipped)
	}

	keptDiffs, warnings = filterOversizedDiffs([]model.Diff{{NewPath: "small.go", Diff: "small"}}, 0)
	if len(keptDiffs) != 1 || len(warnings) != 0 {
		t.Fatalf("default token limit kept=%d warnings=%+v, want keep", len(keptDiffs), warnings)
	}
}

func TestEstimateAgentTokenHelpers(t *testing.T) {
	diffTokens := estimateDiffManifestTokens([]Bundle{{
		Files: []File{
			{Path: "reviewed.go", Reviewable: true, Patch: "package main\n"},
			{Path: "skipped.go", Reviewable: false, Patch: strings.Repeat("skip ", 100)},
		},
	}})
	if diffTokens != int64(llm.CountTokens("package main\n")) {
		t.Fatalf("estimateDiffManifestTokens() = %d, want only reviewable patch counted", diffTokens)
	}

	contentTokens := estimateAgentContentTokens([]model.ScanItem{
		{Path: "a.go", Content: "package a\n"},
		{Path: "b.go", Content: "package b\n"},
	})
	wantContentTokens := int64(llm.CountTokens("package a\n") + llm.CountTokens("package b\n"))
	if contentTokens != wantContentTokens {
		t.Fatalf("estimateAgentContentTokens() = %d, want sum of scan content tokens", contentTokens)
	}
}

func TestValidateCommentsChecksExistingCodeAgainstTargetContent(t *testing.T) {
	repository := t.TempDir()
	content := []byte("dup\nunique\ndup\n")
	if err := os.WriteFile(filepath.Join(repository, "main.go"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := validationBundle()
	bundle.Target.Mode = TargetScan
	bundle.Files[0].ContentSHA256 = hashFields(content)
	bundle.Files[0].Hunks = nil
	bundle.Summary.ReviewableFiles = 1
	comments := &Comments{
		SchemaVersion: CommentsSchemaVersion,
		BundleID:      bundle.BundleID,
		Summary:       CommentsSummary{FilesReviewed: 1, IssuesFound: 3},
		Comments: []ReviewComment{
			{
				Path: "main.go", StartLine: 1, EndLine: 1,
				Priority: "high", Category: "bug", Title: "mismatch",
				Content: "mismatch", Recommendation: "fix", Confidence: 1,
				ExistingCode: "missing",
			},
			{
				Path: "main.go", StartLine: 2, EndLine: 2,
				Priority: "medium", Category: "maintainability", Title: "ambiguous",
				Content: "ambiguous", Recommendation: "fix", Confidence: 0.8,
				ExistingCode: "dup",
			},
			{
				Path: "main.go", StartLine: 0, EndLine: 0,
				FileLevelComment: true,
				Priority:         "low", Category: "test", Title: "file",
				Content: "file", Recommendation: "fix", Confidence: 0.5,
			},
		},
	}

	result := ValidateComments(context.Background(), bundle, comments, repository, nil)
	if result.Valid {
		t.Fatalf("ValidateComments() valid = true, errors=%+v", result.Errors)
	}
	assertValidationCode(t, result.Errors, "existing_code_mismatch")
	assertValidationCode(t, result.Errors, "ambiguous_existing_code")
	if len(result.Errors) != 2 {
		t.Fatalf("errors = %+v, want only content evidence errors", result.Errors)
	}
}

func TestDecodeStrictRejectsMultipleJSONValues(t *testing.T) {
	var target map[string]string
	err := decodeStrict(strings.NewReader(`{"a":"b"} {"c":"d"}`), &target)
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("decodeStrict() error = %v, want multiple JSON values", err)
	}

	err = decodeStrict(io.MultiReader(strings.NewReader(`{"a":"b"}`), failingReader{}), &target)
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("decodeStrict(read error) error = %v, want read failure", err)
	}
}

func TestMarkdownValidationRendersErrorsAndWarnings(t *testing.T) {
	var output bytes.Buffer
	writeMarkdownValidation(&output, &ValidationResult{
		Valid: false,
		Errors: []ValidationNotice{{
			Code: "stale_bundle", Message: "target changed",
		}},
		Warnings: []ValidationNotice{{
			Code: "outside_changed_hunk", Message: "outside hunk",
		}},
	})
	text := output.String()
	for _, want := range []string{
		"Validation: INVALID",
		"Validation errors",
		"`stale_bundle`: target changed",
		"Validation warnings",
		"`outside_changed_hunk`: outside hunk",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("validation markdown missing %q:\n%s", want, text)
		}
	}
}

func TestProtocolErrorStringsIncludeCodeAndPartitionSize(t *testing.T) {
	protocolError := (&ProtocolError{Code: "bundle_too_large", Message: "too large"}).Error()
	if !strings.Contains(protocolError, "bundle_too_large") ||
		!strings.Contains(protocolError, "too large") {
		t.Fatalf("ProtocolError string = %q", protocolError)
	}

	partitionError, ok := singleFilePartitionError("large.go", 2048, 1024).(*ProtocolError)
	if !ok ||
		partitionError.Code != "bundle_too_large" ||
		!strings.Contains(partitionError.Error(), "large.go") ||
		!strings.Contains(partitionError.Error(), "2048-byte") {
		t.Fatalf("singleFilePartitionError() = %+v", partitionError)
	}
}
