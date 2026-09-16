// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"
)

func (r *ProcessRunner) run(ctx context.Context, request Request, limits Limits, events chan Event, outcomes chan<- Outcome) {
	defer close(outcomes)
	var closeEvents sync.Once
	defer func() { closeEvents.Do(func() { close(events) }) }()
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	emit := func(event Event) {
		if event.OccurredAt.IsZero() {
			event.OccurredAt = now()
		}
		critical := event.Kind == EventWarning || event.Truncated
		select {
		case events <- event:
		default:
			if !critical {
				return
			}
			// Preserve critical events by evicting one queued best-effort event.
			// Outcome.Warnings remains the authoritative fallback if a consumer
			// races this replacement.
			select {
			case <-events:
			default:
			}
			select {
			case events <- event:
			default:
			}
		}
	}
	finish := func(outcome Outcome) { outcomes <- outcome }
	preterminated := func() bool {
		if cause := localTermination(ctx, request.Deadline); cause != "" {
			finish(Outcome{Kind: cause, ExitCode: -1, Err: terminationError(cause)})
			return true
		}
		return false
	}
	if preterminated() {
		return
	}

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
	if err := configureProcessGroup(cmd); err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorPlatform, "%v", err)})
		return
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "create stdout pipe: %v", err)})
		return
	}
	defer stdoutRead.Close()
	defer stdoutWrite.Close()
	cmd.Stdout = stdoutWrite
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		stdoutWrite.Close()
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "create stderr pipe: %v", err)})
		return
	}
	defer stderrRead.Close()
	defer stderrWrite.Close()
	cmd.Stderr = stderrWrite
	// Recheck after setup, immediately before the OS start boundary. Cancellation
	// between this check and Start is handled by await; the two are not atomic.
	if preterminated() {
		return
	}
	if err := cmd.Start(); err != nil {
		stdoutWrite.Close()
		stderrWrite.Close()
		if preterminated() {
			return
		}
		finish(Outcome{Kind: OutcomeFailed, ExitCode: -1, Err: newError(ErrorStart, "start OCR: %v", err)})
		return
	}

	streams := consumeStreams(stdoutRead, stderrRead, limits, emit)
	defer func() { <-streams.done; closeEvents.Do(func() { close(events) }) }()
	waited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = stdoutWrite.Close()
		_ = stderrWrite.Close()
		waited <- err
	}()

	cause, waitErr, cleanupErr := r.await(ctx, request.Deadline, cmd, waited)
	stream, cleanErr := awaitStreams(streams)
	// Let short-lived helpers finish flushing inherited output before reclaiming
	// the process group. If they hold a pipe past the bounded drain window,
	// force cleanup so no descendant remains behind.
	if cause == "" {
		if err := killProcessGroup(cmd); err != nil {
			cleanupErr = errors.Join(cleanupErr, newError(ErrorCleanup, "clean process group after OCR exit: %v", err))
		}
	}
	exitCode := commandExitCode(waitErr)
	result, decodeErr := decodeResult(request.Args, stream.stdout)
	// Final decision point after stream collection and decoding. A cause already
	// selected by await wins permanently; otherwise observe again before delivery.
	// A cancellation after this point cannot retroactively rewrite completion.
	if cause == "" {
		cause = localTermination(ctx, request.Deadline)
		if cause != "" {
			replayed := make(chan error, 1)
			replayed <- waitErr
			var stopErr error
			waitErr, stopErr = r.stop(cmd, replayed)
			cleanupErr = errors.Join(cleanupErr, stopErr)
		}
	}

	// Local intent always wins. A cancellation may still carry a useful partial
	// document, but inability to decode it must not turn cancellation into an
	// ordinary command failure.
	if cause == OutcomeTimedOut {
		finish(Outcome{Kind: OutcomeTimedOut, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: localOutcomeError(context.DeadlineExceeded, cleanupErr, cleanErr)})
		return
	}
	if cause == OutcomeCancelled {
		finish(Outcome{Kind: OutcomeCancelled, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: localOutcomeError(context.Canceled, cleanupErr, cleanErr)})
		return
	}
	if err := errors.Join(cleanupErr, cleanErr); err != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: err})
		return
	}
	if stream.err != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: newError(ErrorStream, "read OCR output: %v", stream.err)})
		return
	}
	if stream.stdoutExceeded {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: newError(ErrorStdoutLimit, "OCR stdout exceeded %d byte limit", limits.StdoutBytes)})
		return
	}
	if waitErr != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: newError(ErrorExit, "OCR exited with code %d: %v", exitCode, waitErr)})
		return
	}
	if decodeErr != nil {
		finish(Outcome{Kind: OutcomeFailed, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings, Err: decodeErr})
		return
	}
	finish(Outcome{Kind: OutcomeCompleted, Result: result, ExitCode: exitCode, Diagnostics: stream.stderrTail, Warnings: stream.warnings})
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

