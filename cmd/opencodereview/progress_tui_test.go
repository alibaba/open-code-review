// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/alibaba/open-code-review/internal/progress"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// captureStdoutFor redirects stdout.Writer to w for the duration of the test.
// The progress text sink writes there rather than to os.Stdout directly, so
// this is how the tool's real stdout/stderr routing is asserted.
func captureStdoutFor(w io.Writer) func() {
	return stdout.Swap(w)
}

// stripDashboardANSI removes escape sequences so assertions can look at the text a user
// actually reads, rather than at the styling wrapped around it.
func stripDashboardANSI(s string) string {
	return ansi.Strip(s)
}

func testModel(width, height int) dashboardModel {
	m := newDashboardModel(progressOptions{Title: "review", TotalFiles: 10}, 10)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return upd.(dashboardModel)
}

func feed(m dashboardModel, evs ...progress.Event) dashboardModel {
	for _, ev := range evs {
		if ev.Time.IsZero() {
			ev.Time = time.Now()
		}
		upd, _ := m.Update(dashboardEventMsg{Event: ev})
		m = upd.(dashboardModel)
	}
	return m
}

func TestApplyCountsFilesAndFindingsSeparately(t *testing.T) {
	// The bar tracks files, the counter tracks findings, and conflating them
	// is the bug this guards: a group that found nine comments in three files
	// is three files of work, not nine.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go", "b.go", "c.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "g1", Files: 3, Findings: 9},
	)
	if got := m.finishedFiles(); got != 3 {
		t.Errorf("finishedFiles = %d, want 3 (the file count)", got)
	}
	if m.findings != 9 {
		t.Errorf("findings = %d, want 9", m.findings)
	}
	if m.fraction() != 0.3 {
		t.Errorf("fraction = %v, want 0.3", m.fraction())
	}
}

func TestApplyFallsBackToPathCountForFiles(t *testing.T) {
	// A group that forgets to report its file count must still advance the
	// bar; an undercount would leave it short of full on a finished run.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go", "b.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "g1", Findings: 1},
	)
	if got := m.finishedFiles(); got != 2 {
		t.Errorf("finishedFiles = %d, want 2 from the recorded paths", got)
	}
}

func TestApplyNeverCountsZeroFiles(t *testing.T) {
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "g1"},
		progress.Event{Kind: progress.KindGroupDone, Group: "g1"},
	)
	if got := m.finishedFiles(); got != 1 {
		t.Errorf("finishedFiles = %d, want 1: a dispatched group covers at least one file", got)
	}
}

func TestApplyTracksRoundOnRunningRow(t *testing.T) {
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindRoundStart, Group: "g1", Round: 2},
	)
	it := m.lookup("g1")
	if it == nil {
		t.Fatal("expected a work row for g1")
	}
	if it.round != 2 {
		t.Errorf("round = %d, want 2", it.round)
	}
}

func TestApplyAttributesToolCallToItsGroup(t *testing.T) {
	// This is the whole point of threading the task key through: a `▶
	// file_read` line from one of eight concurrent groups has to land on the
	// row that produced it.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupStart, Group: "b.go", Paths: []string{"b.go"}},
		progress.Event{Kind: progress.KindToolStart, Group: "b.go", Detail: `file_read "b.go"`},
	)
	if got := m.lookup("b.go").tool; got != `file_read "b.go"` {
		t.Errorf("b.go tool = %q, want the file_read line", got)
	}
	if got := m.lookup("a.go").tool; got != "" {
		t.Errorf("a.go must not pick up b.go's tool call, got %q", got)
	}
}

func TestApplyMarksFailedGroup(t *testing.T) {
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupFailed, Group: "g1", Files: 1, Err: "context deadline exceeded"},
	)
	it := m.lookup("g1")
	if it.state != dashboardStateFailed {
		t.Errorf("state = %v, want dashboardStateFailed", it.state)
	}
	if m.failedFiles != 1 {
		t.Errorf("failedFiles = %d, want 1", m.failedFiles)
	}
}

func TestApplyIgnoresEventsForUnknownGroup(t *testing.T) {
	// Rounds and tool calls can be published before or without a matching
	// start (a resumed run, a plan-phase line). They must not invent a row.
	m := feed(testModel(80, 24), progress.Event{Kind: progress.KindRoundStart, Group: "ghost", Round: 1})
	if len(m.items) != 0 {
		t.Errorf("expected no rows for an unknown group, got %d", len(m.items))
	}
}

func TestApplySkipsTextlessEventsInTheLog(t *testing.T) {
	// The coverage denominator is state, not a line to say; an empty log row
	// would just be noise competing for space.
	m := feed(testModel(80, 24), progress.Event{Kind: progress.KindPlanned, Files: 4})
	if m.totalFiles != 4 {
		t.Errorf("totalFiles = %d, want 4", m.totalFiles)
	}
	if len(m.log) != 0 {
		t.Errorf("a textless event must not enter the log, got %d entries", len(m.log))
	}
}

func TestApplyRetainsOnlyTheLastEntries(t *testing.T) {
	var evs []progress.Event
	for range dashboardLogRetained + 50 {
		evs = append(evs, progress.Event{Kind: progress.KindNotice, Text: "line"})
	}
	m := feed(testModel(80, 24), evs...)
	if len(m.log) != dashboardLogRetained {
		t.Errorf("log retained %d entries, want the cap of %d", len(m.log), dashboardLogRetained)
	}
}

func TestApplyBudgetReachedIsSticky(t *testing.T) {
	m := feed(testModel(80, 24), progress.Event{Kind: progress.KindBudgetReached, Text: "budget\n"})
	if !m.budgetHit {
		t.Fatal("budgetHit must be set so the status line can say the run stopped early")
	}
	if !strings.Contains(m.viewStats(), "token budget reached") {
		t.Error("the status line must surface budget exhaustion, not bury it in the log")
	}
}

