package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCodeGraphExecuteExplore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is Unix-only")
	}
	dir := t.TempDir()
	bin := writeFakeCodeGraph(t, dir, `#!/bin/sh
printf 'args:%s\n' "$*"
printf '\033[32mSymbol: Foo\033[0m\n'
`)

	p := NewCodeGraph(dir, bin)
	result, err := p.Execute(context.Background(), map[string]any{
		"query":     "Foo",
		"max_files": float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "args:explore -p "+dir+" --max-files 2 -- Foo") {
		t.Fatalf("unexpected command args: %s", result)
	}
	if strings.Contains(result, "\033[") {
		t.Fatalf("expected ANSI escapes to be stripped, got: %q", result)
	}
	if !strings.Contains(result, "Symbol: Foo") {
		t.Fatalf("expected command output, got: %s", result)
	}
}

func TestCodeGraphExecuteSearchWithKindAndLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test helper is Unix-only")
	}
	dir := t.TempDir()
	bin := writeFakeCodeGraph(t, dir, `#!/bin/sh
printf '%s\n' "$*"
`)

	p := NewCodeGraph(dir, bin)
	result, err := p.Execute(context.Background(), map[string]any{
		"mode":  "search",
		"query": "Foo",
		"kind":  "function",
		"limit": float64(99),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "query -p "+dir+" -l 12 -k function -- Foo") {
		t.Fatalf("expected out-of-range limit to fall back to default, got: %s", result)
	}
}

func TestCodeGraphExecuteUnsupportedMode(t *testing.T) {
	p := NewCodeGraph(t.TempDir(), "codegraph")
	result, err := p.Execute(context.Background(), map[string]any{
		"mode":  "trace",
		"query": "Foo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "unsupported mode") {
		t.Fatalf("expected unsupported mode error, got: %s", result)
	}
}

func TestDetectCodeGraphMissingDB(t *testing.T) {
	result := DetectCodeGraph(t.TempDir())
	if result.Available {
		t.Fatal("expected CodeGraph to be unavailable without .codegraph/codegraph.db")
	}
	if result.Reason == "" {
		t.Fatal("expected unavailable reason")
	}
}

func TestTruncateToolOutputPreservesUTF8(t *testing.T) {
	prefix := strings.Repeat("a", codeGraphMaxOutput-1)
	result := truncateToolOutput(prefix + "界")
	if !strings.Contains(result, "[truncated: CodeGraph output exceeded tool limit]") {
		t.Fatalf("expected truncation marker, got: %s", result)
	}
	if !utf8.ValidString(result) {
		t.Fatalf("expected valid UTF-8, got: %q", result)
	}
}

func TestCodeGraphTimeoutMessageIncludesPartialOutput(t *testing.T) {
	result := codeGraphTimeoutMessage("partial stdout\n", "partial stderr\n")
	if !strings.Contains(result, "timed out") || !strings.Contains(result, "partial stdout") || !strings.Contains(result, "partial stderr") {
		t.Fatalf("expected timeout message with partial output, got: %s", result)
	}
}

func writeFakeCodeGraph(t *testing.T, dir, script string) string {
	t.Helper()
	path := filepath.Join(dir, "codegraph")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
