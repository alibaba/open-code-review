//go:build unix

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stubForceExit replaces the process-level force-exit hook with a recording
// stub so tests can observe the second-signal path without terminating the
// test binary. The returned channel carries one entry per forced-exit call.
func stubForceExit(t *testing.T) <-chan os.Signal {
	t.Helper()
	sigs := make(chan os.Signal, 4)
	orig := forceExit
	forceExit = func(sig os.Signal) { sigs <- sig }
	t.Cleanup(func() { forceExit = orig })
	return sigs
}

func TestInterruptContextFirstSignalCancelsGracefully(t *testing.T) {
	sigs := stubForceExit(t)
	ctx, stop := interruptContextWithForcedExit(context.Background())
	t.Cleanup(stop)

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("first SIGINT did not cancel the context within 5s")
	}
	select {
	case sig := <-sigs:
		t.Fatalf("forced exit invoked by the first signal: %v", sig)
	default:
	}
}

func TestInterruptContextSecondSignalForceExits(t *testing.T) {
	sigs := stubForceExit(t)
	ctx, stop := interruptContextWithForcedExit(context.Background())
	t.Cleanup(stop)

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("first SIGTERM did not cancel the context within 5s")
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send second SIGINT: %v", err)
	}
	select {
	case sig := <-sigs:
		if sig != syscall.SIGINT {
			t.Fatalf("forced exit got %v, want SIGINT", sig)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second signal did not trigger the forced exit within 5s")
	}
}

func TestInterruptContextStopBeforeSignalReleasesWatcher(t *testing.T) {
	stubForceExit(t)
	ctx, stop := interruptContextWithForcedExit(context.Background())
	stop()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop did not release the context")
	}
	// The watcher must have exited via done; give it a moment and make sure
	// nothing panics or deadlocks.
	time.Sleep(50 * time.Millisecond)
}

func TestInterruptContextParentCancelReleasesWatcher(t *testing.T) {
	stubForceExit(t)
	parent, parentCancel := context.WithCancel(context.Background())
	ctx, stop := interruptContextWithForcedExit(parent)
	t.Cleanup(stop)
	parentCancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not propagate to the review context")
	}
}

const (
	childEnvVar        = "OCR_INTERRUPT_CHILD"
	childReportEnvVar  = "OCR_CHILD_REPORT"
	childReadyMarker   = "OCR_CHILD_READY"
	childGracefulStart = "OCR_CHILD_GRACEFUL_START"
	childGracefulDone  = "OCR_CHILD_GRACEFUL_DONE"
)

type interruptChild struct {
	cmd    *exec.Cmd
	lines  <-chan string
	waited chan error
}

// startInterruptChild re-executes this test binary as a subprocess running
// TestInterruptChildHarness in the given mode, so the real forceExit hook
// (os.Exit) can be exercised end to end.
func startInterruptChild(t *testing.T, mode, reportPath string) *interruptChild {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestInterruptChildHarness$", "-test.timeout", "120s")
	cmd.Env = append(os.Environ(), childEnvVar+"="+mode, childReportEnvVar+"="+reportPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	ic := &interruptChild{cmd: cmd, lines: lines, waited: make(chan error, 1)}
	go func() { ic.waited <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return ic
}

func (ic *interruptChild) awaitLine(t *testing.T, marker string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-ic.lines:
			if !ok {
				msg := fmt.Sprintf("child exited before emitting %q", marker)
				select {
				case err := <-ic.waited:
					msg += fmt.Sprintf(": %v", err)
				default:
				}
				t.Fatal(msg)
			}
			if strings.Contains(line, marker) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", marker)
		}
	}
}

func (ic *interruptChild) awaitExit(t *testing.T, timeout time.Duration) int {
	t.Helper()
	select {
	case err := <-ic.waited:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		if err != nil {
			t.Fatalf("child wait: %v", err)
		}
		return 0
	case <-time.After(timeout):
		t.Fatalf("child still running after %v", timeout)
		return -1
	}
}

// TestInterruptChildHarness is the subprocess body for the second-signal
// integration tests. In the parent test binary it is skipped; the integration
// tests below re-execute this binary with OCR_INTERRUPT_CHILD set. The child
// installs the real signal handling, reports readiness, and then models the
// review lifecycle: the first signal starts a graceful shutdown whose cleanup
// writes the partial report; in forced mode the cleanup pretends to take far
// longer than any assertion window, so only the forced exit can end it.
func TestInterruptChildHarness(t *testing.T) {
	mode := os.Getenv(childEnvVar)
	if mode == "" {
		t.Skip("harness body only runs in child mode")
	}
	ctx, stop := interruptContextWithForcedExit(context.Background())
	defer stop()

	fmt.Println(childReadyMarker)
	select {
	case <-ctx.Done():
	case <-time.After(30 * time.Second):
		fmt.Println("OCR_CHILD_TIMEOUT")
		os.Exit(3)
	}
	fmt.Println(childGracefulStart)
	if mode == "graceful" {
		// Model the graceful defer chain: the partial report lands on disk
		// before the process exits.
		time.Sleep(200 * time.Millisecond)
		if err := os.WriteFile(os.Getenv(childReportEnvVar), []byte("partial review report\n"), 0o644); err != nil {
			t.Fatalf("write partial report: %v", err)
		}
		fmt.Println(childGracefulDone)
		return
	}
	// Forced mode: only a second signal can end this process inside the
	// parent's assertion window.
	time.Sleep(30 * time.Second)
	fmt.Println(childGracefulDone)
}

// TestSecondSignalExitsImmediately covers the issue #1141 acceptance
// criterion: after the first SIGINT started the graceful shutdown, a second
// SIGINT must end the process right away (well under 100ms) instead of being
// dropped, with exit status 1.
func TestSecondSignalExitsImmediately(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.md")
	child := startInterruptChild(t, "forced", report)
	child.awaitLine(t, childReadyMarker, 30*time.Second)

	if err := child.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send first SIGINT: %v", err)
	}
	child.awaitLine(t, childGracefulStart, 30*time.Second)

	start := time.Now()
	if err := child.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send second SIGINT: %v", err)
	}
	code := child.awaitExit(t, 10*time.Second)
	elapsed := time.Since(start)

	if elapsed >= 100*time.Millisecond {
		t.Fatalf("second SIGINT exited after %v, want <100ms", elapsed)
	}
	if code != 1 {
		t.Fatalf("forced exit status = %d, want 1", code)
	}
}

// TestSingleSignalGracefulShutdownUnaffected covers the other acceptance
// half: a single SIGINT still completes the graceful path — the simulated
// partial report lands on disk and the process exits 0.
func TestSingleSignalGracefulShutdownUnaffected(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.md")
	child := startInterruptChild(t, "graceful", report)
	child.awaitLine(t, childReadyMarker, 30*time.Second)

	if err := child.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}
	child.awaitLine(t, childGracefulDone, 30*time.Second)
	code := child.awaitExit(t, 10*time.Second)
	if code != 0 {
		t.Fatalf("graceful exit status = %d, want 0", code)
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("partial report not persisted: %v", err)
	}
	if string(data) != "partial review report\n" {
		t.Fatalf("unexpected report content: %q", data)
	}
}
