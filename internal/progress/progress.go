// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package progress is the single sink for everything a run wants to tell the
// user while it is still working. The review pipeline used to call
// fmt.Fprintf(stdout.Writer(), "[ocr] ...") at every interesting moment, which
// produced a wall of lines nobody could read at a glance and no answer to
// "which file is it on right now".
//
// Events published here reach one of two consumers. The default text sink
// prints each event as the [ocr] line it always was, so piped, CI, and
// --format json/sarif runs are byte-for-byte unchanged. When a human is
// watching an interactive terminal, the command layer installs a Sink of its
// own — the live dashboard in cmd/opencodereview — which consumes the same
// events and draws a full-screen view instead.
//
// The split is deliberate. This package owns the event stream and the plain
// text that has always been printed; the terminal UI lives with the other
// terminal UI, next to the provider wizard it shares a palette with.
//
// Preserving the text output is a contract, not an accident: an event that
// exists only to update the dashboard (a group starting, a round beginning)
// must carry no Text, because the pipeline already prints its own line for
// that fact at the call site. Publishing both would add a duplicate line to
// every non-interactive run.
//
// The pipeline publishes from many goroutines at once, so every entry point is
// safe for concurrent use. The zero value is usable and discards.
package progress

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/stdout"
)

// prefix is prepended to every text-sink line. It is part of the tool's
// observable output and is asserted by tests, so it lives in exactly one place.
const prefix = "[ocr] "

// Kind classifies an event. The kind decides how the TUI renders the event and
// is the only thing that differs between the two sinks: the text sink
// reconstructs the original line, the TUI maps the kind onto a row, a colour,
// and a counter.
type Kind int

const (
	// KindNotice is a plain informational line, such as the file count at
	// startup. The overwhelming majority of the pipeline's output is this.
	KindNotice Kind = iota
	// KindWarning is a notice that reports a problem the run recovered from.
	KindWarning
	// KindGroupStart announces that a unit of concurrent work (a file, or a
	// group of semantically related files) has begun.
	KindGroupStart
	// KindRoundStart announces a new LLM round within a group.
	KindRoundStart
	// KindGroupDone reports a group that finished. Findings is meaningful only
	// for this kind.
	KindGroupDone
	// KindGroupFailed reports a group that ended in an error.
	KindGroupFailed
	// KindSkipped reports a file the run deliberately did not review.
	KindSkipped
	// KindBudgetReached reports that the run stopped dispatching because it
	// hit a token or cost budget. It is the one condition that changes what a
	// finished run will say, so the TUI promotes it into the status line
	// instead of leaving it in the log where it would be missed.
	KindBudgetReached
	// KindPlanned reports how many files the run intends to review, once
	// selection and filtering have settled it. It carries no text: it is a
	// denominator for the progress bar, not something to say.
	KindPlanned
	// KindReused reports files a resumed run did not have to review, because a
	// previous run already finished them. They count towards the run's
	// coverage — the bar has to reach full — but they get no work row and
	// spend no tokens, so the counter that owns them is separate.
	KindReused
	// KindUsage reports tokens spent by one unit of work. It is published
	// after every LLM response so a row's running total is live, not just its
	// final value.
	KindUsage
	// KindToolStart, KindToolDone, and KindToolError are the lifecycle of a
	// single tool call issued by the agent loop.
	KindToolStart
	KindToolDone
	KindToolError
)

// Plan describes the shape of a run: what it is estimated to cost, and how
// the work is grouped.
//
// The log says both, but only as lines the reader has to find among the tool
// calls. The dashboard shows them where the run is being watched: the estimate
// on a line of its own, and the batch beside each row.
type Plan struct {
	// InputTokens and OutputTokens are the run's estimated token cost. The
	// estimate is a floor — agent tool use multiplies the prompt — so it is
	// shown as what it is.
	InputTokens  int64
	OutputTokens int64
	// Strategy names how the work is grouped — "by-language", "by-directory" —
	// and Groups is how many groups it is split into.
	Strategy string
	Groups   int
	// Batch is the group an item belongs to, as "2 of 5". It is empty when
	// the work is not grouped.
	Batch string
}

