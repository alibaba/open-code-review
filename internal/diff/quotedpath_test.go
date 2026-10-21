// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestUnquoteGitPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		rest string
		ok   bool
	}{
		{"plain", `"a/file.go"`, "a/file.go", "", true},
		{"tab", `"a/tab\tname.go"`, "a/tab\tname.go", "", true},
		{"newline", `"a/nl\nname.go"`, "a/nl\nname.go", "", true},
		{"double quote", `"a/quote\".go"`, `a/quote".go`, "", true},
		{"backslash", `"a/back\\slash.go"`, `a/back\slash.go`, "", true},
		{"other escapes", `"a/\a\b\f\r\v.go"`, "a/\a\b\f\r\v.go", "", true},
		// core.quotepath=true octal-escapes each byte of a multi-byte rune, so
		// the decoder must emit bytes: \303\251 is 0xc3 0xa9, which is U+00E9
		// only once the two sit next to each other again.
		{"octal utf8", `"a/\303\251.go"`, "a/\xc3\xa9.go", "", true},
		{"trailing text", `"a/x.go" b/y.go`, "a/x.go", " b/y.go", true},
		{"not quoted", `a/file.go`, "", `a/file.go`, false},
		{"unterminated", `"a/file.go`, "", `"a/file.go`, false},
		{"dangling backslash", `"a/file.go\`, "", `"a/file.go\`, false},
		{"unknown escape", `"a/\q.go"`, "", `"a/\q.go"`, false},
		{"short octal", `"a/\30"`, "", `"a/\30"`, false},
		{"bad octal digit", `"a/\398"`, "", `"a/\398"`, false},
		// Three octal digits can express 511, which is not a byte. Git never
		// emits one, but truncating to the low bits would return a path that
		// is quietly not the one on disk.
		{"octal above 255", `"a/\400.go"`, "", `"a/\400.go"`, false},
		{"octal at the top of the range", `"a/\777.go"`, "", `"a/\777.go"`, false},
		{"octal at the top of a byte", `"a/\377.go"`, "a/\xff.go", "", true},
		{"empty", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rest, ok := unquoteGitPath(tt.in)
			if ok != tt.ok || got != tt.want || rest != tt.rest {
				t.Errorf("unquoteGitPath(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.in, got, rest, ok, tt.want, tt.rest, tt.ok)
			}
		})
	}
}

// TestParseDiffHeaderLine_QuotingCombinations covers issue #1492. Git quotes
// the two sides of a header independently, so a changed file can arrive in any
// of four shapes and the regex alone matches only one of them.
func TestParseDiffHeaderLine_QuotingCombinations(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantOld string
		wantNew string
		wantOK  bool
	}{
		{
			name:    "neither quoted",
			line:    `diff --git a/plain.go b/plain.go`,
			wantOld: "plain.go", wantNew: "plain.go", wantOK: true,
		},
		{
			name:    "both quoted",
			line:    "diff --git \"a/tab\\tname.go\" \"b/tab\\tname.go\"",
			wantOld: "tab\tname.go", wantNew: "tab\tname.go", wantOK: true,
		},
		{
			name:    "only the new side quoted",
			line:    "diff --git a/plain.go \"b/tab\\tname.go\"",
			wantOld: "plain.go", wantNew: "tab\tname.go", wantOK: true,
		},
		{
			name:    "only the old side quoted",
			line:    "diff --git \"a/tab\\tname.go\" b/plain.go",
			wantOld: "tab\tname.go", wantNew: "plain.go", wantOK: true,
		},
		{
			name:    "unquoted spaces still parse as before",
			line:    `diff --git a/plain space.go b/plain space.go`,
			wantOld: "plain space.go", wantNew: "plain space.go", wantOK: true,
		},
		{
			name: "not a header",
			line: `+++ b/file.go`,
		},
		{
			name: "malformed quoting is refused rather than guessed",
			line: `diff --git "a/unterminated.go b/unterminated.go`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOld, gotNew, ok := parseDiffHeaderLine(tt.line)
			if ok != tt.wantOK || gotOld != tt.wantOld || gotNew != tt.wantNew {
				t.Errorf("parseDiffHeaderLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.line, gotOld, gotNew, ok, tt.wantOld, tt.wantNew, tt.wantOK)
			}
		})
	}
}

