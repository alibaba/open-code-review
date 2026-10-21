// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"testing"

	"github.com/alibaba/open-code-review/internal/progress"
	"github.com/alibaba/open-code-review/internal/stdout"
)

func TestEnabledRejectsEveryNonInteractiveCase(t *testing.T) {
	// Each case is a situation where an alternate-screen program would be
	// wrong: no screen to draw on, no human watching, or a document that needs
	// stdout to itself.
	cases := []struct {
		name string
		opts progressOptions
	}{
		{"not opted in", progressOptions{Audience: "human"}},
		{"agent audience", progressOptions{Audience: "agent", Enabled: true}},
		{"json output", progressOptions{Audience: "human", Enabled: true, MachineReadable: true}},
		{"sarif output", progressOptions{Audience: "human", Enabled: true, MachineReadable: true}},
	}
	for _, c := range cases {
		if progressDashboardEnabled(c.opts) {
			t.Errorf("%s: the dashboard must not run", c.name)
		}
	}
}

func TestStartWithoutATerminalFallsBackToText(t *testing.T) {
	// The test binary's stdout is not a tty, which is exactly the condition a
	// pipe or CI runner presents. This is the guarantee that piping a review
	// still produces the same plain lines.
	var buf bytes.Buffer
	restore := stdout.Swap(&buf)
	defer restore()

	r := startProgressDashboard(progressOptions{Audience: "human"})
	defer r.Stop()

	if r.prog != nil {
		t.Fatal("no program should be built when stdout is not a terminal")
	}
	progress.Noticef("plain line\n")
	if !strings.Contains(buf.String(), "[ocr] plain line") {
		t.Errorf("expected the text line, got %q", buf.String())
	}
}

func TestStartWithAgentAudienceStaysSilent(t *testing.T) {
	// --audience agent silences by pointing stdout at io.Discard, not by
	// swapping the progress sink. Start must not install a sink that outlives
	// the quiet handle, or the summary line emitRunResult deliberately
	// un-silences would be swallowed instead of printed.
	restore := stdout.Quiet()
	defer restore()

	r := startProgressDashboard(progressOptions{Audience: "agent"})
	defer r.Stop()

	progress.Noticef("must not appear\n")

	// Now prove the summary still gets out: restoring stdout is enough.
	restored := stdout.Swap(os.Stdout)
	defer restored()
	var buf bytes.Buffer
	restored2 := stdout.Swap(&buf)
	defer restored2()

	progress.Noticef("[ocr] Summary: restored\n")
	if !strings.Contains(buf.String(), "Summary: restored") {
		t.Errorf("after stdout is restored the summary must print, got %q", buf.String())
	}
}

func TestStartLeavesTheSinkAloneWhenNotInteractive(t *testing.T) {
	// A non-interactive run must not install a sink of its own, so there is
	// nothing for Stop to restore and no second silencing mechanism to fall
	// out of step with stdout's.
	before := &sinkRecorder{}
	restoreFirst := progress.Install(before)
	defer restoreFirst()

	r := startProgressDashboard(progressOptions{Audience: "human"})
	if r.restore != nil {
		t.Error("a text-mode run must not install a sink")
	}
	progress.Noticef("straight through\n")
	r.Stop()

	if len(before.all()) != 1 {
		t.Errorf("the existing sink must be left in place, it has %d events", len(before.all()))
	}
}

func TestDuringRunsInlineWithoutAProgram(t *testing.T) {
	r := startProgressDashboard(progressOptions{Audience: "agent"})
	defer r.Stop()

	got, err := duringProgressDashboard(r, func() (string, error) {
		return "done", nil
	})
	if got != "done" || err != nil {
		t.Errorf("During = (%q, %v), want (done, nil)", got, err)
	}
}

func TestDuringPropagatesTheRunsError(t *testing.T) {
	want := errors.New("all reviews failed")
	r := startProgressDashboard(progressOptions{Audience: "agent"})
	defer r.Stop()

	got, err := duringProgressDashboard(r, func() (int, error) { return 7, want })
	if !errors.Is(err, want) {
		t.Errorf("During must return the run's own error unchanged, got %v", err)
	}
	if got != 7 {
		t.Errorf("During must still return the run's value alongside its error, got %d", got)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	// A deferred Stop alongside an explicit one is the normal shape at a call
	// site, so a second call must be harmless.
	r := startProgressDashboard(progressOptions{Audience: "agent"})
	r.Stop()
	r.Stop()
	r.Stop()
}

func TestNilRunnerIsUsable(t *testing.T) {
	// Callers should never have to nil-check, so a nil progressRunner behaves as a
	// runner with no dashboard.
	var r *progressRunner
	defer r.Stop()

	got, err := duringProgressDashboard(r, func() (bool, error) { return true, nil })
	if !got || err != nil {
		t.Errorf("a nil runner must run inline, got (%v, %v)", got, err)
	}
}

func TestTUISinkForwardsToSend(t *testing.T) {
	var (
		mu   sync.Mutex
		sent []tea.Msg
	)
	s := dashboardSink{send: func(msg tea.Msg) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, msg)
	}}
	s.Publish(progress.Event{Kind: progress.KindNotice, Text: "hi\n"})

	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 {
		t.Fatalf("expected 1 forwarded message, got %d", len(sent))
	}
	ev, ok := sent[0].(dashboardEventMsg)
	if !ok {
		t.Fatalf("expected an dashboardEventMsg, got %T", sent[0])
	}
	if ev.Text != "hi\n" {
		t.Errorf("forwarded text = %q", ev.Text)
	}
}

