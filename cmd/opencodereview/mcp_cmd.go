// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

const (
	mcpConfigVersion            = 1
	mcpDefaultApprovalTimeout   = 60
	mcpDiscoveryTimeout         = 30 * time.Second
	mcpDefaultTransport         = "stdio"
	mcpStatusReady              = "ready"
	mcpStatusDisabled           = "disabled"
	mcpStatusNeedsReview        = "needs-review"
	mcpStatusError              = "error"
	mcpFingerprintRecorded      = "recorded"
	mcpFingerprintNeedsReview   = "needs-review"
	mcpWizardCancelWord         = "cancel"
	mcpRedactedStoredValue      = "(stored value)"
	mcpRedactedEnvironmentValue = "(environment reference)"
)

var (
	mcpServerNamePattern                = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	mcpExactEnvironmentReferencePattern = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)
	mcpEnvironmentReferencePattern      = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)

	// mcpInteractiveTerminal is a variable so command tests can exercise both
	// branches without allocating a real pseudoterminal.
	mcpInteractiveTerminal = defaultMCPInteractiveTerminal

	// mcpDiscoverServerTools is replaceable in tests. The production
	// implementation performs initialization and tools/list only.
	mcpDiscoverServerTools = discoverMCPServerTools

	mcpCIEnvironmentVariables = []string{
		"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "TF_BUILD", "JENKINS_URL",
		"CIRCLECI", "TRAVIS", "TEAMCITY_VERSION", "BITBUCKET_BUILD_NUMBER",
		"CODEBUILD_BUILD_ID", "SYSTEM_TEAMFOUNDATIONCOLLECTIONURI",
	}
)

type mcpAddOptions struct {
	transport         string
	command           string
	args              []string
	env               []string
	url               string
	headers           []string
	allowInsecureHTTP bool
	yes               bool
}

type mcpOutputOptions struct {
	json bool
}

type mcpConsentOptions struct {
	yes bool
}

type mcpToolsOptions struct {
	yes     bool
	enable  []string
	disable []string
	json    bool
}

type mcpPermissionsOptions struct {
	yes               bool
	defaultPermission string
	toolPermissions   []string
	timeoutSeconds    int
}

type mcpServerSummary struct {
	Name                     string                       `json:"name"`
	Transport                string                       `json:"transport"`
	Enabled                  bool                         `json:"enabled"`
	Status                   string                       `json:"status"`
	Endpoint                 string                       `json:"endpoint"`
	Environment              []string                     `json:"environment,omitempty"`
	Headers                  []string                     `json:"headers,omitempty"`
	Tools                    []string                     `json:"tools,omitempty"`
	DefaultPermission        ocrmcp.Permission            `json:"default_permission"`
	ToolPermissions          map[string]ocrmcp.Permission `json:"tool_permissions,omitempty"`
	FingerprintStatus        map[string]string            `json:"fingerprint_status,omitempty"`
	ErrorCategory            string                       `json:"error_category,omitempty"`
	LegacyLiteralCredentials bool                         `json:"legacy_literal_credentials,omitempty"`
}

type mcpDiscoveredToolView struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	DefinitionSHA256  string `json:"definition_sha256"`
	UntrustedMetadata bool   `json:"untrusted_metadata"`
}

var mcpCmd = newMCPCommand()

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers, tools, and permissions",
		Long: `Manage Model Context Protocol servers without editing JSON by hand.

The interactive manager connects only after explicit confirmation. Discovery
initializes the server and lists tools; it never invokes a business tool.`,
		Args: cobra.NoArgs,
		RunE: runMCPRoot,
	}
	cmd.AddCommand(
		newMCPAddCommand(),
		newMCPImportCommand(),
		newMCPListCommand(),
		newMCPShowCommand(),
		newMCPEditCommand(),
		newMCPDiscoverCommand(),
		newMCPToolsCommand(),
		newMCPPermissionsCommand(),
		newMCPEnableCommand(true),
		newMCPEnableCommand(false),
		newMCPRemoveCommand(),
	)
	return cmd
}

