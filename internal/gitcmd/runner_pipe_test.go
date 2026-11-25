//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"bytes"
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
// stderr text attached to a failing command. `git show` is used instead of
// `git rev-parse --verify` because its stderr echoes the offending argument on
// every git version; rev-parse prints a generic message on some platforms.
func TestRunner_StreamFailureIncludesStderr(t *testing.T) {
	dir := initRepo(t)

	err := New(1).Stream(context.Background(), dir, func(stdout io.Reader) error {
		_, _ = io.Copy(io.Discard, stdout)
		return nil
	}, "show", "does-not-exist")
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

	_, stderr, err := New(1).RunSplit(context.Background(), dir, "show", "does-not-exist")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(stderr, "does-not-exist") {
		t.Errorf("stderr = %q, want it to mention the bad revision", stderr)
	}
}

// boundedStderr must keep the head and tail of a stream while draining all of
// it: Output's retention stays bounded like exec.Cmd.Output even when a command
// produces a very large diagnostic stream.
func TestBoundedStderrKeepsPrefixAndSuffix(t *testing.T) {
	var b boundedStderr
	const total = 1 << 20
	chunk := make([]byte, 1024)
	for i := range chunk {
		chunk[i] = byte(0x41 + (i % 26))
	}
	for written := 0; written < total; written += len(chunk) {
		if _, err := b.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}

	got := b.Bytes()
	if len(got) >= total {
		t.Fatalf("retained %d bytes, want far less than the %d written", len(got), total)
	}
	if len(b.prefix) != boundedStderrPrefixLimit || len(b.suffix) != boundedStderrSuffixLimit {
		t.Fatalf("prefix=%d suffix=%d, want %d/%d", len(b.prefix), len(b.suffix), boundedStderrPrefixLimit, boundedStderrSuffixLimit)
	}
	if b.total != total {
		t.Fatalf("total = %d, want %d", b.total, total)
	}
	if !bytes.HasPrefix(got, b.prefix) {
		t.Fatal("retained bytes do not start with the prefix")
	}
	if !bytes.HasSuffix(got, b.suffix) {
		t.Fatal("retained bytes do not end with the suffix")
	}
	if !strings.Contains(string(got), "bytes skipped") {
		t.Fatal("missing truncation marker")
	}
	// Kept bytes plus the skipped count must add back up to the total.
	if skipped := total - len(b.prefix) - len(b.suffix); skipped <= 0 {
		t.Fatalf("skipped = %d, want > 0", skipped)
	}
}

// A failing command with a huge stderr stream must surface bounded stderr on
// the ExitError, matching the old exec.Cmd.Output retention policy.
func TestRunner_OutputFailedCommandBoundsStderr(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	aliasGit(t, dir, "noisy", "!sh -c 'head -c 1048576 /dev/zero >&2; exit 1'")

	_, err := New(1).Output(context.Background(), dir, "noisy")
	if err == nil {
		t.Fatal("expected an error")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("error is %T, want *exec.ExitError", err)
	}
	if len(exitErr.Stderr) >= 1<<20 {
		t.Fatalf("ExitError.Stderr retained %d bytes, want it bounded", len(exitErr.Stderr))
	}
	if !strings.Contains(string(exitErr.Stderr), "bytes skipped") {
		t.Fatalf("ExitError.Stderr lost the truncation marker: %q", exitErr.Stderr[:min(120, len(exitErr.Stderr))])
	}
}

// RunSplit keeps the full stderr stream: callers there want the whole
// diagnostic, not just the head and tail.
func TestRunner_RunSplitKeepsFullStderr(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	aliasGit(t, dir, "noisy", "!sh -c 'head -c 1048576 /dev/zero >&2; exit 1'")

	_, stderr, err := New(1).RunSplit(context.Background(), dir, "noisy")
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(stderr) != 1<<20 {
		t.Fatalf("RunSplit stderr = %d bytes, want the full %d", len(stderr), 1<<20)
	}
}

// Cancellation with separate stdout/stderr orphans, repeated: the grace
// callback and the deferred pipe teardown must not race. Run under -race.
func TestRunner_CancelledWithSeparatePipesRepeated(t *testing.T) {
	skipIfNoSh(t)
	dir := initRepo(t)
	aliasGit(t, dir, "leak", "!sh -c 'sleep 1 2>/dev/null & printf ready; printf oops >&2'")

	orig := cancelGracePeriod
	cancelGracePeriod = 50 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	for range 20 {
		r := New(1)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_, _, _ = r.RunSplit(ctx, dir, "leak")
		cancel()
	}
}
