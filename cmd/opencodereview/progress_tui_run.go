// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/progress"
)

// progressOptions describes the run the dashboard is reporting on.
type progressOptions struct {
	// Title names the operation, shown in the header ("review", "scan").
	Title string
	// Subtitle is optional secondary context.
	Subtitle string
	// Model and Provider identify the LLM the run is using. Both optional.
	Model    string
	Provider string
	// Repo is the working directory, shown shortened.
	Repo string

	// Audience is the --audience value. Anything other than "human" gets no
	// dashboard at all: that audience asked for a summary, not a live view.
	Audience string
	// MachineReadable reports whether the output format writes a structured
	// document to stdout. A dashboard cannot own the screen in that case,
	// because the document has to stay the only thing on stdout.
	MachineReadable bool
	// Enabled is the user's explicit opt-in (--tui). Without it the run prints plain [ocr] progress lines instead of taking over the terminal.
	Enabled bool
	// Color reports the caller's already-resolved --color decision. The
	// dashboard takes this rather than probing the terminal itself, so that
	// `--color=never` means never: an explicit opt-out has to reach the one
	// surface that would otherwise paint color on its own initiative.
	Color bool
	// Tokens reports the run's cumulative input and output token counts. The
	// dashboard polls it on its tick rather than being pushed an event per
	// LLM response: the per-group breakdown already arrives as events, and the
	// run-wide total is only two atomics away from whoever owns them. Nil
	// leaves the counter out.
	Tokens func() (input, output int64)
	// Session reports the run's session id. It is polled for the same reason
	// Tokens is: the id is empty until the session's first write persists, so
	// reading it once at start would usually show nothing.
	Session func() string
	// Cancel abandons the run. It is required because the dashboard puts the
	// terminal in raw mode, which clears the kernel's ISIG bit: no SIGINT is
	// generated while the view is up, so ctrl+c arrives as a key press and
	// only the dashboard can turn it back into a cancellation. Wiring it here
	// also leaves the run's own signal handling, including its second-signal
	// force-exit, as the single owner of the process's exit status.
	Cancel func()

	// TotalFiles is the coverage denominator, when the caller already knows
	// it. Zero leaves the bar empty until a progress.KindPlanned event sets it, which
	// is the normal path: the run freezes the denominator only after
	// selection has settled.
	TotalFiles int
}

// progressDashboardEnabled reports whether the dashboard should run at all.
//
// Every condition here is a case where a full-screen alternate-buffer program
// would be wrong: a pipe has no screen to draw on, --audience agent wants
// silence, and a json or sarif document needs stdout to itself. Detecting the
// terminal here rather than at the call site keeps the rule in one place.
//
// Color is deliberately not a condition. `--color=never` disables color, not
// the dashboard; the run still gets a live view, drawn plainly.
func progressDashboardEnabled(o progressOptions) bool {
	switch {
	case !o.Enabled:
		return false
	case o.Audience != "human":
		return false
	case o.MachineReadable:
		return false
	case !interactiveTerminal():
		return false
	}
	return true
}

// progressRunner owns the dashboard for the duration of one run. The zero value is a
// valid runner that does nothing, so callers never have to nil-check.
type progressRunner struct {
	prog    *tea.Program
	restore func()
	tokens  func() (int64, int64)
	stop    sync.Once
	// ran records whether the program was ever started. Program.Send and
	// Program.Wait both block forever on a program that was never Run, so
	// Stop must not touch a program that never started.
	ran atomic.Bool
}

// dashboardSink bridges the pipeline's Publish calls into the Bubble Tea update loop.
// Program.Send is documented as safe for concurrent use, which is what lets
// eight group goroutines report without any locking on this side.
type dashboardSink struct{ send func(tea.Msg) }

func (s dashboardSink) Publish(ev progress.Event) { s.send(dashboardEventMsg{Event: ev}) }

