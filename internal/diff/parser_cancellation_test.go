// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

const cancellationPatch = "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"

func TestParseDiffTextRejectsStoppedContext(t *testing.T) {
	for _, expired := range []bool{false, true} {
		for name, patch := range map[string]string{
			"empty":    "",
			"modified": cancellationPatch,
			"deleted":  "diff --git a/file.go b/file.go\ndeleted file mode 100644\n--- a/file.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n",
			"binary":   "diff --git a/file.bin b/file.bin\nBinary files a/file.bin and b/file.bin differ\n",
		} {
			if expired {
				name += "/deadline"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				want := context.Canceled
				if expired {
					cancel()
					ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
					want = context.DeadlineExceeded
				} else {
					cancel()
				}
				defer cancel()
				diffs, err := ParseDiffText(ctx, patch, t.TempDir(), "", nil)
				if !errors.Is(err, want) || diffs != nil {
					t.Fatalf("stopped parse returned %d diffs, %v; want nil, %v", len(diffs), err, want)
				}
			})
		}
	}
}

func TestParseDiffTextPropagatesCancellationDuringContentRead(t *testing.T) {
	for _, anotherFile := range []bool{false, true} {
		name := "last_file"
		if anotherFile {
			name = "before_next_file"
		}
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			runGitTest(t, repo, "init", "-q")
			runGitTest(t, repo, "config", "user.name", "Test User")
			runGitTest(t, repo, "config", "user.email", "test@example.com")
			runGitTest(t, repo, "config", "commit.gpgsign", "false")
			if err := os.WriteFile(filepath.Join(repo, "file.go"), []byte("new\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitTest(t, repo, "add", "file.go")
			runGitTest(t, repo, "commit", "-q", "-m", "source")

			runner := gitcmd.New(1)
			started := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				done <- runner.Stream(context.Background(), repo, func(r io.Reader) error {
					_, err := io.Copy(io.Discard, r)
					close(started)
					<-release
					return err
				}, "show", "HEAD:file.go")
			}()
			select {
			case <-started:
			case err := <-done:
				t.Fatalf("start git runner: %v", err)
			}
			t.Cleanup(func() {
				close(release)
				if err := <-done; err != nil {
					t.Errorf("release git runner: %v", err)
				}
			})

			patch := cancellationPatch
			if anotherFile {
				patch += "diff --git a/other.go b/other.go\nnew file mode 100644\n--- /dev/null\n+++ b/other.go\n@@ -0,0 +1 @@\n+other\n"
			}
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			diffs, err := ParseDiffText(ctx, patch, repo, "HEAD", runner)
			if !errors.Is(err, context.DeadlineExceeded) || diffs != nil {
				t.Fatalf("interrupted content load returned %d diffs, %v; want nil and deadline error", len(diffs), err)
			}
		})
	}
}

func TestParseDiffTextKeepsBestEffortContentRead(t *testing.T) {
	diffs, err := ParseDiffText(t.Context(), cancellationPatch, t.TempDir(), "", nil)
	if err != nil || len(diffs) != 1 || diffs[0].Insertions != 1 || diffs[0].Deletions != 1 || diffs[0].NewFileContent != "" {
		t.Fatalf("missing workspace content should remain best effort: %+v, %v", diffs, err)
	}
}