func TestViewFitsTheWindow(t *testing.T) {
	// The layout is a fixed stack, so a frame that overflows would push the
	// keybindings off screen and scroll the whole dashboard.
	for _, size := range [][2]int{{80, 24}, {120, 40}, {60, 20}, {200, 60}, {40, 12}} {
		m := feed(testModel(size[0], size[1]),
			progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go"}},
			progress.Event{Kind: progress.KindGroupStart, Group: "g2", Paths: []string{"b.go", "c.go"}},
		)
		lines := strings.Split(m.View().Content, "\n")
		// JoinVertical leaves no trailing newline, so the last element is the
		// final line rather than an empty one.
		if len(lines) > size[1] {
			t.Errorf("%dx%d: frame is %d lines, overflows the window", size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w > size[0] {
				t.Errorf("%dx%d: line %d is %d cells wide, overflows", size[0], size[1], i, w)
			}
		}
	}
}

func TestViewShowsRunningWorkAndActivity(t *testing.T) {
	m := feed(testModel(100, 30),
		progress.Event{Kind: progress.KindGroupStart, Group: "api/handlers.go", Paths: []string{"api/handlers.go"}},
		progress.Event{Kind: progress.KindToolStart, Group: "api/handlers.go", Detail: `file_read "api/handlers.go"`, Text: "  ▶ file_read \"api/handlers.go\"\n"},
		progress.Event{Kind: progress.KindGroupDone, Group: "done.go", Files: 1, Findings: 2, Text: "done.go: 2 finding(s)\n"},
	)
	out := stripDashboardANSI(m.View().Content)
	for _, want := range []string{"api/handlers.go", "file_read", "activity", "work", "reviewing"} {
		if !strings.Contains(out, want) {
			t.Errorf("frame is missing %q:\n%s", want, out)
		}
	}
}

func TestViewHidesCompletedUntilExpanded(t *testing.T) {
	// A long scan finishes hundreds of files; the finished rows must not push
	// the running ones off the screen. They are folded away until asked for.
	m := feed(testModel(100, 30),
		progress.Event{Kind: progress.KindGroupStart, Group: "finished.go", Paths: []string{"finished.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "finished.go", Files: 1, Text: "done\n"},
		progress.Event{Kind: progress.KindGroupStart, Group: "running.go", Paths: []string{"running.go"}},
	)
	out := stripDashboardANSI(m.viewWork())
	if !strings.Contains(out, "running.go") {
		t.Errorf("the running row must always be visible:\n%s", out)
	}
	if strings.Contains(out, "finished.go") {
		t.Errorf("a finished row must stay folded away by default:\n%s", out)
	}

	m.showCompleted = true
	out = stripDashboardANSI(m.viewWork())
	if !strings.Contains(out, "finished.go") {
		t.Errorf("expanding must reveal the finished row:\n%s", out)
	}
	if !strings.Contains(out, "running.go") {
		t.Errorf("expanding must not hide the running row:\n%s", out)
	}
}

func TestWorkPaneScrolls(t *testing.T) {
	// More groups in flight than the region has rows: the list scrolls, and
	// says so at both ends rather than silently truncating.
	var evs []progress.Event
	for i := range 40 {
		g := fmt.Sprintf("g%02d.go", i)
		evs = append(evs, progress.Event{Kind: progress.KindGroupStart, Group: g, Paths: []string{g}})
	}
	m := feed(testModel(100, 30), evs...)

	top := stripDashboardANSI(m.viewWork())
	if !strings.Contains(top, "g00.go") {
		t.Errorf("an unscrolled list starts at the first row:\n%s", top)
	}

	m.scrollPane(1)
	scrolled := stripDashboardANSI(m.viewWork())
	if strings.Contains(scrolled, "g00.go") {
		t.Errorf("scrolling must move the window off the first row:\n%s", scrolled)
	}
	if !strings.Contains(scrolled, "more") {
		t.Errorf("a partial list must say there is more:\n%s", scrolled)
	}

	m.scrollPane(-100)
	if got := stripDashboardANSI(m.viewWork()); !strings.Contains(got, "g00.go") {
		t.Errorf("scrolling back must return to the first row:\n%s", got)
	}
}

func TestPaneFocusSwitches(t *testing.T) {
	m := testModel(80, 24)
	if m.focus != dashboardPaneWork {
		t.Fatalf("the work pane starts focused, got %v", m.focus)
	}
	upd, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = upd.(dashboardModel)
	if m.focus != dashboardPaneActivity {
		t.Error("tab must move focus to the activity pane")
	}
	// With the activity pane focused, scrolling must move the log, not the
	// work list.
	upd, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = upd.(dashboardModel)
	if m.logOffset != 0 || m.workTop != 0 {
		t.Errorf("scrolling forward from the tail should snap back, got log=%d work=%d", m.logOffset, m.workTop)
	}
	upd, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = upd.(dashboardModel)
	if m.logOffset == 0 {
		t.Error("with the activity pane focused, k must scroll the log")
	}
	if m.workTop != 0 {
		t.Error("and must not scroll the work pane")
	}
}

func TestCompletedToggleKey(t *testing.T) {
	m := testModel(80, 24)
	upd, _ := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = upd.(dashboardModel)
	if !m.showCompleted {
		t.Error("c must expand the completed section")
	}
	upd, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if upd.(dashboardModel).showCompleted {
		t.Error("c must collapse it again")
	}
}

func TestTokenColumnsAppear(t *testing.T) {
	// Header: cumulative in/out beside the model. Row: what the group spent.
	m := newDashboardModel(progressOptions{
		Title: "review", Model: "claude-opus-4-6", Provider: "anthropic", TotalFiles: 10,
	}, 10)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = upd.(dashboardModel)
	m.inputTokens, m.outputTokens = 3_800, 12_400
	m = feed(m,
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindUsage, Group: "a.go", InputTokens: 3800, OutputTokens: 1200},
	)
	head := stripDashboardANSI(m.viewHeader())
	if !strings.Contains(head, "3.8k") || !strings.Contains(head, "12k") {
		t.Errorf("the header must show abbreviated token totals, got %q", head)
	}
	if !strings.Contains(head, "▲") || !strings.Contains(head, "▼") {
		t.Errorf("the header must distinguish input from output with arrows, got %q", head)
	}
	row := stripDashboardANSI(m.viewWork())
	// The row marks its input and output the way the header does, so the two
	// can be read against each other.
	for _, want := range []string{"▲3.8k", "▼1.2k"} {
		if !strings.Contains(row, want) {
			t.Errorf("a running row must show what it has spent, got %q", row)
		}
	}
}

func TestViewSkipsFullWidthOutputWhenDetached(t *testing.T) {
	m := feed(testModel(80, 24), progress.Event{Kind: progress.KindNotice, Text: "x\n"})
	m.detached = true
	if got := m.View().Content; got != "" {
		t.Errorf("a detached dashboard must render nothing, got %q", got)
	}
}

func TestViewBarHandlesZeroAndFull(t *testing.T) {
	m := testModel(80, 24)
	if got := m.viewBarFill(); !strings.Contains(stripDashboardANSI(got), "░") {
		t.Errorf("an empty run should render an empty bar, got %q", stripDashboardANSI(got))
	}
	m = feed(m, progress.Event{Kind: progress.KindGroupDone, Group: "g", Files: 10, Text: "d\n"})
	if got := m.viewBarFill(); !strings.Contains(stripDashboardANSI(got), "█") {
		t.Errorf("a finished run should render a full bar, got %q", stripDashboardANSI(got))
	}
}

func TestKeysDetachAndScroll(t *testing.T) {
	m := testModel(80, 24)
	// The work pane is focused by default; move to the activity pane so the
	// arrow keys drive the log.
	upd, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = upd.(dashboardModel)

	upd, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = upd.(dashboardModel)
	if !m.holding() || m.logOffset == 0 {
		t.Error("scrolling up must hold the log away from the tail")
	}

	upd, _ = m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = upd.(dashboardModel)
	if m.holding() || m.logOffset != 0 {
		t.Error("pressing u must snap back to following the tail")
	}

	upd, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = upd.(dashboardModel)
	if !m.paused {
		t.Error("p must pause the log")
	}
	upd, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = upd.(dashboardModel)
	if m.paused {
		t.Error("p must toggle back")
	}
}

func TestKeyQDetachesRatherThanCancelling(t *testing.T) {
	// Quitting the view must not abandon a run that is already in flight on
	// other goroutines; it detaches and the text sink takes over.
	m := testModel(80, 24)
	upd, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = upd.(dashboardModel)
	if !m.detached {
		t.Error("q must detach the dashboard")
	}
	if cmd == nil {
		t.Error("q must return a command so the program exits")
	}
}

func TestScrollClampsAtTheTail(t *testing.T) {
	m := testModel(80, 24)
	m.scroll(1)
	if m.logOffset != 1 || !m.holding() {
		t.Errorf("scrolling up should hold the view, got offset %d holding %v", m.logOffset, m.holding())
	}
	m.scroll(-5)
	if m.logOffset != 0 || m.holding() {
		t.Errorf("scrolling past the tail must snap back, got offset %d holding %v", m.logOffset, m.holding())
	}
}

func TestLogWindowFollowsTheTail(t *testing.T) {
	var evs []progress.Event
	for i := range 20 {
		evs = append(evs, progress.Event{Kind: progress.KindNotice, Text: string(rune('a'+i%26)) + "\n"})
	}
	m := feed(testModel(80, 24), evs...)
	out := stripDashboardANSI(m.viewLog())
	if !strings.Contains(out, "t") {
		t.Errorf("with no scroll the newest entries must be visible:\n%s", out)
	}
}

func TestHelpers(t *testing.T) {
	if got := humanDuration(90); got != "1m30s" {
		t.Errorf("humanDuration(90s) = %q, want 1m30s", got)
	}
	if got := humanDuration(-1); got != "0s" {
		t.Errorf("humanDuration must not go negative, got %q", got)
	}
	if got := shortDuration(500 * time.Millisecond); got != "<1s" {
		t.Errorf("shortDuration(500ms) = %q", got)
	}
	if got := shortDuration(90 * time.Second); got != "1m30s" {
		t.Errorf("shortDuration(90s) = %q", got)
	}
	if got := identity("anthropic", "opus"); got != "anthropic/opus" {
		t.Errorf("identity = %q", got)
	}
	if got := identity("", "opus"); got != "opus" {
		t.Errorf("identity with no provider = %q", got)
	}
	if got := identity("anthropic", ""); got != "anthropic" {
		t.Errorf("identity with no model = %q", got)
	}
	if got := shortenPath("/a/b/c/d.go", 40); got != "/a/b/c/d.go" {
		t.Errorf("a short path must not be truncated, got %q", got)
	}
	if got := shortenPath("/a/b/c/d.go", 5); !strings.HasSuffix(got, "d.go") {
		t.Errorf("shortenPath must keep the identifying tail, got %q", got)
	}
	if got := clip("abcdef", 3); lipgloss.Width(got) != 3 {
		t.Errorf("clip must cut to the requested width, got %q (%d cells)", got, lipgloss.Width(got))
	}
	if got := clip("ab", 10); got != "ab" {
		t.Errorf("clip must leave short strings alone, got %q", got)
	}
}

func TestItemDisplay(t *testing.T) {
	if got := (&dashboardItem{paths: []string{"a.go"}}).display(); got != "a.go" {
		t.Errorf("single-file display = %q", got)
	}
	if got := (&dashboardItem{paths: []string{"a.go", "b.go", "c.go"}}).display(); got != "a.go (+2)" {
		t.Errorf("grouped display = %q", got)
	}
	if got := (&dashboardItem{label: "budget plumbing"}).display(); got != "budget plumbing" {
		t.Errorf("a row with no paths falls back to its label, got %q", got)
	}
}

func TestStartItemIsIdempotentPerGroup(t *testing.T) {
	// A resumed run re-announces groups it is picking back up. Re-announcing
	// must refresh the row's clock rather than stack a duplicate.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupStart, Group: "g1", Paths: []string{"a.go"}},
	)
	if len(m.items) != 1 {
		t.Errorf("expected 1 row after a repeated start, got %d", len(m.items))
	}
}

func TestFractionGuardsAgainstAnUnknownTotal(t *testing.T) {
	m := newDashboardModel(progressOptions{Title: "review"}, 0)
	if got := m.fraction(); got != 0 {
		t.Errorf("fraction with no denominator = %v, want 0 (not a division by zero)", got)
	}
}

func TestRemainingKeys(t *testing.T) {
	m := testModel(80, 24)
	for _, k := range []string{"esc", "j", "home", "z"} {
		upd, _ := m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		next := upd.(dashboardModel)
		if next.detached && k != "esc" {
			t.Errorf("key %q must not detach; only q and esc do", k)
		}
		m = next
		m.detached = false
	}
}

func TestStatsAndItemRenderEveryState(t *testing.T) {
	m := feed(testModel(100, 30),
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindSkipped, Text: "Skipping b.png — binary file\n"},
		progress.Event{Kind: progress.KindGroupStart, Group: "b.go", Paths: []string{"b.go"}},
		progress.Event{Kind: progress.KindGroupFailed, Group: "b.go", Files: 1, Err: "context deadline exceeded"},
	)
	stats := stripDashboardANSI(m.viewStats())
	if !strings.Contains(stats, "skipped") || !strings.Contains(stats, "failed") {
		t.Errorf("the status row must surface skips and failures, got %q", stats)
	}
	m.showCompleted = true
	out := stripDashboardANSI(m.viewWork())
	if !strings.Contains(out, "context deadline exceeded") {
		t.Errorf("a failed row must show why it failed, got %q", out)
	}
}