// startProgressDashboard installs the sink this run should use. When the dashboard is enabled
// it also builds the program; otherwise the run gets the plain text sink, or a
// discarding one if the audience asked for silence.
//
// The returned progressRunner must be released with Stop.
func startProgressDashboard(o progressOptions) *progressRunner {
	r := &progressRunner{}

	if !progressDashboardEnabled(o) {
		// Leave the default text sink in place rather than installing one
		// here. It already writes to stdout.Writer(), which is how
		// --audience agent's silence and --format json's stderr redirection
		// are expressed. A second silencing mechanism would have to be
		// restored in lockstep with the first, and the one restored later
		// would silently swallow the summary line that emitRunResult
		// deliberately un-silences.
		return r
	}

	m := newDashboardModel(o, o.TotalFiles)
	return newDashboardRunner(newDashboardProgram(m))
}

// newDashboardProgram builds the dashboard's Bubble Tea program.
//
// The two options are load-bearing. WithoutSignalHandler: the run's own
// signal handling owns cancellation, and a bubbletea SIGINT handler would
// quit the program so the terminal never saw the interrupt that the graceful
// shutdown path depends on. WithFPS: the loop already redraws on every
// message, and messages arrive at tool-call granularity, so an uncapped frame
// rate spends a bursty run's time writing to the terminal.
func newDashboardProgram(m dashboardModel) *tea.Program {
	return tea.NewProgram(m,
		tea.WithoutSignalHandler(),
		tea.WithFPS(20),
		tea.WithColorProfile(progressColorProfile()),
	)
}

// newDashboardRunner wires a program up as the progress sink. It is separate from
// startProgressDashboard so a test can supply a program wired to pipes and
// drive the real program loop without a terminal.
func newDashboardRunner(prog *tea.Program) *progressRunner {
	return &progressRunner{
		prog:    prog,
		restore: progress.Install(dashboardSink{send: prog.Send}),
	}
}

// duringProgressDashboard runs fn with the dashboard up and returns whatever fn
// returns.
//
// In text mode this is a direct call, so the non-interactive path costs one
// function indirection and nothing else. In TUI mode fn runs on its own
// goroutine while the program renders, and the dashboard is torn down before
// this returns — the caller writes its report to a restored terminal.
func duringProgressDashboard[T any](r *progressRunner, fn func() (T, error)) (T, error) {
	if r == nil || r.prog == nil {
		return fn()
	}

	type outcome struct {
		v   T
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		v, err := fn()
		done <- outcome{v, err}
		// The run is over, so the dashboard has nothing left to show. Quit
		// here rather than waiting for a key press: Run blocks until the
		// program quits, so without this a finished review would hang on a
		// screen nobody is going to dismiss.
		r.prog.Quit()
	}()

	// Set before Run returns: Send and Wait both block on a program that is
	// not running, so Stop has to know the difference.
	r.ran.Store(true)
	_, runErr := r.prog.Run()

	// If the user detached first, Run has already returned while the run is
	// still going. Waiting here keeps the caller from writing its report
	// until the review it is reporting on has actually finished.
	res := <-done
	// Stop before returning, so the caller emits its report onto a terminal
	// that has already left the alternate screen.
	r.Stop()
	// A dashboard failure is not a review failure. If the terminal could not
	// host the program, the run itself still succeeded and its results are
	// still valid; swallowing the error here keeps a cosmetic problem from
	// failing a review that worked.
	_ = runErr
	return res.v, res.err
}

// Stop tears the dashboard down and restores the previous sink. It is
// idempotent, so a deferred Stop alongside an explicit one is safe.
func (r *progressRunner) Stop() {
	if r == nil {
		return
	}
	r.stop.Do(func() {
		if r.prog != nil && r.ran.Load() {
			// Quit rather than Kill: the program gets to leave the
			// alternate screen and restore the cursor on its way out.
			r.prog.Quit()
			r.prog.Wait()
		}
		if r.restore != nil {
			r.restore()
		}
	})
}
