// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

type mcpDashboardItem struct{ label, action, identity string }
type mcpDashboardCheck struct {
	tools  []ocrmcp.DiscoveredTool
	at     time.Time
	failed bool
}

// The dashboard owns navigation and ephemeral discovery results, never a live
// client. Existing transactional commands remain the only configuration writers.
type mcpDashboardModel struct {
	cfg                                *Config
	server, tool, page, action, notice string
	cursor, height, width              int
	checks                             map[string]mcpDashboardCheck
}

func (m mcpDashboardModel) Init() tea.Cmd { return nil }

func (m mcpDashboardModel) items() []mcpDashboardItem {
	var items []mcpDashboardItem
	switch m.page {
	case "server":
		verb := "Enable"
		if optionalBool(m.cfg.MCPServers[m.server].Enabled, true) {
			verb = "Disable"
		}
		return []mcpDashboardItem{
			{"Tools", "tools", ""},
			{"Test connection", "discover", ""},
			{"Edit connection", "edit", ""},
			{"Permissions", "permissions", ""},
			{verb + " server", strings.ToLower(verb), ""},
			{"Remove server", "remove", ""},
			{"Back to servers", "back", ""},
		}
	case "tools":
		for _, name := range m.toolNames() {
			items = append(items, mcpDashboardItem{m.safe(name) + "  |  " + m.toolState(name), "tool", name})
		}
		return append(items, mcpDashboardItem{"Refresh tools (connect)", "discover", ""}, mcpDashboardItem{"Back", "back", ""})
	case "tool":
		if slices.Contains(m.cfg.MCPServers[m.server].Tools, m.tool) {
			items = append(items, mcpDashboardItem{"Disable tool (works offline)", "revoke", ""}, mcpDashboardItem{"Change permission", "tool-permission", ""}, mcpDashboardItem{"Review current definition (reconnect; resets to ask)", "select-tool", ""})
		} else {
			items = append(items, mcpDashboardItem{"Enable tool with approval (reconnect)", "select-tool", ""})
		}
		return append(items, mcpDashboardItem{"Back to tools", "back", ""})
	default:
		names := make([]string, 0, len(m.cfg.MCPServers))
		for name := range m.cfg.MCPServers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			s := mcpServerSummaryFor(name, m.cfg, m.cfg.MCPServers[name])
			items = append(items, mcpDashboardItem{fmt.Sprintf("%s  |  %s  |  %s  |  %d selected", m.safe(name), m.safe(s.Transport), s.Status, len(s.Tools)), "server", name})
		}
		return append(items, mcpDashboardItem{"Add server", "add", ""}, mcpDashboardItem{"Import (JSON / TOML)", "import", ""}, mcpDashboardItem{"Permissions / timeout", "global", ""}, mcpDashboardItem{"Quit", "quit", ""})
	}
}

func (m mcpDashboardModel) toolNames() []string {
	names := append([]string(nil), m.cfg.MCPServers[m.server].Tools...)
	for _, tool := range m.checks[m.server].tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return slices.Compact(names)
}

func (m mcpDashboardModel) toolState(name string) string {
	s := m.cfg.MCPServers[m.server]
	if !slices.Contains(s.Tools, name) {
		return "Not enabled; hidden from model"
	}
	global := ocrmcp.MCPConfig{}
	if m.cfg.MCP != nil {
		global = *m.cfg.MCP
	}
	permission, err := ocrmcp.ResolvePermission(global, s, name)
	if err != nil {
		return "Blocked: invalid policy"
	}
	if permission == ocrmcp.PermissionDeny {
		return "Blocked by policy or disabled server"
	}
	check, checked := m.checks[m.server]
	if checked && !check.failed {
		idx := slices.IndexFunc(check.tools, func(t ocrmcp.DiscoveredTool) bool { return t.Name == name })
		if idx < 0 || check.tools[idx].DefinitionSHA256 != s.ToolDefinitionSHA256[name] {
			return "Needs review: missing or changed definition"
		}
	}
	if !mcpToolFingerprintValid(s, name) {
		return "Needs review: no accepted fingerprint"
	}
	if permission == ocrmcp.PermissionAllow {
		return "No prompt (allow)"
	}
	return "Ask before calling; hidden in CI"
}

