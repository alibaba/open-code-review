// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/spf13/cobra"
)

var errMCPPromptCancelled = errors.New("MCP prompt cancelled")

type mcpPrompter struct {
	ctx context.Context
	in  io.Reader
	out io.Writer
}

func newMCPPrompter(cmd *cobra.Command) *mcpPrompter {
	return &mcpPrompter{
		ctx: cmd.Context(),
		in:  cmd.InOrStdin(),
		out: cmd.ErrOrStderr(),
	}
}

func (p *mcpPrompter) prompt(label, fallback string) (string, error) {
	if p.usesBubbleTea() {
		return runMCPManagementTextPrompt(p.ctx, label, fallback, p.in, p.out)
	}
	if fallback == "" {
		fmt.Fprintf(p.out, "%s: ", label)
	} else if mcpHiddenPrompt(label) {
		fmt.Fprintf(p.out, "%s [(stored value hidden; blank keeps it)]: ", label)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", label, fallback)
	}
	line, err := readMCPPromptLine(p.in)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read MCP prompt: %w", err)
	}
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, mcpWizardCancelWord) {
		return "", errMCPPromptCancelled
	}
	if line == "" {
		if errors.Is(err, io.EOF) {
			return "", errMCPPromptCancelled
		}
		return fallback, nil
	}
	return line, nil
}

func mcpHiddenPrompt(label string) bool {
	label = strings.ToLower(label)
	return strings.Contains(label, "url") || strings.Contains(label, "argument") || strings.Contains(label, "environment") || strings.Contains(label, "header")
}

// readMCPPromptLine deliberately reads one byte at a time. Management actions
// may hand control to a nested transactional wizard; avoiding buffered
// read-ahead ensures the nested prompt receives the next terminal line.
func readMCPPromptLine(in io.Reader) (string, error) {
	var value strings.Builder
	buffer := []byte{0}
	for {
		n, err := in.Read(buffer)
		if n > 0 {
			if buffer[0] == '\n' {
				return value.String(), nil
			}
			if buffer[0] != '\r' {
				value.WriteByte(buffer[0])
			}
		}
		if err != nil {
			return value.String(), err
		}
	}
}

func (p *mcpPrompter) confirm(label string) (bool, error) {
	if p.usesBubbleTea() {
		return runMCPManagementConfirmPrompt(p.ctx, label, p.in, p.out)
	}
	answer, err := p.prompt(label+" [y/N]", "")
	if errors.Is(err, errMCPPromptCancelled) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func runMCPManager(cmd *cobra.Command) error {
	if newMCPPrompter(cmd).usesBubbleTea() {
		return runMCPDashboard(cmd)
	}
	prompt := newMCPPrompter(cmd)
	defaultAction := "quit"
	if prompt.usesBubbleTea() {
		defaultAction = "add"
		fmt.Fprintln(cmd.ErrOrStderr(), "MCP manager. Arrows select, Enter continues, Esc cancels.")
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), "MCP manager. Type 'cancel' at any prompt to leave without saving.")
	}
	for {
		if err := runMCPList(cmd, false); err != nil {
			return err
		}
		action, err := prompt.choose("Action (add, edit, discover, tools, permissions, enable, disable, remove, quit)", defaultAction, []string{"add", "edit", "discover", "tools", "permissions", "global permissions", "enable", "disable", "remove", "quit"})
		if errors.Is(err, errMCPPromptCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		action = strings.ToLower(strings.TrimSpace(action))
		if action == "quit" || action == "q" {
			return nil
		}
		if action == "add" {
			if err := runMCPConnectionWizard(cmd, "", false, mcpAddOptions{}); err != nil {
				return err
			}
			continue
		}
		if action == "global permissions" {
			if err := runMCPPermissions(cmd, "", mcpPermissionsOptions{}); err != nil {
				return err
			}
			continue
		}
		cfg, err := loadReadOnlyMCPConfig()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(cfg.MCPServers))
		for name := range cfg.MCPServers {
			names = append(names, name)
		}
		sort.Strings(names)
		if prompt.usesBubbleTea() && len(names) == 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), "No servers yet. Choose Add to create a connection.")
			continue
		}
		name, err := prompt.choose("Server name", "", names)
		if errors.Is(err, errMCPPromptCancelled) {
			continue
		}
		if err != nil {
			return err
		}
		switch action {
		case "edit":
			err = runMCPConnectionWizard(cmd, name, true, mcpAddOptions{})
		case "discover":
			err = runMCPDiscoverConfirmed(cmd, name, false, false)
		case "tools":
			err = runMCPTools(cmd, name, mcpToolsOptions{})
		case "permissions":
			err = runMCPPermissions(cmd, name, mcpPermissionsOptions{})
		case "enable":
			err = runMCPEnable(cmd, name, true, false)
		case "disable":
			err = runMCPEnable(cmd, name, false, false)
		case "remove":
			err = runMCPRemove(cmd, name, false)
		default:
			fmt.Fprintf(cmd.ErrOrStderr(), "Unknown action %q.\n", action)
			continue
		}
		if err != nil {
			return err
		}
	}
}

