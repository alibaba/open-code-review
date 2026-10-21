// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestOutputCodeQuality_EmptyIsArrayNotNull(t *testing.T) {
	var buf bytes.Buffer
	if err := outputCodeQuality(nil, &buf); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("empty report = %q, want []", got)
	}
}

func TestOutputCodeQuality_RequiredFields(t *testing.T) {
	comments := []model.LlmComment{{
		Path: "internal/db/query.go", StartLine: 42, EndLine: 45,
		Category: "security", Severity: "critical",
		Content: "SQL built by string concatenation", ExistingCode: `q := "SELECT " + id`,
	}}
	var buf bytes.Buffer
	if err := outputCodeQuality(comments, &buf); err != nil {
		t.Fatal(err)
	}
	var issues []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &issues); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	is := issues[0]
	for _, k := range []string{"description", "check_name", "fingerprint", "severity", "location"} {
		if _, ok := is[k]; !ok {
			t.Errorf("missing required field %q", k)
		}
	}
	loc := is["location"].(map[string]any)
	lines := loc["lines"].(map[string]any)
	if loc["path"] != "internal/db/query.go" || lines["begin"].(float64) != 42 || lines["end"].(float64) != 45 {
		t.Errorf("location = %v", loc)
	}
	if is["check_name"] != "ocr/security" || is["severity"] != "critical" {
		t.Errorf("check_name/severity = %v/%v", is["check_name"], is["severity"])
	}
	if cats := is["categories"].([]any); len(cats) != 1 || cats[0] != "Security" {
		t.Errorf("categories = %v", cats)
	}
}

func TestCodeQualitySeverityMapping(t *testing.T) {
	for in, want := range map[string]string{
		"critical": "critical", "high": "major", "medium": "minor", "low": "info", "": "info", "weird": "info",
	} {
		if got := codeQualitySeverity(in); got != want {
			t.Errorf("codeQualitySeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodeQualityIssues_SkipsPathlessAndFixesLines(t *testing.T) {
	issues := codeQualityIssues([]model.LlmComment{
		{Content: "project-level remark"},                                     // no path: GitLab would reject it
		{Path: "README.md", Content: "file-level", Category: "documentation"}, // no lines
		{Path: "a.go", StartLine: 10, EndLine: 3, Content: "inverted range"},
	})
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2 (pathless finding dropped)", len(issues))
	}
	if l := issues[0].Location.Lines; l.Begin != 1 || l.End != 0 {
		t.Errorf("file-level lines = %+v, want begin=1", l)
	}
	if issues[0].Categories[0] != "Clarity" {
		t.Errorf("documentation category = %v", issues[0].Categories)
	}
	if l := issues[1].Location.Lines; l.Begin != 10 || l.End != 0 {
		t.Errorf("inverted range lines = %+v, want begin=10 without end", l)
	}
	if issues[1].CheckName != "ocr/other" {
		t.Errorf("empty category check_name = %q", issues[1].CheckName)
	}
}

func TestCodeQualityIssues_FingerprintsAreStableAndUnique(t *testing.T) {
	dup := model.LlmComment{Path: "a.go", StartLine: 5, EndLine: 5, Category: "bug", ExistingCode: "x := y", Content: "first wording"}
	again := dup
	again.Content = "different LLM wording, same finding"
	issues := codeQualityIssues([]model.LlmComment{dup, again})
	if issues[0].Fingerprint == issues[1].Fingerprint {
		t.Fatal("duplicate findings must get distinct fingerprints")
	}
	if issues[1].Fingerprint != issues[0].Fingerprint+"#1" {
		t.Errorf("second fingerprint = %q, want base+#1", issues[1].Fingerprint)
	}
	// Same identity as SARIF, so both reports agree across runs.
	if want := sarifFingerprints(dup, "bug")[sarifFingerprintKey]; issues[0].Fingerprint != want {
		t.Errorf("fingerprint %q differs from SARIF %q", issues[0].Fingerprint, want)
	}
}

func TestValidateOutputFormat_AcceptsCodeQuality(t *testing.T) {
	got, err := validateOutputFormat("  CodeQuality ")
	if err != nil || got != "codequality" {
		t.Fatalf("validateOutputFormat = %q, %v", got, err)
	}
	if !isMachineReadable("codequality") {
		t.Error("codequality must be machine-readable so progress stays off stdout")
	}
}

func TestOutputPreview_RejectsCodeQuality(t *testing.T) {
	err := outputPreview(nil, "codequality", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "codequality") {
		t.Fatalf("err = %v, want rejection mentioning codequality", err)
	}
}

func TestCodeQualityCategoryMapping(t *testing.T) {
	for in, want := range map[string]string{
		"bug": "Bug Risk", "test": "Bug Risk", "security": "Security", "performance": "Performance",
		"maintainability": "Complexity", "style": "Style", "documentation": "Clarity", "other": "Bug Risk",
	} {
		if got := codeQualityCategory(in); got != want {
			t.Errorf("codeQualityCategory(%q) = %q, want %q", in, got, want)
		}
	}
}

// A run that reviewed nothing must still hand GitLab a Code Quality array:
// the shared no-files shortcut used to fall through to SARIF for every
// machine-readable format other than json.
func TestEmitRunResult_CodeQualityNoFilesIsEmptyArray(t *testing.T) {
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), &mockResultProvider{}, nil, time.Now(), "codequality", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if strings.TrimSpace(got) != "[]" {
		t.Fatalf("no-files codequality output = %q, want []", got)
	}
}

func TestEmitRunResult_CodeQualityWritesIssues(t *testing.T) {
	comments := []model.LlmComment{{Path: "main.go", StartLine: 3, EndLine: 3, Category: "bug", Severity: "high", Content: "nil dereference"}}
	got := captureStdout(t, func() {
		err := emitRunResult(context.Background(), &mockResultProvider{filesReviewed: 1}, comments, time.Now(), "codequality", "developer", nil, nil, os.Stdout, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	var issues []codeQualityIssue
	if err := json.Unmarshal([]byte(got), &issues); err != nil {
		t.Fatalf("stdout is not a Code Quality array: %v\n%s", err, got)
	}
	if len(issues) != 1 || issues[0].Severity != "major" || issues[0].Location.Path != "main.go" {
		t.Fatalf("issues = %+v", issues)
	}
}
