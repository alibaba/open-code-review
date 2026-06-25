package scan

import (
	"reflect"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

func itemList(paths ...string) []model.ScanItem {
	out := make([]model.ScanItem, len(paths))
	for i, p := range paths {
		out[i] = model.ScanItem{Path: p}
	}
	return out
}

func batchPaths(b [][]model.ScanItem) [][]string {
	out := make([][]string, len(b))
	for i, batch := range b {
		ps := make([]string, len(batch))
		for j, it := range batch {
			ps[j] = it.Path
		}
		out[i] = ps
	}
	return out
}

func TestParseBatchStrategy(t *testing.T) {
	cases := map[string]BatchStrategy{
		"":               BatchNone,
		"   ":            BatchNone,
		"by-language":    BatchByLanguage,
		"BY-LANGUAGE":    BatchByLanguage,
		"by-directory":   BatchByDirectory,
		"none":           BatchNone,
		"by-author":      BatchNone, // unknown → safe default
		"  by-language ": BatchByLanguage,
	}
	for in, want := range cases {
		if got := parseBatchStrategy(in); got != want {
			t.Errorf("parseBatchStrategy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGroupBatches_ByLanguage(t *testing.T) {
	items := itemList(
		"cmd/main.go",
		"internal/scan/agent.go",
		"docs/README.md",
		"scripts/build.sh",
		"internal/scan/preview.go",
		"docs/intro.md",
	)
	got := batchPaths(groupBatches(items, BatchByLanguage, 0))
	// Batches are emitted in lexicographic key order: .go < .md < .sh.
	// Within a batch, input order is preserved.
	want := [][]string{
		{"cmd/main.go", "internal/scan/agent.go", "internal/scan/preview.go"}, // .go
		{"docs/README.md", "docs/intro.md"},                                   // .md
		{"scripts/build.sh"},                                                  // .sh
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestGroupBatches_ByDirectory(t *testing.T) {
	items := itemList(
		"README.md",       // <root>
		"cmd/main.go",     // cmd
		"internal/a/x.go", // internal
		"internal/b/y.go", // internal
		"cmd/scan.go",     // cmd
		"LICENSE",         // <root>
	)
	got := batchPaths(groupBatches(items, BatchByDirectory, 0))
	want := [][]string{
		{"README.md", "LICENSE"},               // <root>
		{"cmd/main.go", "cmd/scan.go"},         // cmd
		{"internal/a/x.go", "internal/b/y.go"}, // internal
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestGroupBatches_None(t *testing.T) {
	items := itemList("a.go", "b.go", "c.py")
	got := batchPaths(groupBatches(items, BatchNone, 0))
	want := [][]string{{"a.go"}, {"b.go"}, {"c.py"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestGroupBatches_BatchSizeCap(t *testing.T) {
	items := itemList("a.go", "b.go", "c.go", "d.go", "e.go")
	// All .go in one natural group; size=2 → 3 chunks of 2,2,1
	got := batchPaths(groupBatches(items, BatchByLanguage, 2))
	want := [][]string{
		{"a.go", "b.go"},
		{"c.go", "d.go"},
		{"e.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestGroupBatches_Empty(t *testing.T) {
	if got := groupBatches(nil, BatchByLanguage, 0); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestLanguageKey_ExtensionlessAndDotfiles(t *testing.T) {
	cases := map[string]string{
		"Makefile":           "<no-ext>",
		"src/Dockerfile":     "<no-ext>",
		".gitignore":         "<no-ext>", // dotfile w/o extension
		".github/CODEOWNERS": "<no-ext>",
		"cmd/main.go":        ".go",
		"docs/README.MD":     ".md",
		"a/b/c.Test.go":      ".go",
	}
	for path, want := range cases {
		got := languageKey(model.ScanItem{Path: path})
		if got != want {
			t.Errorf("languageKey(%q) = %q, want %q", path, got, want)
		}
	}
}