func runMCPConnectionWizard(cmd *cobra.Command, initialName string, editing bool, opts mcpAddOptions) error {
	if newMCPPrompter(cmd).usesBubbleTea() {
		return runMCPSetupTUI(cmd, initialName, editing, opts)
	}
	configPath, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	prompt := newMCPPrompter(cmd)
	cancelled := func() error {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
		return nil
	}

	name := strings.TrimSpace(initialName)
	if name == "" {
		name, err = prompt.prompt("Server name", "")
		if errors.Is(err, errMCPPromptCancelled) {
			return cancelled()
		}
		if err != nil {
			return err
		}
	}
	if err := validateMCPServerName(name); err != nil {
		return err
	}
	existing, exists := draft.MCPServers[name]
	if editing && !exists {
		return fmt.Errorf("MCP server %q not found", name)
	}
	if !editing && exists {
		return fmt.Errorf("MCP server %q already exists", name)
	}

	server := existing
	if !editing {
		server = MCPServerConfig{
			Type:              strings.ToLower(strings.TrimSpace(opts.transport)),
			Command:           strings.TrimSpace(opts.command),
			Args:              append([]string(nil), opts.args...),
			Env:               append([]string(nil), opts.env...),
			URL:               strings.TrimSpace(opts.url),
			AllowInsecureHTTP: opts.allowInsecureHTTP,
			DefaultPermission: ocrmcp.PermissionInherit,
		}
		headers, parseErr := parseMCPHeaderFlags(opts.headers)
		if parseErr != nil {
			return parseErr
		}
		server.Headers = headers
	}
	transportDefault := server.Type
	if transportDefault == "" {
		transportDefault = mcpDefaultTransport
	}
	transport, err := prompt.prompt("Transport (stdio or remote)", transportDefault)
	if errors.Is(err, errMCPPromptCancelled) {
		return cancelled()
	}
	if err != nil {
		return err
	}
	server.Type = strings.ToLower(strings.TrimSpace(transport))

	switch server.Type {
	case "stdio":
		command, promptErr := prompt.prompt("Executable", server.Command)
		if errors.Is(promptErr, errMCPPromptCancelled) {
			return cancelled()
		}
		if promptErr != nil {
			return promptErr
		}
		server.Command = strings.TrimSpace(command)
		argsLine, promptErr := prompt.prompt("Arguments as a JSON array (blank keeps current or none)", "")
		if errors.Is(promptErr, errMCPPromptCancelled) {
			return cancelled()
		}
		if promptErr != nil {
			return promptErr
		}
		if argsLine != "" {
			server.Args, promptErr = parseMCPWizardList(argsLine)
			if promptErr != nil {
				return fmt.Errorf("parse arguments: %w", promptErr)
			}
		}
		envLine, promptErr := prompt.prompt("Environment entries as a JSON array (blank keeps current or none; values stay hidden)", "")
		if errors.Is(promptErr, errMCPPromptCancelled) {
			return cancelled()
		}
		if promptErr != nil {
			return promptErr
		}
		if envLine != "" {
			server.Env, promptErr = parseMCPWizardList(envLine)
			if promptErr != nil {
				return fmt.Errorf("parse environment: %w", promptErr)
			}
			if promptErr = validateNewMCPCredentialTemplates(MCPServerConfig{Env: server.Env}); promptErr != nil {
				return promptErr
			}
		}
		server.URL = ""
		server.Headers = nil
		server.AllowInsecureHTTP = false
	case "remote":
		remoteURL, promptErr := prompt.prompt("Remote URL", server.URL)
		if errors.Is(promptErr, errMCPPromptCancelled) {
			return cancelled()
		}
		if promptErr != nil {
			return promptErr
		}
		server.URL = strings.TrimSpace(remoteURL)
		headerLine, promptErr := prompt.prompt("Headers as a JSON object (blank keeps current or none; values stay hidden)", "")
		if errors.Is(promptErr, errMCPPromptCancelled) {
			return cancelled()
		}
		if promptErr != nil {
			return promptErr
		}
		if headerLine != "" {
			server.Headers, promptErr = parseMCPHeaders(headerLine)
			if promptErr != nil {
				return fmt.Errorf("parse headers: %w", promptErr)
			}
			if promptErr = validateNewMCPCredentialTemplates(MCPServerConfig{Headers: server.Headers}); promptErr != nil {
				return promptErr
			}
		}
		server.Command = ""
		server.Args = nil
		server.Env = nil
		parsed, parseErr := url.Parse(server.URL)
		if parseErr == nil && parsed.Scheme == "http" && !mcpLoopbackHost(parsed.Hostname()) && !server.AllowInsecureHTTP {
			allow, confirmErr := prompt.confirm("This sends MCP traffic over plain HTTP to a non-loopback host. Allow this connection?")
			if confirmErr != nil {
				return confirmErr
			}
			if !allow {
				return cancelled()
			}
			server.AllowInsecureHTTP = true
		}
	default:
		return fmt.Errorf("invalid MCP server type %q: must be stdio or remote", server.Type)
	}

	if !editing {
		if err := validateNewMCPCredentialTemplates(server); err != nil {
			return err
		}
	}
	if err := validateMCPConnection(server); err != nil {
		return err
	}
	printMCPConnectionPreview(cmd.ErrOrStderr(), server)
	connect, err := prompt.confirm("Connect for discovery? This only initializes the server and lists tools")
	if err != nil {
		return err
	}
	if !connect {
		return cancelled()
	}
	tools, err := discoverToolsWithTimeout(cmd.Context(), name, server)
	if err != nil {
		return err
	}
	views := discoveredToolViews(tools)
	printDiscoveredTools(cmd.OutOrStdout(), name, views)
	selection, err := prompt.prompt("Enable tools by exact name or 1-based number, comma-separated (blank enables none)", "")
	if errors.Is(err, errMCPPromptCancelled) {
		return cancelled()
	}
	if err != nil {
		return err
	}
	selected, err := selectMCPDiscoveredTools(tools, selection)
	if err != nil {
		return err
	}
	setMCPSelectedTools(&server, selected)
	server.Enabled = boolPointer(true)
	if server.DefaultPermission == "" {
		server.DefaultPermission = ocrmcp.PermissionInherit
	}
	if err := ocrmcp.ValidateMCPServerConfig(server); err != nil {
		return err
	}
	if draft.MCPServers == nil {
		draft.MCPServers = make(map[string]MCPServerConfig)
	}
	draft.MCPServers[name] = server
	prepareMCPManagerSave(draft, name)
	if err := validateMCPPersistentAllow(draft); err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Configuration to save:")
	printMCPServerSummary(cmd.ErrOrStderr(), mcpServerSummaryFor(name, draft, draft.MCPServers[name]))
	if existing.Setup != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "  Legacy setup command: will be removed and will not be executed")
	}
	confirmed, err := prompt.confirm("Save this configuration")
	if err != nil {
		return err
	}
	if !confirmed {
		return cancelled()
	}
	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved MCP server %q with %d enabled tool(s).\n", name, len(server.Tools))
	return nil
}