func newMCPAddCommand() *cobra.Command {
	opts := new(mcpAddOptions)
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Add an MCP server with a guided setup",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPAdd(cmd, args, *opts)
		},
	}
	cmd.Flags().StringVar(&opts.transport, "type", "", "transport type: stdio or remote")
	cmd.Flags().StringVar(&opts.command, "command", "", "stdio server executable")
	cmd.Flags().StringArrayVar(&opts.args, "arg", nil, "stdio argument (repeatable)")
	cmd.Flags().StringArrayVar(&opts.env, "env", nil, "stdio KEY=VALUE or KEY=${ENV_NAME} entry (repeatable)")
	cmd.Flags().StringVar(&opts.url, "url", "", "remote MCP URL")
	cmd.Flags().StringArrayVar(&opts.headers, "header", nil, "remote NAME=VALUE or NAME=${ENV_NAME} header (repeatable)")
	cmd.Flags().BoolVar(&opts.allowInsecureHTTP, "allow-insecure-http", false, "allow plain HTTP to a non-loopback remote server")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm non-interactive configuration")
	return cmd
}

func newMCPListCommand() *cobra.Command {
	opts := new(mcpOutputOptions)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured MCP servers without connecting",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCPList(cmd, opts.json)
		},
	}
	cmd.Flags().BoolVar(&opts.json, "json", false, "emit redacted JSON")
	return cmd
}

func newMCPShowCommand() *cobra.Command {
	opts := new(mcpOutputOptions)
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show one MCP server with credentials redacted",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPShow(cmd, args[0], opts.json)
		},
	}
	cmd.Flags().BoolVar(&opts.json, "json", false, "emit redacted JSON")
	return cmd
}

func newMCPEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a server and rediscover its tools",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !mcpInteractiveSession() {
				return fmt.Errorf("ocr mcp edit requires an interactive terminal")
			}
			return runMCPConnectionWizard(cmd, args[0], true, mcpAddOptions{})
		},
	}
}

func newMCPDiscoverCommand() *cobra.Command {
	opts := struct {
		yes  bool
		json bool
	}{}
	cmd := &cobra.Command{
		Use:   "discover <name>",
		Short: "Connect and list tools without enabling or invoking them",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPDiscoverConfirmed(cmd, args[0], opts.yes, opts.json)
		},
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm starting or connecting to the server")
	cmd.Flags().BoolVar(&opts.json, "json", false, "emit redacted JSON")
	return cmd
}

func newMCPToolsCommand() *cobra.Command {
	opts := new(mcpToolsOptions)
	cmd := &cobra.Command{
		Use:   "tools <name>",
		Short: "Discover and change the explicit model-visible tool allowlist",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPTools(cmd, args[0], *opts)
		},
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm starting the server and saving the selection")
	cmd.Flags().StringArrayVar(&opts.enable, "enable", nil, "enable an exact discovered tool name (repeatable)")
	cmd.Flags().StringArrayVar(&opts.disable, "disable", nil, "disable an exact tool name (repeatable)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "emit a redacted JSON summary")
	return cmd
}

func newMCPPermissionsCommand() *cobra.Command {
	opts := new(mcpPermissionsOptions)
	cmd := &cobra.Command{
		Use:   "permissions [name]",
		Short: "Set global, server, or enabled-tool execution permissions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("timeout") && (opts.timeoutSeconds < 1 || opts.timeoutSeconds > 600) {
				return fmt.Errorf("approval timeout must be between 1 and 600 seconds")
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runMCPPermissions(cmd, name, *opts)
		},
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm non-interactive permission changes")
	cmd.Flags().StringVar(&opts.defaultPermission, "default", "", "default permission: inherit, ask, allow, or deny")
	cmd.Flags().StringArrayVar(&opts.toolPermissions, "tool", nil, "exact TOOL=PERMISSION override (repeatable)")
	cmd.Flags().IntVar(&opts.timeoutSeconds, "timeout", 0, "global approval timeout in seconds (1-600)")
	return cmd
}

func newMCPEnableCommand(enable bool) *cobra.Command {
	verb := "enable"
	short := "Enable an MCP server without changing its tool allowlist"
	if !enable {
		verb = "disable"
		short = "Disable an MCP server without deleting its configuration"
	}
	opts := new(mcpConsentOptions)
	cmd := &cobra.Command{
		Use:   verb + " <name>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPEnable(cmd, args[0], enable, opts.yes)
		},
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm the change without a prompt")
	return cmd
}

func newMCPRemoveCommand() *cobra.Command {
	opts := new(mcpConsentOptions)
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an MCP server",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPRemove(cmd, args[0], opts.yes)
		},
	}
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm removal without a prompt")
	return cmd
}

