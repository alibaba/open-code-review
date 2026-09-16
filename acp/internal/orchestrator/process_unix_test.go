//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunnerReclaimsDescendantsAfterLeaderExit(t *testing.T) {
	for _, command := range []string{"review", "scan"} {
		for _, exitCode := range []int{0, 7} {
			for _, inherited := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/exit=%d/inherited=%t", command, exitCode, inherited), func(t *testing.T) {
					dir := t.TempDir()
					pidFile := filepath.Join(dir, "child.pid")
					redirect := " </dev/null >/dev/null 2>&1"
					if inherited {
						redirect = ""
					}
					// The helper stays in the OCR process group. It either holds the
					// output pipes open or lets them reach EOF before cleanup.
					binary := writeExecutable(t, dir, "ocr", "#!/bin/sh\nsleep 30"+redirect+" &\necho $! > child.pid\necho '[ocr] finished' >&2\necho '{\"status\":\"success\",\"message\":\"complete output\"}'\nexit "+strconv.Itoa(exitCode)+"\n")
					t.Cleanup(func() {
						if data, err := os.ReadFile(pidFile); err == nil {
							if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
								_ = syscall.Kill(pid, syscall.SIGKILL)
							}
						}
					})
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					outcome, _ := runRequest(t, NewRunner(binary), ctx, Request{CWD: dir, Args: []string{command}})
					data, err := os.ReadFile(pidFile)
					if err != nil {
						t.Fatal(err)
					}
					pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
					if err != nil {
						t.Fatal(err)
					}
					deadline := time.Now().Add(time.Second)
					for versionChildRunning(pid) && time.Now().Before(deadline) {
						time.Sleep(10 * time.Millisecond)
					}
					if versionChildRunning(pid) {
						t.Fatalf("descendant survived leader exit: kind=%s err=%v", outcome.Kind, outcome.Err)
					}
					if inherited {
						var typed *Error
						if outcome.ExitCode != exitCode || outcome.Kind != OutcomeFailed || !errors.As(outcome.Err, &typed) || typed.Kind != ErrorCleanup {
							t.Fatalf("unbounded pipe holder did not report cleanup failure: %+v", outcome)
						}
						return
					}
					if outcome.ExitCode != exitCode || !strings.Contains(outcome.Diagnostics, "[ocr] finished") || outcome.Result == nil {
						t.Fatalf("lost exit status or output: %+v", outcome)
					}
					if command == "review" && (outcome.Result.Review == nil || outcome.Result.Review.Message != "complete output") {
						t.Fatalf("lost review output: %+v", outcome.Result)
					}
					if command == "scan" && (outcome.Result.Scan == nil || outcome.Result.Scan.Message != "complete output") {
						t.Fatalf("lost scan output: %+v", outcome.Result)
					}
					if exitCode == 0 {
						if outcome.Kind != OutcomeCompleted || outcome.Err != nil {
							t.Fatalf("successful leader changed outcome: %+v", outcome)
						}
					} else {
						var typed *Error
						if outcome.Kind != OutcomeFailed || !errors.As(outcome.Err, &typed) || typed.Kind != ErrorExit {
							t.Fatalf("leader exit error was masked: %+v", outcome)
						}
					}
				})
			}
		}
	}
}

func TestRunnerAllowsBriefInheritedOutputWait(t *testing.T) {
	for _, command := range []string{"review", "scan"} {
		for _, exitCode := range []int{0, 7} {
			t.Run(fmt.Sprintf("%s/exit=%d", command, exitCode), func(t *testing.T) {
				dir := t.TempDir()
				binary := writeExecutable(t, dir, "ocr", "#!/bin/sh\nprintf '{\"status\":'\n(sleep 0.3; printf '\"success\",\"message\":\"flushed\"}'; echo 'late diagnostics' >&2) &\nexit "+strconv.Itoa(exitCode)+"\n")
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				outcome, _ := runRequest(t, NewRunner(binary), ctx, Request{CWD: dir, Args: []string{command}})
				if outcome.ExitCode != exitCode || outcome.Result == nil || !strings.Contains(outcome.Diagnostics, "late diagnostics") {
					t.Fatalf("helper output lost: %+v", outcome)
				}
				if command == "review" && (outcome.Result.Review == nil || outcome.Result.Review.Message != "flushed") {
					t.Fatalf("review JSON incomplete: %+v", outcome)
				}
				if command == "scan" && (outcome.Result.Scan == nil || outcome.Result.Scan.Message != "flushed") {
					t.Fatalf("scan JSON incomplete: %+v", outcome)
				}
				if exitCode == 0 {
					if outcome.Kind != OutcomeCompleted || outcome.Err != nil {
						t.Fatalf("helper output rejected: %+v", outcome)
					}
				} else {
					var typed *Error
					if outcome.Kind != OutcomeFailed || !errors.As(outcome.Err, &typed) || typed.Kind != ErrorExit {
						t.Fatalf("exit failure masked: %+v", outcome)
					}
				}
			})
		}
	}
}

func TestStopKillsIgnoringMemberAfterLeaderExit(t *testing.T) {
	// Both processes are children of the test so their exit can be reaped and
	// verified even on containers whose PID 1 does not reap orphan zombies.
	leader := exec.Command("sh", "-c", "read line")
	input, err := leader.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := configureProcessGroup(leader); err != nil {
		t.Fatal(err)
	}
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = leader.Process.Kill(); _ = leader.Wait() }()
	child := exec.Command("sh", "-c", "trap '' INT; echo READY; exec sleep 30")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: leader.Process.Pid}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "READY\n" {
		t.Fatalf("child readiness = %q, %v", line, err)
	}
	_ = input.Close()
	waited := make(chan error, 1)
	waited <- leader.Wait()
	if err := syscall.Kill(child.Process.Pid, 0); err != nil {
		t.Fatalf("group member exited before cleanup: %v", err)
	}
	if _, err := NewRunner("").stop(leader, waited); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
			t.Fatalf("expected forced group termination, got %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = child.Process.Kill()
		<-done
		t.Fatal("group member survived cleanup after leader exit")
	}
}

func TestRunnerCancellationReapsProcessGroupChild(t *testing.T) {
	binary := buildPhaseThreeMock(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := NewRunner(binary)
	events, outcomes := runner.Run(ctx, Request{
		CWD:  t.TempDir(),
		Args: []string{"review", "-scenario", "spawn-child", "-child-pid-file", pidFile},
	})
	for event := range events {
		if event.Message == "READY" {
			cancel()
			break
		}
	}
	outcome := <-outcomes
	if outcome.Kind != OutcomeCancelled {
		t.Fatalf("outcome = %+v", outcome)
	}
	contents, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child PID: %v", err)
	}
	pid, err := strconv.Atoi(string(contents[:len(contents)-1]))
	if err != nil {
		t.Fatalf("parse child PID %q: %v", contents, err)
	}
	if err := syscall.Kill(pid, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child process %d is still alive: %v", pid, err)
	}
}