func runMCPTools(cmd *cobra.Command, name string, opts mcpToolsOptions) error {
	configPath, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	server, ok := cfg.MCPServers[name]
	if !ok {
		return fmt.Errorf("MCP server %q not found", name)
	}
	interactiveSelection := len(opts.enable) == 0 && len(opts.disable) == 0
	localRevocation := len(opts.enable) == 0 && len(opts.disable) > 0
	if !opts.yes && !localRevocation {
		if !mcpInteractiveSession() {
			return fmt.Errorf("non-interactive tool discovery requires --yes")
		}
		printMCPConnectionPreview(cmd.ErrOrStderr(), server)
		confirmed, confirmErr := newMCPPrompter(cmd).confirm("Connect to discover tools? This will not invoke or enable a tool")
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. The server was not started or contacted.")
			return nil
		}
	}
	if interactiveSelection && !mcpInteractiveSession() {
		return fmt.Errorf("non-interactive tool changes require at least one --enable or --disable flag")
	}
	var tools []ocrmcp.DiscoveredTool
	if !localRevocation {
		tools, err = discoverToolsWithTimeout(cmd.Context(), name, server)
		if err != nil {
			return err
		}
	}
	discovered := make(map[string]ocrmcp.DiscoveredTool, len(tools))
	for _, tool := range tools {
		discovered[tool.Name] = tool
	}

	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	server = draft.MCPServers[name]
	if interactiveSelection && newMCPPrompter(cmd).usesBubbleTea() {
		selected, selectErr := runMCPToolSelection(cmd.Context(), tools, cmd.InOrStdin(), cmd.ErrOrStderr())
		if errors.Is(selectErr, errMCPPromptCancelled) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
			return nil
		}
		if selectErr != nil {
			return selectErr
		}
		setMCPSelectedTools(&server, selected)
	} else if interactiveSelection {
		views := discoveredToolViews(tools)
		printDiscoveredTools(cmd.OutOrStdout(), name, views)
		selection, promptErr := newMCPPrompter(cmd).prompt("Enabled tools by exact name or 1-based number, comma-separated (blank keeps current)", "")
		if errors.Is(promptErr, errMCPPromptCancelled) {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
			return nil
		}
		if promptErr != nil {
			return promptErr
		}
		if selection != "" {
			selected, selectErr := selectMCPDiscoveredTools(tools, selection)
			if selectErr != nil {
				return selectErr
			}
			setMCPSelectedTools(&server, selected)
		}
	} else {
		if err := applyMCPToolChanges(&server, discovered, opts.enable, opts.disable); err != nil {
			return err
		}
	}
	if !localRevocation {
		markMCPDefinitionDrift(&server, discovered)
	}
	if err := ocrmcp.ValidateMCPServerConfig(server); err != nil {
		return err
	}
	draft.MCPServers[name] = server
	prepareMCPManagerSave(draft, name)
	if err := validateMCPPersistentAllow(draft); err != nil {
		return err
	}
	if interactiveSelection && !opts.yes {
		prompt := newMCPPrompter(cmd)
		printMCPServerSummary(cmd.ErrOrStderr(), mcpServerSummaryFor(name, draft, draft.MCPServers[name]))
		if cfg.MCPServers[name].Setup != "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "  Legacy setup command: will be removed and will not be executed")
		}
		confirmed, confirmErr := prompt.confirm("Save this tool allowlist")
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
			return nil
		}
	}
	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	summary := mcpServerSummaryFor(name, draft, draft.MCPServers[name])
	if opts.json {
		return writeMCPJSON(cmd.OutOrStdout(), summary)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved %d enabled tool(s) for MCP server %q.\n", len(server.Tools), name)
	if cfg.MCPServers[name].Setup != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Removed the legacy setup command without executing it.")
	}
	return nil
}

