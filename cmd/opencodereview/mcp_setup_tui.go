// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/spf13/cobra"
)

type mcpSetupScreen int

const (
	mcpSetupName mcpSetupScreen = iota
	mcpSetupTransport
	mcpSetupEndpoint
	mcpSetupArguments
	mcpSetupCredentials
	mcpSetupConnect
	mcpSetupTools
	mcpSetupSave
)

// The entire connection transaction has one terminal owner. Commands capture
// immutable connection copies; only Update mutates the draft or selection.
type mcpSetupModel struct {
	ctx            context.Context
	config         *Config
	name           string
	editing        bool
	server         MCPServerConfig
	screen         mcpSetupScreen
	history        []mcpSetupScreen
	input          textinput.Model
	choice         int
	credentialKey  string
	credentialEnv  string
	credentialPart int
	tools          []ocrmcp.DiscoveredTool
	selected       map[string]bool
	busy           bool
	confirmed      bool
	cancelled      bool
	errorText      string
	width          int
	height         int
}

type mcpSetupDiscovered struct {
	tools []ocrmcp.DiscoveredTool
	err   error
}

func newMCPSetupModel(ctx context.Context, config *Config, name string, editing bool, opts mcpAddOptions) (mcpSetupModel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	draft, err := cloneAppConfig(config)
	if err != nil {
		return mcpSetupModel{}, err
	}
	m := mcpSetupModel{ctx: ctx, config: draft, name: name, editing: editing, selected: make(map[string]bool), width: 80, height: 24}
	if editing {
		var ok bool
		m.server, ok = draft.MCPServers[name]
		if !ok {
			return m, fmt.Errorf("MCP server %q not found", name)
		}
	} else {
		headers, err := parseMCPHeaderFlags(opts.headers)
		if err != nil {
			return m, err
		}
		m.server = MCPServerConfig{Type: opts.transport, Command: opts.command, Args: append([]string(nil), opts.args...), Env: append([]string(nil), opts.env...), URL: opts.url, Headers: headers, AllowInsecureHTTP: opts.allowInsecureHTTP, DefaultPermission: ocrmcp.PermissionInherit}
		if err := validateNewMCPCredentialTemplates(m.server); err != nil {
			return m, err
		}
	}
	if m.server.Type == "" {
		m.server.Type = "stdio"
	}
	m.resetInput()
	return m, nil
}

func (m mcpSetupModel) Init() tea.Cmd { return textinput.Blink }

func (m *mcpSetupModel) resetInput() {
	m.input = textinput.New()
	m.input.CharLimit = 8192
	m.input.SetWidth(max(20, min(m.width-6, 76)))
	_ = m.input.Focus()
	m.choice = 0
	m.errorText = ""
	m.credentialPart = 0
	m.credentialKey, m.credentialEnv = "", ""
	switch m.screen {
	case mcpSetupName:
		m.input.SetValue(m.name)
	case mcpSetupTransport:
		if m.server.Type == "remote" {
			m.choice = 1
		}
	case mcpSetupEndpoint:
		if m.server.Type == "remote" {
			// Do not render a stored URL containing query credentials, even as a placeholder.
			m.input.EchoMode = textinput.EchoPassword
			m.input.Placeholder = "https://example.com/mcp (hidden)"
		} else {
			m.input.SetValue(m.server.Command)
		}
	case mcpSetupArguments:
		m.input.EchoMode = textinput.EchoPassword
		m.input.Placeholder = "Argument (hidden)"
	case mcpSetupCredentials:
		m.input.Placeholder = "Name (optional)"
	}
}

func (m *mcpSetupModel) next(screen mcpSetupScreen) {
	m.history = append(m.history, m.screen)
	m.screen = screen
	m.resetInput()
}

func (m *mcpSetupModel) back() {
	if len(m.history) == 0 {
		return
	}
	m.screen = m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.resetInput()
}

