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
	"runtime"
	"slices"
	"testing"

	"github.com/alibaba/open-code-review/internal/vcs"
)

func TestWorkspaceReadFailuresDoNotInvokeConsumer(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name string
		root string
	}{
		{name: "missing file", root: root},
		{name: "missing repository directory", root: filepath.Join(root, "missing")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var source vcs.FileSource = New(test.root, nil)
			called := false
			err := source.ReadFile(context.Background(), vcs.ContentRef{}, "missing.go", func(io.Reader) error {
				called = true
				return nil
			})
			if !errors.Is(err, os.ErrNotExist) || called {
				t.Fatalf("missing workspace content: error=%v, consumer called=%t", err, called)
			}
		})
	}
}

func TestNotGitRepositoryProbeUsesExitStatus(t *testing.T) {
	plain := t.TempDir()
	err := exec.Command("git", "-C", plain, "rev-parse", "--git-dir").Run()
	if err == nil || !New(plain, nil).isNotGitRepository(context.Background(), err) {
		t.Fatalf("plain directory was not detected without an English diagnostic: %v", err)
	}
	dir, _ := contentTestRepo(t)
	if New(dir, nil).isNotGitRepository(context.Background(), err) {
		t.Fatal("a failed operation in a repository must not enable the walk fallback")
	}
}

func TestPlainDirectorySkipsUnreadableChild(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires enforced Unix directory permissions")
	}
	dir := t.TempDir()
	contentWrite(t, dir, "ok.go", "package ok\n")
	contentWrite(t, dir, "locked/secret.go", "package secret\n")
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0700) })
	files, err := New(dir, nil).ListFiles(context.Background(), vcs.ContentRef{})
	if err != nil || !slices.Equal(files, []string{"ok.go"}) {
		t.Fatalf("unreadable child prevented listing readable files: %v, %v", files, err)
	}
}

func TestWorkspaceReadPreservesConsumerErrorAndCancellation(t *testing.T) {
	dir := t.TempDir()
	contentWrite(t, dir, "source.go", "workspace content\n")
	var source vcs.FileSource = New(dir, nil)
	consumerErr := errors.New("consumer rejected content")
	err := source.ReadFile(context.Background(), vcs.ContentRef{}, "source.go", func(reader io.Reader) error {
		if _, err := io.ReadFull(reader, make([]byte, 1)); err != nil {
			return err
		}
		return consumerErr
	})
	if !errors.Is(err, consumerErr) {
		t.Fatalf("consumer error changed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = source.ReadFile(ctx, vcs.ContentRef{}, "source.go", func(reader io.Reader) error {
		if _, err := io.ReadFull(reader, make([]byte, 1)); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("callback cancellation was reported as success: %v", err)
	}
	// A failed callback or canceled read must not poison later workspace reads.
	err = source.ReadFile(context.Background(), vcs.ContentRef{}, "source.go", func(reader io.Reader) error {
		first := make([]byte, 1)
		if _, err := io.ReadFull(reader, first); err != nil {
			return err
		}
		if string(first) != "w" {
			t.Fatalf("unexpected workspace prefix: %q", first)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("later workspace prefix read failed: %v", err)
	}
}

func TestReadFileWithoutGitKeepsWorkspaceAndHistoricalSemantics(t *testing.T) {
	dir := t.TempDir()
	contentWrite(t, dir, "source.go", "workspace content\n")
	t.Setenv("PATH", "")
	var source vcs.FileSource = New(dir, nil)
	var content []byte
	err := source.ReadFile(context.Background(), vcs.ContentRef{}, "source.go", func(reader io.Reader) error {
		var err error
		content, err = io.ReadAll(reader)
		return err
	})
	if err != nil || string(content) != "workspace content\n" {
		t.Fatalf("workspace read required Git: %q, %v", content, err)
	}
	called := false
	err = source.ReadFile(context.Background(), vcs.ContentRef{Revision: "HEAD"}, "source.go", func(io.Reader) error {
		called = true
		return nil
	})
	if !errors.Is(err, exec.ErrNotFound) || called {
		t.Fatalf("historical read fell back: error=%v, consumer called=%t", err, called)
	}
}

func TestListFilesMissingDirectoryDoesNotBecomeEmptyWorkspace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	t.Setenv("PATH", "")
	var source vcs.FileSource = New(dir, nil)
	files, err := source.ListFiles(context.Background(), vcs.ContentRef{})
	if !errors.Is(err, os.ErrNotExist) || len(files) != 0 {
		t.Fatalf("missing directory was hidden by fallback: %v, %v", files, err)
	}
}

func TestCorruptIndexListingErrorDoesNotFallbackToWorkspace(t *testing.T) {
	dir, _ := contentTestRepo(t)
	contentWrite(t, dir, ".git/index", "invalid index\n")
	repository := New(dir, nil)
	var files vcs.FileSource = repository
	paths, err := files.ListFiles(context.Background(), vcs.ContentRef{})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || len(paths) != 0 {
		t.Fatalf("corrupt index listing fell back: %v, %v", paths, err)
	}
}
