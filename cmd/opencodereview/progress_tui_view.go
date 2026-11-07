// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/alibaba/open-code-review/internal/progress"
)

// The dashboard is five stacked regions:
//
//	header   what is being reviewed, on what, for how long
//	bar      overall file progress plus the counters worth watching
//	plan     what the run is estimated to cost, and how the work is grouped
//	work     one row per unit of concurrent work, live
//	log      the scrolling activity feed
//	help     the keybindings
//
// Everything above the log repaints in place; the log is the only region with
// history. The work list and the log split whatever height the fixed rows
// leave, so the frame always fits the terminal exactly.

const (
	// fixedRows are the regions whose height does not vary: header, bar,
	// stats, plan, a blank separator, and the help line.
	fixedRows = 6

	// minWorkRows and minLogRows are the floors for the two flexible regions.
	// Below these a run would show headers and nothing else, which is worse
	// than showing less of everything.
	minWorkRows = 2
	minLogRows  = 3

	// maxWorkRows stops a run with fifty in-flight groups from crowding the
	// log off the screen entirely.
	maxWorkRows = 12

	// minNoteWidth is how much room the note column gets before the name
	// column starts giving ground. The note is the part of a row that says
	// what is happening, so a long path may not squeeze it out.
	minNoteWidth = 24

	// minNameWidth and maxNameWidth bound the name column.
	minNameWidth = 16
	maxNameWidth = 60

	// minFrameWidth and minFrameHeight are the floor for the reported window.
	// Every region clips itself to the frame width, so a zero would blank the
	// dashboard instead of just looking cramped. A terminal below the floor
	// gets the too-small message rather than a frame that does not fit.
	//
	// The height is the layout's own minimum: the fixed rows, each region's
	// heading, and the floors for the two flexible regions. A frame that
	// cannot fit is worse than one that is not drawn at all.
	minFrameWidth  = 40
	minFrameHeight = fixedRows + minWorkRows + minLogRows + 2
)

// tooSmallMessage is what a terminal too small for the dashboard is told
// instead of being shown a frame that would not fit. It is derived from the
// floor so it cannot drift from it, and it is two short lines so that it
// fits the narrowest terminal the dashboard would otherwise have drawn.
var tooSmallMessage = fmt.Sprintf(
	"too small: the dashboard needs %dx%d\nctrl+c to stop",
	minFrameWidth, minFrameHeight)