// TestParseDiffText_QuotedPathIsNotDropped is the coverage-gap assertion: the
// file must appear in the parsed diff at all. Before the fix the header did not
// match, no section was opened, and the changed file was absent from the result
// with no error and no warning.
func TestParseDiffText_QuotedPathIsNotDropped(t *testing.T) {
	diffText := "diff --git a/plain.go b/plain.go\n" +
		"--- a/plain.go\n" +
		"+++ b/plain.go\n" +
		"@@ -1 +1,2 @@\n" +
		" package p\n" +
		"+// changed\n" +
		"diff --git \"a/tab\\tname.go\" \"b/tab\\tname.go\"\n" +
		"--- \"a/tab\\tname.go\"\n" +
		"+++ \"b/tab\\tname.go\"\n" +
		"@@ -1 +1,2 @@\n" +
		" package q\n" +
		"+// changed too\n"

	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 2 {
		got := make([]string, 0, len(diffs))
		for _, d := range diffs {
			got = append(got, d.NewPath)
		}
		t.Fatalf("parsed %d files %q, want 2 — a changed file must never be silently omitted", len(diffs), got)
	}
	if diffs[1].NewPath != "tab\tname.go" {
		t.Errorf("NewPath = %q, want %q", diffs[1].NewPath, "tab\tname.go")
	}
	if diffs[1].OldPath != "tab\tname.go" {
		t.Errorf("OldPath = %q, want %q", diffs[1].OldPath, "tab\tname.go")
	}
	if diffs[1].Insertions != 1 {
		t.Errorf("Insertions = %d, want 1", diffs[1].Insertions)
	}
}

// TestParseDiffText_QuotedRenamePaths pins the other place the same quoting
// appears. "rename from"/"rename to" are authoritative for renames and
// overwrite whatever the header produced, so leaving them encoded would put a
// literal backslash-t into the path even when the header parsed correctly.
func TestParseDiffText_QuotedRenamePaths(t *testing.T) {
	diffText := "diff --git a/plain.go \"b/tab\\tname.go\"\n" +
		"similarity index 100%\n" +
		"rename from plain.go\n" +
		"rename to \"tab\\tname.go\"\n"

	diffs, err := ParseDiffText(context.Background(), diffText, t.TempDir(), "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("parsed %d files, want 1", len(diffs))
	}
	if !diffs[0].IsRenamed {
		t.Error("IsRenamed = false, want true")
	}
	if diffs[0].OldPath != "plain.go" {
		t.Errorf("OldPath = %q, want %q", diffs[0].OldPath, "plain.go")
	}
	if diffs[0].NewPath != "tab\tname.go" {
		t.Errorf("NewPath = %q, want %q", diffs[0].NewPath, "tab\tname.go")
	}
}

// TestParseDiffText_QuotedPathFromRealGit runs the whole path against git
// itself rather than a handwritten fixture, so the escape spelling under test
// is the one git actually produces and stays correct if that ever changes.
// It also checks the decoded path is usable: the file content must be read
// back, which is the step the encoded name would fail at.
func TestParseDiffText_QuotedPathFromRealGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a tab is not a legal filename character on windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	git("init", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	git("config", "commit.gpgsign", "false")

	name := "tab\tname.go"
	if err := os.WriteFile(filepath.Join(repo, name), []byte("package q\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	git("add", "-A")
	git("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, name), []byte("package q\n\nfunc Changed() {}\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	diffText := git("-c", "core.quotepath=false", "diff", "--no-ext-diff", "--no-color", "--unified=3", "HEAD", "--")
	if !strings.Contains(diffText, `"a/tab\tname.go"`) {
		t.Fatalf("this git does not quote the header as expected:\n%s", diffText)
	}

	diffs, err := ParseDiffText(context.Background(), diffText, repo, "", nil)
	if err != nil {
		t.Fatalf("ParseDiffText: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("parsed %d files, want 1 — the changed file was dropped", len(diffs))
	}
	if diffs[0].NewPath != name {
		t.Fatalf("NewPath = %q, want %q", diffs[0].NewPath, name)
	}
	if !strings.Contains(diffs[0].NewFileContent, "func Changed()") {
		t.Errorf("NewFileContent = %q; the decoded path must be readable from disk", diffs[0].NewFileContent)
	}
}

// TestUntrackedFilesList_PreservesAwkwardNames covers the untracked half of the
// same coverage gap. The listing ran `git ls-files --others` without -z and
// then trimmed each line, so three different names never reached review: git
// quoted the tab one and the escaped spelling matched nothing on disk, and the
// trim silently renamed the two whitespace ones.
func TestUntrackedFilesList_PreservesAwkwardNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows normalises away the filenames this fixture depends on")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	want := []string{" leading.go", "plain.go", "tab\tname.go", "trailing.go "}
	for _, name := range want {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("package p\n"), 0o644); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}

	got, err := NewWorkspaceProvider(repo, nil).untrackedFilesList(context.Background())
	if err != nil {
		t.Fatalf("untrackedFilesList: %v", err)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("listed %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}
