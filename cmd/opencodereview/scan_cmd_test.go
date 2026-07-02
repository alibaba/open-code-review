package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
)

func TestExcludeToolDef(t *testing.T) {
	defs := []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
		{Type: "function", Function: llm.FunctionDef{Name: "file_read_diff"}},
		{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
	}
	got := excludeToolDef(defs, "file_read_diff")
	if len(got) != 3 {
		t.Fatalf("expected 3 defs, got %d", len(got))
	}
	for _, d := range got {
		if d.Function.Name == "file_read_diff" {
			t.Errorf("file_read_diff should have been removed")
		}
	}
	// Input slice must not be mutated.
	if len(defs) != 4 {
		t.Errorf("input slice was mutated: len=%d, want 4", len(defs))
	}
}

func TestExcludeToolDef_AbsentName(t *testing.T) {
	defs := []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
	}
	got := excludeToolDef(defs, "does_not_exist")
	if !reflect.DeepEqual(got, defs) {
		t.Errorf("removing absent name should return identical content")
	}
}

func TestSplitPaths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "internal/agent", []string{"internal/agent"}},
		{"multiple", "a.go,b.go,c.go", []string{"a.go", "b.go", "c.go"}},
		{"trims whitespace", "  a.go ,  b.go  ", []string{"a.go", "b.go"}},
		{"drops empty segments", "a.go,,b.go,", []string{"a.go", "b.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPaths(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitPaths(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseScanFlags_BareCommandScansWholeRepo(t *testing.T) {
	opts, err := parseScanFlags([]string{}) // no flags
	if err != nil {
		t.Fatalf("bare `ocr scan` should not error, got: %v", err)
	}
	if opts.paths != "" {
		t.Errorf("default paths should be empty (= whole repo), got %q", opts.paths)
	}
}

func TestParseScanFlags_RejectsInvalidAudience(t *testing.T) {
	_, err := parseScanFlags([]string{"--audience", "robot"})
	if err == nil {
		t.Fatal("expected error for invalid --audience")
	}
	if !strings.Contains(err.Error(), "invalid --audience") {
		t.Errorf("error message = %q; want invalid --audience", err.Error())
	}
}

func TestParseScanFlags_RejectsNegativeMaxTools(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-tools", "-1"})
	if err == nil {
		t.Fatal("expected error for negative --max-tools")
	}
	if !strings.Contains(err.Error(), "--max-tools") {
		t.Errorf("error message = %q; want it to mention --max-tools", err.Error())
	}
}

func TestParseScanFlags_RejectsNegativeMaxGitProcs(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-git-procs", "-3"})
	if err == nil {
		t.Fatal("expected error for negative --max-git-procs")
	}
	if !strings.Contains(err.Error(), "--max-git-procs") {
		t.Errorf("error message = %q; want it to mention --max-git-procs", err.Error())
	}
}

func TestParseScanFlags_DefaultsValid(t *testing.T) {
	opts, err := parseScanFlags([]string{}) // bare command
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.paths != "" {
		t.Errorf("opts = %+v; want paths empty (whole repo)", opts)
	}
	if opts.audience != "human" {
		t.Errorf("default audience = %q, want \"human\"", opts.audience)
	}
	if opts.outputFormat != "text" {
		t.Errorf("default outputFormat = %q, want \"text\"", opts.outputFormat)
	}
	if opts.concurrency != 8 {
		t.Errorf("default concurrency = %d, want 8", opts.concurrency)
	}
}

func TestParseScanFlags_PathNarrowsScope(t *testing.T) {
	opts, err := parseScanFlags([]string{"--path", "internal/agent,internal/diff"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := splitPaths(opts.paths); !reflect.DeepEqual(got, []string{"internal/agent", "internal/diff"}) {
		t.Errorf("splitPaths(opts.paths) = %v", got)
	}
}

func TestParseScanFlags_HelpFlag(t *testing.T) {
	opts, err := parseScanFlags([]string{"-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.showHelp {
		t.Error("opts.showHelp should be true when -h is supplied")
	}
}

func TestParseScanFlags_RejectsNegativeMaxTokensBudget(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-tokens-budget", "-100"})
	if err == nil {
		t.Fatal("expected error for negative --max-tokens-budget")
	}
	if !strings.Contains(err.Error(), "--max-tokens-budget") {
		t.Errorf("error message = %q; want it to mention --max-tokens-budget", err.Error())
	}
}

func TestParseScanFlags_BooleanFlags(t *testing.T) {
	opts, err := parseScanFlags([]string{"--no-plan", "--no-dedup", "--no-summary", "--preview"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.noPlan {
		t.Error("noPlan should be true")
	}
	if !opts.noDedup {
		t.Error("noDedup should be true")
	}
	if !opts.noSummary {
		t.Error("noSummary should be true")
	}
	if !opts.preview {
		t.Error("preview should be true")
	}
}

func TestParseScanFlags_ModelOverride(t *testing.T) {
	opts, err := parseScanFlags([]string{"--model", "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.model != "claude-opus-4-6" {
		t.Errorf("model = %q, want claude-opus-4-6", opts.model)
	}
}

func TestParseScanFlags_AllStringFlags(t *testing.T) {
	opts, err := parseScanFlags([]string{
		"--tools", "/tmp/tools.json",
		"--rule", "/tmp/rule.json",
		"--repo", "/tmp/repo",
		"--exclude", "*.md,*.txt",
		"--batch", "by-language",
		"--background", "test context",
		"--audience", "agent",
		"-f", "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.toolConfigPath != "/tmp/tools.json" {
		t.Errorf("toolConfigPath = %q", opts.toolConfigPath)
	}
	if opts.rulePath != "/tmp/rule.json" {
		t.Errorf("rulePath = %q", opts.rulePath)
	}
	if opts.repoDir != "/tmp/repo" {
		t.Errorf("repoDir = %q", opts.repoDir)
	}
	if opts.excludes != "*.md,*.txt" {
		t.Errorf("excludes = %q", opts.excludes)
	}
	if opts.batch != "by-language" {
		t.Errorf("batch = %q", opts.batch)
	}
	if opts.background != "test context" {
		t.Errorf("background = %q", opts.background)
	}
	if opts.audience != "agent" {
		t.Errorf("audience = %q", opts.audience)
	}
	if opts.outputFormat != "json" {
		t.Errorf("outputFormat = %q", opts.outputFormat)
	}
}

func TestParseScanFlags_IntFlags(t *testing.T) {
	opts, err := parseScanFlags([]string{
		"--concurrency", "16",
		"--timeout", "20",
		"--max-tools", "50",
		"--max-git-procs", "32",
		"--max-tokens-budget", "100000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.concurrency != 16 {
		t.Errorf("concurrency = %d", opts.concurrency)
	}
	if opts.perFileTimeout != 20 {
		t.Errorf("perFileTimeout = %d", opts.perFileTimeout)
	}
	if opts.maxTools != 50 {
		t.Errorf("maxTools = %d", opts.maxTools)
	}
	if opts.maxGitProcs != 32 {
		t.Errorf("maxGitProcs = %d", opts.maxGitProcs)
	}
	if opts.maxTokensBudget != 100000 {
		t.Errorf("maxTokensBudget = %d", opts.maxTokensBudget)
	}
}
