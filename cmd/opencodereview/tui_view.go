// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m reviewTUIModel) bodyWidth() int { return max(1, min(110, m.width-4)) }

func (m reviewTUIModel) menuSize() int {
	available := m.height - 8
	if m.notice != "" {
		available--
	}
	return max(1, min(6, available))
}

func (m reviewTUIModel) scope() string {
	if m.opts.commit != "" {
		return m.tr("Commit %s", sanitizeTerminal(m.opts.commit))
	}
	if m.opts.from != "" {
		return sanitizeTerminal(m.opts.from + " .. " + m.opts.to)
	}
	return m.tr("Working tree")
}

func (m reviewTUIModel) modelLabel() string {
	if m.opts.model != "" {
		return sanitizeTerminal(m.opts.model)
	}
	if m.opts.provider != "" {
		return sanitizeTerminal(m.opts.provider) + " / " + m.tr("default model")
	}
	return m.tr("Configured default")
}

func (m *reviewTUIModel) refresh(bottom bool) {
	m.input.Placeholder = m.tr("Type a command or describe your changes")
	reserve := 6
	if m.notice != "" {
		reserve++
	}
	if choices := m.commandChoices(); len(choices) > 0 {
		reserve += min(m.menuSize(), len(choices)) + 1
	}
	if m.screen == tuiSettings {
		reserve = 3
	}
	m.viewport.SetWidth(m.bodyWidth())
	m.viewport.SetHeight(max(1, m.height-reserve))
	m.viewport.SetContent(m.content())
	m.viewport.SetHeight(min(m.viewport.Height(), max(1, m.viewport.TotalLineCount())))
	if bottom {
		m.viewport.GotoBottom()
	}
}

func (m reviewTUIModel) View() tea.View {
	if m.width < 24 || m.height < 10 {
		v := tea.NewView(ansi.Truncate(m.tr("Enlarge terminal. Ctrl+C exits."), m.width, ""))
		v.AltScreen = true
		return v
	}
	w := m.bodyWidth()
	lines := []string{m.viewport.View()}
	if m.screen == tuiSettings {
		lines = append(lines, "", m.noticeLine(w), m.tint(ansi.Truncate(m.tr("↑/↓ choose · Enter apply · Esc back"), w, "…"), tuiBrandMuted))
	} else {
		if m.notice != "" {
			lines = append(lines, m.noticeLine(w))
		}
		rule := m.tint(strings.Repeat("─", w), "#284B37")
		input := m.input.View()
		if !m.colored {
			input = ansi.Strip(input)
		}
		lines = append(lines, "", rule, input, rule)
		choices := m.commandChoices()
		if len(choices) > 0 {
			start, end := tuiVisibleRange(len(choices), m.menuSelected, m.menuSize())
			for i := start; i < end; i++ {
				prefix := "  "
				if i == m.menuSelected {
					prefix = "> "
				}
				row := prefix + fmt.Sprintf("%-13s", choices[i].name) + m.tr(choices[i].description)
				lines = append(lines, m.selection(ansi.Truncate(row, w, "…"), i == m.menuSelected, w))
			}
			lines = append(lines, m.tint(ansi.Truncate(m.tr("Enter select · Tab complete · Esc dismiss"), w, "…"), tuiBrandMuted))
		}
		status := sanitizeTerminal(filepath.Base(m.opts.repoDir)) + "  ·  " + m.modelLabel() + "  ·  " + m.scope()
		if m.busy {
			state := m.tr(m.jobName)
			if m.stopping {
				state = m.tr("Saving results…")
			}
			status = "● " + state + "  " + time.Since(m.started).Truncate(time.Second).String()
		}
		hint := m.tr("Type / for commands") + "  ·  /settings"
		if m.busy {
			hint = m.tr("Esc cancel · PgUp/PgDn scroll · /logs progress")
		}
		if (m.screen == tuiResults && len(m.result.Comments) > 0) || (m.screen == tuiSessions && len(m.sessions) > 0) {
			hint = m.tr("↑/↓ select · Enter open · Esc back")
		} else if m.screen == tuiFinding || m.screen == tuiHelp || m.screen == tuiResults || m.screen == tuiSessions {
			hint = m.tr("PgUp/PgDn scroll · Esc back")
		}
		lines = append(lines, m.tint(ansi.Truncate(status, w, "…"), tuiBrandMuted), m.tint(ansi.Truncate(hint, w, "…"), tuiBrandMuted))
	}
	body := lipgloss.NewStyle().Padding(0, 2).Render(strings.Join(lines, "\n"))
	rows := strings.Split(body, "\n")
	for i := range rows {
		rows[i] = ansi.Truncate(rows[i], m.width, "")
	}
	body = strings.Join(rows, "\n")
	v := tea.NewView(body)
	v.AltScreen = true
	if m.colored {
		v.BackgroundColor = lipgloss.Color(tuiBrandBackground)
		v.ForegroundColor = lipgloss.Color(tuiBrandText)
	}
	return v
}

