// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

func TestCommandNeedsGit(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{name: "review", cmd: &cobra.Command{Use: "review"}, want: true},
		{name: "scan", cmd: &cobra.Command{Use: "scan"}, want: true},
		{name: "delegate", cmd: &cobra.Command{Use: "delegate"}, want: true},
		{name: "version", cmd: &cobra.Command{Use: "version"}, want: false},
		{name: "completion", cmd: &cobra.Command{Use: "completion"}, want: false},
		{name: "help", cmd: &cobra.Command{Use: "help"}, want: false},
		{name: "config", cmd: &cobra.Command{Use: "config"}, want: false},
		{name: "llm", cmd: &cobra.Command{Use: "llm"}, want: false},
		{name: "viewer", cmd: &cobra.Command{Use: "viewer"}, want: false},
		{name: "session", cmd: &cobra.Command{Use: "session"}, want: false},
		{name: "rules", cmd: &cobra.Command{Use: "rules"}, want: false},
		{name: "root", cmd: &cobra.Command{Use: "ocr"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandNeedsGit(tt.cmd); got != tt.want {
				t.Errorf("commandNeedsGit(%q) = %v, want %v", tt.cmd.Use, got, tt.want)
			}
		})
	}
}

// TestCommandNeedsGit_Subcommands verifies that the git check keys off the
// top-level command rather than the leaf: the delegate subcommands shell out to
// git, while the no-op parent help commands do not.
func TestCommandNeedsGit_Subcommands(t *testing.T) {
	root := &cobra.Command{Use: "ocr"}
	delegate := &cobra.Command{Use: "delegate"}
	delegatePreview := &cobra.Command{Use: "preview"}
	delegateRule := &cobra.Command{Use: "rule"}
	delegate.AddCommand(delegatePreview, delegateRule)
	root.AddCommand(delegate)

	rules := &cobra.Command{Use: "rules"}
	rulesCheck := &cobra.Command{Use: "check"}
	rules.AddCommand(rulesCheck)
	root.AddCommand(rules)

	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{name: "delegate preview", cmd: delegatePreview, want: true},
		{name: "delegate rule", cmd: delegateRule, want: true},
		{name: "delegate", cmd: delegate, want: true},
		{name: "rules check", cmd: rulesCheck, want: false},
		{name: "rules", cmd: rules, want: false},
		{name: "root", cmd: root, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandNeedsGit(tt.cmd); got != tt.want {
				t.Errorf("commandNeedsGit(%q) = %v, want %v", tt.cmd.CommandPath(), got, tt.want)
			}
		})
	}
}

func TestCommandNeedsGit_RootVersionFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "ocr"}
	cmd.Flags().BoolP("version", "V", false, "version for ocr")
	if err := cmd.Flags().Set("version", "true"); err != nil {
		t.Fatalf("set version flag: %v", err)
	}
	if commandNeedsGit(cmd) {
		t.Error("commandNeedsGit() = true for root --version, want false")
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

func TestConcurrencyFlagUsageUsesSubtask(t *testing.T) {
	var reviewOpts reviewOptions
	reviewCmd := &cobra.Command{Use: "review"}
	registerReviewFlags(reviewCmd, &reviewOpts)
	reviewFlag := reviewCmd.Flags().Lookup("concurrency")
	if reviewFlag == nil {
		t.Fatal("review --concurrency flag missing")
	}

	var scanOpts scanOptions
	scanCmd := &cobra.Command{Use: "scan"}
	registerScanFlags(scanCmd, &scanOpts)
	scanFlag := scanCmd.Flags().Lookup("concurrency")
	if scanFlag == nil {
		t.Fatal("scan --concurrency flag missing")
	}

	for _, tc := range []struct {
		cmd   string
		usage string
	}{
		{"review", reviewFlag.Usage},
		{"scan", scanFlag.Usage},
	} {
		if !strings.Contains(tc.usage, "subtask") {
			t.Errorf("%s --concurrency usage %q: want subtask unit", tc.cmd, tc.usage)
		}
		for _, leaked := range []string{"file-group", "file group", "file scans"} {
			if strings.Contains(tc.usage, leaked) {
				t.Errorf("%s --concurrency usage %q: leaked %q", tc.cmd, tc.usage, leaked)
			}
		}
	}
	if reviewFlag.Usage != scanFlag.Usage {
		t.Errorf("review and scan --concurrency should share phrasing: review %q scan %q", reviewFlag.Usage, scanFlag.Usage)
	}
}

func TestScanMaxToolsFlagUsageUsesSubtask(t *testing.T) {
	var reviewOpts reviewOptions
	reviewCmd := &cobra.Command{Use: "review"}
	registerReviewFlags(reviewCmd, &reviewOpts)
	reviewFlag := reviewCmd.Flags().Lookup("max-tools")
	if reviewFlag == nil {
		t.Fatal("review --max-tools flag missing")
	}

	var scanOpts scanOptions
	scanCmd := &cobra.Command{Use: "scan"}
	registerScanFlags(scanCmd, &scanOpts)
	scanFlag := scanCmd.Flags().Lookup("max-tools")
	if scanFlag == nil {
		t.Fatal("scan --max-tools flag missing")
	}

	for _, tc := range []struct {
		cmd   string
		usage string
	}{
		{"review", reviewFlag.Usage},
		{"scan", scanFlag.Usage},
	} {
		if !strings.Contains(tc.usage, "subtask") {
			t.Errorf("%s --max-tools usage %q: want subtask unit", tc.cmd, tc.usage)
		}
		if strings.Contains(tc.usage, "per file") {
			t.Errorf("%s --max-tools usage %q: leaked per file", tc.cmd, tc.usage)
		}
	}
}
