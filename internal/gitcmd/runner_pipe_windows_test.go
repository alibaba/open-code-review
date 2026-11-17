//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests run on Windows and use portable Go helper processes instead of
// POSIX sh/sleep fixtures. The helper is this test binary re-executed via a git
// alias; it holds the Runner's output pipe the way a clean filter or hook
// descendant would, without any shell scripting.
//
// The alias itself is dispatched by git through its bundled sh (Git for Windows
// always ships one), but everything that matters — how long the descendant
// lives, what it writes, when it exits — is Go code, so the scenarios behave
// identically across platforms.

const (
	helperModeEnv  = "GITCMD_TEST_HELPER_MODE"
	helperSleepEnv = "GITCMD_TEST_HELPER_SLEEP_SECS"
)

// TestGitcmdPipeHelper is the helper entry point. It does nothing unless the
// mode env var is set, so a normal test run ignores it.
func TestGitcmdPipeHelper(t *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		t.Skip("helper process, not a test")
	}
	// git starts aliases with the repo as the working directory. Move off it so
	// an orphaned helper does not hold the temp repo open and the test's
	// TempDir cleanup can delete it.
	if err := os.Chdir(os.TempDir()); err != nil {
		os.Exit(4)
	}

	switch mode {
	case "sleep":
		// Hold the inherited stdout/stderr descriptors open for a bounded time,
		// then exit. This is the "descendant that outlives the direct child".
		time.Sleep(helperSleepDuration())
		return
	case "orphan":
		// Spawn a detached grandchild that inherits our stdout/stderr (i.e. the
		// Runner's pipe), then print and exit immediately, like a filter that
		// backgrounds a helper.
		child := exec.Command(os.Args[0], "-test.run=^TestGitcmdPipeHelper$")
		child.Env = append(os.Environ(),
			helperModeEnv+"=sleep",
			helperSleepEnv+"="+helperGrandchildSecs(),
		)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		_, _ = os.Stdout.WriteString("ready")
		return
	case "orphan-stdout":
		// Only stdout is inherited. stderr stays on the null device, so this is a
		// genuine stdout-only orphan: the case the original Stream regression
		// reported (stderr may already be at EOF while a descendant holds stdout).
		child := exec.Command(os.Args[0], "-test.run=^TestGitcmdPipeHelper$")
		child.Env = append(os.Environ(),
			helperModeEnv+"=sleep",
			helperSleepEnv+"="+helperGrandchildSecs(),
		)
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		_, _ = os.Stdout.WriteString("ready")
		return
	case "stderr-orphan":
		// Same idea but the grandchild retains only stderr, so stdout reaches EOF
		// promptly while the stderr pipe stays held.
		child := exec.Command(os.Args[0], "-test.run=^TestGitcmdPipeHelper$")
		child.Env = append(os.Environ(),
			helperModeEnv+"=sleep",
			helperSleepEnv+"="+helperGrandchildSecs(),
		)
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		_, _ = os.Stdout.WriteString("ready")
		return
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

// helperGrandchildSecs is how long the detached grandchild keeps the inherited
// pipe open; tests may shorten it, but it must stay well above the assertion
// windows used for cancellation.
func helperGrandchildSecs() string {
	if v := os.Getenv(helperSleepEnv); v != "" {
		return v
	}
	return "3"
}

func helperSleepDuration() time.Duration {
	secs := os.Getenv(helperSleepEnv)
	if secs == "" {
		return 5 * time.Second
	}
	d, err := time.ParseDuration(secs + "s")
	if err != nil {
		return 5 * time.Second
	}
	return d
}

// helperAlias configures a git alias that runs the test binary as the helper in
// the given mode. git dispatches the alias through its bundled sh; the path is
// quoted so spaces and non-ASCII characters survive.
func helperAlias(t *testing.T, dir, name, mode string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	value := `!"` + filepath.ToSlash(exe) + `" -test.run=^TestGitcmdPipeHelper$`
	if out, err := exec.Command("git", "-C", dir, "config", "alias."+name, value).CombinedOutput(); err != nil {
		t.Fatalf("git config alias.%s: %v\n%s", name, err, out)
	}
	t.Setenv(helperModeEnv, mode)
}

// requireGitSh skips when git cannot dispatch aliases (no bundled sh). On a
// stock Git for Windows install this is always present.
func requireGitSh(t *testing.T) {
	t.Helper()
	if out, err := exec.Command("git", "--exec-path").Output(); err != nil {
		t.Skipf("git unavailable: %v", err)
	} else if strings.TrimSpace(string(out)) == "" {
		t.Skip("git --exec-path is empty")
	}
}

// A cancelled RunSplit must return within the grace period, report the context
// cause, and not wait for a descendant that inherited only stdout.
func TestWindows_RunSplitCancelledWithOrphanHoldingStdout(t *testing.T) {
	requireGitSh(t)
	dir := initRepo(t)
	helperAlias(t, dir, "leak", "orphan-stdout")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := New(1).RunSplit(ctx, dir, "leak")
	elapsed := time.Since(start)
	t.Logf("RunSplit cancelled: elapsed=%s err=%v", elapsed, err)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("RunSplit took %s; the orphan (5s) still held the pipe", elapsed)
	}
}