func runMCPRoot(cmd *cobra.Command, _ []string) error {
	if !mcpInteractiveSession() {
		if err := runMCPList(cmd, false); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout())
		return cmd.Help()
	}
	return runMCPManager(cmd)
}

func runMCPAdd(cmd *cobra.Command, args []string, opts mcpAddOptions) error {
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	if mcpInteractiveSession() {
		return runMCPConnectionWizard(cmd, name, false, opts)
	}
	if name == "" {
		return fmt.Errorf("non-interactive add requires a server name")
	}
	if !opts.yes {
		return fmt.Errorf("non-interactive add requires --yes")
	}
	server, err := mcpServerFromAddOptions(opts)
	if err != nil {
		return err
	}
	if err := validateMCPServerName(name); err != nil {
		return err
	}

	configPath, err := resolveConfigPath()
	if err != nil {
		return err
	}
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if _, exists := cfg.MCPServers[name]; exists {
		return fmt.Errorf("MCP server %q already exists", name)
	}
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	if draft.MCPServers == nil {
		draft.MCPServers = make(map[string]MCPServerConfig)
	}
	disabled := false
	server.Enabled = &disabled
	server.Tools = nil
	server.ToolPermissions = nil
	server.ToolDefinitionSHA256 = nil
	server.Setup = ""
	draft.MCPServers[name] = server
	prepareMCPManagerSave(draft)
	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added disabled MCP server %q with no enabled tools. Run 'ocr mcp tools %s --yes' to discover and select tools.\n", name, name)
	return nil
}

func runMCPList(cmd *cobra.Command, asJSON bool) error {
	cfg, err := loadReadOnlyMCPConfig()
	if err != nil {
		return err
	}
	summaries := mcpServerSummaries(cfg)
	if asJSON {
		return writeMCPJSON(cmd.OutOrStdout(), summaries)
	}
	if len(summaries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No MCP servers configured.")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "MCP servers:")
	for _, summary := range summaries {
		credentialStatus := ""
		if summary.LegacyLiteralCredentials {
			credentialStatus = "  credentials=deprecated-literal"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %-8s %-12s tools=%d  %s%s\n", summary.Name, summary.Transport, summary.Status, len(summary.Tools), summary.Endpoint, credentialStatus)
	}
	return nil
}

func runMCPShow(cmd *cobra.Command, name string, asJSON bool) error {
	cfg, err := loadReadOnlyMCPConfig()
	if err != nil {
		return err
	}
	server, ok := cfg.MCPServers[name]
	if !ok {
		return fmt.Errorf("MCP server %q not found", name)
	}
	summary := mcpServerSummaryFor(name, cfg, server)
	if asJSON {
		return writeMCPJSON(cmd.OutOrStdout(), summary)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nTransport: %s\nStatus: %s\nEndpoint: %s\nDefault permission: %s\n", summary.Name, summary.Transport, summary.Status, summary.Endpoint, summary.DefaultPermission)
	if len(summary.Environment) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Environment names: %s\n", strings.Join(summary.Environment, ", "))
	}
	if len(summary.Headers) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Header names: %s\n", strings.Join(summary.Headers, ", "))
	}
	if summary.LegacyLiteralCredentials {
		fmt.Fprintln(cmd.OutOrStdout(), "Credential values: deprecated literal values are stored; replace them with ${ENV_NAME} references")
	}
	if len(summary.Tools) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Tools: none enabled")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Tools:")
		for _, toolName := range summary.Tools {
			permission := summary.ToolPermissions[toolName]
			if permission == "" {
				permission = ocrmcp.PermissionInherit
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  (%s, fingerprint=%s)\n", sanitizeMCPText(toolName, 160), permission, summary.FingerprintStatus[toolName])
		}
	}
	if summary.ErrorCategory != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Error category: %s\n", summary.ErrorCategory)
	}
	return nil
}

