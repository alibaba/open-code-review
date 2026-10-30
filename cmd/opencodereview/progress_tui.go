// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/alibaba/open-code-review/internal/progress"
)

const (
	// dashboardLogRetained is how many log entries the dashboard keeps. The log is a
	// diagnostic surface, not a transcript: a long run produces thousands of
	// tool events, and holding them all would grow without bound for no gain.
	dashboardLogRetained = 500

	// dashboardTickInterval drives the spinner and the elapsed clock. It matches the
	// refresh rate closely enough that the two never look out of step.
	dashboardTickInterval = 100 * time.Millisecond
)

// dashboardItemState is where a unit of concurrent work has got to.
type dashboardItemState int

const (
	dashboardStateRunning dashboardItemState = iota
	dashboardStateDone
	dashboardStateFailed
)

// dashboardPane identifies which region the arrow keys act on.
type dashboardPane int

const (
	dashboardPaneWork dashboardPane = iota
	dashboardPaneActivity
)

// dashboardItem is one row of the work list. A run dispatches groups of semantically
// related files concurrently, so "which file is it on" is really "which
// groups are in flight and what is each one doing" — one row per group answers
// it, with the group's files behind it.
type dashboardItem struct {
	label string
	paths []string
	state dashboardItemState
	round int
	tool  string
	// toolRunning says whether the tool named above is still in flight. The
	// name stays after the call ends — it is the last thing the row did — and
	// this is what tells a running call from a finished one.
	toolRunning bool
	started     time.Time
	took        time.Duration
	findings    int
	err         string
	// batch is the group of files this row belongs to, as "2 of 5". It is
	// empty when the run does not group its work.
	batch string
	// inputTokens and outputTokens are what this unit of work has spent,
	// republished as cumulative totals after every LLM response so the row is
	// live rather than final. They are kept apart because the header shows
	// the run's input and output the same way, and a row that carried their
	// sum under the input arrow read as more than the whole run had spent.
	inputTokens  int64
	outputTokens int64
}

// display is the short form of the row's subject: the first file, which is what
// a person scanning the list is looking for.
func (it *dashboardItem) display() string {
	if len(it.paths) == 0 {
		return it.label
	}
	if len(it.paths) == 1 {
		return it.paths[0]
	}
	return fmt.Sprintf("%s (+%d)", it.paths[0], len(it.paths)-1)
}

// dashboardLogEntry is one line of the activity feed, stamped so a slow LLM round is
// visibly slow rather than merely absent.
type dashboardLogEntry struct {
	// stamp is the pre-formatted clock reading. Formatting it once here keeps
	// the render path from re-laying out the same timestamp on every frame.
	stamp string
	text  string
	kind  progress.Kind
	// count is how many events this row stands for. It is 1 for an ordinary
	// line and greater when a run of identically-reasoned skips has been
	// collapsed into this row.
	count int
	// reason is the skip reason a collapsed row aggregates.
	reason string
}

// dashboardTickMsg drives the spinner and the clock.
type dashboardTickMsg time.Time

// logWrapCache holds the wrapped lines of the log entries, and the width they
// were wrapped at.
type logWrapCache struct {
	width int
	lines [][]string
	// total is how many lines all the entries take. The scrollbar is drawn
	// from it, so it counts lines rather than entries: an entry whose text
	// wraps takes more than one line, and a bar that counted entries would
	// claim the feed is longer than the screen can show.
	total int
}

// logLines returns the wrapped lines of the log entries, rebuilding them when
// the width changed or the cache fell out of step with the log.
func (m dashboardModel) logLines() [][]string {
	if m.logCache.width != m.width || len(m.logCache.lines) != len(m.log) {
		m.logCache.width = m.width
		m.logCache.lines = make([][]string, len(m.log))
		m.logCache.total = 0
		for i, e := range m.log {
			m.logCache.lines[i] = m.wrapLogEntry(e)
			m.logCache.total += len(m.logCache.lines[i])
		}
	}
	return m.logCache.lines
}

// logLinesBefore is how many lines of the feed sit above the given entry.
func (m dashboardModel) logLinesBefore(entry int) int {
	lines := m.logLines()
	if entry > len(lines) {
		entry = len(lines)
	}
	n := 0
	for i := 0; i < entry; i++ {
		n += len(lines[i])
	}
	return n
}

// dashboardEventMsg carries a published progress.Event into the Bubble Tea update loop. The
// pipeline's goroutines send these from outside; Program.Send is documented as
// safe to call concurrently, which is what makes this the only locking-free
// path from a worker goroutine to the view.
type dashboardEventMsg struct{ progress.Event }