// Event is one thing the run wants to say. Text is the already-formatted
// message: the text sink prints it verbatim (after the prefix) and the TUI
// shows it in the activity log, so neither sink has to re-derive the wording
// and the two can never disagree about what was said.
type Event struct {
	Kind Kind
	// Text is the full line, as the text sink prints it and as the TUI shows
	// it in the activity log.
	Text string
	// Detail is a compact form for the work list's "right now" column, where
	// a timestamp and a marker glyph have no room and no purpose. Empty for
	// events that never appear there.
	Detail string
	Group  string
	// Paths are the files a group covers. The text sink ignores them; the TUI
	// shows the first and counts the rest.
	Paths []string
	// Round is the LLM round number, for KindRoundStart.
	Round int
	// Files is how many files the event accounts for. It is what moves the
	// progress bar, so it must be a file count and never a finding count: a
	// group that produced nine findings is still one group of work done.
	Files int
	// Findings is how many review comments a finished group produced.
	Findings int
	// SkipReason is why a file was skipped, for KindSkipped. A run can skip
	// hundreds of files for the same handful of reasons, and the TUI collapses
	// a run of identical reasons into one row. The text sink ignores it: the
	// per-file lines are printed by the caller.
	SkipReason string
	// Plan carries the run's shape when the event has one: the token estimate
	// and the batch the work belongs to.
	Plan *Plan
	// InputTokens and OutputTokens are what one group has spent so far. For
	// Anthropic, InputTokens already includes cache read and write tokens, so
	// adding those on top here would double-count them.
	InputTokens  int64
	OutputTokens int64
	// Stderr routes the text sink's output to os.Stderr instead of
	// stdout.Writer(). A few diagnostics have always gone to stderr, and
	// moving them would change what a user sees when the two streams are
	// separated. The TUI ignores it, since it owns neither stream.
	Stderr bool
	Err    string
	Time   time.Time
}

// Sink consumes published events. Exactly one is installed at a time.
//
// It is exported because the interactive dashboard lives outside this package:
// it implements Sink and installs itself for the duration of a run. The
// package that owns the pipeline publishes events; the package that owns the
// terminal decides how they are shown.
type Sink interface {
	Publish(Event)
}

var (
	mu sync.RWMutex
	// The default sink is the text one, not a discard. Everything that
	// publishes before Start runs — delegate, preview, and the many code
	// paths that emit without a Runner — must behave exactly as it did when
	// these were bare fmt.Fprintf calls to stdout.Writer(), and tests that
	// capture progress with stdout.Swap depend on that. Silence is expressed
	// by pointing stdout.Writer() at io.Discard, not by swapping this.
	active Sink = defaultText
)

// defaultText is the one text sink, shared so that its mutex serialises every
// plain-text write in the process rather than just those routed through one
// Runner. Start installs this same value rather than a fresh textSink: two
// sinks with two mutexes would leave writes from before and during a run free
// to interleave mid-line, which is the one thing this type exists to prevent.
var defaultText = &textSink{}

// discardSink drops every event.
type discardSink struct{}

func (discardSink) Publish(Event) {}

// Publish hands ev to the installed sink. It is safe to call from any
// goroutine: the text sink takes its lock around a single Write, and the TUI
// sink hands the event to the program's event loop.
func Publish(ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	mu.RLock()
	s := active
	mu.RUnlock()
	s.Publish(ev)
}

// Noticef publishes a KindNotice whose text is the formatted message. The
// message keeps its own trailing newline, exactly as the fmt.Fprintf call it
// replaces did.
func Noticef(format string, a ...any) {
	Publish(Event{Kind: KindNotice, Text: fmt.Sprintf(format, a...)})
}

// Notice publishes a pre-formatted KindNotice.
func Notice(text string) { Publish(Event{Kind: KindNotice, Text: text}) }

// Reused reports that n files were carried over from a previous run's
// checkpoints rather than reviewed again.
func Reused(n int) {
	if n <= 0 {
		return
	}
	Publish(Event{Kind: KindReused, Files: n})
}

// Skipped reports that the run deliberately did not review a file.
//
// text is the exact line the text sink should print, so one call serves both
// consumers. Publishing a separate notice for the line and an event for the
// fact doubled every skip in the dashboard, and the notice landing between two
// skips also broke the log's run-collapsing.
func Skipped(path, reason, text string) {
	Publish(Event{Kind: KindSkipped, Detail: path, SkipReason: reason, Text: text})
}

// warningLabel is prepended to every warning, so a caller routes a line here
// instead of typing the marker into its format string. Keeping the label here
// means the warning is recognisable as one by its kind rather than by matching
// text at the far end.
const warningLabel = "WARNING: "

// Warningf publishes a KindWarning. It supplies the "WARNING: " label itself;
// callers pass the message without it. Warnings are the events the dashboard
// colours and never drops, so they are worth distinguishing from notices.
func Warningf(format string, a ...any) {
	Publish(Event{Kind: KindWarning, Text: warningLabel + fmt.Sprintf(format, a...)})
}

// SetSinkForTest installs s as the sink for subsequent events and returns a
// function that restores the previous one.
//
// It exists so the packages that publish events — internal/agent and
// internal/scan — can assert in their own tests that the events a run
// publishes are the ones the dashboard needs. A package-local export_test.go
// cannot serve that: it is only visible to tests of package progress, and
// none of the call sites live here.
//
// It takes and returns the same unexported sink type, so it adds no
// repository-external API: the caller can only hand over a function that
// accepts an Event, which the progress package already exports.
func SetSinkForTest(s func(Event)) func() {
	if s == nil {
		return Install(nil)
	}
	return Install(sinkFunc(s))
}