func (m mcpDashboardModel) back() mcpDashboardModel {
	identity := ""
	switch m.page {
	case "tool":
		identity = m.tool
		m.page = "tools"
	case "tools":
		m.page = "server"
	case "server":
		identity = m.server
		m.page = ""
		m.server = ""
	default:
		m.action = "quit"
	}
	m.cursor = 0
	for i, item := range m.items() {
		if identity != "" && item.identity == identity {
			m.cursor = i
			break
		}
	}
	return m
}

func (m mcpDashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "tab", "shift+tab":
			if m.page == "" {
				if m.cursor < len(m.cfg.MCPServers) {
					m.cursor = len(m.cfg.MCPServers)
				} else {
					m.cursor = 0
				}
			}
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.items())-1, m.cursor+1)
		case "esc", "left", "ctrl+b":
			m = m.back()
		case "ctrl+c", "ctrl+d", "q":
			m.action = "quit"
		case "enter":
			items := m.items()
			item := items[min(m.cursor, len(items)-1)]
			switch item.action {
			case "server":
				m.server = item.identity
				m.page = "server"
				m.cursor = 0
			case "tools":
				m.page = "tools"
				m.cursor = 0
			case "tool":
				m.tool = item.identity
				m.page = "tool"
				m.cursor = 0
			case "back":
				m = m.back()
			default:
				m.action = item.action
			}
		}
	}
	if m.action != "" {
		return m, tea.Quit
	}
	return m, nil
}

// Use bounded text and never put connection values in the navigation shell.
func (m mcpDashboardModel) safe(value string) string {
	for _, server := range m.cfg.MCPServers {
		value = ocrmcp.RedactConfigText(server, value)
	}
	return sanitizeMCPText(value, 100)
}

func (m mcpDashboardModel) View() tea.View {
	var out strings.Builder
	fmt.Fprintln(&out, tuiTitleStyle.Render("  MCP servers"))
	if m.server == "" {
		if len(m.cfg.MCPServers) == 0 {
			fmt.Fprintln(&out, "No servers yet.")
		}
	} else {
		s := mcpServerSummaryFor(m.server, m.cfg, m.cfg.MCPServers[m.server])
		fmt.Fprintf(&out, "Server: %s | %s | %s | %d selected tool(s)\n", m.safe(m.server), m.safe(s.Transport), s.Status, len(s.Tools))
		fmt.Fprintf(&out, "Connection: %s\n", m.safe(s.Endpoint))
		if c, ok := m.checks[m.server]; ok {
			result := fmt.Sprintf("success, %d tools", len(c.tools))
			if c.failed {
				result = "failed; retry"
			}
			fmt.Fprintf(&out, "Last check: %s (%s)\n", result, c.at.Format("15:04:05"))
		} else {
			fmt.Fprintln(&out, "Connection: not checked this session.")
		}
		if m.page == "tool" {
			fmt.Fprintf(&out, "Tool: %s\nEffective: %s\n", m.safe(m.tool), m.toolState(m.tool))
			for _, t := range m.checks[m.server].tools {
				if t.Name == m.tool {
					fmt.Fprintf(&out, "Untrusted server description: %s\n", sanitizeMCPText(t.Description, 180))
				}
			}
		}
	}
	if m.notice != "" {
		fmt.Fprintln(&out, m.notice)
	}
	fmt.Fprintln(&out)
	items := m.items()
	size := max(2, m.height-14)
	start := max(0, m.cursor-size+1)
	for i := start; i < min(len(items), start+size); i++ {
		if m.page == "" && i == len(m.cfg.MCPServers) && i > start {
			// A blank row distinguishes actions from configured server records.
			fmt.Fprintln(&out)
		}
		if m.page == "" && (i == start || i == len(m.cfg.MCPServers)) {
			section := fmt.Sprintf("CONFIGURED SERVERS (%d)", len(m.cfg.MCPServers))
			if i >= len(m.cfg.MCPServers) {
				section = "MANAGEMENT ACTIONS"
			}
			fmt.Fprintln(&out, tuiTitleStyle.Render("  "+section))
		}
		line := "  " + items[i].label
		if i == m.cursor {
			line = tuiSelectedItemStyle.Render("> " + items[i].label)
		}
		fmt.Fprintln(&out, line)
	}
	if m.page == "" {
		fmt.Fprintln(&out, "\n↑/↓ Select · Tab Section · Enter Open · Esc Quit")
	} else {
		fmt.Fprintf(&out, "\n%d/%d  ↑/↓ Select · Enter Open · Esc Back\n", m.cursor+1, len(items))
	}
	lines := strings.Split(out.String(), "\n")
	if m.width > 0 {
		for i := range lines {
			lines[i] = ansi.Truncate(lines[i], m.width, "...")
		}
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

func runMCPDashboard(cmd *cobra.Command) error {
	cfg, err := loadReadOnlyMCPConfig()
	if err != nil {
		return err
	}
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	m := mcpAppModel{
		dashboard: mcpDashboardModel{cfg: cfg, width: 80, height: 24, checks: make(map[string]mcpDashboardCheck)},
		command:   cmd, ctx: ctx, host: &mcpUIHost{requests: make(chan mcpPageRequest)},
	}
	_, err = tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.ErrOrStderr())).Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return parent.Err()
	}
	return err
}