func TestItemNoteCoversRoundToolAndNeither(t *testing.T) {
	m := testModel(80, 24)
	running := &dashboardItem{round: 2, tool: "file_read"}
	if got := stripDashboardANSI(m.itemNote(running)); !strings.Contains(got, "round 2") || !strings.Contains(got, "file_read") {
		t.Errorf("a running row shows its round and tool, got %q", got)
	}
	bare := &dashboardItem{}
	if got := stripDashboardANSI(m.itemNote(bare)); !strings.Contains(got, "starting") {
		t.Errorf("a row with no round and no tool must say it is starting, got %q", got)
	}
}

func TestSectionHeadingDropsTheHintWhenTooNarrow(t *testing.T) {
	// A hint that cannot fit beside the heading is dropped rather than wrapped
	// onto a second line, which would break the fixed-height region layout.
	m := testModel(minFrameWidth, 24)
	if got := stripDashboardANSI(m.sectionHeading("work", 300, "3 running, 1 failed, 1 skipped")); strings.Contains(got, "running") {
		t.Errorf("a hint that does not fit must be dropped, not wrapped, got %q", got)
	}
	if got := stripDashboardANSI(m.sectionHeading("work", 3, "3 running")); !strings.Contains(got, "running") {
		t.Errorf("a hint that fits must be shown, got %q", got)
	}
	if got := stripDashboardANSI(m.sectionHeading("work", 0, "")); got != "work" {
		t.Errorf("a zero count should omit the parenthetical, got %q", got)
	}
}

