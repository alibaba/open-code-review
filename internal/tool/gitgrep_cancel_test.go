// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// shimBlockingGit puts a fake `git` at the front of PATH that blocks without
// writing to either stream, for the duration of the test.
//
// The shim is the tiny Go helper under testdata/gitshim, compiled into a temp
// directory and named `git` (git.exe on Windows) so the test runs on Windows
// too, where a #!/bin/sh shebang script cannot run and where the bug this test
// pins actually showed up.
func shimBlockingGit(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	shimName := "git"
	if runtime.GOOS == "windows" {
		shimName = "git.exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(dir, shimName), "./testdata/gitshim")
	build.Dir, build.Stderr = ".", os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build git shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestGitGrep_CancelledNilRunnerReportsCancellation pins that a cancelled
// `git grep` on the nil-Runner path reports the cancellation rather than
// "No matches found".
//
// The cancelled process writes nothing, so on Windows it dies with exit code 1
// and an empty stream: the exitCode == 1 branch in gitGrep then read it as a
// grep with no hits. Requiring the signal (-1) in runGitGrep's guard made the
// bug Unix-only-invisible, since a signal there already satisfied it.
func TestGitGrep_CancelledNilRunnerReportsCancellation(t *testing.T) {
	shimBlockingGit(t)

	p := NewCodeSearch(&FileReader{RepoDir: t.TempDir(), Mode: ModeWorkspace})

	// The provider derives its own timeout from the parent context, so
	// cancelling the parent is what ends the shim.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := p.gitGrep(ctx, "Hello", false, false, nil)
	if err == nil {
		t.Fatalf("cancelled git grep reported success: %q", result)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// The path a correctly behaving guard takes: the timeout is named.
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	t.Fatalf("cancelled git grep reported %q, want the cancellation", err)
}