func (m *mcpDashboardModel) perform(cmd *cobra.Command) error {
	// Give reused commands the correct verb for confirmation and success text.
	action := &cobra.Command{Use: m.action}
	action.SetContext(cmd.Context())
	action.SetIn(cmd.InOrStdin())
	action.SetOut(cmd.OutOrStdout())
	action.SetErr(cmd.ErrOrStderr())
	switch m.action {
	case "add":
		return runMCPConnectionWizard(action, "", false, mcpAddOptions{})
	case "import":
		return runMCPImport(action, "", false)
	case "edit":
		delete(m.checks, m.server)
		return runMCPConnectionWizard(action, m.server, true, mcpAddOptions{})
	case "permissions":
		return runMCPPermissions(action, m.server, mcpPermissionsOptions{})
	case "global":
		return runMCPPermissions(action, "", mcpPermissionsOptions{})
	case "enable", "disable":
		return runMCPEnable(action, m.server, m.action == "enable", false)
	case "remove":
		delete(m.checks, m.server)
		return runMCPRemove(action, m.server, false)
	case "revoke":
		return runMCPTools(action, m.server, mcpToolsOptions{disable: []string{m.tool}})
	case "select-tool":
		delete(m.checks, m.server)
		return runMCPTools(action, m.server, mcpToolsOptions{enable: []string{m.tool}})
	case "tool-permission":
		p := newMCPPrompter(action)
		current := m.cfg.MCPServers[m.server].ToolPermissions[m.tool]
		if current == "" {
			current = ocrmcp.PermissionInherit
		}
		value, err := p.choose("Tool permission: ask prompts; allow skips prompts; deny blocks; inherit uses parent", string(current), []string{"ask", "allow", "deny", "inherit"})
		if err != nil {
			if errors.Is(err, errMCPPromptCancelled) {
				return nil
			}
			return err
		}
		return runMCPPermissions(action, m.server, mcpPermissionsOptions{toolPermissions: []string{m.tool + "=" + value}})
	case "discover":
		server := m.cfg.MCPServers[m.server]
		printMCPConnectionPreview(action.ErrOrStderr(), server)
		ok, err := newMCPPrompter(action).confirm("Test connection and list tools? No business tool is called or enabled")
		if err != nil || !ok {
			return err
		}
		fmt.Fprintln(action.ErrOrStderr(), "Checking connection and listing tools (up to 30 seconds)...")
		tools, err := discoverToolsWithTimeout(action.Context(), m.server, server)
		m.checks[m.server] = mcpDashboardCheck{tools: tools, at: time.Now(), failed: err != nil}
		return err
	}
	return nil
}
