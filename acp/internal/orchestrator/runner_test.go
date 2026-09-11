// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerCompletesAndKeepsStderrSeparate(t *testing.T) {
	binary := writeOCRScript(t)
	runner := NewRunner(binary)
	outcome, events := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"review"}})
	if outcome.Kind != OutcomeCompleted || outcome.Err != nil {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Result == nil || outcome.Result.Review == nil || outcome.Result.Review.Status != "complete" {
		t.Fatalf("result = %+v", outcome.Result)
	}
	if !containsEvent(events, "[ocr] running") {
		t.Fatalf("events = %+v, want stderr diagnostic", events)
	}
	if !strings.Contains(outcome.Diagnostics, "[ocr] running") {
		t.Fatalf("diagnostics = %q", outcome.Diagnostics)
	}
}

func TestRunnerConsumesPhaseThreeMockOCR(t *testing.T) {
	binary := buildPhaseThreeMock(t)
	runner := NewRunner(binary)
	outcome, events := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"review"}})
	if outcome.Kind != OutcomeCompleted || outcome.Result == nil || outcome.Result.Review == nil {
		t.Fatalf("outcome = %+v", outcome)
	}
	if got := len(outcome.Result.Review.Comments); got != 2 {
		t.Fatalf("comment count = %d, want 2", got)
	}
	if !containsEvent(events, "[ocr] 1 file(s) changed, reviewing 1") {
		t.Fatalf("events = %+v, want mock OCR progress", events)
	}
}

func buildPhaseThreeMock(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "mock-ocr")
	build := exec.Command("go", "build", "-o", binary, "../../testdata/mock-ocr")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build phase 3 mock: %v\n%s", err, output)
	}
	return binary
}

func TestRunnerRejectsNonZeroExitEvenWithJSON(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	outcome, _ := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"review", "nonzero"}})
	if outcome.Kind != OutcomeFailed || outcome.ExitCode != 7 {
		t.Fatalf("outcome = %+v", outcome)
	}
	assertErrorKind(t, outcome.Err, ErrorExit)
}

func TestRunnerRejectsInvalidAndMultipleJSON(t *testing.T) {
	for _, arg := range []string{"invalid", "multiple"} {
		t.Run(arg, func(t *testing.T) {
			runner := NewRunner(writeOCRScript(t))
			outcome, _ := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"review", arg}})
			if outcome.Kind != OutcomeFailed {
				t.Fatalf("outcome = %+v", outcome)
			}
			assertErrorKind(t, outcome.Err, ErrorDecode)
		})
	}
}

func TestRunnerLocalCancelOverridesExit(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, outcomes := runner.Run(ctx, Request{CWD: t.TempDir(), Args: []string{"review", "block"}})
	for event := range events {
		if event.Message == "READY" {
			cancel()
			break
		}
	}
	outcome := <-outcomes
	if outcome.Kind != OutcomeCancelled || !errors.Is(outcome.Err, context.Canceled) {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", outcome.ExitCode)
	}
}

func TestRunnerDeadlineOverridesExit(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	runner.GracePeriod = 100 * time.Millisecond
	deadline := time.Now().Add(100 * time.Millisecond)
	outcome, _ := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"scan", "block"}, Deadline: deadline})
	if outcome.Kind != OutcomeTimedOut || !errors.Is(outcome.Err, context.DeadlineExceeded) {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestRunnerContextDeadlineOverridesExit(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	outcome, _ := runRequest(t, runner, ctx, Request{CWD: t.TempDir(), Args: []string{"review", "block"}})
	if outcome.Kind != OutcomeTimedOut {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestRunnerScanResult(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	outcome, _ := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"scan"}})
	if outcome.Kind != OutcomeCompleted || outcome.Result == nil || outcome.Result.Scan == nil {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestRunnerBoundsOutputAndStderrTail(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	runner.Limits = Limits{StdoutBytes: 100, StderrBytes: 100, StderrLine: 16, EventQueue: 2}
	outcome, events := runRequest(t, runner, context.Background(), Request{CWD: t.TempDir(), Args: []string{"review", "large"}})
	if outcome.Kind != OutcomeFailed {
		t.Fatalf("outcome = %+v", outcome)
	}
	assertErrorKind(t, outcome.Err, ErrorStdoutLimit)
	if len(outcome.Diagnostics) > 100 {
		t.Fatalf("diagnostics len = %d, want <= 100", len(outcome.Diagnostics))
	}
	if !containsWarning(events) {
		t.Fatalf("events = %+v, want truncation warning", events)
	}
}

func TestRunnerRejectsInvalidCWDAndArgs(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	for _, request := range []Request{
		{CWD: filepath.Join(t.TempDir(), "missing"), Args: []string{"review"}},
		{CWD: t.TempDir(), Args: []string{"unexpected"}},
	} {
		outcome, _ := runRequest(t, runner, context.Background(), request)
		if outcome.Kind != OutcomeFailed {
			t.Fatalf("outcome = %+v", outcome)
		}
		if !errors.Is(outcome.Err, context.Canceled) && outcome.Err == nil {
			t.Fatal("expected validation error")
		}
	}
}

func runRequest(t *testing.T, runner Runner, ctx context.Context, request Request) (Outcome, []Event) {
	t.Helper()
	events, outcomes := runner.Run(ctx, request)
	collected := make(chan []Event, 1)
	go func() {
		var result []Event
		for event := range events {
			result = append(result, event)
		}
		collected <- result
	}()
	outcome := <-outcomes
	return outcome, <-collected
}

func assertErrorKind(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	var got *Error
	if !errors.As(err, &got) || got.Kind != want {
		t.Fatalf("error = %v, want kind %s", err, want)
	}
}

func containsEvent(events []Event, message string) bool {
	for _, event := range events {
		if event.Message == message {
			return true
		}
	}
	return false
}

func containsWarning(events []Event) bool {
	for _, event := range events {
		if event.Kind == EventWarning {
			return true
		}
	}
	return false
}

func writeOCRScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ocr")
	const script = `#!/bin/sh
mode="$2"
case "$mode" in
  nonzero)
    printf '%s\n' '{"status":"complete","comments":[]}'
    exit 7
    ;;
  invalid)
    printf '%s\n' '{invalid'
    exit 0
    ;;
  multiple)
    printf '%s\n%s\n' '{"status":"complete","comments":[]}' '{"status":"complete","comments":[]}'
    exit 0
    ;;
  block)
    trap 'printf "%s\\n" "{\\"status\\":\\"failed\\",\\"comments\\":[]}"; exit 1' INT TERM
    printf '%s\n' 'READY' >&2
    while :; do sleep 1; done
    ;;
  large)
    i=0
    while [ "$i" -lt 20 ]; do printf 'xxxxxxxxxx' >&2; i=$((i + 1)); done
    printf '\n' >&2
    i=0
    while [ "$i" -lt 30 ]; do printf 'xxxxxxxxxx'; i=$((i + 1)); done
    printf '\n'
    ;;
  *)
    printf '%s\n' '[ocr] running' >&2
    if [ "$1" = scan ]; then
      printf '%s\n' '{"status":"success","comments":[]}'
    else
      printf '%s\n' '{"status":"complete","comments":[],"manifest":{"terminal_state":"complete"}}'
    fi
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write OCR script: %v", err)
	}
	return path
}