func runMCPDiscover(cmd *cobra.Command, name string, asJSON bool) error {
	return runMCPDiscoverConfirmed(cmd, name, true, asJSON)
}

func runMCPDiscoverConfirmed(cmd *cobra.Command, name string, yes, asJSON bool) error {
	cfg, err := loadReadOnlyMCPConfig()
	if err != nil {
		return err
	}
	server, ok := cfg.MCPServers[name]
	if !ok {
		return fmt.Errorf("MCP server %q not found", name)
	}
	if !yes {
		if !mcpInteractiveSession() {
			return fmt.Errorf("non-interactive discovery requires --yes")
		}
		printMCPConnectionPreview(cmd.ErrOrStderr(), server)
		confirmed, confirmErr := newMCPPrompter(cmd).confirm("Connect for discovery? This only initializes the server and lists tools")
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. The server was not started or contacted.")
			return nil
		}
	}
	tools, err := discoverToolsWithTimeout(cmd.Context(), name, server)
	if err != nil {
		return err
	}
	views := discoveredToolViews(tools)
	if asJSON {
		return writeMCPJSON(cmd.OutOrStdout(), views)
	}
	printDiscoveredTools(cmd.OutOrStdout(), name, views)
	return nil
}

func runMCPEnable(cmd *cobra.Command, name string, enable, yes bool) error {
	if !yes {
		if !mcpInteractiveSession() {
			return fmt.Errorf("non-interactive %s requires --yes", cmd.Name())
		}
		confirmed, err := newMCPPrompter(cmd).confirm(fmt.Sprintf("%s MCP server %q?", mcpTitle(cmd.Name()), name))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
			return nil
		}
	}
	configPath, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	server, ok := cfg.MCPServers[name]
	if !ok {
		return fmt.Errorf("MCP server %q not found", name)
	}
	hadLegacySetup := server.Setup != ""
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	server = draft.MCPServers[name]
	server.Enabled = boolPointer(enable)
	draft.MCPServers[name] = server
	prepareMCPManagerSave(draft, name)
	if err := validateMCPPersistentAllow(draft); err != nil {
		return err
	}
	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s MCP server %q.\n", mcpPastTense(cmd.Name()), name)
	if hadLegacySetup {
		fmt.Fprintln(cmd.OutOrStdout(), "Removed the legacy setup command without executing it.")
	}
	return nil
}

func runMCPRemove(cmd *cobra.Command, name string, yes bool) error {
	if !yes {
		if !mcpInteractiveSession() {
			return fmt.Errorf("non-interactive remove requires --yes")
		}
		confirmed, err := newMCPPrompter(cmd).confirm(fmt.Sprintf("Remove MCP server %q?", name))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No changes saved.")
			return nil
		}
	}
	configPath, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.MCPServers[name]; !ok {
		return fmt.Errorf("MCP server %q not found", name)
	}
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	delete(draft.MCPServers, name)
	if len(draft.MCPServers) == 0 {
		draft.MCPServers = nil
	}
	prepareMCPManagerSave(draft)
	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed MCP server %q.\n", name)
	return nil
}

func loadReadOnlyMCPConfig() (*Config, error) {
	path, err := resolveConfigPath()
	if err != nil {
		return nil, err
	}
	cfg, err := LoadAppConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if cfg == nil {
		cfg = &Config{}
	}
	return cfg, nil
}

func loadWritableMCPConfig() (string, *Config, error) {
	path, err := resolveConfigPath()
	if err != nil {
		return "", nil, err
	}
	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		return "", nil, fmt.Errorf("load config: %w", err)
	}
	return path, cfg, nil
}