// dashboardModel is the dashboard. It is a value type, as Bubble Tea requires; all
// mutable state is either a pointer field or a slice the Update loop replaces.
type dashboardModel struct {
	title     string
	subtitle  string
	modelName string
	provider  string
	repo      string

	items   []*dashboardItem
	byGroup map[string]*dashboardItem

	log []dashboardLogEntry
	// logCache holds the wrapped lines of the log entries. Wrapping is what
	// the reader sees, and recomputing it for five hundred entries on every
	// frame would cost more than the rest of the view put together. It is a
	// pointer so a render can refresh it: a resize arrives outside the event
	// stream, and the frame after it has to show the new layout.
	logCache *logWrapCache
	// logOffset is how far the visible window has been scrolled back from the
	// tail. Zero means following.
	logOffset int
	// pending counts lines recorded while the log was paused, so releasing the
	// pause can resume following at the true tail rather than where it was.
	pending int
	// showLog is whether the activity feed is open. Folded away, the work list
	// takes the whole height: a reader watching the rows does not need the
	// feed, and a reader reading the feed does not need the rows.
	showLog bool

	totalFiles   int
	running      int
	reusedFiles  int
	doneFiles    int
	failedFiles  int
	skippedFiles int
	findings     int
	budgetHit    bool

	// estInput and estOutput are the run's projected cost, published with the
	// coverage denominator so the estimate is on screen from the first frame
	// instead of scrolling away in the log.
	estInput  int64
	estOutput int64
	// strategy names how the work is grouped and groups is how many groups it
	// is split into. Both are empty for a run that does not group its work.
	strategy string
	groups   int

	startedAt time.Time
	// color mirrors the caller's resolved --color decision. False renders the
	// dashboard without any ANSI styling.
	color bool
	// tokens reports the run's cumulative usage. It is polled on the render
	// tick rather than pushed: the per-group breakdown already arrives as
	// events, and the run-wide total is two atomics away from whoever owns
	// them.
	tokens  func() (input, output int64)
	session func() string
	// inputTokens and outputTokens are the last polled totals.
	inputTokens  int64
	outputTokens int64
	// sessionID is the last polled id, empty until the session persists.
	sessionID string
	paused    bool
	detached  bool
	// focus is which pane the arrow keys scroll. The work pane holds the
	// concurrent rows, the activity pane the log; both can outgrow their
	// region, so they cannot both be driven by one pair of keys.
	focus   dashboardPane
	workTop int
	// showCompleted expands the finished rows. They are folded away by
	// default because a long scan finishes hundreds of files and the finished
	// ones are not what the reader is watching for.
	showCompleted bool

	width  int
	height int
	// rawWidth and rawHeight are the size the terminal actually reported,
	// before the layout floor was applied. The guard needs them: a terminal
	// smaller than the floor cannot show the dashboard, and saying so beats
	// drawing a frame that does not fit. A reported zero is not "too small"
	// but "size not known yet", which a pty delivers before its first real
	// resize, so it is left to the floor.
	rawWidth  int
	rawHeight int

	spinner spinner.Model
	// cancel abandons the run when the user presses ctrl+c.
	cancel func()
}

// newDashboardModel seeds the dashboard. totalFiles is the frozen coverage denominator
// the run already computed, so the bar is meaningful from the first frame.
func newDashboardModel(o progressOptions, totalFiles int) dashboardModel {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(tuiBusyStyle))
	return dashboardModel{
		color:      o.Color,
		tokens:     o.Tokens,
		session:    o.Session,
		title:      o.Title,
		subtitle:   o.Subtitle,
		modelName:  o.Model,
		provider:   o.Provider,
		repo:       o.Repo,
		byGroup:    map[string]*dashboardItem{},
		focus:      dashboardPaneWork,
		showLog:    true,
		logCache:   &logWrapCache{},
		totalFiles: totalFiles,
		startedAt:  time.Now(),
		spinner:    sp,
		cancel:     o.Cancel,
		width:      80,
		height:     24,
	}
}

// Init starts the spinner and the clock.
func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tick())
}

func tick() tea.Cmd {
	return tea.Tick(dashboardTickInterval, func(t time.Time) tea.Msg { return dashboardTickMsg(t) })
}