func runMCPPermissions(cmd *cobra.Command, name string, opts mcpPermissionsOptions) error {
	configPath, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	interactive := opts.defaultPermission == "" && len(opts.toolPermissions) == 0 && opts.timeoutSeconds == 0
	if interactive && !mcpInteractiveSession() {
		return fmt.Errorf("non-interactive permission changes require flags and --yes")
	}
	if !interactive && !opts.yes && !mcpInteractiveSession() {
		return fmt.Errorf("non-interactive permission changes require --yes")
	}

	prompt := newMCPPrompter(cmd)
	if name == "" {
		if len(opts.toolPermissions) > 0 {
			return fmt.Errorf("--tool requires an MCP server name")
		}
		if draft.MCP == nil {
			draft.MCP = &ocrmcp.MCPConfig{}
		}
		if interactive {
			fallback := draft.MCP.DefaultPermission
			if fallback == "" {
				fallback = ocrmcp.PermissionAsk
			}
			value, promptErr := prompt.choose("Global default permission: ask prompts; allow skips prompts; deny blocks all MCP calls", string(fallback), []string{"ask", "allow", "deny"})
			if errors.Is(promptErr, errMCPPromptCancelled) {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
				return nil
			}
			if promptErr != nil {
				return promptErr
			}
			opts.defaultPermission = value
			timeout := draft.MCP.ApprovalTimeoutSeconds
			if timeout == 0 {
				timeout = mcpDefaultApprovalTimeout
			}
			value, promptErr = prompt.prompt("Approval timeout in seconds (1-600)", strconv.Itoa(timeout))
			if errors.Is(promptErr, errMCPPromptCancelled) {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
				return nil
			}
			if promptErr != nil {
				return promptErr
			}
			opts.timeoutSeconds, promptErr = strconv.Atoi(value)
			if promptErr != nil {
				return fmt.Errorf("approval timeout must be an integer from 1 to 600")
			}
			if opts.timeoutSeconds < 1 || opts.timeoutSeconds > 600 {
				return fmt.Errorf("approval timeout must be between 1 and 600 seconds")
			}
		}
		if opts.defaultPermission != "" {
			permission, parseErr := parseMCPPermission(opts.defaultPermission, false)
			if parseErr != nil {
				return parseErr
			}
			draft.MCP.DefaultPermission = permission
		}
		if opts.timeoutSeconds != 0 {
			if opts.timeoutSeconds < 1 || opts.timeoutSeconds > 600 {
				return fmt.Errorf("approval timeout must be between 1 and 600 seconds")
			}
			draft.MCP.ApprovalTimeoutSeconds = opts.timeoutSeconds
		}
		prepareMCPManagerSave(draft)
		if err := ocrmcp.ValidateMCPConfig(*draft.MCP); err != nil {
			return err
		}
		if err := validateMCPPersistentAllow(draft); err != nil {
			return err
		}
		if mcpInteractiveSession() && !opts.yes {
			confirmed, confirmErr := prompt.confirm(fmt.Sprintf("Save global permission %s and timeout %d seconds", draft.MCP.DefaultPermission, draft.MCP.ApprovalTimeoutSeconds))
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
				return nil
			}
		}
	} else {
		server, ok := draft.MCPServers[name]
		if !ok {
			return fmt.Errorf("MCP server %q not found", name)
		}
		if opts.timeoutSeconds != 0 {
			return fmt.Errorf("--timeout is global and cannot be used with a server name")
		}
		if interactive {
			fallback := server.DefaultPermission
			if fallback == "" {
				fallback = ocrmcp.PermissionInherit
			}
			value, promptErr := prompt.choose("Server default permission (parent deny always wins)", string(fallback), []string{"inherit", "ask", "allow", "deny"})
			if errors.Is(promptErr, errMCPPromptCancelled) {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
				return nil
			}
			if promptErr != nil {
				return promptErr
			}
			opts.defaultPermission = value
			for _, toolName := range server.Tools {
				fallback := server.ToolPermissions[toolName]
				if fallback == "" {
					fallback = ocrmcp.PermissionInherit
				}
				value, promptErr = prompt.choose(fmt.Sprintf("Permission for %s (allow skips approval, not the tool allowlist)", sanitizeMCPText(toolName, 160)), string(fallback), []string{"inherit", "ask", "allow", "deny"})
				if errors.Is(promptErr, errMCPPromptCancelled) {
					fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
					return nil
				}
				if promptErr != nil {
					return promptErr
				}
				opts.toolPermissions = append(opts.toolPermissions, toolName+"="+value)
			}
		}
		if opts.defaultPermission != "" {
			permission, parseErr := parseMCPPermission(opts.defaultPermission, true)
			if parseErr != nil {
				return parseErr
			}
			server.DefaultPermission = permission
		}
		toolPermissions, parseErr := parseMCPPermissionFlags(opts.toolPermissions)
		if parseErr != nil {
			return parseErr
		}
		allowed := make(map[string]struct{}, len(server.Tools))
		for _, toolName := range server.Tools {
			allowed[toolName] = struct{}{}
		}
		if server.ToolPermissions == nil && len(toolPermissions) > 0 {
			server.ToolPermissions = make(map[string]ocrmcp.Permission)
		}
		for toolName, permission := range toolPermissions {
			if _, ok := allowed[toolName]; !ok {
				return fmt.Errorf("permission for MCP tool %q cannot enable a tool outside the explicit allowlist", toolName)
			}
			if permission == ocrmcp.PermissionInherit {
				delete(server.ToolPermissions, toolName)
			} else {
				server.ToolPermissions[toolName] = permission
			}
		}
		if len(server.ToolPermissions) == 0 {
			server.ToolPermissions = nil
		}
		if err := ocrmcp.ValidateMCPServerConfig(server); err != nil {
			return err
		}
		draft.MCPServers[name] = server
		prepareMCPManagerSave(draft, name)
		if err := validateMCPPersistentAllow(draft); err != nil {
			return err
		}
		if mcpInteractiveSession() && !opts.yes {
			printMCPServerSummary(cmd.ErrOrStderr(), mcpServerSummaryFor(name, draft, draft.MCPServers[name]))
			if cfg.MCPServers[name].Setup != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "  Legacy setup command: will be removed and will not be executed")
			}
			confirmed, confirmErr := prompt.confirm("Save these permissions")
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
				return nil
			}
		}
	}

	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	if name == "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Saved global MCP permission %s with a %d-second approval timeout.\n", draft.MCP.DefaultPermission, draft.MCP.ApprovalTimeoutSeconds)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Saved permissions for MCP server %q.\n", name)
		if cfg.MCPServers[name].Setup != "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Removed the legacy setup command without executing it.")
		}
	}
	return nil
}