func TestViewStatsShowsPaused(t *testing.T) {
	m := testModel(80, 24)
	m.paused = true
	if got := stripDashboardANSI(m.viewStats()); !strings.Contains(got, "paused") {
		t.Errorf("a paused log must be visible in the status row, got %q", got)
	}
}

func TestShortenPathHandlesDegenerateWidths(t *testing.T) {
	if got := shortenPath("/a/b.go", 1); got != "o" {
		t.Errorf("shortenPath(1) = %q, want the last character", got)
	}
	if got := shortenPath("/a/b.go", 0); got != "/a/b.go" {
		t.Errorf("shortenPath(0) must not truncate, got %q", got)
	}
}

// logBody is the log pane's rows, without the heading.
//
// The scrollbar column is dropped as well: it says how much of the feed is
// left, which changes when the feed grows even while the window is held, and
// the question this answers is whether the text the reader is looking at
// moved.
func logBody(m dashboardModel) string {
	lines := strings.Split(stripDashboardANSI(m.viewLog()), "\n")
	rows := make([]string, 0, len(lines)-1)
	for _, l := range lines[1:] {
		rows = append(rows, trimScrollbarColumn(l))
	}
	return strings.Join(rows, "\n")
}

// trimScrollbarColumn drops the scrollbar glyph from the end of a row.
func trimScrollbarColumn(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	switch r[len(r)-1] {
	case '┃', '│':
		return string(r[:len(r)-1])
	}
	return s
}

func TestPauseFreezesTheLogWindow(t *testing.T) {
	// Pausing has to actually hold the window. Keeping the label change but
	// letting new lines drag the view is the failure this guards: the reader
	// is trying to read, and the line they are on moves.
	var evs []progress.Event
	for i := range 10 {
		evs = append(evs, progress.Event{Kind: progress.KindNotice, Text: string(rune('a'+i)) + "\n"})
	}
	m := feed(testModel(80, 24), evs...)

	upd, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = upd.(dashboardModel)
	if !m.paused {
		t.Fatal("p must pause")
	}

	// The window the reader is looking at, before and after more activity. The
	// heading is excluded on purpose: its entry count is meant to keep
	// climbing so the reader can see how much they have missed.
	before := logBody(m)
	m = feed(m, progress.Event{Kind: progress.KindNotice, Text: "later\n"})
	if after := logBody(m); after != before {
		t.Errorf("a paused log must not repaint as new lines arrive:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// Nothing is lost: the entry was still recorded, and the heading count
	// reflects it, so releasing the pause brings it into view.
	if m.pending == 0 {
		t.Error("a line recorded while paused must be counted as pending")
	}
	upd, _ = m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = upd.(dashboardModel)
	if got := stripDashboardANSI(m.viewLog()); !strings.Contains(got, "later") {
		t.Errorf("resuming must show what arrived while paused, got:\n%s", got)
	}
}

func TestTextlessStructuralEventsStayOutOfTheLog(t *testing.T) {
	// The structural kinds carry state for rows and counters. They must not
	// print a line, because each already has a Noticef beside it that says
	// the same thing — printing both would add a duplicate line to every
	// piped and CI run.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindPlanned, Files: 4},
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindRoundStart, Group: "a.go", Round: 1},
		progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 1, Findings: 2},
	)
	if len(m.log) != 0 {
		t.Errorf("structural events must not print lines, log has %d entries: %+v", len(m.log), m.log)
	}
	if m.totalFiles != 4 || m.findings != 2 || m.finishedFiles() != 1 {
		t.Errorf("structural events must still update state: total=%d findings=%d done=%d",
			m.totalFiles, m.findings, m.finishedFiles())
	}
}