// Update folds one message into the dashboard.
func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Clamp to a usable minimum. A terminal can legitimately report 0
		// (a pty with no size yet, or the window between a close and the
		// resize that follows), and every region clips itself to the frame
		// width — so a zero would blank the whole dashboard rather than
		// merely look wrong.
		m.rawWidth = msg.Width
		m.rawHeight = msg.Height
		m.width = max(msg.Width, minFrameWidth)
		m.height = max(msg.Height, minFrameHeight)
		// The wrapped lines were laid out for the old width, so the cache is
		// dropped and rebuilt on the next render.
		m.logCache.width = 0
		m.logCache.lines = nil
		return m, nil

	case dashboardTickMsg:
		if m.tokens != nil {
			m.inputTokens, m.outputTokens = m.tokens()
		}
		if m.session != nil {
			m.sessionID = m.session()
		}
		return m, tick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case dashboardEventMsg:
		m.apply(msg.Event)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m dashboardModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		// Detach, do not cancel. The run is already in flight on other
		// goroutines; quitting the view must not abandon it or lose its
		// results. The runner swaps back to the text sink on the way out, so
		// progress after this point simply returns to the terminal as the
		// [ocr] lines it used to be.
		m.detached = true
		return m, tea.Quit

	case "ctrl+c":
		// Raw mode clears the kernel's ISIG bit, so no SIGINT is delivered
		// while this view is up and the run's signal handler never fires.
		// The dashboard therefore has to hand the request back to the run
		// itself, which is the only thing that knows how to shut down
		// gracefully. The view stays up until the run returns, so the
		// shutdown is visible rather than instantaneous.
		if m.cancel != nil {
			m.cancel()
		}
		return m, nil

	case "p":
		// Pausing freezes the log window where it is. It does not stop the
		// run: pausing is for reading, not for throttling work.
		m.paused = !m.paused
		if !m.paused {
			m.logOffset = 0
			m.pending = 0
		}
		return m, nil

	case "c":
		// Fold the finished rows away or back out. On a long scan they
		// outnumber the running ones and push them off the screen.
		m.showCompleted = !m.showCompleted
		m.workTop = 0
		return m, nil

	case "l":
		// Fold the activity feed away or back out. The work list takes the
		// height the feed was using, which is what a reader watching the rows
		// wants; the feed itself is untouched and comes back as it was.
		m.showLog = !m.showLog
		if !m.showLog && m.focus == dashboardPaneActivity {
			// The pane the arrows drive has just gone away, so the focus
			// moves to the one that is still there.
			m.focus = dashboardPaneWork
		}
		return m, nil

	case "tab", "shift+tab":
		m.focus = oppositePane(m.focus)
		return m, nil

	case "u", "home":
		// Snap back to the tail of whatever is focused.
		m.logOffset = 0
		m.workTop = 0
		return m, nil

	case "j", "down":
		m.scrollPane(1)
		return m, nil

	case "k", "up":
		m.scrollPane(-1)
		return m, nil
	}
	return m, nil
}

func oppositePane(p dashboardPane) dashboardPane {
	if p == dashboardPaneWork {
		return dashboardPaneActivity
	}
	return dashboardPaneWork
}

// scrollPane moves the focused dashboardPane one step. delta is +1 for "further
// forward" (down the list) and -1 for "back" (up it).
//
// The two panes count in opposite directions, which is why the sign is
// flipped for the log: a work row's offset grows as you move down the list,
// while the log's offset is measured backwards from the tail, so moving down
// towards the newest line shrinks it. Normalising here keeps every caller
// thinking in one direction.
func (m *dashboardModel) scrollPane(delta int) {
	if m.focus == dashboardPaneActivity {
		m.scroll(-delta)
		return
	}
	m.workTop += delta
	if m.workTop < 0 {
		m.workTop = 0
	}
	if last := m.workRows() - 1; m.workTop > last {
		m.workTop = last
	}
}

// scroll moves the activity window. delta is positive for older entries.
// Reaching the tail clears the hold, so a reader who scrolls to the bottom
// starts following again without a second keypress.
func (m *dashboardModel) scroll(delta int) {
	m.logOffset += delta
	if m.logOffset <= 0 {
		m.logOffset = 0
	}
}

// holding reports whether the reader has scrolled the activity log away from
// the tail.
func (m dashboardModel) holding() bool { return m.logOffset > 0 }

// sanitizeProgressEvent strips control characters from the parts of an event
// that came from outside the program.
//
// A file name, a branch, or an error message can carry an escape sequence or
// a carriage return. Painting it as-is would let a crafted name repaint a
// region of the dashboard or hide the text around it. Sanitizing here, where
// the event enters the model, means every render site can trust what it
// draws.
func sanitizeProgressEvent(ev progress.Event) progress.Event {
	ev.Text = sanitizeTerminal(ev.Text)
	ev.Detail = sanitizeTerminal(ev.Detail)
	ev.Err = sanitizeTerminal(ev.Err)
	ev.SkipReason = sanitizeTerminal(ev.SkipReason)
	ev.Group = sanitizeTerminal(ev.Group)
	if len(ev.Paths) > 0 {
		// Copy rather than edit in place: the slice belongs to the publisher,
		// and the same event may be applied to more than one model.
		paths := make([]string, len(ev.Paths))
		for i, p := range ev.Paths {
			paths[i] = sanitizeTerminal(p)
		}
		ev.Paths = paths
	}
	return ev
}

