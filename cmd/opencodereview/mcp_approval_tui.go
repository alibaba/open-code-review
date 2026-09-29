// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

const maxMCPApprovalArgumentsBytes = 8 << 10

type mcpApprovalPromptRunner func(context.Context, ocrmcp.Invocation, io.Reader, io.Writer) (ocrmcp.Decision, error)

var runMCPApprovalPrompt mcpApprovalPromptRunner = runMCPApprovalBubbleTea

// newMCPRuntimeApprovalPrompter returns the terminal adapter used by one review
// runtime. The broker owns serialization and review-scoped decision caching;
// this adapter owns only one context-bound prompt.
func newMCPRuntimeApprovalPrompter() ocrmcp.Prompter {
	return ocrmcp.PromptFunc(func(ctx context.Context, invocation ocrmcp.Invocation) (ocrmcp.Decision, error) {
		if !mcpInteractiveSession() {
			return ocrmcp.DecisionDenyOnce, ocrmcp.ErrInteractionUnavailable
		}
		return runMCPApprovalPrompt(ctx, invocation, os.Stdin, os.Stderr)
	})
}

type mcpApprovalChoice struct {
	label    string
	decision ocrmcp.Decision
}

var mcpApprovalChoices = []mcpApprovalChoice{
	{label: "Allow once", decision: ocrmcp.DecisionAllowOnce},
	{label: "Allow this review", decision: ocrmcp.DecisionAllowReview},
	{label: "Deny once (default)", decision: ocrmcp.DecisionDenyOnce},
	{label: "Deny this review", decision: ocrmcp.DecisionDenyReview},
}

type mcpApprovalModel struct {
	invocation ocrmcp.Invocation
	arguments  string
	selected   int
	decision   ocrmcp.Decision
	width      int
}

func newMCPApprovalModel(invocation ocrmcp.Invocation) mcpApprovalModel {
	return mcpApprovalModel{
		invocation: invocation,
		arguments:  formatMCPApprovalArguments(invocation.Arguments),
		selected:   2,
	}
}

func (m mcpApprovalModel) Init() tea.Cmd { return nil }

func (m mcpApprovalModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		return m, nil
	case tea.KeyPressMsg:
		switch message.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(mcpApprovalChoices)-1 {
				m.selected++
			}
		case "1", "2", "3", "4":
			m.selected = int(message.String()[0] - '1')
			m.decision = mcpApprovalChoices[m.selected].decision
			return m, tea.Quit
		case "enter":
			m.decision = mcpApprovalChoices[m.selected].decision
			return m, tea.Quit
		case "esc", "ctrl+c", "q":
			m.decision = ocrmcp.DecisionDenyOnce
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m mcpApprovalModel) View() tea.View {
	server := sanitizeMCPText(m.invocation.Grant.ID.Server, 80)
	tool := sanitizeMCPText(m.invocation.Grant.ID.Name, 160)
	alias := sanitizeMCPText(m.invocation.Grant.ModelAlias, 160)
	var view strings.Builder
	fmt.Fprintln(&view, "MCP tool approval required")
	fmt.Fprintf(&view, "Server: %s\nTool: %s\nModel alias: %s\n", server, tool, alias)
	if description := sanitizeMCPApprovalText(m.invocation.Grant.UntrustedDescription, 500); description != "" {
		fmt.Fprintf(&view, "Untrusted server description: %s\n", description)
	}
	if fingerprint := m.invocation.Grant.DefinitionSHA256; len(fingerprint) >= 12 {
		fmt.Fprintf(&view, "Definition: %s...\n", fingerprint[:12])
	}
	fmt.Fprintf(&view, "Arguments (not logged):\n%s\n\n", m.arguments)
	for index, choice := range mcpApprovalChoices {
		cursor := "  "
		if index == m.selected {
			cursor = "> "
		}
		fmt.Fprintf(&view, "%s%d. %s\n", cursor, index+1, choice.label)
	}
	fmt.Fprint(&view, "\nArrows select, Enter confirms. Keys 1-4 answer immediately. Esc denies once.")
	return tea.NewView(view.String())
}

func runMCPApprovalBubbleTea(ctx context.Context, invocation ocrmcp.Invocation, input io.Reader, output io.Writer) (ocrmcp.Decision, error) {
	if err := ctx.Err(); err != nil {
		return ocrmcp.DecisionDenyOnce, err
	}
	program := tea.NewProgram(
		newMCPApprovalModel(invocation),
		tea.WithContext(ctx),
		tea.WithInput(input),
		tea.WithOutput(output),
	)
	result, err := program.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ocrmcp.DecisionDenyOnce, ctxErr
		}
		if errors.Is(err, tea.ErrProgramKilled) {
			return ocrmcp.DecisionDenyOnce, context.Canceled
		}
		return ocrmcp.DecisionDenyOnce, fmt.Errorf("run MCP approval prompt: %w", err)
	}
	model, ok := result.(mcpApprovalModel)
	if !ok || model.decision == "" {
		return ocrmcp.DecisionDenyOnce, nil
	}
	return model.decision, nil
}

func formatMCPApprovalArguments(arguments map[string]any) string {
	data, err := json.MarshalIndent(redactMCPApprovalValue(arguments, ""), "", "  ")
	if err != nil {
		return "(arguments could not be rendered; deny unless independently verified)"
	}
	data = []byte(strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		default:
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}
	}, string(data)))
	if len(data) > maxMCPApprovalArgumentsBytes {
		data = append(data[:maxMCPApprovalArgumentsBytes], []byte("\n... (truncated)")...)
	}
	return string(data)
}

func redactMCPApprovalValue(value any, key string) any {
	if mcpApprovalSensitiveKey(key) {
		return "[redacted]"
	}
	switch value := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(value))
		for childKey, child := range value {
			redacted[childKey] = redactMCPApprovalValue(child, childKey)
		}
		return redacted
	case []any:
		redacted := make([]any, len(value))
		for index, child := range value {
			redacted[index] = redactMCPApprovalValue(child, key)
		}
		return redacted
	default:
		return value
	}
}

func mcpApprovalSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	normalized = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(normalized)
	for _, marker := range []string{
		"token", "secret", "password", "authorization", "cookie", "header",
		"env", "api_key", "apikey", "private_key", "privatekey", "credential",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func sanitizeMCPApprovalText(value string, maxRunes int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}