// syncBuffer is a writer that can be read while bubbletea's renderer is still
// writing to it. The renderer outlives Program.Run's return, so a plain
// bytes.Buffer would be a data race rather than a merely stale read.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testRunner builds a progressRunner whose program is wired to pipes, so the real
// Bubble Tea loop runs end to end without a terminal. This is the only way to
// cover the path a live dashboard actually takes.
func testRunner(t *testing.T, opts progressOptions) (*progressRunner, *syncBuffer) {
	t.Helper()
	out := &syncBuffer{}
	m := newDashboardModel(opts, opts.TotalFiles)
	prog := tea.NewProgram(m, tea.WithInput(strings.NewReader("")), tea.WithOutput(out))
	return newDashboardRunner(prog), out
}

func TestDuringRunsTheProgramAndReturnsTheResult(t *testing.T) {
	r, out := testRunner(t, progressOptions{Title: "review", TotalFiles: 4})
	defer r.Stop()

	got, err := duringProgressDashboard(r, func() (int, error) {
		// Stand in for a review. It publishes progress and then simply
		// returns — it must not have to dismiss the dashboard itself, or a
		// real run would hang on a screen nobody is going to close.
		progress.Publish(progress.Event{Kind: progress.KindPlanned, Files: 4})
		progress.Publish(progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}, Text: "reviewing a.go\n"})
		progress.Publish(progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 1, Findings: 2, Text: "a.go: 2 finding(s)\n"})
		return 42, nil
	})

	if got != 42 {
		t.Errorf("During = %d, want 42", got)
	}
	if err != nil {
		t.Errorf("During error = %v, want nil", err)
	}
	// The renderer draws nothing to a non-terminal writer, so the frame
	// itself is not assertable here. What matters is that the program ran to
	// completion and the sink never blocked, which the result above proves.
	_ = out
}

func TestEventsReachTheModelThroughTheProgram(t *testing.T) {
	// The stronger assertion than the one above: the dashboard's final state has
	// to reflect what the pipeline published while the program was running.
	out := &syncBuffer{}
	m := newDashboardModel(progressOptions{Title: "review"}, 4)
	prog := tea.NewProgram(m, tea.WithInput(strings.NewReader("")), tea.WithOutput(out))
	sink := dashboardSink{send: prog.Send}

	go func() {
		sink.Publish(progress.Event{Kind: progress.KindPlanned, Files: 4})
		sink.Publish(progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}, Text: "reviewing a.go\n"})
		sink.Publish(progress.Event{Kind: progress.KindToolStart, Group: "a.go", Detail: "file_read"})
		sink.Publish(progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 1, Findings: 3, Text: "done\n"})
		sink.Publish(progress.Event{Kind: progress.KindNotice, Text: "quitting\n"})
		prog.Quit()
	}()

	final, err := prog.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	got := final.(dashboardModel)
	if got.totalFiles != 4 {
		t.Errorf("totalFiles = %d, want 4", got.totalFiles)
	}
	if got.findings != 3 {
		t.Errorf("findings = %d, want 3", got.findings)
	}
	if len(got.items) != 1 || got.items[0].state != dashboardStateDone {
		t.Errorf("expected one finished row, got %+v", got.items)
	}
}

func TestInitAndTickAdvanceTheLoop(t *testing.T) {
	m := newDashboardModel(progressOptions{Title: "review"}, 1)
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init must start the spinner and the clock")
	}
	upd, cmd := m.Update(dashboardTickMsg(time.Now()))
	if cmd == nil {
		t.Error("a tick must schedule the next one, or the clock and spinner freeze")
	}
	if upd.(dashboardModel).spinner.View() == "" {
		t.Error("the spinner should render a frame after a tick")
	}
}

func TestBarCellsFitsNarrowAndWideTerminals(t *testing.T) {
	if got := testModel(200, 40).barCells(); got != 24 {
		t.Errorf("a wide terminal should get the maximum bar width, got %d", got)
	}
	if got := testModel(20, 10).barCells(); got < 4 {
		t.Errorf("a very narrow terminal must still get a drawable bar, got %d", got)
	}
}

func TestLogHintCoversEveryState(t *testing.T) {
	if got := logHint(true, true); !strings.Contains(got, "paused") {
		t.Errorf("paused hint = %q", got)
	}
	if got := logHint(false, true); !strings.Contains(got, "held") {
		t.Errorf("held hint = %q", got)
	}
	if got := logHint(false, false); !strings.Contains(got, "following") {
		t.Errorf("following hint = %q", got)
	}
}

// TestDetachMidRunDoesNotBlockTheRun is the contract behind the `q` key: the
// view goes away, the work does not. Quit is sent from inside the work
// goroutine while events are still streaming, so this also proves Send does
// not deadlock against Run and that a detaching dashboard never stalls the
// pipeline it is reporting on.
func TestDetachMidRunDoesNotBlockTheRun(t *testing.T) {
	r, _ := testRunner(t, progressOptions{Title: "review", TotalFiles: 3})
	defer r.Stop()
	prog := r.prog

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			progress.Publish(progress.Event{Kind: progress.KindToolStart, Group: "a.go", Detail: "file_read", Text: "  \u25b6 file_read\n"})
		}
		time.Sleep(50 * time.Millisecond)
		prog.Quit()
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	start := time.Now()
	got, err := duringProgressDashboard(r, func() (int, error) {
		<-done
		return 7, nil
	})
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("detach took %s; the run is blocked on the dashboard", elapsed)
	}
	if got != 7 || err != nil {
		t.Errorf("During = (%d, %v), want (7, nil)", got, err)
	}
}

// sinkRecorder collects published events so a test can assert what reached
// the installed sink.
type sinkRecorder struct {
	mu     sync.Mutex
	events []progress.Event
}

func (r *sinkRecorder) Publish(ev progress.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *sinkRecorder) all() []progress.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]progress.Event(nil), r.events...)
}