func (m reviewTUIModel) noticeLine(width int) string {
	color := tuiBrandMuted
	if m.isError {
		color = "#E8AF87"
	}
	notice := strings.Join(strings.Fields(sanitizeTerminal(m.tr(m.notice))), " ")
	return m.tint(ansi.Truncate(notice, width, "…"), color)
}

func (m reviewTUIModel) selection(text string, active bool, width int) string {
	if !active || !m.colored {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(tuiBrandHighlight)).
		Background(lipgloss.Color("#12281D")).Width(width).Render(text)
}

// Frame widths use terminal cells, so CJK labels never shift the border.
func (m reviewTUIModel) frame(title string, rows []string) string {
	w := m.bodyWidth()
	label := " " + ansi.Truncate(title, max(1, w-6), "…") + " "
	top := "╭─" + label + strings.Repeat("─", max(0, w-3-ansi.StringWidth(label))) + "╮"
	lines := []string{m.tint(top, tuiBrandFace)}
	for _, row := range rows {
		row = ansi.Truncate(row, max(1, w-4), "…")
		lines = append(lines, m.tint("│", "#284B37")+" "+row+strings.Repeat(" ", max(0, w-4-ansi.StringWidth(row)))+" "+m.tint("│", "#284B37"))
	}
	return strings.Join(append(lines, m.tint("╰"+strings.Repeat("─", max(0, w-2))+"╯", "#284B37")), "\n")
}

func (m reviewTUIModel) homeContent() string {
	inner := m.bodyWidth() - 4
	if inner >= tuiBrandFullWidth && m.viewport.Height() >= 17 && len(m.commandChoices()) == 0 {
		rows := strings.Split(renderTUIWordmark(inner, m.colored), "\n")
		if inner >= 88 {
			side := []string{m.tr("Review your changes"), "", m.tr("Scope"), m.scope(), "", m.tr("Model"), m.modelLabel(), "", "/preview", m.tr("Inspect files"), "/review", m.tr("Start review"), "", "/settings", m.tr("Interface language")}
			for i := range rows {
				rows[i] += strings.Repeat(" ", max(0, 67-ansi.StringWidth(rows[i]))) + m.tint("│", "#284B37") + "  " + ansi.Truncate(side[i], inner-70, "…")
			}
		}
		return m.frame("Open Code Review", rows)
	}
	return m.frame("Open Code Review", []string{
		m.tint(m.tr("Review your changes"), tuiBrandText), "",
		m.tr("Scope") + "  " + m.scope(),
		m.tr("Model") + "  " + m.modelLabel(), "",
		m.tint("/preview", tuiBrandFace) + "  " + m.tr("Inspect files"),
		m.tint("/review", tuiBrandFace) + "   " + m.tr("Start review"),
		m.tint("/sessions", tuiBrandFace) + " " + m.tr("Saved sessions"),
		m.tint("/settings", tuiBrandFace) + " " + m.tr("Interface language"),
	})
}