func parseMCPWizardList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "[") {
		var items []string
		if err := json.Unmarshal([]byte(value), &items); err != nil {
			return nil, fmt.Errorf("expected a JSON string array: %w", err)
		}
		return items, nil
	}
	items := strings.Split(value, ",")
	for index := range items {
		items[index] = strings.TrimSpace(items[index])
	}
	return items, nil
}

func selectMCPDiscoveredTools(tools []ocrmcp.DiscoveredTool, selection string) ([]ocrmcp.DiscoveredTool, error) {
	tools = sortedMCPTools(tools)
	selection = strings.TrimSpace(selection)
	if selection == "" {
		return nil, nil
	}
	values, err := parseMCPWizardList(selection)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]ocrmcp.DiscoveredTool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	selected := make([]ocrmcp.DiscoveredTool, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		tool, ok := byName[value]
		if !ok {
			index, parseErr := strconv.Atoi(value)
			if parseErr == nil && index >= 1 && index <= len(tools) {
				tool = tools[index-1]
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("MCP tool %q was not returned by discovery", value)
		}
		if _, duplicate := seen[tool.Name]; duplicate {
			continue
		}
		seen[tool.Name] = struct{}{}
		selected = append(selected, tool)
	}
	return selected, nil
}

func applyMCPToolChanges(server *MCPServerConfig, discovered map[string]ocrmcp.DiscoveredTool, enable, disable []string) error {
	selected := make(map[string]struct{}, len(server.Tools)+len(enable))
	for _, toolName := range server.Tools {
		selected[toolName] = struct{}{}
	}
	disabled := make(map[string]struct{}, len(disable))
	for _, toolName := range disable {
		if _, duplicate := disabled[toolName]; duplicate {
			continue
		}
		disabled[toolName] = struct{}{}
		delete(selected, toolName)
		delete(server.ToolPermissions, toolName)
		delete(server.ToolDefinitionSHA256, toolName)
	}
	for _, toolName := range enable {
		if _, conflict := disabled[toolName]; conflict {
			return fmt.Errorf("MCP tool %q cannot be both enabled and disabled", toolName)
		}
		tool, ok := discovered[toolName]
		if !ok {
			return fmt.Errorf("MCP tool %q was not returned by discovery", toolName)
		}
		selected[toolName] = struct{}{}
		if server.ToolPermissions == nil {
			server.ToolPermissions = make(map[string]ocrmcp.Permission)
		}
		if server.ToolDefinitionSHA256 == nil {
			server.ToolDefinitionSHA256 = make(map[string]string)
		}
		previousFingerprint := server.ToolDefinitionSHA256[toolName]
		if server.ToolPermissions[toolName] == "" || previousFingerprint != tool.DefinitionSHA256 {
			// A changed definition must not inherit a previous persistent allow.
			// Accepting the new fingerprint restores visibility at ask; only the
			// permissions command can opt it back into unattended execution.
			server.ToolPermissions[toolName] = ocrmcp.PermissionAsk
		}
		server.ToolDefinitionSHA256[toolName] = tool.DefinitionSHA256
	}
	server.Tools = server.Tools[:0]
	for toolName := range selected {
		server.Tools = append(server.Tools, toolName)
	}
	sort.Strings(server.Tools)
	if len(server.ToolPermissions) == 0 {
		server.ToolPermissions = nil
	}
	if len(server.ToolDefinitionSHA256) == 0 {
		server.ToolDefinitionSHA256 = nil
	}
	return nil
}

