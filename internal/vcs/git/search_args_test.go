// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"slices"
	"testing"

	"github.com/alibaba/open-code-review/internal/vcs"
)

func TestBuildGrepArgs_WorkspaceMode(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "myFunc", CaseSensitive: false, PerlRegexp: false, Patterns: nil, Limit: 100}, false)

	assertContainsInOrder(t, args, "-e", "myFunc", "--")
	assertContains(t, args, "-i")
	assertContains(t, args, "--untracked")
	if idx := slices.Index(args, "--"); idx >= 0 {
		for i := 0; i < idx; i++ {
			if args[i] == "myFunc" && (i == 0 || args[i-1] != "-e") {
				t.Error("myFunc should only appear as argument to -e, not as positional")
			}
		}
	}
}

func TestBuildGrepArgs_CommitMode(t *testing.T) {
	ref := vcs.ContentRef{Revision: "abc1234"}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "myFunc", CaseSensitive: false, PerlRegexp: false, Patterns: []string{"pkg/"}, Limit: 100}, false)

	assertContainsInOrder(t, args, "-e", "myFunc", "abc1234", "--", "pkg/")
	assertNotContains(t, args, "--untracked")
	assertNotContains(t, args, "--end-of-options")
}

func TestBuildGrepArgs_RejectsOptionLikeRef(t *testing.T) {
	ref := vcs.ContentRef{Revision: "-O./pwn.sh"}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "myFunc", CaseSensitive: false, PerlRegexp: false, Patterns: nil, Limit: 100}, false)
	if args != nil {
		t.Fatalf("expected buildGrepArgs to return nil for option-like ref, got %v", args)
	}
}

func TestBuildGrepArgs_PatternStartingWithDash(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "-myOption", CaseSensitive: false, PerlRegexp: false, Patterns: nil, Limit: 100}, false)

	idx := slices.Index(args, "-e")
	if idx < 0 || idx+1 >= len(args) || args[idx+1] != "-myOption" {
		t.Errorf("expected -e to immediately precede -myOption, got %v", args)
	}
}

func TestBuildGrepArgs_CaseSensitive(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "foo", CaseSensitive: true, PerlRegexp: false, Patterns: nil, Limit: 100}, false)

	assertNotContains(t, args, "-i")
}

func TestBuildGrepArgs_CaseInsensitive(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "foo", CaseSensitive: false, PerlRegexp: false, Patterns: nil, Limit: 100}, false)

	assertContains(t, args, "-i")
}

func TestBuildGrepArgs_PerlRegexp(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "foo", CaseSensitive: false, PerlRegexp: true, Patterns: nil, Limit: 100}, false)

	assertContains(t, args, "-P")
	assertNotContains(t, args, "-F")
}

func TestBuildGrepArgs_FixedString(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "foo", CaseSensitive: false, PerlRegexp: false, Patterns: nil, Limit: 100}, false)

	assertContains(t, args, "-F")
	assertNotContains(t, args, "-E")
	assertNotContains(t, args, "-P")
}

func TestTrimGitUsage(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			name:   "English",
			stderr: "error: unknown option `max-count'\nusage: git grep [<options>]\n\n    --cached",
			want:   "error: unknown option `max-count'",
		},
		{
			name:   "Chinese",
			stderr: "\u9519\u8bef\uff1a\u672a\u77e5\u9009\u9879 `max-count'\n\u7528\u6cd5\uff1agit grep [<\u9009\u9879>]\n\n    --cached",
			want:   "\u9519\u8bef\uff1a\u672a\u77e5\u9009\u9879 `max-count'",
		},
		{
			name:   "French",
			stderr: "erreur : option inconnue `max-count'\nutilisation : git grep [<options>]\n\n    --cached",
			want:   "erreur : option inconnue `max-count'",
		},
		{
			name:   "Japanese",
			stderr: "\u30a8\u30e9\u30fc: \u4e0d\u660e\u306a\u30aa\u30d7\u30b7\u30e7\u30f3 `max-count'\n\u4f7f\u7528\u6cd5: git grep [<\u30aa\u30d7\u30b7\u30e7\u30f3>]\n\n    --cached",
			want:   "\u30a8\u30e9\u30fc: \u4e0d\u660e\u306a\u30aa\u30d7\u30b7\u30e7\u30f3 `max-count'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimGitUsage(tt.stderr, 129); got != tt.want {
				t.Errorf("trimGitUsage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrimGitUsage_PreservesDiagnosticWithoutUsage(t *testing.T) {
	stderr := "fatal: ambiguous argument 'missing': unknown revision\nhint: verify the revision name"
	got := trimGitUsage(stderr, 128)
	if got != stderr {
		t.Errorf("trimGitUsage() = %q, want %q", got, stderr)
	}
}

func TestTrimGitUsage_WhitespaceOnly(t *testing.T) {
	if got := trimGitUsage(" \n\t", 129); got != "" {
		t.Errorf("trimGitUsage() = %q, want empty string", got)
	}
}

func assertContains(t *testing.T, args []string, val string) {
	t.Helper()
	if !slices.Contains(args, val) {
		t.Errorf("expected args to contain %q, got %v", val, args)
	}
}

func assertNotContains(t *testing.T, args []string, val string) {
	t.Helper()
	if slices.Contains(args, val) {
		t.Errorf("expected args NOT to contain %q, got %v", val, args)
	}
}

func assertContainsInOrder(t *testing.T, args []string, vals ...string) {
	t.Helper()
	idx := 0
	for _, a := range args {
		if idx < len(vals) && a == vals[idx] {
			idx++
		}
	}
	if idx != len(vals) {
		t.Errorf("expected args to contain %v in order, got %v (matched up to index %d)", vals, args, idx)
	}
}

func TestBuildGrepArgs_NoIndex(t *testing.T) {
	ref := vcs.ContentRef{Revision: ""}
	args, _ := buildGrepArgs(ref, vcs.SearchQuery{Text: "foo", CaseSensitive: false, PerlRegexp: false, Patterns: nil, Limit: 100}, true)

	assertContains(t, args, "--no-index")
	assertContains(t, args, "--exclude-standard")
	assertNotContains(t, args, "--untracked")
}
