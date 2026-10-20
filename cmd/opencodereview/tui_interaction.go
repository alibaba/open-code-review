// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type tuiCommand struct {
	name        string
	description string
	usage       string
	immediate   bool
}

var tuiCommands = []tuiCommand{
	{"/review", "Start a review of the selected scope", "/review", true},
	{"/preview", "List files without calling the model", "/preview", true},
	{"/scope", "Select the changes to review", "/scope workspace | range BASE HEAD | commit REF", false},
	{"/settings", "Change the interface language", "/settings", true},
	{"/model", "Configure models or override this run", "/model [NAME]", true},
	{"/provider", "Configure providers or override this run", "/provider [NAME]", true},
	{"/login", "Sign in with ChatGPT", "/login", true},
	{"/sessions", "Browse previous runs in this repository", "/sessions", true},
	{"/resume", "Prepare a saved range/commit for resuming", "/resume [SESSION_ID]", true},
	{"/background", "Set context (plain text also works)", "/background TEXT", false},
	{"/effort", "Select review depth", "/effort low|medium|high", false},
	{"/budget", "Set total token budget; 0 is unlimited", "/budget TOKENS", false},
	{"/results", "Return to the latest findings", "/results", true},
	{"/logs", "View the last 250 progress lines", "/logs", true},
	{"/help", "Show all commands", "/help", true},
	{"/cancel", "Cancel the running task and save results", "/cancel", true},
	{"/quit", "Exit (waits for an active task to stop)", "/quit", true},
}

func (m reviewTUIModel) commandChoices() []tuiCommand {
	value := m.input.Value()
	if m.menuDismissed || m.screen == tuiSettings || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, " \t\n") {
		return nil
	}
	var choices []tuiCommand
	for _, cmd := range tuiCommands {
		if strings.HasPrefix(cmd.name, value) {
			choices = append(choices, cmd)
		}
	}
	return choices
}

func (m *reviewTUIModel) rememberCommand(line string) {
	if len(m.history) == 0 || m.history[len(m.history)-1] != line {
		m.history = append(m.history, line)
		if len(m.history) > 100 {
			m.history = m.history[len(m.history)-100:]
		}
	}
	m.historyIndex, m.historyDraft = len(m.history), ""
}

func (m *reviewTUIModel) moveHistory(delta int) {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex == len(m.history) {
		m.historyDraft = m.input.Value()
	}
	m.historyIndex = max(0, min(len(m.history), m.historyIndex+delta))
	value := m.historyDraft
	if m.historyIndex < len(m.history) {
		value = m.history[m.historyIndex]
	}
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.menuDismissed = true
}

func (m reviewTUIModel) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = m.settingsReturn
		m.notice, m.isError = "", false
		m.viewport.GotoTop()
	case "up", "k":
		if !m.savingPreferences {
			m.settingsIndex = (m.settingsIndex + len(tuiLanguageIDs) - 1) % len(tuiLanguageIDs)
		}
	case "down", "j":
		if !m.savingPreferences {
			m.settingsIndex = (m.settingsIndex + 1) % len(tuiLanguageIDs)
		}
	case "enter":
		if m.savingPreferences {
			return m, nil
		}
		prefs := tuiPreferences{Language: tuiLanguageIDs[m.settingsIndex]}
		m.savingPreferences = true
		m.notice, m.isError = m.tr("Saving settings…"), false
		m.refresh(false)
		return m, m.work(func() tea.Msg {
			return tuiPreferencesSavedMsg{preferences: prefs, err: m.services.savePreferences(prefs)}
		})
	}
	m.refresh(false)
	if m.screen == tuiSettings {
		m.viewport.EnsureVisible(4+m.settingsIndex, 0, 0)
	}
	return m, nil
}
