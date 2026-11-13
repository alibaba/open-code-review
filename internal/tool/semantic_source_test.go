// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/vcs"
)

type semanticFiles struct {
	text  string
	paths []string
	err   error
	ref   vcs.ContentRef
	path  string
}

func (s *semanticFiles) ReadFile(ctx context.Context, ref vcs.ContentRef, path string, consume func(io.Reader) error) error {
	s.ref, s.path = ref, path
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	return consume(strings.NewReader(s.text))
}

func (s *semanticFiles) ListFiles(ctx context.Context, ref vcs.ContentRef) ([]string, error) {
	s.ref = ref
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.paths, s.err
}

type semanticSearch struct {
	semanticFiles
	query  vcs.SearchQuery
	result vcs.SearchResult
}

func (s *semanticSearch) Search(ctx context.Context, ref vcs.ContentRef, query vcs.SearchQuery) (vcs.SearchResult, error) {
	s.ref, s.query = ref, query
	if err := ctx.Err(); err != nil {
		return vcs.SearchResult{}, err
	}
	return s.result, s.err
}

func TestFileReaderUsesInjectedSourceAndResolvedContent(t *testing.T) {
	source := &semanticFiles{text: "one\ntwo\nthree\n"}
	ref := vcs.ContentRef{Revision: "external-revision"}
	reader := &FileReader{Source: source, Content: ref, RepoDir: "missing", Ref: "legacy", Mode: ModeCommit}
	text, err := reader.Read(context.Background(), "folder/file.go")
	if err != nil || text != source.text || source.ref != ref || source.path != "folder/file.go" {
		t.Fatalf("semantic full read: %q, %+v, %v", text, source, err)
	}
	lines, total, err := reader.ReadLines(context.Background(), "folder/file.go", 2, 1)
	if err != nil || total != 4 || !slices.Equal(lines, []string{"two"}) || source.ref != ref {
		t.Fatalf("semantic line read: %v, %d, %v", lines, total, err)
	}
	source.err = errors.New("provider read failed")
	if text, err := reader.Read(context.Background(), "folder/file.go"); text != "" || !errors.Is(err, source.err) {
		t.Fatalf("read error lost: %q, %v", text, err)
	}
	if _, _, err := reader.ReadLines(context.Background(), "folder/file.go", 1, 1); !errors.Is(err, source.err) {
		t.Fatalf("line read error lost: %v", err)
	}
}

func TestFileFindUsesInjectedSourceAndPropagatesFailure(t *testing.T) {
	source := &semanticFiles{paths: []string{"external/source.go", "binary", "Makefile"}}
	ref := vcs.ContentRef{Revision: "external-revision"}
	provider := NewFileFind(&FileReader{Source: source, Content: ref})
	text, err := provider.Execute(context.Background(), map[string]any{"query_name": "source"})
	if err != nil || text != "external/source.go" || source.ref != ref {
		t.Fatalf("semantic file find: %q, %v", text, err)
	}
	source.err = errors.New("provider list failed")
	if _, err := provider.Execute(context.Background(), map[string]any{"query_name": "source"}); !errors.Is(err, source.err) {
		t.Fatalf("file find error lost: %v", err)
	}
}

func TestCodeSearchUsesOptionalSemanticSource(t *testing.T) {
	source := &semanticSearch{result: vcs.SearchResult{
		Matches:      []vcs.Match{{Path: "source.go", Line: 3, Text: "match: text"}, {Path: "source.go", Line: 7, Text: "second"}, {Path: "other.go", Line: 1, Text: "other"}},
		MatchedFiles: 4, Truncated: true, Warning: "partial provider result",
	}}
	ref := vcs.ContentRef{Revision: "external-revision"}
	provider := NewCodeSearch(&FileReader{Source: source, Content: ref})
	text, err := provider.Execute(context.Background(), map[string]any{
		"search_text": "needle", "case_sensitive": true, "use_perl_regexp": true,
		"file_patterns": []any{"*.go"},
	})
	if err != nil || source.ref != ref || source.query.Text != "needle" || !source.query.CaseSensitive || !source.query.PerlRegexp || source.query.Limit != 100 || !slices.Equal(source.query.Patterns, []string{"*.go"}) {
		t.Fatalf("semantic search request: %+v, %v", source, err)
	}
	for _, expected := range []string{"first 3 matches across 4 matching files", "File: source.go\nMatch lines: 2\n3|match: text\n7|second\n", "File: other.go", "Warning: partial provider result"} {
		if !strings.Contains(text, expected) {
			t.Errorf("missing %q in %q", expected, text)
		}
	}
	source.err = errors.New("provider search failed")
	if _, err := provider.Execute(context.Background(), map[string]any{"search_text": "needle"}); !errors.Is(err, source.err) {
		t.Fatalf("provider search error lost: %v", err)
	}
	unsupported := NewCodeSearch(&FileReader{Source: &semanticFiles{}})
	if _, err := unsupported.Execute(context.Background(), map[string]any{"search_text": "needle"}); !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatalf("optional search must report unsupported: %v", err)
	}
}

func TestHistoricalLegacyReaderRequiresRevision(t *testing.T) {
	reader := &FileReader{RepoDir: t.TempDir(), Mode: ModeCommit}
	if _, err := reader.Read(context.Background(), "file.go"); err == nil {
		t.Fatal("historical reader without revision used workspace")
	}
	if _, _, err := reader.ReadLines(context.Background(), "file.go", 1, 1); err == nil {
		t.Fatal("historical line reader without revision used workspace")
	}
}

func TestInjectedHistoricalReadersRequireContentRevision(t *testing.T) {
	for _, mode := range []ReviewMode{ModeRange, ModeCommit} {
		reader := &FileReader{Source: &semanticSearch{}, Mode: mode, Ref: "legacy-revision"}
		checks := []func() error{
			func() error { _, err := reader.Read(context.Background(), "file.go"); return err },
			func() error { _, _, err := reader.ReadLines(context.Background(), "file.go", 1, 1); return err },
			func() error {
				_, err := NewFileFind(reader).Execute(context.Background(), map[string]any{"query_name": "file"})
				return err
			},
			func() error {
				_, err := NewCodeSearch(reader).Execute(context.Background(), map[string]any{"search_text": "needle"})
				return err
			},
		}
		for _, check := range checks {
			if err := check(); err == nil || !strings.Contains(err.Error(), "requires a revision") {
				t.Fatalf("historical source used the workspace: %v", err)
			}
		}
	}
}