func markMCPDefinitionDrift(server *MCPServerConfig, discovered map[string]ocrmcp.DiscoveredTool) {
	for _, toolName := range server.Tools {
		tool, ok := discovered[toolName]
		if !ok || server.ToolDefinitionSHA256[toolName] != tool.DefinitionSHA256 {
			delete(server.ToolDefinitionSHA256, toolName)
			if server.ToolPermissions == nil {
				server.ToolPermissions = make(map[string]ocrmcp.Permission)
			}
			// An explicit ask override also prevents a global/server allow from
			// turning a stale definition into an unattended tool after this save.
			server.ToolPermissions[toolName] = ocrmcp.PermissionAsk
		}
	}
	if len(server.ToolDefinitionSHA256) == 0 {
		server.ToolDefinitionSHA256 = nil
	}
}

func parseMCPPermissionFlags(values []string) (map[string]ocrmcp.Permission, error) {
	permissions := make(map[string]ocrmcp.Permission, len(values))
	for _, entry := range values {
		separator := strings.LastIndex(entry, "=")
		if separator < 1 {
			return nil, fmt.Errorf("invalid --tool value %q: expected exact TOOL=PERMISSION", entry)
		}
		name, value := entry[:separator], entry[separator+1:]
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid --tool: tool name is empty")
		}
		permission, err := parseMCPPermission(value, true)
		if err != nil {
			return nil, fmt.Errorf("invalid permission for MCP tool %q: %w", name, err)
		}
		permissions[name] = permission
	}
	return permissions, nil
}