func TestColorOffRendersPlainly(t *testing.T) {
	// --color=never must reach the dashboard: it is the one surface that
	// would otherwise paint color on its own initiative, and an explicit
	// opt-out the user typed has to be honoured there too.
	m := testModel(100, 30)
	m.color = false
	m.showCompleted = true
	m = feed(m,
		progress.Event{Kind: progress.KindNotice, Text: "plain\n"},
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupFailed, Group: "a.go", Files: 1, Err: "boom", Text: "boom\n"},
	)
	out := m.View().Content
	if strings.Contains(out, "\x1b[") {
		t.Errorf("color is off but the frame carries ANSI escapes:\n%q", out)
	}
	// It must still be a usable dashboard, not an empty screen.
	if !strings.Contains(ansi.Strip(out), "a.go") {
		t.Errorf("the plain frame lost its content:\n%s", ansi.Strip(out))
	}

	m.color = true
	if got := m.View().Content; !strings.Contains(got, "\x1b[") {
		t.Error("color on but the frame has no ANSI escapes")
	}
}

func TestCtrlCHandsTheRequestBackToTheRun(t *testing.T) {
	// Raw mode clears ISIG, so no SIGINT reaches the run's own handler while
	// the dashboard is up. The dashboard has to be the thing that turns
	// ctrl+c back into a cancellation, or the key silently does nothing.
	var cancelled bool
	m := testModel(80, 24)
	m.cancel = func() { cancelled = true }

	upd, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	next := upd.(dashboardModel)
	if !cancelled {
		t.Error("ctrl+c must cancel the run")
	}
	// The view stays up so the shutdown is visible rather than instantaneous.
	if next.detached {
		t.Error("ctrl+c must not detach; the run decides when it is over")
	}
}

func TestCtrlCWithoutAHandlerIsHarmless(t *testing.T) {
	m := testModel(80, 24)
	upd, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if upd.(dashboardModel).detached {
		t.Error("with no cancel handler there is nothing to do and nothing to detach from")
	}
}

func TestZeroSizedWindowStillRenders(t *testing.T) {
	// A pty with no size yet, or the moment between a window closing and the
	// resize that follows, reports 0. Every region clips to the frame width,
	// so taking that at face value blanks the whole dashboard.
	upd, _ := newDashboardModel(progressOptions{Title: "review"}, 4).Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m := upd.(dashboardModel)
	if m.width <= 0 || m.height <= 0 {
		t.Fatalf("window clamped to %dx%d, want a usable size", m.width, m.height)
	}
	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}, Text: "reviewing a.go\n"})
	if got := strings.TrimSpace(stripDashboardANSI(m.View().Content)); got == "" {
		t.Error("a zero-sized window must still render a usable frame")
	}
}