// apply folds one event into the dashboard state. It is the only writer, so
// the counters and the row list can never disagree.
func (m *dashboardModel) apply(ev progress.Event) {
	ev = sanitizeProgressEvent(ev)
	switch ev.Kind {
	case progress.KindGroupStart:
		if m.startItem(ev) {
			m.running++
		}

	case progress.KindRoundStart:
		if it := m.lookup(ev.Group); it != nil {
			it.round = ev.Round
		}

	case progress.KindGroupDone:
		m.doneFiles += m.finish(ev, dashboardStateDone)
		m.findings += ev.Findings

	case progress.KindGroupFailed:
		m.failedFiles += m.finish(ev, dashboardStateFailed)

	case progress.KindSkipped:
		m.skippedFiles++
		m.appendSkip(ev)
		return

	case progress.KindBudgetReached:
		m.budgetHit = true

	case progress.KindPlanned:
		m.totalFiles = ev.Files
		if ev.Plan != nil {
			m.estInput = ev.Plan.InputTokens
			m.estOutput = ev.Plan.OutputTokens
			m.strategy = ev.Plan.Strategy
			m.groups = ev.Plan.Groups
		}

	case progress.KindToolStart:
		if it := m.lookup(ev.Group); it != nil {
			it.tool = ev.Detail
			it.toolRunning = true
		}

	case progress.KindToolDone:
		// The call is over, but the name stays: it is the last thing the
		// row did, and clearing it left the row reading "starting…" while
		// the activity log moved on. The flag says the call has finished, so
		// the view can tell a running call from a finished one.
		if it := m.lookup(ev.Group); it != nil {
			it.toolRunning = false
		}

	case progress.KindToolError:
		// A failed call is not running either, and the failure itself is
		// already in the activity log. The name stays: it is still the last
		// thing the row did.
		if it := m.lookup(ev.Group); it != nil {
			it.toolRunning = false
		}

	case progress.KindReused:
		// Reused files count as covered but have no row: nothing is being
		// done to them now, and listing hundreds of them would bury the work.
		// Their findings still count — they are real findings this run is
		// going to report, found by the run it is resuming.
		m.reusedFiles += ev.Files
		m.findings += ev.Findings

	case progress.KindUsage:
		// Cumulative totals from the pipeline, not deltas: the row shows what
		// the group has spent in total, which is also what the header sums.
		if it := m.lookup(ev.Group); it != nil {
			it.inputTokens = ev.InputTokens
			it.outputTokens = ev.OutputTokens
		}
	}

	// The log is the complete record: every kind lands here, because a warning
	// is exactly the line a user needs to be able to scroll back to. Events
	// that carry no text at all (the coverage denominator) are the exception —
	// they are state, not something to say.
	//
	// A paused dashboard keeps recording, so nothing is lost while the user
	// reads; what pausing does is stop the visible window following the tail,
	// which is what would otherwise drag their line off the bottom.
	text := strings.TrimRight(ev.Text, "\n")
	if text == "" {
		return
	}
	if m.paused {
		// Hold the visible window where the reader left it. The entry is
		// still recorded, so nothing is lost — the count in the heading keeps
		// climbing, and releasing the pause brings it into view.
		m.pending++
	}
	m.log = append(m.log, dashboardLogEntry{stamp: ev.Time.Format("15:04:05"), text: text, kind: ev.Kind})
	m.cacheLogEntry(m.log[len(m.log)-1])
	if len(m.log) > dashboardLogRetained {
		m.log = m.log[len(m.log)-dashboardLogRetained:]
		m.trimLogCache()
	}
}

// cacheLogEntry keeps the wrapped-lines cache in step with the log.
//
// A cache laid out for another width is dropped rather than patched: the
// next render rebuilds it for the width it is about to draw.
func (m *dashboardModel) cacheLogEntry(e dashboardLogEntry) {
	if m.logCache.width != m.width {
		m.logCache.lines = nil
		m.logCache.width = 0
		return
	}
	lines := m.wrapLogEntry(e)
	m.logCache.lines = append(m.logCache.lines, lines)
	m.logCache.total += len(lines)
}

