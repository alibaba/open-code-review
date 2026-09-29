// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package orchestrator executes the external OCR CLI without exposing process
// lifecycle details to the ACP protocol adapter.
package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
)

const (
	defaultStdoutLimit = 10 << 20
	defaultStderrLimit = 1 << 20
	defaultStderrLine  = 64 << 10
	defaultEventQueue  = 64
	defaultGracePeriod = 2 * time.Second
	drainGracePeriod   = 2 * time.Second
)

// EventKind identifies a non-terminal message emitted while OCR runs.
type EventKind string

const (
	EventProgress   EventKind = "progress"
	EventDiagnostic EventKind = "diagnostic"
	EventWarning    EventKind = "warning"
)

// Event is a bounded, best-effort progress or diagnostic message. The runner
// may drop an event when its consumer is slow. Critical stream warnings are
// also retained in Outcome.Warnings, which is the authoritative record.
type Event struct {
	Kind       EventKind
	Message    string
	Truncated  bool
	OccurredAt time.Time
}

// OutcomeKind is the local terminal state. A local timeout or cancellation
// takes precedence over the child process exit code and JSON status.
type OutcomeKind string

const (
	OutcomeCompleted OutcomeKind = "completed"
	OutcomeFailed    OutcomeKind = "failed"
	OutcomeCancelled OutcomeKind = "cancelled"
	OutcomeTimedOut  OutcomeKind = "timed_out"
)

// Result is the decoded stdout document. Exactly one of Review and Scan is
// set, based on the OCR subcommand supplied in Request.Args.
type Result struct {
	Review *contract.ReviewResult
	Scan   *contract.ScanResult
}

// Outcome is delivered exactly once, after the process and stream readers
// have been reaped or their bounded cleanup period has elapsed.
type Outcome struct {
	Kind        OutcomeKind
	Result      *Result
	ExitCode    int
	Diagnostics string
	Warnings    []Event
	Err         error
}

// Request contains a prevalidated OCR invocation. Args must begin with review
// or scan; this package never reparses an intent or changes the argument list.
type Request struct {
	CWD      string
	Args     []string
	Deadline time.Time
}

// Limits bounds data retained by a single execution. Zero fields use the
// documented defaults. EventQueue is intentionally small and lossy.
type Limits struct {
	StdoutBytes int
	StderrBytes int
	StderrLine  int
	EventQueue  int
}

func (l Limits) normalized() Limits {
	if l.StdoutBytes <= 0 {
		l.StdoutBytes = defaultStdoutLimit
	}
	if l.StderrBytes <= 0 {
		l.StderrBytes = defaultStderrLimit
	}
	if l.StderrLine <= 0 {
		l.StderrLine = defaultStderrLine
	}
	if l.EventQueue <= 0 {
		l.EventQueue = defaultEventQueue
	}
	return l
}

// ErrorKind makes failures machine-classifiable without forcing callers to
// match untrusted command text.
type ErrorKind string

const (
	ErrorInvalidRequest ErrorKind = "invalid_request"
	ErrorInvalidCWD     ErrorKind = "invalid_cwd"
	ErrorStart          ErrorKind = "start"
	ErrorStream         ErrorKind = "stream"
	ErrorStdoutLimit    ErrorKind = "stdout_limit"
	ErrorExit           ErrorKind = "exit"
	ErrorDecode         ErrorKind = "decode"
	ErrorResult         ErrorKind = "result"
	ErrorCleanup        ErrorKind = "cleanup"
	ErrorPlatform       ErrorKind = "platform_unsupported"
)

// Error describes a local orchestration failure.
type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return string(e.Kind)
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func newError(kind ErrorKind, format string, args ...any) error {
	return &Error{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// Runner is the phase-six-facing execution contract.
type Runner interface {
	Run(ctx context.Context, request Request) (<-chan Event, <-chan Outcome)
}

// ProcessRunner invokes one fixed OCR executable. ResolveBinary performs
// discovery at adapter startup; ProcessRunner deliberately accepts only that
// resolved path.
type ProcessRunner struct {
	Binary      string
	Limits      Limits
	GracePeriod time.Duration
	Now         func() time.Time
}

// NewRunner creates a runner with the documented resource limits.
func NewRunner(binary string) *ProcessRunner {
	return &ProcessRunner{Binary: binary}
}

// Run starts asynchronous execution and returns immediately. The events
// channel closes after stream readers finish; it may drop best-effort events
// when full. The outcomes channel has one buffered slot and receives exactly
// one Outcome before it closes, even for failures before process startup.
// It never writes child output to this process's stdout or stderr.
func (r *ProcessRunner) Run(ctx context.Context, request Request) (<-chan Event, <-chan Outcome) {
	limits := r.Limits.normalized()
	events := make(chan Event, limits.EventQueue)
	outcomes := make(chan Outcome, 1)
	go r.run(ctx, request, limits, events, outcomes)
	return events, outcomes
}