func TestReusedFilesBringTheBarToFull(t *testing.T) {
	// The denominator is every selected file, but a resumed run only
	// dispatches what is left. Without counting the reused ones, a resume
	// with three files left to do would sit at 3/17 for the whole run and
	// never look finished.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindPlanned, Files: 17},
		progress.Event{Kind: progress.KindReused, Files: 14},
	)
	if m.finishedFiles() != 14 {
		t.Fatalf("finishedFiles = %d, want 14", m.finishedFiles())
	}

	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 1})
	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "b.go", Paths: []string{"b.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "b.go", Files: 1})
	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "c.go", Paths: []string{"c.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "c.go", Files: 1})

	if got := m.fraction(); got != 1.0 {
		t.Errorf("fraction = %v, want 1.0 — the bar must be able to reach full on a resume", got)
	}
}

func TestReusedFilesGetNoWorkRow(t *testing.T) {
	// Hundreds of carried-over files must not bury the handful actually
	// being reviewed.
	m := feed(testModel(80, 24),
		progress.Event{Kind: progress.KindPlanned, Files: 300},
		progress.Event{Kind: progress.KindReused, Files: 297},
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
	)
	if len(m.items) != 1 {
		t.Errorf("reused files must not create rows, got %d", len(m.items))
	}
	if !strings.Contains(stripDashboardANSI(m.viewWork()), "a.go") {
		t.Error("the files actually being reviewed must still be listed")
	}
}

// TestResumeArithmeticMatchesASelectionRun pins the arithmetic of a real
// resumed scan: 759 files discovered, 272 skipped during selection, 487
// selected for review, 48 of those already done in a previous run.
func TestResumeArithmeticMatchesASelectionRun(t *testing.T) {
	const (
		discovered = 759
		skipped    = 272
		selected   = discovered - skipped // 487
		reused     = 48
	)
	if selected != 487 {
		t.Fatalf("fixture is wrong: selected = %d", selected)
	}

	m := testModel(110, 30)
	// Selection runs first and reports its skips...
	for range skipped {
		m = feed(m, progress.Event{Kind: progress.KindSkipped, Detail: "some/file.ts"})
	}
	// ...then the run announces the denominator, which already excludes them.
	m = feed(m, progress.Event{Kind: progress.KindPlanned, Files: selected})
	// ...then resume reports the carried-over work.
	m = feed(m, progress.Event{Kind: progress.KindReused, Files: reused})

	if got := m.finishedFiles(); got != reused {
		t.Errorf("finishedFiles = %d, want %d — skipped files are not in the bar", got, reused)
	}
	if got := m.skippedFiles; got != skipped {
		t.Errorf("skippedFiles = %d, want %d — the count must still be reported", got, skipped)
	}
	// The bar must open at the reuse count, not at reuse+skipped: the
	// denominator never contained the skipped files.
	if got := m.fraction(); got != float64(reused)/float64(selected) {
		t.Errorf("fraction = %v, want %v", got, float64(reused)/float64(selected))
	}
	if strings.Contains(m.View().Content, "487/487") {
		t.Error("the bar must not open already full")
	}

	stats := stripDashboardANSI(m.viewStats())
	if !strings.Contains(stats, "272 skipped") {
		t.Errorf("the status row must still report the skipped count, got %q", stats)
	}
}

func TestReusedFindingsCountTowardsTheTotal(t *testing.T) {
	// Findings a resumed session already produced are real findings: the run
	// collects them and reports them in its final report. Leaving them out of
	// the live count makes the dashboard show 0 findings while it is actively
	// going to report dozens.
	m := feed(testModel(110, 30),
		progress.Event{Kind: progress.KindPlanned, Files: 487},
		progress.Event{Kind: progress.KindReused, Files: 48, Findings: 137},
	)
	if m.findings != 137 {
		t.Errorf("findings = %d, want 137 from the resumed session", m.findings)
	}
	if m.reusedFiles != 48 {
		t.Errorf("reusedFiles = %d, want 48", m.reusedFiles)
	}
	stats := stripDashboardANSI(m.viewStats())
	if !strings.Contains(stats, "137 findings") {
		t.Errorf("the status row must include reused findings, got %q", stats)
	}
	if !strings.Contains(stats, "48 reused") {
		t.Errorf("the status row must distinguish reused files, got %q", stats)
	}
}

func TestBarSegmentsReusedFromDone(t *testing.T) {
	// A resumed run that has since reviewed more files must show both
	// segments, so "how much was inherited" is readable off the bar itself.
	m := feed(testModel(110, 30),
		progress.Event{Kind: progress.KindPlanned, Files: 100},
		progress.Event{Kind: progress.KindReused, Files: 50},
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 25},
	)
	if got := m.finishedFiles(); got != 75 {
		t.Errorf("finishedFiles = %d, want 75 (50 reused + 25 done)", got)
	}
	if got := m.fraction(); got != 0.75 {
		t.Errorf("fraction = %v, want 0.75", got)
	}
	legend := stripDashboardANSI(m.viewBarLegend())
	if !strings.Contains(legend, "reused") || !strings.Contains(legend, "done") {
		t.Errorf("the legend must name both non-empty segments, got %q", legend)
	}
}

func TestBarLegendOmitsEmptySegments(t *testing.T) {
	// A run that reused nothing should not advertise a reused segment.
	m := feed(testModel(110, 30), progress.Event{Kind: progress.KindPlanned, Files: 10})
	if got := stripDashboardANSI(m.viewBarLegend()); got != "" {
		t.Errorf("legend = %q, want empty when nothing has been reused or failed", got)
	}
}

func TestDashboardSanitizesUntrustedStrings(t *testing.T) {
	// A file name that carries an escape sequence must not be able to paint
	// over the dashboard or hide the text around it.
	m := newDashboardModel(progressOptions{Title: "review"}, 1)
	m = feed(m, progress.Event{
		Kind:  progress.KindGroupStart,
		Group: "a.go",
		Paths: []string{"a\x1b[31m.go"},
		Text:  "reviewing a\x1b[31m.go\n",
	})
	it := m.lookup("a.go")
	if it == nil {
		t.Fatal("the group must still be listed")
	}
	if strings.ContainsRune(it.display(), '\x1b') {
		t.Errorf("a file name must not carry an escape sequence into the view: %q", it.display())
	}
	if len(m.log) == 0 {
		t.Fatal("the log must still record the line")
	}
	if strings.ContainsRune(m.log[0].text, '\x1b') {
		t.Errorf("the log must not carry an escape sequence either: %q", m.log[0].text)
	}
}

func TestDashboardRefusesATooSmallTerminal(t *testing.T) {
	// Below the floor the frame cannot be laid out, so the dashboard says so
	// rather than drawing something that does not fit.
	upd, _ := newDashboardModel(progressOptions{Title: "review"}, 1).
		Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	m := upd.(dashboardModel)
	got := stripDashboardANSI(m.View().Content)
	if !strings.Contains(got, "too small") {
		t.Errorf("a terminal below the floor must be told to enlarge, got %q", got)
	}
	if !strings.Contains(got, "ctrl+c to stop") {
		t.Errorf("a terminal below the floor must say how to stop, got %q", got)
	}
}

func TestClipMarksTruncation(t *testing.T) {
	// A cut row has to say that it continues, or a reader cannot tell a short
	// row from a clipped one.
	got := clip("abcdefgh", 5)
	if lipgloss.Width(got) != 5 {
		t.Errorf("clip must cut to the requested width, got %q (%d cells)", got, lipgloss.Width(got))
	}
	if !strings.Contains(got, "\u2026") {
		t.Errorf("a truncated row must show an ellipsis, got %q", got)
	}
}

// A row names the tool it called last. The name stays after the call ends —
// clearing it left the row reading "starting…" while the activity log moved
// on — and the row says whether the call is still in flight.
func TestRowKeepsTheToolAfterTheCallEnds(t *testing.T) {
	m := testModel(100, 30)
	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}})
	m = feed(m, progress.Event{Kind: progress.KindToolStart, Group: "a.go", Detail: "file_read"})

	if note := stripDashboardANSI(m.itemNote(m.lookup("a.go"))); !strings.Contains(note, "file_read") {
		t.Fatalf("the row must name the tool it called, got %q", note)
	}

	m = feed(m, progress.Event{Kind: progress.KindToolDone, Group: "a.go"})
	note := stripDashboardANSI(m.itemNote(m.lookup("a.go")))
	if !strings.Contains(note, "file_read") {
		t.Errorf("a finished call must keep its name, got %q", note)
	}
	if m.lookup("a.go").toolRunning {
		t.Error("a finished call must not be marked as running")
	}
}

