// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package progress

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/stdout"
	"testing"
)

// captureStdoutFor redirects stdout.Writer to w for the duration of the test.
// The text Sink writes there rather than to os.Stdout directly, so this is
// both how the Sink is exercised and how the tool's real stdout/stderr routing
// is asserted.
func captureStdoutFor(w io.Writer) func() {
	return stdout.Swap(w)
}

// sinkRecorder collects published events for assertions about routing.
type sinkRecorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *sinkRecorder) Publish(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *sinkRecorder) all() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// install points the package at s and restores the previous Sink afterwards.
func install(t *testing.T, s Sink) {
	t.Helper()
	resetDiag(t)
	t.Cleanup(Install(s))
}

// resetDiag empties the process-wide diagnostic buffer and fails the test if it
// was not already empty. The buffer is a deliberate singleton, so a test that
// leaves bytes in it would have them flushed into whatever the next test
// captures.
func resetDiag(t *testing.T) {
	t.Helper()
	if n := flushDiag(t); n != 0 {
		t.Fatalf("%d buffered diagnostic bytes leaked from an earlier test", n)
	}
}

// flushDiag closes the current partial line so it is published, and returns
// how many bytes were buffered beforehand.
func flushDiag(t *testing.T) int {
	t.Helper()
	diag.mu.Lock()
	defer diag.mu.Unlock()
	n := len(diag.buf)
	if n > 0 {
		diag.publishLine(string(diag.buf))
		diag.buf = nil
	}
	return n
}

func TestNoticefWritesPrefixedLine(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)
	Noticef("reviewing %d file(s)\n", 3)
	got := rec.all()
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Kind != KindNotice {
		t.Errorf("Kind = %v, want KindNotice", got[0].Kind)
	}
	if got[0].Text != "reviewing 3 file(s)\n" {
		t.Errorf("Text = %q, want the formatted message with its newline", got[0].Text)
	}
	if got[0].Time.IsZero() {
		t.Error("Publish must stamp Time when the caller left it zero")
	}
}

func TestWarningfIsItsOwnKind(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)
	Warningf("MCP server failed\n")
	if got := rec.all(); len(got) != 1 || got[0].Kind != KindWarning {
		t.Fatalf("expected one KindWarning, got %+v", got)
	}
}

func TestSetNilDiscards(t *testing.T) {
	var buf bytes.Buffer
	stdoutRestore := captureStdoutFor(&buf)
	defer stdoutRestore()

	install(t, nil)
	Noticef("must not appear\n")
	if buf.Len() != 0 {
		t.Errorf("a nil Sink must discard, got %q", buf.String())
	}
}

func TestSetRestoresPrevious(t *testing.T) {
	first := &sinkRecorder{}
	restoreFirst := Install(first)
	defer restoreFirst()

	second := &sinkRecorder{}
	restoreSecond := Install(second)
	Noticef("goes to second\n")
	restoreSecond()

	Noticef("goes to first\n")
	if len(second.all()) != 1 {
		t.Errorf("second Sink should have received only its own event, got %d", len(second.all()))
	}
	if len(first.all()) != 1 {
		t.Errorf("first Sink should have received the event after restore, got %d", len(first.all()))
	}
}

// TestTextSinkSerializesWrites is the regression guard for the interleaving
// hazard: stdout.Writer's mutex guards the pointer, not the write, and the
// pipeline publishes from one goroutine per in-flight group.
func TestTextSinkSerializesWrites(t *testing.T) {
	var buf bytes.Buffer
	restore := captureStdoutFor(&buf)
	defer restore()

	install(t, &textSink{})

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Noticef("line %d with some padding text\n", i)
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(lines))
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, prefix) {
			t.Errorf("line %q lost its prefix, so a write was torn", l)
		}
		if !strings.Contains(l, "padding text") {
			t.Errorf("line %q is truncated, so a write was torn", l)
		}
	}
}