func prepareMCPManagerSave(cfg *Config, managedServerNames ...string) {
	if cfg.MCP == nil {
		cfg.MCP = &ocrmcp.MCPConfig{}
	}
	cfg.MCP.Version = mcpConfigVersion
	if cfg.MCP.Enabled == nil {
		cfg.MCP.Enabled = boolPointer(true)
	}
	if cfg.MCP.DefaultPermission == "" {
		cfg.MCP.DefaultPermission = ocrmcp.PermissionAsk
	}
	if cfg.MCP.ApprovalTimeoutSeconds == 0 {
		cfg.MCP.ApprovalTimeoutSeconds = mcpDefaultApprovalTimeout
	}
	for _, name := range managedServerNames {
		server, ok := cfg.MCPServers[name]
		if !ok {
			continue
		}
		server.Setup = ""
		cfg.MCPServers[name] = server
	}
}

func cloneAppConfig(cfg *Config) (*Config, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("clone config: %w", err)
	}
	var clone Config
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("clone config: %w", err)
	}
	clone.revision = cfg.revision
	return &clone, nil
}

func mcpServerFromAddOptions(opts mcpAddOptions) (MCPServerConfig, error) {
	transport := strings.ToLower(strings.TrimSpace(opts.transport))
	if transport == "" {
		return MCPServerConfig{}, fmt.Errorf("non-interactive add requires --type stdio or --type remote")
	}
	headers, err := parseMCPHeaderFlags(opts.headers)
	if err != nil {
		return MCPServerConfig{}, err
	}
	server := MCPServerConfig{
		Type:              transport,
		Command:           strings.TrimSpace(opts.command),
		Args:              append([]string(nil), opts.args...),
		Env:               append([]string(nil), opts.env...),
		URL:               strings.TrimSpace(opts.url),
		Headers:           headers,
		AllowInsecureHTTP: opts.allowInsecureHTTP,
	}
	if err := validateMCPConnection(server); err != nil {
		return MCPServerConfig{}, err
	}
	if err := validateNewMCPCredentialTemplates(server); err != nil {
		return MCPServerConfig{}, err
	}
	return server, nil
}

func validateMCPServerName(name string) error {
	if !mcpServerNamePattern.MatchString(name) {
		return fmt.Errorf("invalid MCP server name %q: use 1-64 ASCII letters, digits, underscores, or hyphens, starting with a letter or digit", name)
	}
	return nil
}

func validateMCPConnection(server MCPServerConfig) error {
	if err := ocrmcp.ValidateMCPConnection(server); err != nil {
		return fmt.Errorf("invalid MCP connection: %w", err)
	}
	return nil
}

func mcpLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseMCPHeaderFlags(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	headers := make(map[string]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, entry := range values {
		name, value, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || value == "" || strings.ContainsAny(name+value, "\r\n") {
			return nil, fmt.Errorf("invalid --header value: expected NAME=VALUE without line breaks")
		}
		identity := strings.ToLower(name)
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("MCP header %q is configured more than once", name)
		}
		seen[identity] = struct{}{}
		headers[name] = value
	}
	return headers, nil
}

func validateNewMCPCredentialTemplates(server MCPServerConfig) error {
	for _, entry := range server.Env {
		_, value, ok := strings.Cut(entry, "=")
		if !ok || !mcpExactEnvironmentReferencePattern.MatchString(value) {
			return fmt.Errorf("new MCP environment values must use an exact ${ENV_NAME} reference; literal values are not accepted by the wizard")
		}
	}
	for _, value := range server.Headers {
		if !mcpEnvironmentReferencePattern.MatchString(value) {
			return fmt.Errorf("new MCP header values must contain a ${ENV_NAME} reference; literal values are not accepted by the wizard")
		}
	}
	return nil
}

func hasLegacyLiteralMCPCredentials(server MCPServerConfig) bool {
	for _, entry := range server.Env {
		_, value, ok := strings.Cut(entry, "=")
		if !ok || !mcpExactEnvironmentReferencePattern.MatchString(value) {
			return true
		}
	}
	for _, value := range server.Headers {
		if !mcpEnvironmentReferencePattern.MatchString(value) {
			return true
		}
	}
	return false
}

