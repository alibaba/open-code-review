// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestIgnoreRuleWhitespace(t *testing.T) {
	data := "\xef\xbb\xbf out\r\n# ignored\n #literal  \nend\\   \nend\\\\   \n\tname\t\nfile\x00discard\n"
	want := []string{" out", " #literal", `end\ `, `end\\`, "\tname\t", "file"}
	if got := parseGitignorePatterns([]byte(data)); !slices.Equal(got, want) {
		t.Errorf("parsed %q, want %q", got, want)
	}
}

func TestIgnoreEntryAgainstGit(t *testing.T) {
	repo := t.TempDir()
	if output, err := exec.Command("git", "init", "--template=", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	for _, test := range []struct {
		pattern string
		paths   []string
	}{
		{"docs/api.md", []string{"docs/api.md", "sub/docs/api.md"}},
		{"/?.go", []string{"a.go", "\u00e9.go"}},
		{"/??.go", []string{"a.go", "\u00e9.go"}},
		{"build/***/result.log", []string{"build/result.log", "build/a/result.log", "build/a/b/result.log"}},
		{"src/[]a/]/x.go", []string{"src/a/x.go", "src/]/x.go", "src/b/x.go"}},
		{"src/[!]/]/x.go", []string{"src/a/x.go", "src/]/x.go"}},
		{"[[:a].go", []string{"a.go", "[.go", "b.go"}},
		{`dir\/**/[][:digit:]].go`, []string{"dir/].go", "dir/sub/4.go", "dir/sub/x.go"}},
		{`**\/keep.go`, []string{"keep.go", "dir/keep.go", "dir/sub/keep.go"}},
		{`*[[:punct:]].go`, []string{"3.go", "[.go", "{.go", "name].go"}},
		{`*\*.go`, []string{"main.go", "name*.go"}},
		{"/sRc/KEEP*.go", []string{"src/keep.go", "SRC/keepMore.GO", "Src/other.go"}},
		{"/[A-Z][[:upper:]].go", []string{"az.go", "AZ.go", "1a.go"}},
		{"/[A].go", []string{"a.go", "A.go"}},
		{`/\A.go`, []string{"a.go", "A.go"}},
		{"/\u00c4.go", []string{"\u00c4.go", "\u00e4.go"}},
	} {
		if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(test.pattern+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, name := range test.paths {
			for _, ignoreCase := range []bool{false, true} {
				cmd := exec.Command("git", "-C", repo, "-c", "core.ignorecase="+strconv.FormatBool(ignoreCase), "-c", "core.excludesFile=", "check-ignore", "-q", "--no-index", "--", name)
				output, err := cmd.CombinedOutput()
				var exit *exec.ExitError
				if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
					t.Fatalf("git check-ignore %q: %v: %s", name, err, output)
				}
				if got := matchGitignoreEntry(name, test.pattern, false, ignoreCase); got != (err == nil) {
					t.Errorf("pattern %q, path %q, ignoreCase=%t: matched %t, git says %t", test.pattern, name, ignoreCase, got, err == nil)
				}
			}
		}
	}
}
