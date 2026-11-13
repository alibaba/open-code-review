//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// aliasGit configures a git alias so the spawned descendant inherits git's own
// stdout descriptor directly.
//
// A clean/smudge filter is NOT usable here: git talks to a filter over an
// internal pipe and copies its output, so the filter never holds the pipe the
// Runner owns and neither bug reproduces. Aliases inherit the descriptor.
func aliasGit(t *testing.T, dir, name, script string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "config", "alias."+name, script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config alias.%s: %v\n%s", name, err, out)
	}
}

// skipIfNoSh skips tests that need a POSIX shell and sleep utility.
func skipIfNoSh(t *testing.T) {
	t.Helper()
	if _, err := exec.Command("sh", "-c", "true").CombinedOutput(); err != nil {
		t.Skip("sh unavailable")
	}
	if _, err := exec.Command("sh", "-c", "sleep 0.01").CombinedOutput(); err != nil {
		t.Skip("sleep unavailable")
	}
}

// A cancelled Stream whose descendant retains only stdout must return shortly
// after its deadline instead of waiting for that descendant (#1909, probe A).
func TestRunner_StreamCancelledWithOrphanHoldingStdout(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	// The alias prints "ready" immediately and leaves a background child that
	// inherited git's stdout open for a second.
	aliasGit(t, dir, "leak", "!sh -c 'sleep 1 2>/dev/null & printf ready'")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := New(1).Stream(ctx, dir, func(stdout io.Reader) error {
		_, readErr := io.ReadAll(stdout)
		return readErr
	}, "leak")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 900*time.Millisecond {
		t.Errorf("Stream took %s after a 200ms deadline; the orphan still held the pipe", elapsed)
	}
}

// A cancelled Run must also return early when a descendant holds the pipe.
func TestRunner_RunCancelledWithOrphanHoldingStdout(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	aliasGit(t, dir, "leak", "!sh -c 'sleep 1 2>/dev/null & printf ready'")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := New(1).Run(ctx, dir, "leak")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 900*time.Millisecond {
		t.Errorf("Run took %s after a 200ms deadline; the orphan still held the pipe", elapsed)
	}
}

// The semaphore must be released even when the pipe wait is cut short,
// otherwise one cancelled call starves every later caller.
func TestRunner_CancelledCallReleasesSemaphore(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	aliasGit(t, dir, "leak", "!sh -c 'sleep 1 2>/dev/null & printf ready'")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	r := New(1)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, _ = r.RunSplit(ctx, dir, "leak")

	next, nextCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer nextCancel()
	start := time.Now()
	if _, err := r.Run(next, dir, "status"); err != nil {
		t.Fatalf("follow-up call failed after %s: %v", time.Since(start), err)
	}
}

// A successful command whose descendant flushes late must still succeed with
// complete output. This is the success-path regression that exec.Cmd.WaitDelay
// introduces (#1909, probe B).
func TestRunner_SuccessWithLateDescendantOutput(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	aliasGit(t, dir, "late", "!sh -c '(sleep 0.3; printf late) & printf early'")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := New(1).Run(ctx, dir, "late")
	if err != nil {
		t.Fatalf("Run error = %v, want nil (a late descendant must not fail a successful command)", err)
	}
	if out != "earlylate" {
		t.Errorf("output = %q, want %q", out, "earlylate")
	}
}

// The Stream contract (consume drains stdout) must keep working, including the
// stderr text attached to a failing command.
func TestRunner_StreamFailureIncludesStderr(t *testing.T) {
	dir := initRepo(t)

	err := New(1).Stream(context.Background(), dir, func(stdout io.Reader) error {
		_, _ = io.Copy(io.Discard, stdout)
		return nil
	}, "rev-parse", "--verify", "does-not-exist")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q should carry git's stderr", err)
	}
}

// RunSplit must still expose stderr on the returned error path.
func TestRunner_RunSplitReportsStderr(t *testing.T) {
	dir := initRepo(t)

	_, stderr, err := New(1).RunSplit(context.Background(), dir, "rev-parse", "--verify", "does-not-exist")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(stderr, "does-not-exist") {
		t.Errorf("stderr = %q, want it to mention the bad revision", stderr)
	}
}
