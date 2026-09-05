// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"
)

func TestParseReviewFlagsBackgroundFile(t *testing.T) {
	for _, flag := range []string{"--background-file", "-B"} {
		t.Run(flag, func(t *testing.T) {
			opts, err := parseReviewFlags([]string{flag, "./docs/req.md"})
			if err != nil {
				t.Fatalf("parseReviewFlags: %v", err)
			}
			if opts.backgroundFile != "./docs/req.md" {
				t.Errorf("backgroundFile = %q, want %q", opts.backgroundFile, "./docs/req.md")
			}
		})
	}
}

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

func TestParseReviewFlagsProviderAndModelOverrides(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--provider", "anthropic", "--model", "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}
	if opts.provider != "anthropic" || opts.model != "claude-opus-4-6" {
		t.Fatalf("provider=%q model=%q", opts.provider, opts.model)
	}
}

func TestParseReviewFlagsResume(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "feature", "--resume", "session-123"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}
	if opts.resume != "session-123" {
		t.Errorf("resume = %q, want session-123", opts.resume)
	}
}

func TestParseReviewFlags_PreviewWithResume(t *testing.T) {
	_, err := parseReviewFlags([]string{"--commit", "abc123", "--preview", "--resume", "session-123"})
	if err == nil {
		t.Fatal("expected error for --preview with --resume")
	}
}

func TestParseReviewFlags_ReuseFrom(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "feature", "--reuse-from", "ocr-result.json"})
	if err != nil {
		t.Fatalf("parseReviewFlags: %v", err)
	}
	if opts.reuseFrom != "ocr-result.json" {
		t.Errorf("reuseFrom = %q, want ocr-result.json", opts.reuseFrom)
	}
}

func TestParseReviewFlags_ReuseWithResumeRejected(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main", "--to", "feature", "--reuse-from", "prev.json", "--resume", "session-123"})
	if err == nil {
		t.Fatal("expected error for --reuse-from with --resume")
	}
}

func TestParseReviewFlags_PreviewWithReuseFromRejected(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main", "--to", "feature", "--reuse-from", "prev.json", "--preview"})
	if err == nil {
		t.Fatal("expected error for --preview with --reuse-from")
	}
}

func TestParseReviewFlags_InvalidAudience(t *testing.T) {
	_, err := parseReviewFlags([]string{"--audience", "robot"})
	if err == nil {
		t.Fatal("expected error for invalid audience")
	}
}

func TestParseReviewFlags_NegativeMaxTools(t *testing.T) {
	_, err := parseReviewFlags([]string{"--max-tools", "-1"})
	if err == nil {
		t.Fatal("expected error for negative max-tools")
	}
}

func TestParseReviewFlags_MaxToolsBelowMin(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--max-tools", "30"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.maxTools != 50 {
		t.Errorf("maxTools = %d, want 50 (clamped to min)", opts.maxTools)
	}
}

func TestParseReviewFlags_NegativeMaxGitProcs(t *testing.T) {
	_, err := parseReviewFlags([]string{"--max-git-procs", "-1"})
	if err == nil {
		t.Fatal("expected error for negative max-git-procs")
	}
}

func TestParseReviewFlags_NegativeMaxTokensBudget(t *testing.T) {
	_, err := parseReviewFlags([]string{"--max-tokens-budget", "-1"})
	if err == nil {
		t.Fatal("expected error for negative max-tokens-budget")
	}
}

func TestParseReviewFlags_NegativeMaxTokens(t *testing.T) {
	_, err := parseReviewFlags([]string{"--max-tokens", "-1"})
	if err == nil {
		t.Fatal("expected error for negative max-tokens")
	}
}

func TestParseReviewFlags_MaxTokensParsed(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--max-tokens", "200000"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.maxTokens != 200000 {
		t.Errorf("maxTokens = %d, want 200000", opts.maxTokens)
	}
}

func TestParseReviewFlags_BudgetFlagsDefaultZero(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.maxTokensBudget != 0 {
		t.Errorf("maxTokensBudget = %d, want 0 (default unlimited)", opts.maxTokensBudget)
	}
}

func TestParseReviewFlags_BudgetFlagsParsed(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--max-tokens-budget", "120000"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.maxTokensBudget != 120000 {
		t.Errorf("maxTokensBudget = %d, want 120000", opts.maxTokensBudget)
	}
}

func TestParseReviewFlags_ConflictingModes(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main", "--to", "dev", "--commit", "abc"})
	if err == nil {
		t.Fatal("expected error for conflicting modes")
	}
}

func TestParseReviewFlags_FromWithoutTo(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main"})
	if err == nil {
		t.Fatal("expected error for --from without --to")
	}
}

func TestParseReviewFlags_ToWithoutFrom(t *testing.T) {
	_, err := parseReviewFlags([]string{"--to", "dev"})
	if err == nil {
		t.Fatal("expected error for --to without --from")
	}
}

func TestParseReviewFlags_ShortFlags(t *testing.T) {
	opts, err := parseReviewFlags([]string{"-c", "abc123", "-f", "json", "-p"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.commit != "abc123" {
		t.Errorf("commit = %q, want abc123", opts.commit)
	}
	if opts.outputFormat != "json" {
		t.Errorf("outputFormat = %q, want json", opts.outputFormat)
	}
	if !opts.preview {
		t.Error("expected preview=true")
	}
}

func TestParseReviewFlags_OutputPath(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"long flag", []string{"--output", "result.json"}, "result.json"},
		{"short flag", []string{"-o", "result.json"}, "result.json"},
		{"stdout dash", []string{"-o", "-"}, "-"},
		{"default empty", []string{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseReviewFlags(tc.args)
			if err != nil {
				t.Fatalf("parseReviewFlags: %v", err)
			}
			if opts.outputPath != tc.want {
				t.Errorf("outputPath = %q, want %q", opts.outputPath, tc.want)
			}
		})
	}
}

func TestParseScanFlags_OutputPath(t *testing.T) {
	opts, err := parseScanFlags([]string{"--output", "scan.json"})
	if err != nil {
		t.Fatalf("parseScanFlags: %v", err)
	}
	if opts.outputPath != "scan.json" {
		t.Errorf("outputPath = %q, want scan.json", opts.outputPath)
	}
}

func TestParseReviewFlags_InvalidFormat(t *testing.T) {
	_, err := parseReviewFlags([]string{"--format", "xml"})
	if err == nil {
		t.Fatal("expected error for invalid format 'xml'")
	}
}

func TestParseScanFlags_InvalidFormat(t *testing.T) {
	_, err := parseScanFlags([]string{"--format", "yaml"})
	if err == nil {
		t.Fatal("expected error for invalid format 'yaml'")
	}
}

func TestParseReviewFlags_NormalizedFormat(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--format", " JSON "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.outputFormat != "json" {
		t.Errorf("outputFormat = %q, want json", opts.outputFormat)
	}
}

func TestParseScanFlags_NormalizedFormat(t *testing.T) {
	opts, err := parseScanFlags([]string{"--format", " SARIF "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.outputFormat != "sarif" {
		t.Errorf("outputFormat = %q, want sarif", opts.outputFormat)
	}
}