func discoverMCPServerTools(ctx context.Context, name string, server MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
	if err := validateMCPConnection(server); err != nil {
		return nil, err
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	client, err := ocrmcp.NewConfiguredClient(ctx, name, server, workingDir, Version)
	if err != nil {
		return nil, sanitizeMCPDiscoveryError(name, err)
	}
	defer client.Close()
	return client.DiscoveredTools(), nil
}

func discoverToolsWithTimeout(parent context.Context, name string, server MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, mcpDiscoveryTimeout)
	defer cancel()
	tools, err := mcpDiscoverServerTools(ctx, name, server)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func sanitizeMCPDiscoveryError(name string, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "deadline") {
		return fmt.Errorf("MCP server %q discovery timed out", name)
	}
	return fmt.Errorf("MCP server %q discovery failed; verify the redacted connection settings", name)
}

func mcpServerSummaries(cfg *Config) []mcpServerSummary {
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]mcpServerSummary, 0, len(names))
	for _, name := range names {
		out = append(out, mcpServerSummaryFor(name, cfg, cfg.MCPServers[name]))
	}
	return out
}

func mcpServerSummaryFor(name string, cfg *Config, server MCPServerConfig) mcpServerSummary {
	transport := server.Type
	if transport == "" {
		transport = mcpDefaultTransport
	}
	var globalConfig *ocrmcp.MCPConfig
	if cfg != nil {
		globalConfig = cfg.MCP
	}
	enabled := mcpEnabled(globalConfig) && optionalBool(server.Enabled, true)
	status := mcpStatusReady
	errorCategory := ""
	global := ocrmcp.MCPConfig{}
	if cfg != nil && cfg.MCP != nil {
		global = *cfg.MCP
	}
	if err := ocrmcp.ValidateMCPConfig(global); err != nil {
		status = mcpStatusError
		errorCategory = "invalid_global_policy"
	} else if err := ocrmcp.ValidateMCPServerConfig(server); err != nil {
		status = mcpStatusError
		errorCategory = "invalid_server_policy"
	} else if err := validateMCPConnection(server); err != nil {
		status = mcpStatusError
		errorCategory = "invalid_connection"
	} else if !enabled {
		status = mcpStatusDisabled
	} else if len(server.Tools) == 0 || mcpToolFingerprintsNeedReview(server) {
		status = mcpStatusNeedsReview
	}
	permission := server.DefaultPermission
	if permission == "" {
		permission = ocrmcp.PermissionInherit
	}
	tools := append([]string(nil), server.Tools...)
	sort.Strings(tools)
	permissions := make(map[string]ocrmcp.Permission, len(server.ToolPermissions))
	for toolName, toolPermission := range server.ToolPermissions {
		permissions[toolName] = toolPermission
	}
	fingerprintStatus := make(map[string]string, len(tools))
	for _, toolName := range tools {
		state := mcpFingerprintNeedsReview
		if mcpToolFingerprintValid(server, toolName) {
			state = mcpFingerprintRecorded
		}
		fingerprintStatus[toolName] = state
	}
	return mcpServerSummary{
		Name:                     sanitizeMCPText(name, 80),
		Transport:                transport,
		Enabled:                  enabled,
		Status:                   status,
		Endpoint:                 sanitizeMCPText(redactedMCPEndpoint(server), 300),
		Environment:              mcpEnvironmentNames(server.Env),
		Headers:                  mcpHeaderNames(server.Headers),
		Tools:                    tools,
		DefaultPermission:        permission,
		ToolPermissions:          permissions,
		FingerprintStatus:        fingerprintStatus,
		ErrorCategory:            errorCategory,
		LegacyLiteralCredentials: hasLegacyLiteralMCPCredentials(server),
	}
}

func mcpToolFingerprintsNeedReview(server MCPServerConfig) bool {
	for _, toolName := range server.Tools {
		fingerprint := server.ToolDefinitionSHA256[toolName]
		if len(fingerprint) != 64 {
			return true
		}
		for _, r := range fingerprint {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				return true
			}
		}
	}
	return false
}