// trimLogCache drops the wrapped lines of the entries the log dropped.
func (m *dashboardModel) trimLogCache() {
	if n := len(m.logCache.lines) - len(m.log); n > 0 {
		for _, l := range m.logCache.lines[:n] {
			m.logCache.total -= len(l)
		}
		m.logCache.lines = m.logCache.lines[n:]
	}
}

// rewrapLogEntry re-wraps one entry, for the collapsed skip rows whose text
// changes as the run of them grows.
func (m *dashboardModel) rewrapLogEntry(i int) {
	if i < 0 || i >= len(m.logCache.lines) || m.logCache.width != m.width {
		return
	}
	old := len(m.logCache.lines[i])
	m.logCache.lines[i] = m.wrapLogEntry(m.log[i])
	m.logCache.total += len(m.logCache.lines[i]) - old
}

// appendSkip adds a skipped file to the activity log, collapsing a run of
// files skipped for the same reason into one row. The event's own per-file
// text is deliberately not logged separately: printing it as well would give
// every skipped file two rows, and the per-file row sitting between two skips
// would break the run this collapses.
//
// Selection can skip hundreds of files for one of a handful of reasons, and
// printing one log line each buries everything else: on a real scan the first
// thing the reader saw was 272 identical-looking lines and none of the lines
// that explain what the run was about to do. The collapse is TUI-only — the
// per-file lines are still printed for piped and CI output, where they are the
// record of what was left out.
func (m *dashboardModel) appendSkip(ev progress.Event) {
	reason := ev.SkipReason
	if n := len(m.log); n > 0 {
		last := &m.log[n-1]
		if last.kind == progress.KindSkipped && last.reason == reason {
			last.count++
			last.text = fmt.Sprintf("Skipping %d file(s) — %s", last.count, reason)
			m.rewrapLogEntry(n - 1)
			return
		}
	}
	entry := dashboardLogEntry{
		stamp:  ev.Time.Format("15:04:05"),
		text:   fmt.Sprintf("Skipping %s — %s", ev.Detail, reason),
		kind:   progress.KindSkipped,
		count:  1,
		reason: reason,
	}
	m.log = append(m.log, entry)
	m.cacheLogEntry(entry)
}

// finish closes out a work row and returns how many files it accounts for.
// Done and failed differ only in the state and the one extra field, so they
// share this rather than repeating the same ten lines twice.
func (m *dashboardModel) finish(ev progress.Event, state dashboardItemState) int {
	it := m.lookup(ev.Group)
	if it == nil {
		// The row is missing (a resumed run, a late event). The files still
		// have to be counted, or the bar would stall short of full.
		return max(ev.Files, 1)
	}
	if it.state == dashboardStateRunning {
		m.running--
	}
	it.state = state
	it.tool = ""
	it.took = time.Since(it.started)
	if state == dashboardStateDone {
		it.findings = ev.Findings
	} else {
		it.err = ev.Err
	}
	return m.fileCount(ev, it)
}

// startItem creates or refreshes a work row, reporting whether it is new.
// A repeat announcement is a resumed group being re-announced: it must refresh
// the row's clock, not stack a duplicate or count as extra concurrent work.
func (m *dashboardModel) startItem(ev progress.Event) bool {
	if it, ok := m.byGroup[ev.Group]; ok {
		it.started = time.Now()
		return false
	}
	it := &dashboardItem{label: ev.Group, paths: ev.Paths, state: dashboardStateRunning, started: time.Now()}
	if ev.Plan != nil {
		it.batch = ev.Plan.Batch
	}
	m.byGroup[ev.Group] = it
	m.items = append(m.items, it)
	return true
}

func (m *dashboardModel) lookup(group string) *dashboardItem {
	if group == "" {
		return nil
	}
	return m.byGroup[group]
}

// fileCount is how many files a finished group accounts for. It prefers the
// event's own count, falls back to the group's recorded paths, and never
// reports zero: a dispatched group always covers at least one file, and
// undercounting would stall the bar short of 100% on a run that finished.
func (m *dashboardModel) fileCount(ev progress.Event, it *dashboardItem) int {
	if ev.Files > 0 {
		return ev.Files
	}
	if len(it.paths) > 0 {
		return len(it.paths)
	}
	return 1
}

// fraction is the bar's progress, over the files selected for review. See
// finishedFiles for why skipped files are not in the numerator.
func (m dashboardModel) fraction() float64 {
	if m.totalFiles <= 0 {
		return 0
	}
	return float64(m.finishedFiles()) / float64(m.totalFiles)
}