// The dashboard shares the palette provider_tui.go declares for the config
// wizards, so the two terminal interfaces look like one program rather than
// two. The styles the provider flow does not have are declared here, in the
// same tui*Style shape.
var (
	tuiOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	tuiWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	tuiBusyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	tuiSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	// The bar's segment colours. Reused is dim because it is inherited
	// rather than earned by this run; done is green; failures are red, so
	// they are visible in the bar itself and not only in the status row.
	tuiReusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiDoneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// paint renders s with style, or plainly when color is off.
func (m dashboardModel) paint(style lipgloss.Style, s string) string {
	if !m.color {
		return tuiItemStyle.Render(s)
	}
	return style.Render(s)
}

// View draws the whole dashboard. The frame is composed with lipgloss joins
// rather than a viewport so the top regions stay pinned while the log scrolls.
func (m dashboardModel) View() tea.View {
	if m.tooSmall() {
		return tea.NewView(m.paint(tuiDimStyle, tooSmallMessage))
	}
	if m.detached {
		// Update already quits on detach, so a frame rendered on the way out
		// must not paint over the text the runner has just restored.
		return tea.NewView("")
	}

	regions := []string{
		m.viewHeader(),
		m.viewBar(),
		m.viewPlan(),
		"",
		m.viewWork(),
		m.viewLog(),
		m.viewHelp(),
	}
	// Every region is already clipped to the frame width and none sets a
	// background, so a plain join is equivalent to lipgloss's and skips its
	// per-line re-measurement on every frame.
	v := tea.NewView(strings.Join(regions, "\n"))
	v.AltScreen = true
	return v
}

// tooSmall reports whether the terminal is too narrow or too short for the
// dashboard to draw itself.
//
// A reported zero is the pty that has not been sized yet, not a terminal
// that cannot be shown, so it does not count.
func (m dashboardModel) tooSmall() bool {
	return (m.rawWidth > 0 && m.rawWidth < minFrameWidth) ||
		(m.rawHeight > 0 && m.rawHeight < minFrameHeight)
}

func (m dashboardModel) viewHeader() string {
	left := m.paint(tuiTitleStyle, "ocr "+m.title)
	if m.repo != "" {
		left += "  " + m.paint(tuiDimStyle, shortenPath(m.repo, m.width/3))
	}
	if m.subtitle != "" {
		left += "  " + m.paint(tuiDimStyle, m.subtitle)
	}
	right := m.paint(tuiDimStyle, humanDuration(int64(time.Since(m.startedAt).Seconds())))
	ident := identity(m.provider, m.modelName)
	if ident == "" {
		return joinEnds(m.width, left, right)
	}
	return joinEnds(m.width, left, m.paint(tuiTitleStyle, ident)+m.tokenBadge()+"  "+right)
}

// tokenBadge is the run's cumulative spend, beside the model name: arrows
// rather than words, because the two numbers are read constantly and the
// labels cost more width than they are worth.
func (m dashboardModel) tokenBadge() string {
	if m.inputTokens == 0 && m.outputTokens == 0 {
		return ""
	}
	in := m.paint(tuiBusyStyle, "▲"+humanTokens(m.inputTokens))
	out := m.paint(tuiWarnStyle, "▼"+humanTokens(m.outputTokens))
	return m.paint(tuiDimStyle, " [") + in + m.paint(tuiDimStyle, " ") + out + m.paint(tuiDimStyle, "]")
}

func (m dashboardModel) viewBar() string {
	row := lipgloss.JoinHorizontal(lipgloss.Top,
		m.paint(tuiDimStyle, "reviewing"),
		" ",
		m.viewBarFill(),
		" ",
		m.paint(tuiTitleStyle, m.barCounter()),
		" ",
		m.viewBarLegend(),
	)
	return lipgloss.JoinVertical(lipgloss.Left, clip(row, m.width), m.viewStats())
}

// viewPlan is the run's shape: what it is estimated to cost, and how the work
// is grouped.
//
// It stays on screen for the whole run. The log says the same thing once, and
// the estimate is exactly the fact a reader wants to keep in view while the
// run spends it — the header carries what has been spent so far, and this
// carries what the run is expected to spend.
func (m dashboardModel) viewPlan() string {
	if m.estInput == 0 && m.estOutput == 0 {
		return ""
	}
	parts := []string{
		m.paint(tuiSectionStyle, "plan"),
		m.paint(tuiDimStyle, " est "),
		m.paint(tuiBusyStyle, humanTokens(m.estInput)),
		m.paint(tuiDimStyle, " in + "),
		m.paint(tuiWarnStyle, humanTokens(m.estOutput)),
		m.paint(tuiDimStyle, " out ≈ "),
		m.paint(tuiDimStyle, humanTokens(m.estInput+m.estOutput)),
		m.paint(tuiDimStyle, " total"),
	}
	if m.strategy != "" && m.groups > 0 {
		parts = append(parts,
			m.paint(tuiDimStyle, " · "),
			m.paint(tuiDimStyle, fmt.Sprintf("%d batches · %s", m.groups, strings.ReplaceAll(m.strategy, "-", " "))),
		)
	}
	return clip(strings.Join(parts, ""), m.width)
}

// viewBarFill draws the progress bar itself.
//
// This is hand-rolled rather than using bubbles' progress component, which
// animates a spring toward its target. Files complete in discrete steps at
// LLM-round granularity, so a spring would spend frames animating between
// values a user reads as the same number, and it would pull in a physics
// dependency for the privilege.
func (m dashboardModel) viewBarFill() string {
	cells := m.barCells()
	body := ""
	if m.totalFiles > 0 {
		// The bar is segmented rather than a single fill, because "already
		// reviewed in a previous run" and "reviewed during this one" are very
		// different states for the reader: the first means the run inherited
		// its result, the second means it is doing the work. Reusing a
		// monochrome fill made a resumed run look further along than the
		// effort actually being spent.
		reused, done, failed := m.barSegments(cells)

		body = m.paint(tuiReusedStyle, strings.Repeat("█", reused)) +
			m.paint(tuiDoneStyle, strings.Repeat("█", done)) +
			m.paint(tuiErrorStyle, strings.Repeat("█", failed))
		if used := reused + done + failed; used < cells {
			body += m.paint(tuiDimStyle, strings.Repeat("░", cells-used))
		}
	} else {
		body = m.paint(tuiDimStyle, strings.Repeat("░", cells))
	}
	return m.paint(tuiDimStyle, "▕") + body + m.paint(tuiDimStyle, "▏")
}

// barSegments is how many cells of the bar each of its three segments gets.
//
// Each segment is measured against what is left of the bar, not against the
// whole of it. Measuring them all against the total lets them add up to more
// than the bar has whenever the counters do not divide the denominator
// exactly, and the bar then runs past the counter and the legend beside it.
//
// A failure always gets a cell. A share small enough to round away left the
// bar showing the run as finished and clean while the count beside it said
// files had failed — the two disagreed, and the bar was the one hiding the
// news.
func (m dashboardModel) barSegments(cells int) (reused, done, failed int) {
	if m.totalFiles <= 0 {
		return 0, 0, 0
	}
	// The segments are measured by where they end rather than by how long
	// each one is, so they stack without a gap between them. Measuring each
	// one separately and truncating left the bar a cell short of full
	// whenever the counts did not divide the width exactly — the bar never
	// quite filled, however far the run got.
	at := func(n int) int {
		c := int(math.Round(float64(n) / float64(m.totalFiles) * float64(cells)))
		switch {
		case c < 0:
			c = 0
		case c > cells:
			c = cells
		}
		return c
	}
	endReused := at(m.reusedFiles)
	endDone := at(m.reusedFiles + m.doneFiles)
	endFailed := at(m.reusedFiles + m.doneFiles + m.failedFiles)
	reused = endReused
	done = endDone - endReused
	failed = endFailed - endDone
	// A failure must be visible. A share small enough to round away left the
	// bar showing the run as finished and clean while the count beside it
	// said files had failed — the two disagreed, and the bar was the one
	// hiding the news.
	if m.failedFiles > 0 && failed == 0 {
		switch {
		case done > 0:
			done--
		case reused > 0:
			reused--
		}
		failed = 1
	}
	return reused, done, failed
}

// viewBarLegend names the bar's segments. A bar in three colours is not
// self-explanatory, and the reused segment in particular would otherwise be
// guesswork: only the count beside it says what the first block means.
func (m dashboardModel) viewBarLegend() string {
	seg := func(style lipgloss.Style, label string, n int) string {
		if n == 0 {
			return ""
		}
		return m.paint(style, "▇") + m.paint(tuiDimStyle, " "+label)
	}
	parts := []string{
		seg(tuiReusedStyle, "reused", m.reusedFiles),
		seg(tuiDoneStyle, "done", m.doneFiles),
		seg(tuiErrorStyle, "failed", m.failedFiles),
	}
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += m.paint(tuiDimStyle, " ")
		}
		out += p
		_ = i
	}
	return out
}

