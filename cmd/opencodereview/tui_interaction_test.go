// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/charmbracelet/x/ansi"
)

func pressTUI(m reviewTUIModel, code rune) (reviewTUIModel, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: code})
	return next.(reviewTUIModel), cmd
}

func TestTUIPaletteSelectsAndCompletesCommands(t *testing.T) {
	m := testReviewTUI()
	m.input.SetValue("/set")
	m.refresh(false)
	if choices := m.commandChoices(); len(choices) != 1 || choices[0].name != "/settings" {
		t.Fatalf("filtered choices = %v", choices)
	}
	m, _ = pressTUI(m, tea.KeyEnter)
	if m.screen != tuiSettings || m.input.Value() != "" {
		t.Fatal("Enter did not open selected settings command")
	}
	m, _ = pressTUI(m, tea.KeyEscape)
	m.input.SetValue("/sco")
	m.menuDismissed = false
	m, _ = pressTUI(m, tea.KeyTab)
	if m.input.Value() != "/scope " || m.busy {
		t.Fatal("Tab did not prepare the command arguments")
	}
	m.input.SetValue("/")
	m.menuDismissed = false
	m, _ = pressTUI(m, tea.KeyDown)
	if m.menuSelected != 1 {
		t.Fatal("Down did not move the command selection")
	}
	m, _ = pressTUI(m, tea.KeyEscape)
	if len(m.commandChoices()) != 0 || m.input.Value() != "/" {
		t.Fatal("Escape should dismiss the menu without losing the draft")
	}
}

func TestTUIHistoryRestoresDraft(t *testing.T) {
	m := testReviewTUI()
	m.rememberCommand("/scope range main HEAD")
	m.rememberCommand("Previous background")
	m.input.SetValue("Current draft")
	m, _ = pressTUI(m, tea.KeyUp)
	if m.input.Value() != "Previous background" {
		t.Fatal("history did not move up")
	}
	m, _ = pressTUI(m, tea.KeyUp)
	if m.input.Value() != "/scope range main HEAD" {
		t.Fatal("history skipped oldest command")
	}
	m, _ = pressTUI(m, tea.KeyDown)
	m, _ = pressTUI(m, tea.KeyDown)
	if m.input.Value() != "Current draft" {
		t.Fatal("history navigation lost the draft")
	}
}

func TestTUISettingsApplyAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			m := testReviewTUI()
			m.screen = tuiResults
			m.opts.background = "Review this exact context"
			var saved tuiPreferences
			m.services.savePreferences = func(p tuiPreferences) error {
				saved = p
				if fail {
					return errors.New("permission denied")
				}
				return nil
			}
			next, _ := m.command("/settings")
			m = next.(reviewTUIModel)
			m, _ = pressTUI(m, tea.KeyDown)
			if m.preferences.Language != "en" {
				t.Fatal("moving selection applied settings")
			}
			m, cmd := pressTUI(m, tea.KeyEnter)
			if cmd == nil || !m.savingPreferences {
				t.Fatal("selection was not saved")
			}
			next, _ = m.Update(cmd())
			m = next.(reviewTUIModel)
			if saved.Language != "zh-CN" || m.savingPreferences {
				t.Fatal("wrong saved language or pending state")
			}
			if fail {
				if m.preferences.Language != "en" || !m.isError || !strings.Contains(m.notice, "permission denied") {
					t.Fatal("failed save changed the language or hid the error")
				}
			} else if m.preferences.Language != "zh-CN" || m.input.Placeholder == "Type a command or describe your changes" {
				t.Fatal("saved language did not update the interface")
			}
			m, _ = pressTUI(m, tea.KeyEscape)
			if m.screen != tuiResults || m.opts.background != "Review this exact context" {
				t.Fatal("settings lost the previous view or review context")
			}
		})
	}
}

func TestTUISettingsCancelDoesNotSave(t *testing.T) {
	m := testReviewTUI()
	m.services.savePreferences = func(tuiPreferences) error {
		t.Fatal("Escape saved an unconfirmed selection")
		return nil
	}
	next, _ := m.command("/settings")
	m = next.(reviewTUIModel)
	m, _ = pressTUI(m, tea.KeyDown)
	m, cmd := pressTUI(m, tea.KeyEscape)
	if cmd != nil || m.preferences.Language != "en" || m.screen != tuiHome {
		t.Fatal("settings cancel changed the language or view")
	}
}

func TestTUITranslatedLayoutsAndPaletteFit(t *testing.T) {
	for _, locale := range tuiLanguageIDs {
		for _, size := range [][2]int{{110, 36}, {80, 24}, {40, 18}, {24, 10}} {
			t.Run(fmt.Sprintf("%s-%v", locale, size), func(t *testing.T) {
				m := testReviewTUI()
				m.preferences.Language = locale
				next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m = next.(reviewTUIModel)
				for _, screen := range []tuiScreen{tuiHome, tuiSettings, tuiHelp} {
					m.screen = screen
					m.notice = "Settings saved."
					m.input.SetValue("/")
					m.refresh(false)
					view := m.View().Content
					lines := strings.Split(view, "\n")
					if len(lines) > size[1] {
						t.Fatalf("screen %d has %d rows, want <= %d", screen, len(lines), size[1])
					}
					for _, row := range lines {
						if ansi.StringWidth(row) > size[0] {
							t.Fatalf("screen %d exceeds width: %q", screen, row)
						}
					}
					if ansi.Strip(view) != view {
						t.Fatal("monochrome view contains styles")
					}
				}
			})
		}
	}
}

func TestTUIWelcomeDoesNotPinPromptBelowEmptySpace(t *testing.T) {
	m := testReviewTUI()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 45})
	m = next.(reviewTUIModel)
	if rows := strings.Split(m.View().Content, "\n"); len(rows) >= 30 {
		t.Fatalf("welcome prompt is separated by empty space: %d rows", len(rows))
	}
}

func TestTUISettingsEscRestoresTopOnShortTerminal(t *testing.T) {
	m := testReviewTUI()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = next.(reviewTUIModel)
	next, _ = m.command("/settings")
	m = next.(reviewTUIModel)
	for i := 0; i < 4; i++ {
		m, _ = pressTUI(m, tea.KeyDown)
	}
	m, _ = pressTUI(m, tea.KeyEscape)
	if m.screen != tuiHome || m.viewport.YOffset() != 0 {
		t.Fatal("leaving settings scrolled the restored welcome view")
	}
}

func TestTUISelectedFindingRemainsVisibleWithLongDiagnostic(t *testing.T) {
	m := testReviewTUI()
	m.screen = tuiResults
	m.result = jsonOutput{Status: "partial", SessionID: "saved", Comments: []model.LlmComment{
		{Path: "first.go", Content: "First finding"},
		{Path: "second.go", Content: "Second finding"},
	}}
	m.resultError = strings.Repeat("Long cancellation diagnostic\n", 10)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = next.(reviewTUIModel)
	m, _ = pressTUI(m, tea.KeyDown)
	if m.selected != 1 || !strings.Contains(m.viewport.View(), "second.go") {
		t.Fatal("selected finding is hidden behind the diagnostic")
	}
}
