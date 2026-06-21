package main

import "testing"

func TestParseReviewFlagsModelOverride(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--model", "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}

	if opts.model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", opts.model, "claude-opus-4-6")
	}
	if opts.outputFormat != "text" {
		t.Errorf("outputFormat = %q, want %q", opts.outputFormat, "text")
	}
	if opts.audience != "human" {
		t.Errorf("audience = %q, want %q", opts.audience, "human")
	}
}

func TestParseReviewFlagsSaveResult(t *testing.T) {
	opts, err := parseReviewFlags([]string{
		"--save-result",
		"--result-dir", "/tmp/ocr-results",
		"--result-project", "group/project",
		"--result-source-branch", "feature/a",
		"--result-target-branch", "main",
	})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}

	if !opts.saveResult {
		t.Fatal("saveResult = false, want true")
	}
	if opts.resultDir != "/tmp/ocr-results" {
		t.Errorf("resultDir = %q", opts.resultDir)
	}
	if opts.resultProject != "group/project" {
		t.Errorf("resultProject = %q", opts.resultProject)
	}
	if opts.resultSource != "feature/a" {
		t.Errorf("resultSource = %q", opts.resultSource)
	}
	if opts.resultTarget != "main" {
		t.Errorf("resultTarget = %q", opts.resultTarget)
	}
}

func TestParseReviewFlagsRulesDirAndEnvResultDir(t *testing.T) {
	t.Setenv("OCR_REVIEWS_DIR", "/mnt/ocr/reviews")
	opts, err := parseReviewFlags([]string{"--rules-dir", "/mnt/ocr/rules"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}
	if opts.rulesDir != "/mnt/ocr/rules" {
		t.Errorf("rulesDir = %q", opts.rulesDir)
	}
	if opts.resultDir != "/mnt/ocr/reviews" {
		t.Errorf("resultDir = %q", opts.resultDir)
	}
}