// captureStderr points os.Stderr at a pipe for the duration of fn and returns
// everything written. The read happens on its own goroutine: a pipe holds only
// a bounded amount, so a synchronous read after the writes would deadlock
// whenever the payload outgrows the pipe buffer.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w

	var (
		mu  sync.Mutex
		got bytes.Buffer
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		b, _ := io.Copy(&got, r)
		_ = b
	}()

	fn()

	w.Close()
	os.Stderr = old
	<-done
	mu.Lock()
	defer mu.Unlock()
	return got.String()
}

func TestTextSinkStderrEventGoesToStderr(t *testing.T) {
	install(t, &textSink{})
	out := captureStderr(t, func() {
		Publish(Event{Kind: KindToolError, Text: "boom\n", Stderr: true})
	})
	if !strings.Contains(out, "boom") {
		t.Errorf("Stderr event must reach os.Stderr, got %q", out)
	}
}

func TestErrWriterSplitsLines(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)

	w := ErrWriter()
	io.WriteString(w, "[ocr] WARNING: one\n[ocr] WARNING: two\n")
	if got := rec.all(); len(got) != 2 {
		t.Fatalf("expected 2 events from one Write carrying 2 lines, got %d", len(got))
	}

	// A line split across writes must not be published until it completes.
	io.WriteString(w, "[ocr] WARNING: par")
	io.WriteString(w, "tial\n")
	if got := rec.all(); len(got) != 3 {
		t.Fatalf("expected the split line to complete into a 3rd event, got %d", len(got))
	}
}

func TestErrWriterDropsBlankLines(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)
	io.WriteString(ErrWriter(), "\n   \n")
	if got := rec.all(); len(got) != 0 {
		t.Errorf("blank lines carry no information and must not compete for log rows, got %d events", len(got))
	}
}

func TestErrWriterCapsUnterminatedLine(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)

	// A writer that never sends a newline would otherwise grow the buffer
	// without bound. The payload is many times the cap, so this also proves
	// the cap loops rather than trimming one chunk per Write.
	io.WriteString(ErrWriter(), strings.Repeat("x", maxDiagLine*3+10))

	// The cap leaves at most one chunk behind; closing it out must empty the
	// buffer, or the next Write would keep tripping over the residue.
	flushDiag(t)
	if got := rec.all(); len(got) < 3 {
		t.Errorf("an oversized unterminated line must be flushed in chunks, got %d events", len(got))
	}
	if n := flushDiag(t); n != 0 {
		t.Errorf("buffer not drained: %d bytes left above the cap", n)
	}
}

func TestErrWriterReachesStderrWithoutTUI(t *testing.T) {
	// With the text Sink installed there is no dashboard to protect, so
	// diagnostics go straight to stderr exactly as they always did.
	install(t, &textSink{})
	out := captureStderr(t, func() {
		io.WriteString(ErrWriter(), "[ocr] WARNING: direct\n")
	})
	if !strings.Contains(out, "direct") {
		t.Errorf("expected the diagnostic on stderr, got %q", out)
	}
}

// The text Sink adds the "[ocr] " prefix itself, so a diagnostic that
// already carries one would show it twice in the dashboard's log. A
// diagnostic written straight to stderr brings its own prefix, because the
// diag writer does not add one.
func TestErrWriterStripsThePrefixForTheDashboardLog(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)

	io.WriteString(ErrWriter(), "[ocr] WARNING: direct\n")
	io.WriteString(ErrWriter(), "no prefix here\n")

	got := rec.all()
	for _, ev := range got {
		if strings.HasPrefix(ev.Text, "[ocr] ") {
			t.Errorf("dashboard log row kept the text Sink's prefix: %q", ev.Text)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 log rows, got %d", len(got))
	}
	if !strings.Contains(got[0].Text, "direct") || !strings.Contains(got[1].Text, "no prefix here") {
		t.Errorf("dashboard log lost a diagnostic: %q, %q", got[0].Text, got[1].Text)
	}
}

func TestPublishStampsAndForwardsAnEvent(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)

	// A caller-supplied timestamp must survive; only a zero one is filled in.
	when := time.Now().Add(-time.Minute)
	Publish(Event{Kind: KindNotice, Text: "kept\n", Time: when})
	Publish(Event{Kind: KindNotice, Text: "stamped\n"})
	got := rec.all()
	if !got[0].Time.Equal(when) {
		t.Errorf("Publish overwrote a caller-supplied timestamp: %v", got[0].Time)
	}
	if got[1].Time.IsZero() {
		t.Error("Publish must fill in a zero timestamp")
	}
}

