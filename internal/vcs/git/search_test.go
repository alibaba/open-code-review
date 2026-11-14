// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func TestSearchVersionsPatternsAndGlobalLimit(t *testing.T) {
	dir, head := contentTestRepo(t)
	contentWrite(t, dir, "hello.go", "workspace changed\n")
	contentWrite(t, dir, "untracked.go", "Hello untracked\nHello again\n")
	contentWrite(t, dir, "ignored.go", "Hello ignored\n")
	r := New(dir, gitcmd.New(1))
	query := vcs.SearchQuery{Text: "Hello", CaseSensitive: true, Limit: 100}
	result, err := r.Search(context.Background(), vcs.ContentRef{Revision: head}, query)
	if err != nil || len(result.Matches) != 2 || result.Truncated {
		t.Fatalf("historical search: %+v, %v", result, err)
	}
	for _, match := range result.Matches {
		if strings.HasPrefix(match.Path, head+":") || match.Path == "untracked.go" {
			t.Errorf("incorrect historical path: %q", match.Path)
		}
	}
	query.Patterns = []string{"sub/"}
	result, err = r.Search(context.Background(), vcs.ContentRef{}, query)
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Path != "sub/hello.go" {
		t.Fatalf("scoped workspace search: %+v, %v", result, err)
	}
	query.Patterns = nil
	query.Limit = 1
	result, err = r.Search(context.Background(), vcs.ContentRef{}, query)
	if err != nil || len(result.Matches) != 1 || !result.Truncated {
		t.Fatalf("global result limit: %+v, %v", result, err)
	}
	query.Text = "absent"
	result, err = r.Search(context.Background(), vcs.ContentRef{}, query)
	if err != nil || len(result.Matches) != 0 {
		t.Fatalf("no matches: %+v, %v", result, err)
	}
	query = vcs.SearchQuery{Text: "(unclosed", PerlRegexp: true}
	if _, err := r.Search(context.Background(), vcs.ContentRef{}, query); err == nil {
		t.Fatal("invalid pattern error hidden")
	}
}

func TestSearchPlainDirectoryAndErrors(t *testing.T) {
	dir := t.TempDir()
	contentWrite(t, dir, "file.go", "Hello world\n")
	contentWrite(t, dir, ".gitignore", "ignored.go\n")
	contentWrite(t, dir, "ignored.go", "Hello ignored\n")
	r := New(dir, nil)
	query := vcs.SearchQuery{Text: "Hello"}
	result, err := r.Search(context.Background(), vcs.ContentRef{}, query)
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Path != "file.go" {
		t.Fatalf("plain search: %+v, %v", result, err)
	}
	if _, err := r.Search(context.Background(), vcs.ContentRef{Revision: "HEAD"}, query); err == nil {
		t.Fatal("historical search fell back to plain directory")
	}
	query.Patterns = []string{"../outside"}
	if _, err := r.Search(context.Background(), vcs.ContentRef{}, query); err == nil {
		t.Fatal("search accepted traversal pattern")
	}
	query.Patterns = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Search(ctx, vcs.ContentRef{}, query); !errors.Is(err, context.Canceled) {
		t.Fatalf("search cancellation: %v", err)
	}
	r.Runner = &gitcmd.Runner{}
	if _, err := r.Search(context.Background(), vcs.ContentRef{}, query); err == nil {
		t.Fatal("runner failure was hidden")
	}
}

func TestParseGrepOutputPreservesFilenameAndContent(t *testing.T) {
	ref := vcs.ContentRef{Revision: "HEAD"}
	out := "HEAD:dir/colon:name\nwith tab\t.go\x002\x00value: text\nHEAD:other.go\x003\x00last"
	result, err := parseGrepOutput(out, ref, 2)
	if err != nil || len(result.Matches) != 2 || result.Truncated {
		t.Fatalf("parse output: %+v, %v", result, err)
	}
	if result.Matches[0].Path != "dir/colon:name\nwith tab\t.go" || result.Matches[0].Text != "value: text" || result.Matches[1].Text != "last" {
		t.Fatalf("record delimiters corrupted content: %+v", result)
	}
	for _, malformed := range []string{"file:1:text", "file\x001:text", "file\x00bad\x00text", "file\x000\x00text"} {
		if _, err := parseGrepOutput(malformed, vcs.ContentRef{}, 0); err == nil {
			t.Errorf("accepted malformed record %q", malformed)
		}
	}
}

func TestParseGrepOutputCountsFilesBeyondLimit(t *testing.T) {
	out := "first.go\x001\x00match\nfirst.go\x002\x00match\nlater.go\x001\x00match\nlast.go\x001\x00match\n"
	result, err := parseGrepOutput(out, vcs.ContentRef{}, 1)
	if err != nil || len(result.Matches) != 1 || !result.Truncated || result.MatchedFiles != 3 {
		t.Fatalf("omitted files were not counted: %+v, %v", result, err)
	}
}

func TestSearchRejectsPatternsOutsideRepositoryBeforeGit(t *testing.T) {
	patterns := []string{
		"../outside",
		":(glob)../outside",
		":(top)../outside",
		":(top,glob)folder/../../outside",
		":(glob)/outside",
		"/outside",
		`\\server\outside`,
		`folder\..\outside`,
	}
	if runtime.GOOS == "windows" {
		patterns = append(patterns, "C:/outside", `C:\outside`, "C:outside")
	}
	// A runner that cannot launch Git distinguishes provider validation from
	// Git's own pathspec errors, which must not be used to enforce containment.
	r := New(t.TempDir(), &gitcmd.Runner{})
	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			result, err := r.Search(context.Background(), vcs.ContentRef{}, vcs.SearchQuery{Text: "secret", Patterns: []string{pattern}})
			if err == nil || !strings.HasPrefix(err.Error(), "invalid search pattern") || len(result.Matches) != 0 {
				t.Fatalf("pattern reached Git instead of validation: %q, %+v, %v", pattern, result, err)
			}
		})
	}
}

func TestSearchPreservesSupportedGitPathspecMagic(t *testing.T) {
	dir, _ := contentTestRepo(t)
	r := New(dir, nil)
	for _, pattern := range []string{":(top)hello.go", ":(top,glob)**/hello.go"} {
		t.Run(pattern, func(t *testing.T) {
			result, err := r.Search(context.Background(), vcs.ContentRef{}, vcs.SearchQuery{Text: "Hello", Patterns: []string{pattern}})
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if strings.Contains(pattern, "glob") {
				want = 2
			}
			if len(result.Matches) != want || result.Matches[0].Path != "hello.go" {
				t.Fatalf("pathspec semantics changed: %+v", result)
			}
		})
	}
}
