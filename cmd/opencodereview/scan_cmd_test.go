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