func printMCPConnectionPreview(out io.Writer, server MCPServerConfig) {
	if server.Type == "remote" {
		fmt.Fprintf(out, "Connection endpoint: %s\n", mcpRemoteEndpointPreview(server.URL))
		if names := mcpHeaderNames(server.Headers); len(names) > 0 {
			fmt.Fprintf(out, "Header names: %s (values hidden)\n", strings.Join(names, ", "))
		}
		return
	}
	command := append([]string{server.Command}, redactedMCPCommandArguments(server.Args)...)
	quoted := make([]string, len(command))
	for index, value := range command {
		quoted[index] = strconv.Quote(value)
	}
	fmt.Fprintf(out, "Command: %s\n", strings.Join(quoted, " "))
	if names := mcpEnvironmentNames(server.Env); len(names) > 0 {
		fmt.Fprintf(out, "Environment names: %s (values hidden)\n", strings.Join(names, ", "))
	}
}

func redactedMCPCommandArguments(arguments []string) []string {
	return ocrmcp.SafeCommandArguments(arguments)
}

func mcpSensitiveArgumentName(value string) bool {
	return ocrmcp.SensitiveArgumentName(value)
}

func mcpRemoteEndpointPreview(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "(invalid remote URL)"
	}
	return (&url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   parsed.Path,
	}).String()
}

func printMCPServerSummary(out io.Writer, summary mcpServerSummary) {
	fmt.Fprintf(out, "  Server: %s\n  Transport: %s\n  Endpoint: %s\n  Status: %s\n  Enabled tools: %d\n  Default permission: %s\n", summary.Name, summary.Transport, summary.Endpoint, summary.Status, len(summary.Tools), summary.DefaultPermission)
	if summary.ErrorCategory != "" {
		fmt.Fprintf(out, "  Error category: %s\n", summary.ErrorCategory)
	}
	for _, toolName := range summary.Tools {
		fmt.Fprintf(out, "  Tool %s: fingerprint=%s\n", sanitizeMCPText(toolName, 160), summary.FingerprintStatus[toolName])
	}
	if len(summary.Environment) > 0 {
		fmt.Fprintf(out, "  Environment names: %s (%s)\n", strings.Join(summary.Environment, ", "), mcpRedactedStoredValue)
	}
	if len(summary.Headers) > 0 {
		fmt.Fprintf(out, "  Header names: %s (%s)\n", strings.Join(summary.Headers, ", "), mcpRedactedEnvironmentValue)
	}
	if summary.LegacyLiteralCredentials {
		fmt.Fprintln(out, "  Credentials: deprecated literal values are stored; replace them with ${ENV_NAME} references")
	}
}
