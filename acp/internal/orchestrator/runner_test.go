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

	"github.com/alibaba/open-code-review/acp/internal/testutil"
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
	binary := filepath.Join(t.TempDir(), testutil.ExeName("mock-ocr"))
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

func TestRunnerRetainsTruncationWarningWithoutEventConsumer(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	runner.Limits = Limits{StdoutBytes: 100, StderrBytes: 100, StderrLine: 16, EventQueue: 1}
	events, outcomes := runner.Run(context.Background(), Request{CWD: t.TempDir(), Args: []string{"review", "large"}})
	outcome := <-outcomes
	if outcome.Kind != OutcomeFailed {
		t.Fatalf("outcome = %+v", outcome)
	}
	if len(outcome.Warnings) != 1 || outcome.Warnings[0].Kind != EventWarning || !outcome.Warnings[0].Truncated {
		t.Fatalf("warnings = %+v", outcome.Warnings)
	}
	for range events {
	}
}

func TestRunnerChannelsCloseAfterSingleOutcome(t *testing.T) {
	runner := NewRunner(writeOCRScript(t))
	events, outcomes := runner.Run(context.Background(), Request{CWD: t.TempDir(), Args: []string{"review"}})
	outcome, ok := <-outcomes
	if !ok || outcome.Kind != OutcomeCompleted {
		t.Fatalf("outcome = %+v, open=%v", outcome, ok)
	}
	if _, ok := <-outcomes; ok {
		t.Fatal("outcomes channel remained open after terminal outcome")
	}
	for range events {
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

func TestConsumeStreamsCanCancelBlockedReaders(t *testing.T) {
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutWrite.Close()
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		stdoutRead.Close()
		t.Fatal(err)
	}
	defer stderrWrite.Close()

	streams := consumeStreams(stdoutRead, stderrRead, Limits{}, func(Event) {})
	streams.cancel()
	select {
	case <-streams.done:
	case <-time.After(time.Second):
		t.Fatal("stream readers did not stop after cancellation")
	}
	select {
	case result := <-streams.result:
		if result.err == nil {
			t.Fatal("expected reader error after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("stream result was not reported after cancellation")
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
	return testutil.Install(t, t.TempDir(), "ocr", nil)
}
