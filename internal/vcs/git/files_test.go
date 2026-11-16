// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func contentTestRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		contentGit(t, dir, args...)
	}
	contentWrite(t, dir, "hello.go", "package main\nfunc Hello() {}\n")
	contentWrite(t, dir, "sub/hello.go", "package sub\nfunc HelloAgain() {}\n")
	contentWrite(t, dir, "big.txt", strings.Repeat("historical content\n", 65536))
	contentWrite(t, dir, "extensionless", "tracked\n")
	contentWrite(t, dir, ".gitignore", "ignored.go\n")
	contentGit(t, dir, "add", ".")
	contentGit(t, dir, "commit", "-m", "Initial content")
	return dir, strings.TrimSpace(contentGit(t, dir, "rev-parse", "HEAD"))
}

func contentGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func contentWrite(t *testing.T, dir, path, text string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadFileVersionsAndEarlyReturn(t *testing.T) {
	dir, head := contentTestRepo(t)
	r := New(dir, gitcmd.New(1))
	contentWrite(t, dir, "hello.go", "workspace\n")
	contentWrite(t, dir, "untracked.go", "workspace only\n")
	var data []byte
	consume := func(reader io.Reader) error {
		var err error
		data, err = io.ReadAll(reader)
		return err
	}
	if err := r.ReadFile(context.Background(), vcs.ContentRef{}, "hello.go", consume); err != nil || string(data) != "workspace\n" {
		t.Fatalf("workspace read: %q, %v", data, err)
	}
	ref := vcs.ContentRef{Revision: head}
	if err := r.ReadFile(context.Background(), ref, "hello.go", consume); err != nil || !strings.Contains(string(data), "Hello") {
		t.Fatalf("historical read: %q, %v", data, err)
	}
	if err := r.ReadFile(context.Background(), ref, "untracked.go", consume); err == nil {
		t.Fatal("historical read used workspace content")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.ReadFile(ctx, ref, "big.txt", func(reader io.Reader) error {
		_, err := io.ReadFull(reader, make([]byte, 1))
		return err
	}); err != nil {
		t.Fatalf("successful prefix read did not drain: %v", err)
	}
	callbackErr := errors.New("callback failed")
	if err := r.ReadFile(ctx, ref, "big.txt", func(io.Reader) error { return callbackErr }); !errors.Is(err, callbackErr) {
		t.Fatalf("callback error lost: %v", err)
	}
	if _, err := r.ListFiles(ctx, ref); err != nil {
		t.Fatalf("runner slot remained occupied: %v", err)
	}
}

func TestReadFileContainmentAndCancellation(t *testing.T) {
	dir, head := contentTestRepo(t)
	r := New(dir, gitcmd.New(1))
	for _, path := range []string{"../secret", "sub/../../secret", "/secret", "\\\\server\\secret", ""} {
		for _, ref := range []vcs.ContentRef{{}, {Revision: head}} {
			if err := r.ReadFile(context.Background(), ref, path, func(io.Reader) error { return nil }); err == nil {
				t.Errorf("accepted path %q at %+v", path, ref)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := r.ReadFile(ctx, vcs.ContentRef{Revision: head}, "big.txt", func(reader io.Reader) error {
		if _, err := io.ReadFull(reader, make([]byte, 1)); err != nil {
			return err
		}
		cancel()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("read cancellation lost: %v", err)
	}
	if err := r.ReadFile(ctx, vcs.ContentRef{}, "hello.go", func(io.Reader) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled workspace read: %v", err)
	}
	outside := t.TempDir()
	contentWrite(t, outside, "secret", "secret")
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := r.ReadFile(context.Background(), vcs.ContentRef{}, "link", func(io.Reader) error { return nil }); err == nil {
		t.Fatal("read symlink outside repository")
	}
}

func TestListFilesWorkspaceAndRevision(t *testing.T) {
	dir, head := contentTestRepo(t)
	contentWrite(t, dir, "untracked.go", "new")
	contentWrite(t, dir, "ignored.go", "ignored")
	contentWrite(t, dir, "space name.go", "new")
	r := New(dir, nil)
	workspace, err := r.ListFiles(context.Background(), vcs.ContentRef{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"hello.go", "sub/hello.go", "extensionless", "untracked.go", "space name.go"} {
		if !slices.Contains(workspace, path) {
			t.Errorf("workspace listing omitted %q: %v", path, workspace)
		}
	}
	if slices.Contains(workspace, "ignored.go") {
		t.Fatal("ignored workspace file was included")
	}
	historical, err := r.ListFiles(context.Background(), vcs.ContentRef{Revision: head})
	if err != nil || !slices.Contains(historical, "extensionless") || slices.Contains(historical, "untracked.go") {
		t.Fatalf("revision listing: %v, %v", historical, err)
	}
	if _, err := r.ListFiles(context.Background(), vcs.ContentRef{Revision: "missing"}); err == nil {
		t.Fatal("invalid historical listing fell back")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.ListFiles(ctx, vcs.ContentRef{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("listing cancellation: %v", err)
	}
}

func TestListFilesPlainDirectoryAndCommandFailure(t *testing.T) {
	dir := t.TempDir()
	contentWrite(t, dir, "source.go", "source")
	contentWrite(t, dir, "extensionless", "source")
	contentWrite(t, dir, ".gitignore", "ignored.go\n")
	contentWrite(t, dir, "ignored.go", "ignored")
	contentWrite(t, dir, "node_modules/lib.js", "generated")
	r := New(dir, nil)
	files, err := r.ListFiles(context.Background(), vcs.ContentRef{})
	if err != nil || !slices.Contains(files, "source.go") || !slices.Contains(files, "extensionless") || slices.Contains(files, "ignored.go") || slices.Contains(files, "node_modules/lib.js") {
		t.Fatalf("plain directory listing: %v, %v", files, err)
	}
	if _, err := r.ListFiles(context.Background(), vcs.ContentRef{Revision: "HEAD"}); err == nil {
		t.Fatal("plain directory historical listing fell back")
	}
	r.Runner = &gitcmd.Runner{}
	if _, err := r.ListFiles(context.Background(), vcs.ContentRef{}); err == nil {
		t.Fatal("invalid runner error was hidden by filesystem fallback")
	}
}

func TestListFilesWithoutGitExecutable(t *testing.T) {
	dir := t.TempDir()
	contentWrite(t, dir, "source.go", "source")
	contentWrite(t, dir, ".gitignore", "ignored.go\n")
	contentWrite(t, dir, "ignored.go", "ignored")
	t.Setenv("PATH", "")
	r := New(dir, nil)
	files, err := r.ListFiles(context.Background(), vcs.ContentRef{})
	if err != nil || !slices.Contains(files, "source.go") || slices.Contains(files, "ignored.go") {
		t.Fatalf("workspace without Git: %v, %v", files, err)
	}
	if _, err := r.ListFiles(context.Background(), vcs.ContentRef{Revision: "HEAD"}); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("historical listing without Git: %v", err)
	}
}