func (m reviewTUIModel) settingsContent() string {
	rows := []string{m.tint(m.tr("Interface language"), tuiBrandText), m.tr("Changes apply immediately and are saved for next time."), ""}
	for i, id := range tuiLanguageIDs {
		prefix := "  ○ "
		if i == m.settingsIndex {
			prefix = "> ○ "
		}
		if id == m.preferences.Language {
			prefix = "  ● "
			if i == m.settingsIndex {
				prefix = "> ● "
			}
		}
		row := prefix + tuiLanguageName(id)
		if id == m.preferences.Language {
			row += "  " + m.tr("Current")
		}
		rows = append(rows, m.selection(ansi.Truncate(row, m.bodyWidth()-4, "…"), i == m.settingsIndex, m.bodyWidth()-4))
	}
	return m.frame(m.tr("Settings"), append(rows, ""))
}

func (m reviewTUIModel) content() string {
	switch m.screen {
	case tuiHome:
		return m.homeContent()
	case tuiSettings:
		return m.settingsContent()
	case tuiLog:
		lines := []string{m.tint(m.tr("Review activity"), tuiBrandFace), m.scope(), ""}
		if m.progress != nil {
			lines = append(lines, m.progress.snapshot()...)
		}
		if len(lines) == 3 {
			lines = append(lines, m.tr("Waiting for review activity..."))
		}
		return strings.Join(lines, "\n")
	case tuiPreview:
		lines := []string{m.tint(m.tr("Review scope"), tuiBrandFace), m.tr("%d reviewable files, %d excluded   +%d -%d", m.preview.ReviewableCount, m.preview.ExcludedCount, m.preview.TotalInsertions, m.preview.TotalDeletions), ""}
		if m.previewError != "" {
			return strings.Join([]string{lines[0], "", m.tr("Unable to inspect changes"), m.previewError}, "\n")
		}
		for _, f := range m.preview.Entries {
			state := m.tr("review")
			if !f.WillReview {
				state = m.tr(string(f.ExcludeReason))
			}
			lines = append(lines, fmt.Sprintf("%-8s %s  +%d -%d  %s", sanitizeTerminal(f.Status), sanitizeTerminal(f.Path), f.Insertions, f.Deletions, state))
		}
		return strings.Join(append(lines, "", m.tr("/review starts this scope. /scope changes it.")), "\n")
	case tuiResults:
		return m.resultContent()
	case tuiFinding:
		return m.findingContent()
	case tuiSessions:
		lines := []string{m.tint(m.tr("Saved sessions"), tuiBrandFace), m.tr("Select a run to inspect its findings."), ""}
		if len(m.sessions) == 0 {
			return strings.Join(append(lines, m.tr("No saved sessions for this repository yet. /review starts one.")), "\n")
		}
		start, end := tuiVisibleRange(len(m.sessions), m.selected, max(1, m.viewport.Height()-4))
		for i := start; i < end; i++ {
			s := m.sessions[i]
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			status := m.tr(describeStatus(s))
			if s.RunManifest == nil && !s.Aborted && s.FailedFiles > 0 {
				status = m.tr("legacy (%d fail)", s.FailedFiles)
			}
			line := prefix + s.StartTime.Format("01-02 15:04") + "  " + status + "  " + m.tr("%d findings", s.TotalComments) + "  " + shortSessionID(s.SessionID)
			lines = append(lines, m.selection(ansi.Truncate(line, m.bodyWidth(), "…"), i == m.selected, m.bodyWidth()))
		}
		return strings.Join(lines, "\n")
	case tuiHelp:
		lines := []string{m.tint(m.tr("Review workspace"), tuiBrandFace), ""}
		for _, cmd := range tuiCommands {
			lines = append(lines, m.tint(cmd.usage, tuiBrandFace), "  "+m.tr(cmd.description))
		}
		lines = append(lines, "", m.tr("Enter opens a selected finding or session. Esc returns or cancels."), m.tr("PgUp/PgDn scroll content. Tab completes commands."), m.tr("Reviews use your existing provider configuration and review rules."), m.tr("Credential commands must work without interactive stdin prompts."))
		return strings.Join(lines, "\n")
	}
	return ""
}

func tuiVisibleRange(total, selected, size int) (int, int) {
	start := max(0, selected-size+1)
	return start, min(total, start+size)
}

