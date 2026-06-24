package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if !strings.Contains(result, "args:explore -p "+dir+" --max-files 2 Foo") {
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
	if !strings.Contains(result, "query -p "+dir+" -l 12 -k function Foo") {
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

func writeFakeCodeGraph(t *testing.T, dir, script string) string {
	t.Helper()
	path := filepath.Join(dir, "codegraph")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