// localTermination is a synchronous observation, not a watcher notification.
// Once an observation selects a cause, callers retain it throughout cleanup.
// If both reasons are first observed together, an expired deadline wins; a
// previously observed cancellation is never overwritten by a later deadline.
// Context exposes no manual-cancellation timestamp, so this does not claim to
// reconstruct the order of events that happened before the first observation.
func localTermination(ctx context.Context, deadline time.Time) OutcomeKind {
	err := ctx.Err()
	now := time.Now()
	if !deadline.IsZero() && !now.Before(deadline) {
		return OutcomeTimedOut
	}
	if ctxDeadline, ok := ctx.Deadline(); ok && !now.Before(ctxDeadline) {
		return OutcomeTimedOut
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OutcomeTimedOut
	}
	if err != nil {
		return OutcomeCancelled
	}
	return ""
}

func terminationError(kind OutcomeKind) error {
	if kind == OutcomeTimedOut {
		return context.DeadlineExceeded
	}
	return context.Canceled
}

func (r *ProcessRunner) await(ctx context.Context, deadline time.Time, cmd *exec.Cmd, waited <-chan error) (OutcomeKind, error, error) {
	if cause := localTermination(ctx, deadline); cause != "" {
		waitErr, cleanupErr := r.stop(cmd, waited)
		return cause, waitErr, cleanupErr
	}
	var timerC <-chan time.Time
	if !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		timerC = timer.C
	}
	var waitErr error
	completed := false
	select {
	case waitErr = <-waited:
		completed = true
	case <-ctx.Done():
	case <-timerC:
	}
	// This is the process-completion decision point. Re-observe local state
	// even when select picked waited, so ready cases cannot randomly decide
	// the outcome. No watcher has to run before this observation.
	cause := localTermination(ctx, deadline)
	if cause == "" {
		return "", waitErr, nil
	}
	if completed {
		// Preserve the consumed result for stop's existing cleanup path.
		// Reading waited a second time would deadlock.
		replayed := make(chan error, 1)
		replayed <- waitErr
		waited = replayed
	}
	waitErr, cleanupErr := r.stop(cmd, waited)
	return cause, waitErr, cleanupErr
}

func (r *ProcessRunner) stop(cmd *exec.Cmd, waited <-chan error) (error, error) {
	// SIGINT gives OCR a chance to emit its cancellation document before the
	// bounded grace period expires.
	var cleanupErrs []error
	if err := interruptProcessGroup(cmd); err != nil {
		cleanupErrs = append(cleanupErrs, newError(ErrorCleanup, "interrupt OCR process group: %v", err))
	}
	grace := r.GracePeriod
	if grace <= 0 {
		grace = defaultGracePeriod
	}
	select {
	case err := <-waited:
		// The leader may exit while descendants remain alive; always perform
		// the bounded process-group cleanup before returning.
		if cleanupErr := killProcessGroup(cmd); cleanupErr != nil {
			return err, newError(ErrorCleanup, "clean process group: %v", cleanupErr)
		}
		return err, errors.Join(cleanupErrs...)
	case <-time.After(grace):
		if err := killProcessGroup(cmd); err != nil {
			cleanupErrs = append(cleanupErrs, newError(ErrorCleanup, "kill OCR process group: %v", err))
		}
		select {
		case err := <-waited:
			return err, errors.Join(cleanupErrs...)
		case <-time.After(grace):
			cleanupErrs = append(cleanupErrs, newError(ErrorCleanup, "OCR process did not exit after forced termination"))
			return nil, errors.Join(cleanupErrs...)
		}
	}
}

func localOutcomeError(cause error, cleanupErrs ...error) error {
	errs := []error{cause}
	for _, err := range cleanupErrs {
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func awaitStreams(streams streamHandle) (streamResult, error) {
	select {
	case stream := <-streams.result:
		return stream, nil
	case <-time.After(drainGracePeriod):
		// Do not wait forever for a descendant retaining a pipe. The process
		// group was killed on local abort; on ordinary leader exit the caller
		// reclaims remaining helpers after this bounded drain attempt.
		streams.cancel()
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