func mcpToolFingerprintValid(server MCPServerConfig, toolName string) bool {
	fingerprint := server.ToolDefinitionSHA256[toolName]
	if len(fingerprint) != 64 {
		return false
	}
	for _, r := range fingerprint {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// validateMCPPersistentAllow makes auto-run contingent on an exact discovered
// definition. It never expands Tools; a tools/edit rediscovery is required to
// accept a new or changed definition before allow can be persisted.
func validateMCPPersistentAllow(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	global := ocrmcp.MCPConfig{}
	if cfg.MCP != nil {
		global = *cfg.MCP
	}
	for serverName, server := range cfg.MCPServers {
		for _, toolName := range server.Tools {
			permission, err := ocrmcp.ResolvePermission(global, server, toolName)
			if err != nil {
				return err
			}
			if permission == ocrmcp.PermissionAllow && !mcpToolFingerprintValid(server, toolName) {
				return fmt.Errorf("cannot allow MCP tool %q on server %q without a current definition fingerprint; run 'ocr mcp tools %s --yes' or 'ocr mcp edit %s' first", toolName, serverName, serverName, serverName)
			}
		}
	}
	return nil
}

func mcpEnabled(config *ocrmcp.MCPConfig) bool {
	return config == nil || optionalBool(config.Enabled, true)
}

func optionalBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPointer(value bool) *bool { return &value }

func redactedMCPEndpoint(server MCPServerConfig) string {
	if server.Type == "remote" {
		parsed, err := url.Parse(server.URL)
		if err != nil || parsed.Host == "" {
			return "(invalid remote URL)"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	command := strings.TrimSpace(server.Command)
	if command == "" {
		return "(missing command)"
	}
	if len(server.Args) == 0 {
		return command
	}
	return fmt.Sprintf("%s (%d argument(s), redacted)", command, len(server.Args))
}

func mcpEnvironmentNames(entries []string) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name, _, _ := strings.Cut(entry, "=")
		name = sanitizeMCPText(strings.TrimSpace(name), 160)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func mcpHeaderNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		if name = sanitizeMCPText(name, 160); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func discoveredToolViews(tools []ocrmcp.DiscoveredTool) []mcpDiscoveredToolView {
	views := make([]mcpDiscoveredToolView, 0, len(tools))
	for _, tool := range tools {
		views = append(views, mcpDiscoveredToolView{
			Name:              sanitizeMCPText(tool.Name, 160),
			Description:       sanitizeMCPText(tool.Description, 240),
			DefinitionSHA256:  tool.DefinitionSHA256,
			UntrustedMetadata: true,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

func printDiscoveredTools(out io.Writer, serverName string, tools []mcpDiscoveredToolView) {
	fmt.Fprintf(out, "Discovered %d tool(s) from %q. No tool was invoked or enabled.\n", len(tools), serverName)
	fmt.Fprintln(out, "Descriptions below are untrusted server-provided metadata and never grant permission.")
	for _, tool := range tools {
		if tool.Description == "" {
			fmt.Fprintf(out, "  %s\n", tool.Name)
			continue
		}
		fmt.Fprintf(out, "  %s - %s\n", tool.Name, tool.Description)
	}
}

func sanitizeMCPText(value string, maxRunes int) string {
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}

func writeMCPJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func defaultMCPInteractiveTerminal() bool {
	if mcpCIEnvironment() || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd())
}

// mcpInteractiveSession is the single TTY/CI gate shared by management and
// runtime approval flows. Callers must still fail closed on prompt errors.
func mcpInteractiveSession() bool {
	return mcpInteractiveTerminal()
}

func mcpCIEnvironment() bool {
	for _, name := range mcpCIEnvironmentVariables {
		if mcpTruthyEnvironment(os.Getenv(name)) {
			return true
		}
	}
	return false
}

func mcpTruthyEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func mcpTitle(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func mcpPastTense(value string) string {
	switch value {
	case "enable":
		return "Enabled"
	case "disable":
		return "Disabled"
	default:
		return mcpTitle(value) + "d"
	}
}