// sinkFunc adapts a plain function to the sink interface.
type sinkFunc func(Event)

func (f sinkFunc) Publish(ev Event) { f(ev) }

// Install installs s as the sink for subsequent events and returns a function
// that restores the previous one.
//
// It exists for the interactive dashboard, which lives outside this package
// and installs itself for the duration of a run. Like stdout.Swap, the
// restore must run on the same goroutine that called Install: nested or
// concurrent swaps restore in a non-deterministic order.
//
// s may be nil, which discards.
func Install(s Sink) func() {
	mu.Lock()
	old := active
	if s == nil {
		active = discardSink{}
	} else {
		active = s
	}
	mu.Unlock()
	return func() {
		mu.Lock()
		active = old
		mu.Unlock()
	}
}

// textSink reproduces the historical output: one [ocr] line per event, written
// to whatever stdout.Writer() currently is. That indirection is what keeps
// --format json/sarif (progress redirected to stderr) and --audience agent
// (progress discarded) working without this package knowing about either.
//
// stdout.Writer's mutex guards the pointer swap, not the write that follows it,
// and the pipeline publishes from one goroutine per in-flight group. This
// sink therefore serialises its own writes: without it a captured buffer in a
// test, or a pipe in CI, would interleave half-lines.
type textSink struct {
	mu sync.Mutex
}

func (s *textSink) Publish(ev Event) {
	// An event with no text is state for the dashboard only — a group
	// starting, a round beginning, the coverage denominator. Writing the
	// prefix for one of those would emit "[ocr] " with no newline, and the
	// next real line would run into it as a doubled prefix.
	if ev.Text == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev.Stderr {
		io.WriteString(os.Stderr, prefix+ev.Text)
		return
	}
	io.WriteString(stdout.Writer(), prefix+ev.Text)
}

// text reports whether the active sink is a plain text one. Only the TUI
// needs to know, to route stray stderr diagnostics into its activity log
// instead of letting them scribble over the dashboard.
func text() bool {
	mu.RLock()
	defer mu.RUnlock()
	_, isText := active.(*textSink)
	return isText
}

// maxDiagLine bounds the unflushed tail of the diagnostic writer. No real
// diagnostic line comes close, and an unbounded buffer would let a writer that
// never sends a newline grow without limit.
const maxDiagLine = 8 << 10

// diag is the process-wide diagnostic writer behind ErrWriter. Terminal
// diagnostics are low-volume compared to pipeline events, so one mutex and one
// buffer are enough.
var diag = &diagWriter{}

// ErrWriter returns the writer that stray line-oriented diagnostics should go
// to. Most of them are fmt.Fprintf(os.Stderr, "[ocr] WARNING: ...") calls that
// predate this package. Routing them through here means that while the TUI
// owns the screen, they land in its activity log instead of corrupting the
// dashboard; with no TUI running they reach os.Stderr unchanged.
func ErrWriter() io.Writer { return diag }

// diagWriter buffers partial lines and republishes each completed one. A single
// Write may carry many lines and one line may be split across writes, so the
// state has to live in the writer rather than at the call site.
type diagWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (d *diagWriter) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.buf = append(d.buf, p...)
	for {
		i := bytes.IndexByte(d.buf, '\n')
		if i < 0 {
			break
		}
		line := string(d.buf[:i])
		d.buf = d.buf[i+1:]
		d.publishLine(line)
	}
	// Flush any overlong tail. This has to loop: a single Write can be many
	// times the cap, and a lone check would trim one chunk and leave the rest
	// above the cap for every subsequent Write to trip over.
	for len(d.buf) > maxDiagLine {
		d.publishLine(string(d.buf[:maxDiagLine]))
		d.buf = d.buf[maxDiagLine:]
	}
	return len(p), nil
}

func (d *diagWriter) publishLine(line string) {
	// Keep blank lines out of the log; they carry no information and the log
	// is a fixed number of rows that every line competes for.
	if strings.TrimSpace(line) == "" {
		return
	}
	if text() {
		io.WriteString(os.Stderr, line+"\n")
		return
	}
	// The text sink adds the prefix itself, so a line that already carries
	// one would show it twice. A diagnostic written straight to stderr
	// brings its own "[ocr] ", because the diag writer does not add one; the
	// events published through Noticef and Warningf do not, because the
	// prefix is the sink's job. Strip it here so the dashboard's log shows
	// the same rows either way.
	Publish(Event{Kind: KindWarning, Text: strings.TrimPrefix(line, prefix) + "\n"})
}