func (m reviewTUIModel) resultContent() string {
	lines := []string{m.tint(m.tr("Review results"), tuiBrandFace)}
	if m.resultError != "" && m.result.Manifest == nil && m.result.Status == "" && len(m.result.Comments) == 0 {
		return strings.Join([]string{
			m.tint(m.tr("Review failed"), "#E8AF87"),
			m.tr("No review result was produced."), "",
			m.tr("Use /provider to check model settings, or /review to retry."), "",
			m.tint(m.tr("Run diagnostic"), tuiBrandMuted), m.resultError,
		}, "\n")
	}
	if manifest := m.result.Manifest; manifest != nil {
		coverage := manifest.Coverage
		lines = append(lines, m.tr("Review %s: %d finding(s) across %d selected item(s).", m.tr(string(manifest.TerminalState)), len(m.result.Comments), len(coverage.Selected)))
		lines = append(lines, m.tr("Coverage: %d completed, %d reused, %d failed, %d waived", len(coverage.Completed), len(coverage.Reused), len(coverage.Failed), len(coverage.Waived)))
		if manifest.RunFailure != nil {
			lines = append(lines, m.tr("Reason: %s", m.tr(string(manifest.RunFailure.Classification))))
		}
	} else if m.result.Message != "" {
		lines = append(lines, m.tr(sanitizeTerminal(m.result.Message)))
	} else if m.result.Status == "" {
		lines = append(lines, m.tr("No review result available. /review starts a run."))
	}
	if s := m.result.Summary; s != nil {
		lines = append(lines, m.tr("%d tokens   %s", s.TotalTokens, sanitizeTerminal(s.Elapsed)))
	}
	if m.result.SessionID != "" {
		lines = append(lines, m.tr("Saved session: %s", sanitizeTerminal(m.result.SessionID)))
	}
	lines = append(lines, "")
	start, end := tuiVisibleRange(len(m.result.Comments), m.selected, max(1, (m.viewport.Height()-len(lines)-1)/2))
	for i := start; i < end; i++ {
		c := m.result.Comments[i]
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		line := fmt.Sprintf("%s%s:%d  %s", prefix, sanitizeTerminal(c.Path), c.StartLine, m.tr(sanitizeTerminal(c.Severity)))
		lines = append(lines, m.selection(ansi.Truncate(line, m.bodyWidth(), "…"), i == m.selected, m.bodyWidth()), "  "+ansi.Truncate(strings.ReplaceAll(sanitizeTerminal(c.Content), "\n", " "), max(1, m.bodyWidth()-2), "…"))
	}
	if m.resultError != "" {
		lines = append(lines, "", m.tint(m.tr("Run diagnostic"), "#E8AF87"), m.resultError)
	}
	return strings.Join(lines, "\n")
}

func (m *reviewTUIModel) ensureSelectionVisible() {
	row := 0
	for _, line := range strings.Split(m.content(), "\n") {
		if strings.HasPrefix(ansi.Strip(line), "> ") {
			m.viewport.EnsureVisible(row, 0, 0)
			return
		}
		row += max(1, (ansi.StringWidth(line)+m.bodyWidth()-1)/m.bodyWidth())
	}
}

func (m reviewTUIModel) findingContent() string {
	if m.selected < 0 || m.selected >= len(m.result.Comments) {
		return m.tr("No finding selected.")
	}
	c := m.result.Comments[m.selected]
	lines := []string{m.tint(fmt.Sprintf("%s:%d-%d", sanitizeTerminal(c.Path), c.StartLine, c.EndLine), tuiBrandFace), sanitizeTerminal(buildBadge(c)), "", sanitizeTerminal(c.Content), ""}
	if c.ExistingCode != "" {
		lines = append(lines, m.tint(m.tr("Existing code"), tuiBrandMuted), sanitizeTerminal(c.ExistingCode), "")
	}
	if c.SuggestionCode != "" {
		lines = append(lines, m.tint(m.tr("Suggested change"), tuiBrandFace), sanitizeTerminal(c.SuggestionCode), "")
	}
	return strings.Join(append(lines, m.tr("Esc returns to findings.")), "\n")
}
