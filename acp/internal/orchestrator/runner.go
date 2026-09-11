// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

func (r *ProcessRunner) run(ctx context.Context, request Request, limits Limits, events chan<- Event, outcomes chan<- Outcome) {
	defer close(events)
	defer close(outcomes)
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	emit := func(event Event) {
		if event.OccurredAt.IsZero() {
			event.OccurredAt = now()
		}
		select {
		case events <- event:
		default:
			// Diagnostic/progress messages are explicitly best effort. Blocking
			// here could keep a child alive forever through pipe backpressure.
		}
	}
	finish := func(outcome Outcome) { outcomes <- outcome }

	if err := validateRequest(r.Binary, request); err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: err})
		return
	}

	stdin, err := os.Open(os.DevNull)
	if err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "open null stdin: %v", err)})
		return
	}
	defer stdin.Close()

	cmd := exec.Command(r.Binary, request.Args...)
	cmd.Dir = request.CWD
	cmd.Stdin = stdin
	configureProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "create stdout pipe: %v", err)})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "create stderr pipe: %v", err)})
		return
	}
	if err := cmd.Start(); err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "start OCR: %v", err)})
		return
	}

	streams := consumeStreams(stdout, stderr, limits, emit)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	cause, waitErr := r.await(ctx, request.Deadline, cmd, waited)
	stream, cleanErr := awaitStreams(streams)
	exitCode := commandExitCode(waitErr)
	result, decodeErr := decodeResult(request.Args, stream.stdout)

	// Local intent always wins. A cancellation may still carry a useful partial
	// document, but inability to decode it must not turn cancellation into an
	// ordinary command failure.
	if cause == OutcomeTimedOut {
		finish(Outcome{Kind: OutcomeTimedOut, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: context.DeadlineExceeded})
		return
	}
	if cause == OutcomeCancelled {
		finish(Outcome{Kind: OutcomeCancelled, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: context.Canceled})
		return
	}
	if cleanErr != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: cleanErr})
		return
	}
	if stream.err != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: newError(ErrorStream, "read OCR output: %v", stream.err)})
		return
	}
	if stream.stdoutExceeded {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: newError(ErrorStdoutLimit, "OCR stdout exceeded %d byte limit", limits.StdoutBytes)})
		return
	}
	if waitErr != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: newError(ErrorExit, "OCR exited with code %d: %v", exitCode, waitErr)})
		return
	}
	if decodeErr != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Err: decodeErr})
		return
	}
	finish(Outcome{Kind: OutcomeCompleted, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail})
}

func validateRequest(binary string, request Request) error {
	if binary == "" {
		return newError(ErrorInvalidRequest, "OCR binary is empty")
	}
	if len(request.Args) == 0 || (request.Args[0] != "review" && request.Args[0] != "scan") {
		return newError(ErrorInvalidRequest, "OCR arguments must start with review or scan")
	}
	if request.CWD == "" {
		return newError(ErrorInvalidCWD, "working directory is empty")
	}
	info, err := os.Stat(request.CWD)
	if err != nil {
		return newError(ErrorInvalidCWD, "stat working directory %q: %v", request.CWD, err)
	}
	if !info.IsDir() {
		return newError(ErrorInvalidCWD, "working directory %q is not a directory", request.CWD)
	}
	return nil
}

func (r *ProcessRunner) await(ctx context.Context, deadline time.Time, cmd *exec.Cmd, waited <-chan error) (OutcomeKind, error) {
	var deadlineTimer *time.Timer
	var deadlineCh <-chan time.Time
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d < 0 {
			d = 0
		}
		deadlineTimer = time.NewTimer(d)
		deadlineCh = deadlineTimer.C
		defer deadlineTimer.Stop()
	}
	select {
	case err := <-waited:
		return "", err
	case <-deadlineCh:
		return OutcomeTimedOut, r.stop(cmd, waited)
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (!deadline.IsZero() && !time.Now().Before(deadline)) {
			return OutcomeTimedOut, r.stop(cmd, waited)
		}
		return OutcomeCancelled, r.stop(cmd, waited)
	}
}

func (r *ProcessRunner) stop(cmd *exec.Cmd, waited <-chan error) error {
	// SIGINT gives OCR a chance to emit its cancellation document before the
	// bounded grace period expires.
	_ = interruptProcessGroup(cmd)
	grace := r.GracePeriod
	if grace <= 0 {
		grace = defaultGracePeriod
	}
	select {
	case err := <-waited:
		return err
	case <-time.After(grace):
		_ = killProcessGroup(cmd)
		select {
		case err := <-waited:
			return err
		case <-time.After(grace):
			return newError(ErrorCleanup, "OCR process did not exit after forced termination")
		}
	}
}

func awaitStreams(streams <-chan streamResult) (streamResult, error) {
	select {
	case stream := <-streams:
		return stream, nil
	case <-time.After(drainGracePeriod):
		// Do not wait forever for a descendant retaining a pipe. The process
		// group was already killed on local abort; report a bounded cleanup
		// failure on an otherwise completed invocation.
		return streamResult{}, newError(ErrorCleanup, "OCR stream readers did not exit within %s", drainGracePeriod)
	}
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		return exitErr.ProcessState.ExitCode()
	}
	return -1
}