func TestNoticePublishesPreformattedText(t *testing.T) {
	rec := &sinkRecorder{}
	install(t, rec)
	Notice("already formatted\n")
	if got := rec.all(); len(got) != 1 || got[0].Text != "already formatted\n" {
		t.Errorf("Notice = %+v, want the text verbatim", got)
	}
}

// TestTextOutputIsUnchangedByTheMigration is the guarantee that made this
// refactor safe: every line the pipeline used to print with
// fmt.Fprintf(stdout.Writer(), "[ocr] ...") must arrive at stdout byte for
// byte, because pipes, CI logs, and the --format json contract all depend on
// it.
func TestTextOutputIsUnchangedByTheMigration(t *testing.T) {
	var buf bytes.Buffer
	restore := captureStdoutFor(&buf)
	defer restore()

	install(t, &textSink{})
	Noticef("%d file(s) changed, reviewing %d in %s\n", 12, 9, "/repo")
	Noticef("Skipping %s — binary file\n", "imgs/logo.png")
	Warningf("%s for group %q\n", "budget", "a.go,b.go")

	want := "" +
		"[ocr] 12 file(s) changed, reviewing 9 in /repo\n" +
		"[ocr] Skipping imgs/logo.png — binary file\n" +
		"[ocr] WARNING: budget for group \"a.go,b.go\"\n"
	if got := buf.String(); got != want {
		t.Errorf("text output changed.\n got: %q\nwant: %q", got, want)
	}
}

// TestTextSinkIgnoresStatelessEvents is the regression guard for a subtle one.
// The dashboard-only kinds (a group starting, a round beginning, the coverage
// denominator) carry no Text, because the pipeline already prints its own line
// for those facts. A Sink that wrote the prefix for them anyway would emit
// "[ocr] " with no newline, and the next real line would run into it — which
// looks exactly like a doubled prefix on that line rather than like the stray
// newline-free write it is.
func TestTextSinkIgnoresStatelessEvents(t *testing.T) {
	var buf bytes.Buffer
	restore := captureStdoutFor(&buf)
	defer restore()

	install(t, &textSink{})
	Publish(Event{Kind: KindPlanned, Files: 3})
	Publish(Event{Kind: KindGroupStart, Group: "a.go", Paths: []string{"a.go"}})
	Publish(Event{Kind: KindRoundStart, Group: "a.go", Round: 1})
	Noticef("reviewing a.go\n")
	Publish(Event{Kind: KindGroupDone, Group: "a.go", Files: 1, Findings: 2})

	want := "[ocr] reviewing a.go\n"
	if got := buf.String(); got != want {
		t.Errorf("stateless events leaked into the text stream.\n got: %q\nwant: %q", got, want)
	}
}

func TestPublishIsSafeForConcurrentGroups(t *testing.T) {
	// The pipeline publishes from one goroutine per in-flight group.
	var buf = new(strings.Builder)
	restore := captureStdoutFor(buf)
	defer restore()

	install(t, &textSink{})
	done := make(chan struct{})
	for i := range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 20 {
				Noticef("group %d line\n", i)
			}
		}()
	}
	for range 8 {
		<-done
	}
	if got := strings.Count(buf.String(), "\n"); got != 160 {
		t.Errorf("expected 160 lines, got %d — a write was torn", got)
	}
}

func TestSkippedFilesPrintNoTextLine(t *testing.T) {
	// The caller prints its own per-file line; the skip event must not add a
	// second one to piped output.
	var buf = new(strings.Builder)
	restore := captureStdoutFor(buf)
	defer restore()

	install(t, &textSink{})
	Skipped("a.ts", "binary file", "Skipping a.ts — binary file\n")
	if got := buf.String(); strings.Count(got, "Skipping") != 1 {
		t.Errorf("expected exactly one line, got %q", got)
	}
}
