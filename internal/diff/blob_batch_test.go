// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestReadBlobsAtRef(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")
	runGitTest(t, repo, "config", "core.autocrlf", "false")
	files := map[string]string{
		"a.go":                 "package a\n",
		"dir with spaces/b.go": "package b\n",
		"crlf.txt":             "one\r\ntwo\r\n",
		"empty.txt":            "",
		"no-newline.txt":       "last line",
	}
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitTest(t, repo, "add", "-A")
	runGitTest(t, repo, "commit", "-q", "-m", "files")

	paths := []string{"a.go", "not there", "dir with spaces", "dir with spaces/b.go", "crlf.txt", "empty.txt", "new\nline", "no-newline.txt"}
	for _, runner := range []*gitcmd.Runner{nil, gitcmd.New(2)} {
		got := readBlobsAtRef(context.Background(), repo, "HEAD", paths, runner)
		if !reflect.DeepEqual(got, files) {
			t.Errorf("readBlobsAtRef = %q, want %q", got, files)
		}
	}
}

func TestParseDiffTextAtRefMatchesWorkspaceContent(t *testing.T) {
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")
	runGitTest(t, repo, "config", "core.autocrlf", "false")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("keep.go", "package keep\n")
	write("gone.go", "package gone\n")
	runGitTest(t, repo, "add", "-A")
	runGitTest(t, repo, "commit", "-q", "-m", "base")
	write("keep.go", "package keep\n\nfunc F() {}\n")
	write("added.go", "package added\n")
	write("blob.bin", "\x00\x01\x02")
	if err := os.Remove(filepath.Join(repo, "gone.go")); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "-A")
	runGitTest(t, repo, "commit", "-q", "-m", "change")

	provider := NewCommitProvider(repo, "HEAD", gitcmd.New(2))
	set, err := provider.GetDiffSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range append(set.Included, set.Excluded...) {
		got[d.NewPath] = d.NewFileContent
	}
	want := map[string]string{
		"added.go":  "package added\n",
		"blob.bin":  "",
		"keep.go":   "package keep\n\nfunc F() {}\n",
		"/dev/null": "",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NewFileContent by path = %q, want %q", got, want)
	}
}
