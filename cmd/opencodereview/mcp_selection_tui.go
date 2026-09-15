// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

func sortedMCPTools(tools []ocrmcp.DiscoveredTool) []ocrmcp.DiscoveredTool {
	ordered := append([]ocrmcp.DiscoveredTool(nil), tools...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	return ordered
}

func mcpURLIsLoopback(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && mcpLoopbackHost(parsed.Hostname())
}

// Both setup and allowlist editing use the same ordered identity mapping.
func mcpRenderToolChecklist(out *strings.Builder, tools []ocrmcp.DiscoveredTool, selected map[string]bool, cursor, pageSize int) {
	if len(tools) == 0 {
		fmt.Fprintln(out, "No tools found. Save disabled and try again later.")
		return
	}
	start := max(0, cursor-pageSize+1)
	end := min(len(tools), start+pageSize)
	for i := start; i < end; i++ {
		marker := "[ ]"
		if selected[tools[i].Name] {
			marker = "[x]"
		}
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		fmt.Fprintf(out, "%s%s %s\n", prefix, marker, sanitizeMCPText(tools[i].Name, 100))
	}
	fmt.Fprintf(out, "Tool %d/%d\n%s\n", cursor+1, len(tools), sanitizeMCPText(tools[cursor].Description, 240))
}

type mcpSelectionModel struct {
	label     string
	choices   []string
	cursor    int
	height    int
	confirmed bool
}

func (m mcpSelectionModel) Init() tea.Cmd { return nil }
func (m mcpSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(max(0, len(m.choices)-1), m.cursor+1)
		case "enter":
			m.confirmed = len(m.choices) > 0
			return m, tea.Quit
		case "esc", "ctrl+c", "ctrl+d":
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m mcpSelectionModel) View() tea.View {
	var out strings.Builder
	fmt.Fprintln(&out, tuiTitleStyle.Render("  MCP manager"))
	fmt.Fprintln(&out, m.label)
	start := max(0, m.cursor-max(3, m.height-7)+1)
	end := min(len(m.choices), start+max(3, m.height-7))
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		fmt.Fprintln(&out, prefix+sanitizeMCPText(m.choices[i], 120))
	}
	fmt.Fprint(&out, "\nArrows navigate. Enter confirms. Esc cancels.")
	return tea.NewView(out.String())
}

func (p *mcpPrompter) choose(label, fallback string, choices []string) (string, error) {
	if !p.usesBubbleTea() {
		return p.prompt(label, fallback)
	}
	model := mcpSelectionModel{label: label, choices: choices, height: 24}
	for i, value := range choices {
		if value == fallback {
			model.cursor = i
		}
	}
	result, err := runMCPModel(p.ctx, model, p.in, p.out)
	if err != nil {
		return "", err
	}
	final, ok := result.(mcpSelectionModel)
	if !ok || !final.confirmed {
		return "", errMCPPromptCancelled
	}
	return final.choices[final.cursor], nil
}

// Reuse setup's checklist and terminal lifecycle for allowlist editing.
func runMCPToolSelection(ctx context.Context, tools []ocrmcp.DiscoveredTool, in io.Reader, out io.Writer) ([]ocrmcp.DiscoveredTool, error) {
	model := mcpSetupModel{ctx: ctx, screen: mcpSetupTools, tools: sortedMCPTools(tools), selected: make(map[string]bool), width: 80, height: 24}
	result, err := runMCPModel(ctx, mcpToolSelectionModel{model}, in, out)
	if err != nil {
		return nil, err
	}
	final, ok := result.(mcpToolSelectionModel)
	if !ok || final.cancelled || !final.confirmed {
		return nil, errMCPPromptCancelled
	}
	var selected []ocrmcp.DiscoveredTool
	for _, item := range final.tools {
		if final.selected[item.Name] {
			selected = append(selected, item)
		}
	}
	return selected, nil
}

type mcpToolSelectionModel struct{ mcpSetupModel }

func (m mcpToolSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		m.confirmed = true
		return m, tea.Quit
	}
	updated, command := m.mcpSetupModel.Update(msg)
	m.mcpSetupModel = updated.(mcpSetupModel)
	return m, command
}