func (m mcpSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(20, min(msg.Width-6, 76)))
		return m, nil
	case mcpSetupDiscovered:
		m.busy = false
		if msg.err != nil {
			m.errorText = sanitizeMCPDiscoveryError(m.name, msg.err).Error() + ". Edit settings with Ctrl-B, or select Connect to retry."
			m.choice = 0
			return m, nil
		}
		m.tools = sortedMCPTools(msg.tools)
		m.selected = make(map[string]bool)
		m.next(mcpSetupTools)
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "esc" || key == "ctrl+c" || key == "ctrl+d" {
			m.cancelled = true
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if key == "ctrl+b" {
			m.back()
			return m, nil
		}
		if m.screen == mcpSetupTools {
			switch key {
			case "up", "k":
				m.choice = max(0, m.choice-1)
			case "down", "j":
				m.choice = min(max(0, len(m.tools)-1), m.choice+1)
			case "space", " ":
				if len(m.tools) > 0 {
					name := m.tools[m.choice].Name
					m.selected[name] = !m.selected[name]
				}
			case "enter":
				m.next(mcpSetupSave)
			}
			return m, nil
		}
		if m.screen == mcpSetupTransport || m.screen == mcpSetupConnect || m.screen == mcpSetupSave {
			switch key {
			case "up", "left", "k":
				m.choice = max(0, m.choice-1)
			case "down", "right", "j":
				limit := 1
				if m.screen == mcpSetupConnect {
					limit = 2
				}
				m.choice = min(limit, m.choice+1)
			case "enter":
				return m.advance()
			}
			return m, nil
		}
		if key == "ctrl+x" {
			if m.screen == mcpSetupArguments && len(m.server.Args) > 0 {
				m.server.Args = m.server.Args[:len(m.server.Args)-1]
			}
			if m.screen == mcpSetupCredentials {
				if m.server.Type == "remote" {
					names := mcpHeaderNames(m.server.Headers)
					if len(names) > 0 {
						delete(m.server.Headers, names[len(names)-1])
					}
				} else if len(m.server.Env) > 0 {
					m.server.Env = m.server.Env[:len(m.server.Env)-1]
				}
			}
			return m, nil
		}
		if key == "enter" {
			return m.advance()
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(msg)
	return m, command
}

func (m mcpSetupModel) advance() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	m.errorText = ""
	switch m.screen {
	case mcpSetupName:
		if m.editing && value != m.name {
			m.errorText = "Server names cannot be changed here; add a new connection instead."
			break
		}
		if err := validateMCPServerName(value); err != nil {
			m.errorText = "Use 1-64 letters, numbers, hyphens or underscores; start with a letter or number."
			break
		}
		if _, exists := m.config.MCPServers[value]; exists && !m.editing {
			m.errorText = "That server already exists. Choose another name or cancel and use Edit."
			break
		}
		m.name = value
		m.next(mcpSetupTransport)
	case mcpSetupTransport:
		kind := "stdio"
		if m.choice == 1 {
			kind = "remote"
		}
		if kind != m.server.Type {
			m.server = MCPServerConfig{Type: kind, DefaultPermission: ocrmcp.PermissionInherit}
		}
		m.next(mcpSetupEndpoint)
	case mcpSetupEndpoint:
		candidate := m.server
		if candidate.Type == "remote" {
			if value != "" {
				candidate.URL = value
				candidate.AllowInsecureHTTP = false
			}
		} else {
			candidate.Command = value
		}
		// Structural validation here does not authorize an insecure connection.
		validation := candidate
		if validation.Type == "remote" {
			validation.AllowInsecureHTTP = true
		}
		if err := validateMCPConnection(validation); err != nil {
			m.errorText = "Invalid connection. Use an executable or an http(s) MCP URL without userinfo or a fragment."
			break
		}
		m.server = candidate
		if candidate.Type == "stdio" {
			m.next(mcpSetupArguments)
		} else {
			m.next(mcpSetupCredentials)
		}
	case mcpSetupArguments:
		if value == "" {
			m.next(mcpSetupCredentials)
			break
		}
		m.server.Args = append(m.server.Args, m.input.Value())
		m.input.Reset()
	case mcpSetupCredentials:
		if err := m.addCredential(value); err != nil {
			m.errorText = err.Error()
		}
	case mcpSetupConnect:
		if m.choice == 0 {
			m.back()
			break
		}
		candidate := m.server
		// The warning also applies when saving an HTTP exception for later use.
		if candidate.Type == "remote" {
			candidate.AllowInsecureHTTP = true
		}
		if err := validateMCPConnection(candidate); err != nil {
			m.errorText = "Connection settings are invalid. Use Ctrl-B to edit."
			break
		}
		candidate.AllowInsecureHTTP = mcpNeedsInsecureHTTP(candidate)
		m.server = candidate
		if m.choice == 2 {
			m.tools = nil
			m.selected = make(map[string]bool)
			m.next(mcpSetupSave)
			break
		}
		m.server = candidate
		m.busy = true
		name, ctx := m.name, m.ctx
		return m, func() tea.Msg {
			tools, err := discoverToolsWithTimeout(ctx, name, candidate)
			return mcpSetupDiscovered{tools, err}
		}
	case mcpSetupSave:
		if m.choice == 0 {
			m.back()
			break
		}
		m.confirmed = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *mcpSetupModel) addCredential(value string) error {
	if m.credentialPart == 0 {
		if value == "" {
			m.next(mcpSetupConnect)
			return nil
		}
		if m.server.Type == "stdio" && !mcpExactEnvironmentReferencePattern.MatchString("${"+value+"}") {
			return errors.New("Use a valid environment variable name, not its secret value.")
		}
		if m.server.Type == "remote" {
			probe := m.server
			probe.AllowInsecureHTTP = true
			probe.Headers = map[string]string{value: "${TOKEN}"}
			if err := validateMCPConnection(probe); err != nil {
				return errors.New("Use a valid, non-reserved HTTP header name.")
			}
			value = http.CanonicalHeaderKey(value)
		}
		m.credentialKey = value
		m.credentialPart = 1
		m.input.Reset()
		m.input.Placeholder = "Environment variable name, e.g. DOCS_TOKEN"
		return nil
	}
	if m.credentialPart == 1 {
		if !mcpExactEnvironmentReferencePattern.MatchString("${" + value + "}") {
			return errors.New("Enter the name of an existing environment variable, such as DOCS_TOKEN.")
		}
		m.credentialEnv = value
		if m.server.Type == "remote" {
			m.credentialPart = 2
			m.input.Reset()
			m.input.EchoMode = textinput.EchoPassword
			m.input.Placeholder = "Prefix (optional), e.g. Bearer + space"
			return nil
		}
	}
	reference := "${" + m.credentialEnv + "}"
	if m.server.Type == "remote" {
		probe := m.server
		probe.AllowInsecureHTTP = true
		probe.Headers = map[string]string{m.credentialKey: m.input.Value() + reference}
		if err := validateMCPConnection(probe); err != nil {
			return errors.New("Invalid header prefix. Use plain text, such as Bearer followed by a space.")
		}
		if m.server.Headers == nil {
			m.server.Headers = make(map[string]string)
		}
		for key := range m.server.Headers {
			if strings.EqualFold(key, m.credentialKey) {
				delete(m.server.Headers, key)
			}
		}
		m.server.Headers[m.credentialKey] = m.input.Value() + reference
	} else {
		entries := m.server.Env[:0]
		for _, entry := range m.server.Env {
			key, _, _ := strings.Cut(entry, "=")
			if key != m.credentialKey {
				entries = append(entries, entry)
			}
		}
		m.server.Env = append(entries, m.credentialKey+"="+reference)
	}
	m.resetInput()
	return nil
}

func (m mcpSetupModel) View() tea.View {
	var view strings.Builder
	labels := []string{"Connection name", "Transport", "Connection address", "Arguments", "Authentication / environment", "Review connection", "Choose tools", "Save connection"}
	fmt.Fprintln(&view, tuiTitleStyle.Render(fmt.Sprintf("  %s  (%d/8)", labels[m.screen], int(m.screen)+1)))
	fmt.Fprintln(&view)
	if m.busy {
		fmt.Fprintln(&view, "Discovering tools... (30s timeout)\n\nEsc Cancel")
	} else {
		switch m.screen {
		case mcpSetupTransport:
			mcpRenderChoices(&view, []string{"Local process (stdio)", "Remote URL (Streamable HTTP)"}, m.choice)
		case mcpSetupConnect:
			printMCPConnectionPreview(&view, m.server)
			if mcpNeedsInsecureHTTP(m.server) {
				fmt.Fprintln(&view, "WARNING: non-loopback plain HTTP exposes traffic and credentials. Connecting or saving explicitly accepts this exception.")
			}
			fmt.Fprintln(&view, "Starting or connecting may have side effects.\nDiscovery lists tools only; nothing is enabled or called.")
			mcpRenderChoices(&view, []string{"Back", "Connect and discover tools", "Save disabled (no connection)"}, m.choice)
		case mcpSetupTools:
			fmt.Fprintln(&view, "Untrusted tool descriptions. Selected tools require approval.")
			mcpRenderToolChecklist(&view, m.tools, m.selected, m.choice, max(3, m.height-12))
		case mcpSetupSave:
			fmt.Fprintf(&view, "Server: %s\n", m.name)
			printMCPConnectionPreview(&view, m.server)
			names := m.selectedNames()
			fmt.Fprintf(&view, "Enabled tools: %d\n", len(names))
			for _, name := range names {
				fmt.Fprintf(&view, "  %s · ask\n", sanitizeMCPText(name, 100))
			}
			if len(names) == 0 {
				fmt.Fprintln(&view, "Will save disabled; no tools visible to the model.")
			}
			if m.server.Setup != "" {
				fmt.Fprintln(&view, "Legacy setup will be removed, not executed.")
			}
			mcpRenderChoices(&view, []string{"Back", "Save configuration"}, m.choice)
		default:
			if m.screen == mcpSetupName {
				fmt.Fprintln(&view, "e.g. docs")
			}
			if m.screen == mcpSetupEndpoint {
				if m.server.Type == "remote" {
					if m.editing {
						fmt.Fprintln(&view, "Blank keeps the current URL.")
					}
				} else {
					fmt.Fprintln(&view, "Executable, e.g. npx (arguments next)")
				}
			}
			if m.screen == mcpSetupArguments {
				fmt.Fprintln(&view, "One argument at a time; no shell quoting.")
				for i, arg := range redactedMCPCommandArguments(m.server.Args) {
					fmt.Fprintf(&view, "  %d. %s\n", i+1, sanitizeMCPText(arg, 100))
				}
			}
			if m.screen == mcpSetupCredentials {
				fmt.Fprintln(&view, "Use environment variable names, not tokens.")
				if names := append(mcpEnvironmentNames(m.server.Env), mcpHeaderNames(m.server.Headers)...); len(names) > 0 {
					fmt.Fprintf(&view, "Configured: %s\n", strings.Join(names, ", "))
				}
				if m.credentialPart > 0 {
					fmt.Fprintf(&view, "Entry: %s\n", sanitizeMCPText(m.credentialKey, 100))
				}
			}
			fmt.Fprintln(&view, m.input.View())
		}
		if m.errorText != "" {
			fmt.Fprintln(&view, "\nError: "+m.errorText)
		}
		help := "Enter Next · Ctrl-B Back · Esc Cancel"
		switch m.screen {
		case mcpSetupTransport, mcpSetupConnect, mcpSetupSave:
			help = "↑/↓ Select · Enter Confirm · Ctrl-B Back · Esc Cancel"
		case mcpSetupTools:
			help = "↑/↓ Move · Space Select · Enter Next · Ctrl-B Back · Esc Cancel"
		case mcpSetupArguments:
			help = "Enter Add · Blank Next · Ctrl-X Remove · Ctrl-B Back · Esc Cancel"
		case mcpSetupCredentials:
			if m.credentialPart == 0 {
				help = "Enter Add · Blank Next · Ctrl-X Remove · Ctrl-B Back · Esc Cancel"
			}
		}
		fmt.Fprint(&view, "\n"+tuiHelpStyle.Render(help))
	}
	return tea.NewView(view.String())
}

func (m mcpSetupModel) selectedNames() []string {
	var names []string
	for name, yes := range m.selected {
		if yes {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func mcpRenderChoices(out *strings.Builder, choices []string, selected int) {
	for i, label := range choices {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		fmt.Fprintln(out, prefix+label)
	}
}

func mcpNeedsInsecureHTTP(server MCPServerConfig) bool {
	return server.Type == "remote" && strings.HasPrefix(strings.ToLower(server.URL), "http://") && !strings.HasPrefix(mcpRemoteEndpointPreview(server.URL), "(invalid") && !mcpURLIsLoopback(server.URL)
}

func runMCPSetupTUI(cmd *cobra.Command, name string, editing bool, opts mcpAddOptions) error {
	path, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	model, err := newMCPSetupModel(ctx, cfg, name, editing, opts)
	if err != nil {
		return err
	}
	result, err := runMCPModel(ctx, model, cmd.InOrStdin(), cmd.ErrOrStderr())
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("MCP setup could not read the terminal: %w", err)
	}
	final, ok := result.(mcpSetupModel)
	if !ok || !final.confirmed || final.cancelled {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	server := final.server
	var selected []ocrmcp.DiscoveredTool
	for _, item := range final.tools {
		if final.selected[item.Name] {
			selected = append(selected, item)
		}
	}
	setMCPSelectedTools(&server, selected)
	server.Enabled = boolPointer(len(server.Tools) > 0)
	if final.config.MCPServers == nil {
		final.config.MCPServers = make(map[string]MCPServerConfig)
	}
	final.config.MCPServers[final.name] = server
	prepareMCPManagerSave(final.config, final.name)
	if err := ocrmcp.ValidateMCPServerConfig(server); err != nil {
		return err
	}
	if err := validateMCPPersistentAllow(final.config); err != nil {
		return err
	}
	if err := saveConfig(path, final.config); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved MCP server %q with %d tool(s), permission ask.\nUse 'ocr mcp permissions %s' to configure auto-run, or 'ocr mcp' to manage connections.\n", final.name, len(server.Tools), final.name)
	return nil
}

// Accepting a definition enables only that exact capability. Automatic execution
// remains a separate, deliberate operation in the shared permissions command.
func setMCPSelectedTools(server *MCPServerConfig, selected []ocrmcp.DiscoveredTool) {
	server.Tools = nil
	server.ToolPermissions = nil
	server.ToolDefinitionSHA256 = nil
	for _, item := range selected {
		if server.ToolPermissions == nil {
			server.ToolPermissions = make(map[string]ocrmcp.Permission)
			server.ToolDefinitionSHA256 = make(map[string]string)
		}
		server.Tools = append(server.Tools, item.Name)
		server.ToolPermissions[item.Name] = ocrmcp.PermissionAsk
		server.ToolDefinitionSHA256[item.Name] = item.DefinitionSHA256
	}
}
