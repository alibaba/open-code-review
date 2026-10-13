// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// usesBubbleTea keeps the pure reader/writer fallback for deterministic unit
// tests while ensuring every real terminal management prompt uses the MCP TUI.
func (p *mcpPrompter) usesBubbleTea() bool {
	if p == nil {
		return false
	}
	if mcpHost(p.ctx) != nil {
		return true
	}
	_, inputIsTerminalFile := p.in.(*os.File)
	_, outputIsTerminalFile := p.out.(*os.File)
	return inputIsTerminalFile && outputIsTerminalFile
}

type mcpManagementTextModel struct {
	label     string
	fallback  string
	input     textinput.Model
	value     string
	submitted bool
	cancelled bool
}

func newMCPManagementTextModel(label, fallback string) mcpManagementTextModel {
	input := textinput.New()
	input.Placeholder = fallback
	input.SetWidth(72)
	normalizedLabel := strings.ToLower(label)
	if strings.Contains(normalizedLabel, "environment") || strings.Contains(normalizedLabel, "header") {
		input.EchoMode = textinput.EchoPassword
		input.EchoCharacter = '*'
	}
	if mcpHiddenPrompt(label) {
		input.EchoMode = textinput.EchoPassword
		if fallback != "" {
			input.Placeholder = "(stored value hidden; blank keeps it)"
		}
	}
	_ = input.Focus()
	return mcpManagementTextModel{label: label, fallback: fallback, input: input}
}

func (m mcpManagementTextModel) Init() tea.Cmd { return nil }

func (m mcpManagementTextModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.value = strings.TrimSpace(m.input.Value())
			if m.value == "" {
				m.value = m.fallback
			}
			if strings.EqualFold(m.value, mcpWizardCancelWord) {
				m.cancelled = true
			} else {
				m.submitted = true
			}
			return m, tea.Quit
		case "esc", "ctrl+c", "ctrl+d":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(message)
	return m, command
}

func (m mcpManagementTextModel) View() tea.View {
	var view strings.Builder
	fmt.Fprintln(&view, tuiTitleStyle.Render("  MCP setup"))
	fmt.Fprintln(&view)
	fmt.Fprintln(&view, m.label)
	fmt.Fprintln(&view, m.input.View())
	fmt.Fprintln(&view)
	fmt.Fprint(&view, tuiHelpStyle.Render("Enter confirms. Esc or Ctrl-C cancels without saving."))
	return tea.NewView(view.String())
}

type mcpManagementConfirmModel struct {
	label     string
	selected  int // 0 = No, 1 = Yes; No is deliberately the default.
	submitted bool
	cancelled bool
}

func newMCPManagementConfirmModel(label string) mcpManagementConfirmModel {
	return mcpManagementConfirmModel{label: label}
}

func (m mcpManagementConfirmModel) Init() tea.Cmd { return nil }

func (m mcpManagementConfirmModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "left", "h", "up", "k":
		m.selected = 0
	case "right", "l", "down", "j":
		m.selected = 1
	case "y", "Y":
		m.selected = 1
		m.submitted = true
		return m, tea.Quit
	case "n", "N":
		m.selected = 0
		m.submitted = true
		return m, tea.Quit
	case "enter":
		m.submitted = true
		return m, tea.Quit
	case "esc", "ctrl+c", "ctrl+d", "q":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m mcpManagementConfirmModel) View() tea.View {
	choices := []string{"No", "Yes"}
	for index := range choices {
		if index == m.selected {
			choices[index] = tuiSelectedItemStyle.Render(tuiCursor + " " + choices[index])
		} else {
			choices[index] = "  " + choices[index]
		}
	}
	var view strings.Builder
	fmt.Fprintln(&view, tuiTitleStyle.Render("  MCP confirmation"))
	fmt.Fprintln(&view)
	fmt.Fprintln(&view, m.label)
	fmt.Fprintln(&view, strings.Join(choices, "    "))
	fmt.Fprintln(&view)
	fmt.Fprint(&view, tuiHelpStyle.Render("Arrows select, Enter confirms. Y/N answers immediately. Esc cancels. Default: No."))
	return tea.NewView(view.String())
}

func runMCPManagementTextPrompt(
	ctx context.Context,
	label, fallback string,
	input io.Reader,
	output io.Writer,
) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result, err := runMCPModel(ctx, newMCPManagementTextModel(label, fallback), input, output)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, tea.ErrProgramKilled) {
			return "", errMCPPromptCancelled
		}
		return "", fmt.Errorf("run MCP setup prompt: %w", err)
	}
	model, ok := result.(mcpManagementTextModel)
	if !ok || model.cancelled || !model.submitted {
		return "", errMCPPromptCancelled
	}
	return model.value, nil
}

func runMCPManagementConfirmPrompt(
	ctx context.Context,
	label string,
	input io.Reader,
	output io.Writer,
) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	result, err := runMCPModel(ctx, newMCPManagementConfirmModel(label), input, output)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		if errors.Is(err, tea.ErrProgramKilled) {
			return false, nil
		}
		return false, fmt.Errorf("run MCP confirmation prompt: %w", err)
	}
	model, ok := result.(mcpManagementConfirmModel)
	if !ok || model.cancelled || !model.submitted {
		return false, nil
	}
	return model.selected == 1, nil
}
