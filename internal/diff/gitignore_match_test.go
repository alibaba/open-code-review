// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
	} {
		if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(test.pattern+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, name := range test.paths {
			cmd := exec.Command("git", "-C", repo, "-c", "core.ignorecase=false", "-c", "core.excludesFile=", "check-ignore", "-q", "--no-index", "--", name)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
				t.Fatalf("git check-ignore %q: %v: %s", name, err, output)
			}
			if got := matchGitignoreEntry(name, test.pattern, false); got != (err == nil) {
				t.Errorf("pattern %q, path %q: matched %t, git says %t", test.pattern, name, got, err == nil)
			}
		}
	}
}