// barLabel is the text that precedes the bar on its row: the label, the two
// frame glyphs, and the space between them.
const barLabel = "reviewing  ▕▏ "

// barCounter is the "n/m" count beside the bar. It is a function because the
// count changes as the run proceeds, and the bar's width has to follow it.
func (m dashboardModel) barCounter() string {
	return fmt.Sprintf("%d/%d", m.finishedFiles(), m.totalFiles)
}

// barCells is how wide the bar may be. It is derived from the terminal rather
// than fixed, because a fixed bar overflows a narrow window and pushes every
// region below it off screen. What the bar shares its row with is the label,
// the counter and the legend, and the counter grows with the file count — a run
// of a thousand files already needs four digits — so none of it can be
// assumed to be a fixed width.
func (m dashboardModel) barCells() int {
	overhead := lipgloss.Width(barLabel) + lipgloss.Width(m.barCounter()) + lipgloss.Width(m.viewBarLegend())
	cells := m.width - overhead
	switch {
	case cells > 24:
		return 24
	case cells < 4:
		return 4
	default:
		return cells
	}
}

// viewStats is the counter row: everything a person would otherwise have to
// grep the text wall for, in one line.
func (m dashboardModel) viewStats() string {
	parts := []string{
		m.paint(tuiOKStyle, fmt.Sprintf("%d findings", m.findings)),
		m.paint(tuiDimStyle, " · "),
		m.paint(tuiBusyStyle, fmt.Sprintf("%d in flight", m.countRunning())),
	}
	if m.reusedFiles > 0 {
		parts = append(parts, m.paint(tuiDimStyle, " · "), m.paint(tuiReusedStyle, fmt.Sprintf("%d reused", m.reusedFiles)))
	}
	if m.failedFiles > 0 {
		parts = append(parts, m.paint(tuiDimStyle, " · "), m.paint(tuiErrorStyle, fmt.Sprintf("%d failed", m.failedFiles)))
	}
	if m.skippedFiles > 0 {
		parts = append(parts, m.paint(tuiDimStyle, " · "), m.paint(tuiWarnStyle, fmt.Sprintf("%d skipped", m.skippedFiles)))
	}
	// The one thing that must never be buried: a run that is about to stop
	// early has to say so in the status line, not only in the log.
	if m.budgetHit {
		parts = append(parts, m.paint(tuiDimStyle, " · "), m.paint(tuiWarnStyle, "token budget reached — stopping early"))
	}
	if m.paused {
		parts = append(parts, m.paint(tuiDimStyle, " · "), m.paint(tuiWarnStyle, "log paused"))
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// The session id goes last so it can never displace a counter, and it is
	// trimmed from the left if the terminal is too narrow: the tail is the
	// part that identifies a session, and it is what a reader compares
	// against the dashboard.
	if m.sessionID != "" {
		id := m.sessionID
		budget := m.width - lipgloss.Width(row) - len(" · session ")
		if budget > 8 {
			if len(id) > budget {
				id = "…" + id[len(id)-budget+1:]
			}
			row += m.paint(tuiDimStyle, " · session ") + m.paint(tuiDimStyle, id)
		}
	}
	return clip(row, m.width)
}

// viewWork renders the concurrent work list. Running rows sort first because
// they are the answer to "what is it doing right now"; finished rows sink
// below them, most recent first.
func (m dashboardModel) viewWork() string {
	running, done := m.splitItems()
	rows := make([]string, 0, len(running)+len(done)+2)

	// The name column is shared by every row, so it is measured over the
	// whole run rather than over the rows on screen: a column that follows
	// the visible rows would move every note sideways as rows came and went.
	col := m.nameWidth(m.items)

	frame := m.spinner.View()
	for _, it := range running {
		rows = append(rows, m.viewItem(it, frame, col)...)
	}
	if len(running) == 0 {
		rows = append(rows, m.paint(tuiDimStyle, "  waiting for work…"))
	}

	// The running list scrolls independently: a wide review can have more
	// groups in flight than the region has rows.
	if len(rows) > m.workBodyRows() {
		rows = m.scrollWindow(rows, m.workTop)
	}

	if m.showCompleted {
		rows = append(rows, m.sectionHeading("finished", len(done), ""))
		for _, it := range done {
			rows = append(rows, m.viewItem(it, frame, col)...)
		}
	}

	// The heading counts what is being worked on right now, not every row the
	// run has ever opened. The old count was the running total, so a heading
	// reading "work (51)" sat next to a status row saying four files were in
	// flight, and the two were about different things: the reader took the
	// heading for the files being worked on, and it was the files already
	// behind them.
	//
	// The folded rows are called finished rather than done, because some of
	// them failed and the status row's counters say so.
	hint := ""
	if len(done) > 0 {
		hint = fmt.Sprintf("%d finished", len(done))
		if !m.showCompleted {
			hint += " (c)"
		}
	}
	heading := m.sectionHeading("work", m.countRunning(), hint)
	if m.focus == dashboardPaneWork {
		// Mark the pane the arrows are driving, so a key press is never a
		// guess about which list just moved.
		heading = m.paint(tuiTitleStyle, heading)
	}
	return m.clamp(heading, rows, m.workHeight())
}

// splitItems partitions the work list. Running rows come first because they
// are the answer to "what is it doing"; finished rows keep their most-recent
// -first order so the most recent completion is the one you see.
func (m dashboardModel) splitItems() (running, done []*dashboardItem) {
	for _, it := range m.items {
		if it.state == dashboardStateRunning {
			running = append(running, it)
		}
	}
	for i := len(m.items) - 1; i >= 0; i-- {
		if m.items[i].state != dashboardStateRunning {
			done = append(done, m.items[i])
		}
	}
	return running, done
}

// workRows is how many rows the work region can show, heading included.
func (m dashboardModel) workRows() int {
	return m.workHeight()
}

// workBodyRows is the room left for rows once the heading is drawn. When the
// completed section is expanded it takes the rest, so the running list keeps
// its own room rather than being squeezed away by history.
func (m dashboardModel) workBodyRows() int {
	rows := m.workHeight() - 1
	if m.showCompleted {
		rows /= 2
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// scrollWindow shows a window of lines at offset, marking that more exist in
// each direction so a partially visible list is never mistaken for a whole
// one.
//
// The markers are lines of their own. Sharing the last line with a row made
// the frame wider than the terminal, and the terminal wrapped it onto a
// second line, which pushed the help line off the bottom of the screen.
func (m dashboardModel) scrollWindow(rows []string, top int) []string {
	height := m.workBodyRows()
	if len(rows) <= height {
		return rows
	}
	if top > len(rows)-height {
		top = len(rows) - height
	}
	if top < 0 {
		top = 0
	}
	window := rows[top : top+height]
	if top > 0 {
		window = append([]string{m.paint(tuiDimStyle, fmt.Sprintf("  ↑ %d more", top))}, window...)
		window = window[:height]
	}
	if top+height < len(rows) {
		window = window[:height-1]
		window = append(window, m.paint(tuiDimStyle, fmt.Sprintf("  ↓%d more", len(rows)-top-height)))
	}
	return window
}

// viewItem renders one work row.
//
// A row that fits the frame is one line: the icon in a fixed column, the name
// in a fixed column, and the note after it, so a dozen simultaneous rows line
// up and can be scanned. A note that does not fit continues on the next
// lines rather than being cut off — a truncated note says nothing, and the
// note is the part of the row that says what is happening.
func (m dashboardModel) viewItem(it *dashboardItem, frame string, col int) []string {
	var icon, plain, note string
	var style lipgloss.Style
	switch it.state {
	case dashboardStateRunning:
		icon = frame
		style = tuiTitleStyle
		plain = it.display()
		note = m.itemNote(it)
	case dashboardStateDone:
		icon = m.paint(tuiOKStyle, "✓")
		style = tuiDimStyle
		plain = it.display()
		note = m.paint(tuiDimStyle, m.costLine(it))
	case dashboardStateFailed:
		icon = m.paint(tuiErrorStyle, "✗")
		style = tuiErrorStyle
		plain = it.display()
		note = m.paint(tuiErrorStyle, it.err)
	}
	// The tail of a path is what identifies the file, so a name that does not
	// fit its column is shortened from the front rather than cut off at the
	// end.
	if lipgloss.Width(plain) > col {
		plain = shortenPath(plain, col)
	}
	name := m.paint(style, plain)

	// A fixed icon column and a fixed name column are what make a dozen
	// simultaneous rows scannable rather than a ragged mess.
	head := "  " + pad(icon, 2) + " " + pad(name, col) + " "
	lines := wrapNote(note, m.width-lipgloss.Width(head), lipgloss.Width(head))
	if len(lines) == 0 {
		return []string{clip(head, m.width)}
	}
	out := make([]string, 0, len(lines))
	out = append(out, head+lines[0])
	out = append(out, lines[1:]...)
	return out
}

// wrapNote lays a row's note out under the row's own columns: a note that
// fits is one line, and a note that does not continues on the next lines,
// each indented to the note's column so the rows still line up.
func wrapNote(note string, width, indent int) []string {
	if note == "" {
		return nil
	}
	if width < 8 {
		width = 8
	}
	wrapped := strings.Split(lipgloss.Wrap(note, width, ""), "\n")
	out := make([]string, 0, len(wrapped))
	for i, l := range wrapped {
		if i == 0 {
			out = append(out, l)
			continue
		}
		out = append(out, strings.Repeat(" ", indent)+l)
	}
	return out
}

// costLine is the right-hand column of a finished row: what it found, how long
// it took, and what it spent. Findings first because that is why anyone looks,
// then the two numbers that tell the story of the cost.
func (m dashboardModel) costLine(it *dashboardItem) string {
	return fmt.Sprintf("%d findings · %s · %s",
		it.findings, shortDuration(it.took), m.spendPair(it.inputTokens, it.outputTokens))
}

// itemNote is the right-hand column of a running row: which round, what the
// agent is doing right now, and what it has spent so far.
func (m dashboardModel) itemNote(it *dashboardItem) string {
	var b strings.Builder
	// The batch comes first: it frames the rest of the row, saying which
	// part of the run this work belongs to.
	if it.batch != "" {
		b.WriteString(m.paint(tuiDimStyle, "batch "+it.batch))
	}
	// Rounds are published starting at 1, so a non-zero round is what
	// distinguishes "in a round" from "still starting".
	doing := false
	if it.round > 0 {
		doing = true
		if b.Len() > 0 {
			b.WriteString(m.paint(tuiDimStyle, " · "))
		}
		b.WriteString(m.paint(tuiDimStyle, fmt.Sprintf("round %d", it.round)))
	}
	if it.tool != "" {
		doing = true
		if b.Len() > 0 {
			b.WriteString(m.paint(tuiDimStyle, " · "))
		}
		// A call still in flight is bright; one that has finished keeps
		// its name but is dim, because the name is the last thing the row
		// did rather than what it is doing now.
		style := tuiDimStyle
		if it.toolRunning {
			style = tuiBusyStyle
		}
		b.WriteString(m.paint(style, it.tool))
	}
	if !doing {
		if b.Len() > 0 {
			b.WriteString(m.paint(tuiDimStyle, " · "))
		}
		b.WriteString(m.paint(tuiDimStyle, "starting…"))
	}
	// Spend so far, kept last so the round and the current tool — the parts
	// that change as the work proceeds — stay on the left where the eye lands.
	if it.inputTokens > 0 || it.outputTokens > 0 {
		if b.Len() > 0 {
			b.WriteString(m.paint(tuiDimStyle, " · "))
		}
		b.WriteString(m.spendPair(it.inputTokens, it.outputTokens))
	}
	return b.String()
}

// spendPair renders a spend as the header renders the run's: ▲ for input, ▼
// for output. The same two arrows mean the same two things everywhere, so a
// row's numbers can be read against the run's without a second guess.
func (m dashboardModel) spendPair(input, output int64) string {
	s := ""
	if input > 0 {
		s += m.paint(tuiBusyStyle, "▲"+humanTokens(input))
	}
	if output > 0 {
		if s != "" {
			s += " "
		}
		s += m.paint(tuiWarnStyle, "▼"+humanTokens(output))
	}
	return s
}

// viewLog is the scrolling activity feed. It is the only region with history,
// so it takes whatever height the work list does not need.
func (m dashboardModel) viewLog() string {
	if !m.showLog {
		// Folded away, the feed keeps its count and its place in the frame
		// and gives its height to the work list.
		return clip(m.sectionHeading("activity", len(m.log), "press l to expand"), m.width)
	}
	heading := m.sectionHeading("activity", len(m.log), logHint(m.paused, m.holding()))

	height := m.logHeight()
	// One line is spent on the heading, so the body gets the rest.
	avail := height - 1
	if avail < 1 {
		avail = 1
	}
	// The window ends where the reader left it: logOffset lines back from the
	// tail, minus however many lines arrived while paused. Both offsets count
	// backwards from the end, so subtracting them pins the window across a
	// pause without having to freeze the log itself.
	end := len(m.log) - m.logOffset - m.pending
	if end < 0 {
		end = 0
	}
	// The window is counted in lines, not entries: an entry whose text wraps
	// takes more than one, so the feed is walked back from the tail until
	// the line budget is spent.
	wrapped := m.logLines()
	var rows []string
	start := end
	for start > 0 && len(rows) < avail {
		start--
		rows = append(append([]string{}, wrapped[start]...), rows...)
	}
	if len(rows) > avail {
		rows = rows[len(rows)-avail:]
	}
	if len(rows) == 0 {
		rows = append(rows, m.paint(tuiDimStyle, "  no activity yet"))
	}
	rows = m.withScrollbar(rows, m.logLinesBefore(start), m.logCache.total, avail)
	return m.clamp(heading, rows, height)
}

// withScrollbar puts a scroll indicator down the right-hand edge of the log,
// so the reader can see how much of the feed is left and where the window
// sits in it.
//
// The bar is drawn in the last column of the region, so the entries give up
// that column. A log that fits entirely gets a full-height thumb: there is
// nothing to scroll, and the bar says so.
//
// before is how many lines of the feed sit above the window and total is how
// many the feed has, both counted in lines rather than entries: an entry
// whose text wraps takes more than one line, and a bar that counted entries
// would claim the feed is longer than the screen can show.
func (m dashboardModel) withScrollbar(rows []string, before, total, avail int) []string {
	if avail < 1 || m.width < 2 {
		return rows
	}
	width := m.width - 1
	bar := m.scrollbarColumn(before, total, len(rows), avail)
	for i := range rows {
		rows[i] = pad(rows[i], width) + bar[i]
	}
	return rows
}

// scrollbarColumn draws the bar: a thumb whose height is the visible fraction
// of the feed, sitting at the window's position in it.
func (m dashboardModel) scrollbarColumn(before, total, shown, avail int) []string {
	col := make([]string, avail)
	for i := range col {
		col[i] = m.paint(tuiDimStyle, "│")
	}
	if total <= avail {
		// Everything fits, so the whole bar is the thumb.
		for i := range col {
			col[i] = m.paint(tuiDimStyle, "┃")
		}
		return col
	}
	thumb := avail * shown / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > avail {
		thumb = avail
	}
	pos := 0
	if total > shown {
		pos = before * (avail - thumb) / (total - shown)
	}
	for i := 0; i < thumb; i++ {
		if pos+i < avail {
			col[pos+i] = m.paint(tuiBusyStyle, "┃")
		}
	}
	return col
}

// wrapLogEntry lays one activity entry out as the lines it takes.
//
// The stamp is a fixed column and the text follows it, so a long line
// continues under the text rather than under the timestamp: the reader is
// reading the message, and the message is what should stay in one column.
func (m dashboardModel) wrapLogEntry(e dashboardLogEntry) []string {
	// One entry is one row of text. The log region counts lines, so an entry
	// whose text carries a newline — a tool output that spans lines, a
	// warning about an error that does — is folded into one line first.
	text := strings.ReplaceAll(e.text, "\n", " ")
	var body string
	switch e.kind {
	case progress.KindWarning, progress.KindGroupFailed, progress.KindToolError, progress.KindBudgetReached:
		body = m.paint(tuiWarnStyle, text)
	case progress.KindGroupStart, progress.KindGroupDone, progress.KindRoundStart, progress.KindSkipped:
		body = m.paint(tuiDimStyle, text)
	default:
		body = text
	}
	stamp := m.paint(tuiDimStyle, e.stamp)
	head := "  " + stamp + "  "
	lines := wrapNote(body, m.width-lipgloss.Width(head), lipgloss.Width(head))
	if len(lines) == 0 {
		return []string{clip(head, m.width)}
	}
	out := make([]string, 0, len(lines))
	out = append(out, head+lines[0])
	out = append(out, lines[1:]...)
	return out
}

// helpKeys names the scroll hint after the focused pane: one pair of arrows
// now drives two lists, so the binding alone is not enough to know which one
// will move.
func (m dashboardModel) helpKeys() []string {
	return []string{
		"q detach",
		"tab pane",
		"↑↓ " + m.focusName(),
		"c finished",
		"l " + m.logKeyName(),
		"p pause log",
		"ctrl+c cancel",
	}
}

// logKeyName is what the key that folds the activity feed does now.
func (m dashboardModel) logKeyName() string {
	if m.showLog {
		return "hide log"
	}
	return "show log"
}

func (m dashboardModel) focusName() string {
	if m.focus == dashboardPaneWork {
		return "work"
	}
	return "log"
}

func (m dashboardModel) viewHelp() string {
	sep := m.paint(tuiDimStyle, "  ·  ")
	help := m.paint(tuiDimStyle, strings.Join(m.helpKeys(), sep))
	return clip(help, m.width)
}

// sectionHeading is the small label above a region, with a right-aligned hint.
func (m dashboardModel) sectionHeading(name string, count int, hint string) string {
	left := m.paint(tuiSectionStyle, name)
	if count > 0 {
		left += m.paint(tuiDimStyle, fmt.Sprintf(" (%d)", count))
	}
	if hint == "" {
		return left
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(hint)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + m.paint(tuiDimStyle, hint)
}

// clamp joins a heading to its rows and cuts the result to exactly height
// lines, so a long list can never push the keybindings off screen.
func (m dashboardModel) clamp(heading string, rows []string, height int) string {
	all := append([]string{heading}, rows...)
	if height > 0 {
		if len(all) > height {
			all = all[:height]
		} else {
			// Pad to the height the layout gave the pane. Without it a
			// short pane pulled the regions below it up, so the frame ended
			// short of the terminal and the log pane moved as work came
			// and went.
			for len(all) < height {
				all = append(all, "")
			}
		}
	}
	return strings.Join(all, "\n")
}

// Layout arithmetic: the work list and the log split whatever the fixed rows
// leave, each with a floor and the work list with a ceiling.

func (m dashboardModel) flexibleRows() int {
	rows := m.height - fixedRows
	if rows < minWorkRows+minLogRows {
		return minWorkRows + minLogRows
	}
	return rows
}

func (m dashboardModel) workHeight() int {
	if !m.showLog {
		// The feed is folded away, so the work list takes the height the two
		// of them shared, less the line the folded feed keeps for its
		// heading.
		return m.flexibleRows() - 1
	}
	rows := m.flexibleRows() / 3
	if rows < minWorkRows {
		rows = minWorkRows
	}
	if rows > maxWorkRows {
		rows = maxWorkRows
	}
	return rows + 1 // heading
}

// logHeight is whatever the work list did not take, its heading included.
//
// The flexible rows cover both panes and each pane spends one on its heading,
// so the log's own heading has to come out of what the work list leaves. It
// did not, and the frame came out one line taller than the terminal, which
// pushed the help line off the bottom.
func (m dashboardModel) logHeight() int {
	if !m.showLog {
		// One line: the heading that says the feed is there and how to
		// bring it back.
		return 1
	}
	h := m.flexibleRows() - (m.workHeight() - 1) - 1
	if h < minLogRows+1 {
		h = minLogRows + 1
	}
	return h
}

func (m dashboardModel) countRunning() int { return m.running }

// finishedFiles is the bar's numerator: files the run is actually done with.
//
// Skipped files are deliberately excluded. The denominator is the count of
// files selected for review, and selection has already removed everything that
// was skipped — so counting them again here would measure progress against a
// total that never contained them. A scan that discovered 759 files, skipped
// 272 and selected 487 would open at 320/487 instead of 0/487. They are not
// lost: the status row reports the skipped count separately.
func (m dashboardModel) finishedFiles() int {
	return m.reusedFiles + m.doneFiles + m.failedFiles
}

// nameWidth is the width of the name column.
//
// It follows the longest name on screen, so a run of short paths leaves the
// room for the note, and it gives ground to the note when the paths are long:
// the note is what says what is happening, and a name column that squeezes
// it to nothing leaves the row saying only which file it is.
func (m dashboardModel) nameWidth(items []*dashboardItem) int {
	longest := 0
	for _, it := range items {
		if n := lipgloss.Width(it.display()); n > longest {
			longest = n
		}
	}
	if longest > maxNameWidth {
		longest = maxNameWidth
	}
	room := m.width - 5 - minNoteWidth
	if room < minNameWidth {
		room = minNameWidth
	}
	if longest > room {
		longest = room
	}
	if longest < minNameWidth {
		longest = minNameWidth
	}
	return longest
}

func logHint(paused, scrub bool) string {
	switch {
	case paused:
		return "paused — press p to resume"
	case scrub:
		return "held — press u to follow"
	default:
		return "following"
	}
}

// joinEnds puts left and right on one line with the gap between them, or
// drops right when the terminal is too narrow to hold both.
func joinEnds(width int, left, right string) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return clip(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func identity(provider, dashboardModel string) string {
	switch {
	case provider != "" && dashboardModel != "":
		return provider + "/" + dashboardModel
	case dashboardModel != "":
		return dashboardModel
	default:
		return provider
	}
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// pad right-pads to n visible cells, counting ANSI escapes as zero width.
func pad(s string, n int) string {
	gap := n - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// clip hard-wraps to n cells. A hard cut rather than an ellipsis, because
// these rows are status lines that update many times a second: a trailing
// ellipsis on every row would be noise, and a wrapping status line would
// break the fixed height the layout depends on.
// clip cuts s to n columns, marking the cut with an ellipsis.
//
// ansi.Truncate counts the printable columns of a styled string rather than
// its bytes, so a row that carries colour is cut where it looks cut. The
// ellipsis says the row continues past the edge, which a hard cut does not.
func clip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return ansi.Truncate(s, n, "…")
}

// shortenPath keeps the tail of a path, which is the part that identifies it.
func shortenPath(p string, width int) string {
	if width <= 0 {
		return p
	}
	runes := []rune(p)
	if len(runes) <= width {
		return p
	}
	if width <= 1 {
		return string(runes[len(runes)-width:])
	}
	return "…" + string(runes[len(runes)-(width-1):])
}