// A tool that failed is not running either, so the row stops marking it as
// running — but keeps the name, which is still the last thing it did.
func TestRowMarksAFailedCallAsNotRunning(t *testing.T) {
	m := testModel(100, 30)
	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}})
	m = feed(m, progress.Event{Kind: progress.KindToolStart, Group: "a.go", Detail: "file_read"})
	m = feed(m, progress.Event{Kind: progress.KindToolError, Group: "a.go", Err: "permission denied"})

	note := stripDashboardANSI(m.itemNote(m.lookup("a.go")))
	if !strings.Contains(note, "file_read") {
		t.Errorf("a failed call keeps the name of what it last did, got %q", note)
	}
	if m.lookup("a.go").toolRunning {
		t.Error("a failed call must not be marked as running")
	}
}

// The bar shares its row with the label, the counter and the legend, and the
// counter grows with the file count. A bar that assumes a fixed counter width
// takes cells the counter needs, and the legend — which sits last — is what
// gets cut off the edge of the frame.
func TestBarRowKeepsTheLegend(t *testing.T) {
	for _, files := range []int{7, 999, 12345} {
		m := testModel(40, 12)
		m.totalFiles = files
		m.doneFiles = files
		row := stripDashboardANSI(m.viewBar())
		if !strings.Contains(row, "▇ done") {
			t.Errorf("with %d files the legend is cut off: %q", files, row)
		}
	}
}

// The three segments are measured against what is left of the bar, so they
// cannot add up to more than the bar has — whatever the counters say.
func TestBarSegmentsNeverOverflowTheBar(t *testing.T) {
	m := testModel(100, 12)
	m.totalFiles = 100
	m.reusedFiles = 60
	m.doneFiles = 60

	fill := m.viewBarFill()
	if got, want := lipgloss.Width(fill), m.barCells()+2; got > want {
		t.Errorf("the bar is %d cells wide, want at most %d", got, want)
	}
}

// One log entry is one message. An entry whose text spans lines is folded
// into one first, and the frame then lays it out over as many lines as it
// needs to show the whole thing.
func TestLogEntryFoldsItsText(t *testing.T) {
	m := testModel(80, 24)
	m = feed(m, progress.Event{Kind: progress.KindNotice, Text: "first line\nsecond line\n"})

	joined := strings.Join(m.wrapLogEntry(m.log[0]), " ")
	if strings.Contains(joined, "first line\nsecond line") {
		t.Errorf("a log entry must fold its text onto one line, got %q", joined)
	}
	if !strings.Contains(joined, "first line second line") {
		t.Errorf("the entry must keep its text, got %q", joined)
	}
}

// The frame takes exactly the height it was given. A frame that came out
// taller pushed the help line off the bottom; one that came out shorter left
// the terminal half empty and moved the log as work came and went.
func TestViewFillsTheTerminalHeight(t *testing.T) {
	for _, h := range []int{24, 30, 45, 60} {
		m := testModel(120, h)
		got := strings.Count(m.View().Content, "\n") + 1
		if got != h {
			t.Errorf("at height %d the frame is %d lines", h, got)
		}
	}
}

// A failure must be visible in the bar. A share small enough to round away
// left the bar looking clean and finished while the count beside it said
// files had failed.
func TestBarAlwaysShowsAFailure(t *testing.T) {
	m := testModel(120, 30)
	m = feed(m,
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 99},
		progress.Event{Kind: progress.KindGroupFailed, Group: "b.go", Paths: []string{"b.go"}, Files: 1},
	)
	m.totalFiles = 100
	m.doneFiles = 99
	m.failedFiles = 1

	reused, done, failed := m.barSegments(24)
	if failed != 1 {
		t.Errorf("a failure must take a cell of the bar, got %d", failed)
	}
	if reused != 0 {
		t.Errorf("nothing was reused, got %d cells", reused)
	}
	if reused+done+failed != 24 {
		t.Errorf("the bar must be full at the end of the run, got %d of 24 cells", reused+done+failed)
	}
}

// The bar is stacked: the three segments end where the next one starts, so
// they fill the width exactly. Measured separately and truncated, the bar came
// out a cell short whenever the counts did not divide the width exactly, and
// it never quite filled however far the run got.
func TestBarSegmentsFillTheWidthExactly(t *testing.T) {
	m := testModel(120, 30)
	m.totalFiles = 100
	for _, done := range []int{0, 1, 37, 50, 99, 100} {
		m.doneFiles = done
		m.failedFiles = 0
		reused, gotDone, failed := m.barSegments(24)
		if reused+gotDone+failed > 24 {
			t.Errorf("at %d done the bar is %d cells, over the width", done, reused+gotDone+failed)
		}
	}
	m.doneFiles = 100
	m.failedFiles = 0
	reused, done, failed := m.barSegments(24)
	if reused+done+failed != 24 {
		t.Errorf("a finished run must fill the bar, got %d of 24", reused+done+failed)
	}
}

// The estimate has to stay on screen. The log says it once, and the estimate
// is the fact a reader keeps in view while the run spends it: the header
// carries what has been spent so far, and the plan line carries what the run
// is expected to spend.
func TestPlanLineCarriesTheEstimate(t *testing.T) {
	m := testModel(120, 30)
	m = feed(m, progress.Event{
		Kind:  progress.KindPlanned,
		Files: 98,
		Plan: &progress.Plan{
			InputTokens:  3_000_000,
			OutputTokens: 527_000,
			Strategy:     "by-language",
			Groups:       5,
		},
	})

	line := stripDashboardANSI(m.viewPlan())
	for _, want := range []string{"3M", "527k", "3.5M", "by language", "5 batches"} {
		if !strings.Contains(line, want) {
			t.Errorf("the plan line must carry %q, got %q", want, line)
		}
	}
	if !strings.Contains(m.View().Content, "3.5M") {
		t.Error("the estimate must be in the frame, not only in the log")
	}
}