// The same bound and cause must hold for Stream, where the consumer blocks on
// the pipe before Wait is reached.
func TestWindows_StreamCancelledWithOrphanHoldingStdout(t *testing.T) {
	requireGitSh(t)
	dir := initRepo(t)
	helperAlias(t, dir, "leak", "orphan-stdout")

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
	t.Logf("Stream cancelled: elapsed=%s err=%v", elapsed, err)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Stream took %s; the orphan (5s) still held the pipe", elapsed)
	}
}

// A stderr-only orphan must not keep the call alive either: stdout reaches EOF
// immediately, so only the owned stderr pipe can be the blocker.
func TestWindows_OutputCancelledWithOrphanHoldingStderr(t *testing.T) {
	requireGitSh(t)
	dir := initRepo(t)
	helperAlias(t, dir, "leakstderr", "stderr-orphan")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := New(1).Output(ctx, dir, "leakstderr")
	elapsed := time.Since(start)
	t.Logf("Output cancelled: elapsed=%s err=%v", elapsed, err)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Output took %s; the stderr orphan (5s) still held the pipe", elapsed)
	}
}

// A request already queued on a one-slot Runner must proceed once a cancelled
// call releases its slot. The first call is held open until the second one is
// provably waiting in acquire, so this exercises queueing rather than reuse.
func TestWindows_QueuedRequestProceedsAfterCancellation(t *testing.T) {
	requireGitSh(t)
	dir := initRepo(t)
	helperAlias(t, dir, "leak", "orphan-stdout")

	orig := cancelGracePeriod
	cancelGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	r := New(1)

	// Hold the only slot without a deadline; cancel explicitly below.
	ctx1, cancel1 := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := r.RunSplit(ctx1, dir, "leak")
		firstDone <- err
	}()

	// Let the first call acquire the slot and start its git process.
	time.Sleep(400 * time.Millisecond)

	// Queue a second request while the first is still active. It blocks in
	// acquire until the slot is released.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	secondDone := make(chan error, 1)
	go func() {
		_, _, err := r.RunSplit(ctx2, dir, "status", "--short")
		secondDone <- err
	}()

	// Confirm it is actually waiting rather than finished.
	time.Sleep(150 * time.Millisecond)
	select {
	case err := <-secondDone:
		t.Fatalf("queued request finished before the first was cancelled (err=%v); it was not queued", err)
	default:
	}

	// Cancelling the first call must release the slot and let the queued
	// request complete within the cleanup grace period plus scheduling slack.
	cancelledAt := time.Now()
	cancel1()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Errorf("first call error = %v, want context.Canceled", err)
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("queued call failed after %s: %v", time.Since(cancelledAt), err)
		}
		t.Logf("queued request completed %s after the cancellation", time.Since(cancelledAt))
		if elapsed := time.Since(cancelledAt); elapsed > 2*time.Second {
			t.Errorf("queued request waited %s after cancellation, want within the grace bound", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued request did not proceed after the cancelled call released the slot")
	}
}

// A successful command whose descendant flushes late must still succeed with
// complete output, so the grace period never leaks into the success path.
func TestWindows_SuccessWithLateDescendantOutput(t *testing.T) {
	requireGitSh(t)
	dir := initRepo(t)
	// The helper prints "ready" and exits; the grandchild keeps stdout open and
	// writes nothing, so EOF for the copy is delayed by the grandchild's exit.
	helperAlias(t, dir, "late", "orphan")
	t.Setenv(helperSleepEnv, "1")

	orig := cancelGracePeriod
	cancelGracePeriod = 50 * time.Millisecond
	t.Cleanup(func() { cancelGracePeriod = orig })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := New(1).Run(ctx, dir, "late")
	if err != nil {
		t.Fatalf("Run error = %v, want nil (a late descendant must not fail a successful command)", err)
	}
	if !strings.Contains(out, "ready") {
		t.Errorf("output = %q, want it to contain the helper's output", out)
	}
}
