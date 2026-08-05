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
	opts, err := parseReviewFlags([]string{"--max-tools", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.maxTools != 10 {
		t.Errorf("maxTools = %d, want 10 (clamped to min)", opts.maxTools)
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
