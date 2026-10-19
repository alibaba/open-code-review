// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func TestRunnerReclaimsDescendantsAfterLeaderExit(t *testing.T) {
	for _, command := range []string{"review", "scan"} {
		for _, exitCode := range []int{0, 7} {
			for _, inherited := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/exit=%d/inherited=%t", command, exitCode, inherited), func(t *testing.T) {
					dir := t.TempDir()
					pidFile := filepath.Join(dir, "child.pid")
					binary := testutil.Install(t, dir, "ocr", &testutil.Config{
						HoldPIDFile: pidFile,
						HoldInherit: inherited,
						HoldMS:      30000,
						ExitCode:    exitCode,
						StderrLines: []string{"[ocr] finished"},
						Stdout:      `{"status":"success","message":"complete output"}` + "\n",
					})
					t.Cleanup(func() {
						if data, err := os.ReadFile(pidFile); err == nil {
							if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
								_ = killPID(pid)
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
					for processRunning(pid) && time.Now().Before(deadline) {
						time.Sleep(10 * time.Millisecond)
					}
					if processRunning(pid) {
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
				binary := testutil.Install(t, dir, "ocr", &testutil.Config{
					Stdout:      `{"status":`,
					FlushMS:     300,
					FlushStdout: `"success","message":"flushed"}`,
					FlushStderr: "late diagnostics\n",
					ExitCode:    exitCode,
				})
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
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		t.Fatalf("parse child PID %q: %v", contents, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for processRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processRunning(pid) {
		t.Fatalf("child process %d is still alive", pid)
	}
}