// A run that does not group its work has no plan to show beyond the cost, so
// the line must not claim it has batches.
func TestPlanLineWithoutBatches(t *testing.T) {
	m := testModel(120, 30)
	m = feed(m, progress.Event{
		Kind:  progress.KindPlanned,
		Files: 4,
		Plan:  &progress.Plan{InputTokens: 1000, OutputTokens: 200},
	})

	line := stripDashboardANSI(m.viewPlan())
	if strings.Contains(line, "batch") {
		t.Errorf("a run with no batches must not claim any, got %q", line)
	}
}

// A row that does not fit is wrapped, not cut. A truncated note says nothing,
// and the note is the part of the row that says what is happening.
func TestARowWrapsInsteadOfBeingCut(t *testing.T) {
	m := testModel(80, 30)
	m = feed(m, progress.Event{
		Kind:  progress.KindGroupStart,
		Group: "a.go",
		Paths: []string{"a.go"},
	})
	m = feed(m, progress.Event{Kind: progress.KindToolStart, Group: "a.go",
		Detail: "file_read src/app/index.css with an argument long enough to run past the note column"})

	rows := m.viewItem(m.lookup("a.go"), " ", m.nameWidth(m.items))
	if len(rows) < 2 {
		t.Fatalf("a note that does not fit must wrap onto a second line, got %d line(s)", len(rows))
	}
	joined := stripDashboardANSI(strings.Join(rows, " "))
	if !strings.Contains(joined, "src/app/index.css") {
		t.Errorf("the whole note must be shown, got %q", joined)
	}
	for i, r := range rows {
		if w := lipgloss.Width(r); w > m.width {
			t.Errorf("row %d is %d cells wide, over the frame", i, w)
		}
	}
}

// The frame's height is fixed, and a wrapped row takes more than one line, so
// the work region has to give up a row rather than push the help line off the
// bottom of the screen.
func TestAWrappedRowDoesNotOverflowTheFrame(t *testing.T) {
	for _, h := range []int{24, 30, 45} {
		m := testModel(80, h)
		m = feed(m, progress.Event{
			Kind:  progress.KindGroupStart,
			Group: "a.go",
			Paths: []string{"a.go"},
		})
		m = feed(m, progress.Event{Kind: progress.KindToolStart, Group: "a.go", Detail: "file_read src/app/index.css"})
		got := strings.Count(m.View().Content, "\n") + 1
		if got != h {
			t.Errorf("at height %d the frame is %d lines", h, got)
		}
	}
}

// The feed can be folded away, and the work list takes the height it was
// using. The frame still has to fit.
func TestTheLogCanBeFoldedAway(t *testing.T) {
	m := testModel(100, 30)
	m = feed(m,
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindNotice, Text: "something happened\n"},
	)

	before := m.workHeight()
	upd, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = upd.(dashboardModel)
	if m.showLog {
		t.Fatal("l must fold the feed away")
	}
	if m.workHeight() <= before {
		t.Errorf("the work list must take the feed's height: %d rows, was %d", m.workHeight(), before)
	}
	if got := strings.Count(m.View().Content, "\n") + 1; got != 30 {
		t.Errorf("the frame is %d lines, want 30", got)
	}
	if !strings.Contains(m.View().Content, "press l to expand") {
		t.Error("the folded feed must say how to bring it back")
	}

	// And it comes back as it was.
	upd, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = upd.(dashboardModel)
	if !m.showLog {
		t.Fatal("l must bring the feed back")
	}
	if got := strings.Count(m.View().Content, "\n") + 1; got != 30 {
		t.Errorf("the frame is %d lines, want 30", got)
	}
}

// A row says which batch it belongs to, so a reader can tell where in the
// scan the work stands.
// The work heading counts what is being worked on now. It used to count every
// row the run had ever opened, which sat next to a status row saying how many
// files were in flight and read as a count of the files behind them.
func TestTheWorkHeadingCountsWhatIsBeingWorkedOn(t *testing.T) {
	m := testModel(120, 30)
	m = feed(m,
		progress.Event{Kind: progress.KindGroupStart, Group: "a.go", Paths: []string{"a.go"}},
		progress.Event{Kind: progress.KindGroupStart, Group: "b.go", Paths: []string{"b.go"}},
		progress.Event{Kind: progress.KindGroupDone, Group: "a.go", Files: 1},
		progress.Event{Kind: progress.KindGroupFailed, Group: "b.go", Files: 1},
	)
	m = feed(m, progress.Event{Kind: progress.KindGroupStart, Group: "c.go", Paths: []string{"c.go"}})

	heading := stripDashboardANSI(strings.Split(m.viewWork(), "\n")[0])
	if !strings.Contains(heading, "work (1)") {
		t.Errorf("the heading must count the file being worked on, got %q", heading)
	}
	if strings.Contains(heading, "work (3)") {
		t.Errorf("the heading must not count the rows already finished, got %q", heading)
	}
	if !strings.Contains(heading, "2 finished") {
		t.Errorf("the heading must say how many are finished, got %q", heading)
	}
}

func TestARowSaysWhichBatchItIs(t *testing.T) {
	m := testModel(120, 30)
	m = feed(m, progress.Event{
		Kind:  progress.KindGroupStart,
		Group: "a.go",
		Paths: []string{"a.go"},
		Plan:  &progress.Plan{Batch: "2 of 5"},
	})

	row := stripDashboardANSI(strings.Join(m.viewItem(m.lookup("a.go"), " ", m.nameWidth(m.items)), " "))
	if !strings.Contains(row, "batch 2 of 5") {
		t.Errorf("the row must name its batch, got %q", row)
	}
}
