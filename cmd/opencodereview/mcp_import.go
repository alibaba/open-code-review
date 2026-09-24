// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

const mcpImportLimit = 1 << 20

func newMCPImportCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "import [file]", Short: "Import one disabled connection from Cursor JSON or Codex TOML", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		return runMCPImport(cmd, path, yes)
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm non-interactive import (one server only; no connection or authorization)")
	return cmd
}

// Parse with maintained format libraries. Unrelated application configuration
// is never imported. In particular, tools and policies are not capabilities.
func parseMCPImport(data []byte) (map[string]json.RawMessage, error) {
	if len(data) == 0 || len(data) > mcpImportLimit {
		return nil, fmt.Errorf("import must contain 1 byte to 1 MiB")
	}
	var root map[string]json.RawMessage
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		if json.Unmarshal(data, &root) != nil {
			return nil, fmt.Errorf("invalid JSON import; source contents hidden")
		}
	} else {
		var doc map[string]any
		if toml.Unmarshal(data, &doc) != nil {
			return nil, fmt.Errorf("invalid TOML import; source contents hidden")
		}
		encoded, err := json.Marshal(doc)
		if err != nil || json.Unmarshal(encoded, &root) != nil {
			return nil, fmt.Errorf("unsupported TOML import")
		}
	}
	_, cursor := root["mcpServers"]
	_, codex := root["mcp_servers"]
	if cursor == codex {
		return nil, fmt.Errorf("provide exactly one mcpServers (JSON) or mcp_servers (TOML) table")
	}
	key := "mcp_servers"
	if cursor {
		key = "mcpServers"
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(root[key], &servers) != nil || len(servers) == 0 || len(servers) > 64 {
		return nil, fmt.Errorf("import must contain 1-64 servers")
	}
	for name := range servers {
		if validateMCPServerName(name) != nil {
			return nil, fmt.Errorf("import has an invalid server name; use 1-64 ASCII letters, digits, hyphens or underscores")
		}
	}
	return servers, nil
}

func importedMCPConnection(data json.RawMessage) (MCPServerConfig, error) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil || raw == nil {
		return MCPServerConfig{}, fmt.Errorf("invalid server definition")
	}
	// Unsupported execution and auth features must not silently change meaning.
	allowed := strings.Fields("type command args env env_vars url headers http_headers env_http_headers bearer_token_env_var enabled disabled enabled_tools disabled_tools tools tool_permissions tool_definition_sha256 default_permission default_tools_approval_mode startup_timeout_sec tool_timeout_sec required setup")
	for key := range raw {
		if !slices.Contains(allowed, key) {
			return MCPServerConfig{}, fmt.Errorf("unsupported connection field; import supports command/args/env or URL/headers, not OAuth, cwd or helper commands")
		}
	}
	var src struct {
		Type           string            `json:"type"`
		Command        string            `json:"command"`
		Args           []string          `json:"args"`
		Env            map[string]string `json:"env"`
		EnvVars        []string          `json:"env_vars"`
		URL            string            `json:"url"`
		Headers        map[string]string `json:"headers"`
		HTTPHeaders    map[string]string `json:"http_headers"`
		EnvHTTPHeaders map[string]string `json:"env_http_headers"`
		Bearer         string            `json:"bearer_token_env_var"`
	}
	if json.Unmarshal(data, &src) != nil {
		return MCPServerConfig{}, fmt.Errorf("invalid connection field types; source contents hidden")
	}
	s := MCPServerConfig{Type: src.Type, Command: src.Command, Args: src.Args, URL: src.URL, Enabled: boolPointer(false), DefaultPermission: ocrmcp.PermissionInherit}
	if s.Type == "" {
		if s.URL != "" {
			s.Type = "remote"
		} else {
			s.Type = "stdio"
		}
	}
	if s.Type == "http" || s.Type == "streamable-http" {
		s.Type = "remote"
	}
	if (s.Type == "stdio" && (s.URL != "" || len(src.Headers)+len(src.HTTPHeaders)+len(src.EnvHTTPHeaders) > 0 || src.Bearer != "")) || (s.Type == "remote" && (s.Command != "" || len(s.Args)+len(src.Env)+len(src.EnvVars) > 0)) {
		return MCPServerConfig{}, fmt.Errorf("mixed stdio and remote connection settings")
	}
	env := make(map[string]string)
	for _, name := range src.EnvVars {
		if !mcpExactEnvironmentReferencePattern.MatchString("${" + name + "}") {
			return s, fmt.Errorf("invalid environment variable name")
		}
		env[name] = "${" + name + "}"
	}
	for name, value := range src.Env {
		if !mcpExactEnvironmentReferencePattern.MatchString("${" + name + "}") {
			return s, fmt.Errorf("invalid environment variable name")
		}
		env[name] = importMCPCredential(value, name)
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		s.Env = append(s.Env, key+"="+env[key])
	}
	s.Headers = make(map[string]string)
	addHeader := func(name, value string) error {
		for existing := range s.Headers {
			if strings.EqualFold(existing, name) {
				return fmt.Errorf("duplicate imported header")
			}
		}
		s.Headers[name] = value
		return nil
	}
	for _, headers := range []map[string]string{src.Headers, src.HTTPHeaders} {
		for name, value := range headers {
			reference := "OCR_MCP_" + strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' {
					return r - 'a' + 'A'
				}
				if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
					return r
				}
				return '_'
			}, name)
			if err := addHeader(name, importMCPHeaderCredential(value, reference)); err != nil {
				return s, err
			}
		}
	}
	for name, variable := range src.EnvHTTPHeaders {
		if !mcpExactEnvironmentReferencePattern.MatchString("${" + variable + "}") {
			return s, fmt.Errorf("invalid header environment variable name")
		}
		if err := addHeader(name, "${"+variable+"}"); err != nil {
			return s, err
		}
	}
	if src.Bearer != "" {
		if !mcpExactEnvironmentReferencePattern.MatchString("${" + src.Bearer + "}") {
			return s, fmt.Errorf("invalid bearer environment variable name")
		}
		if err := addHeader("Authorization", "Bearer ${"+src.Bearer+"}"); err != nil {
			return s, err
		}
	}
	if validateMCPConnection(s) != nil {
		return s, fmt.Errorf("invalid imported connection; verify executable, HTTPS URL and header names")
	}
	return s, nil
}

func importMCPCredential(value, fallback string) string {
	value = strings.ReplaceAll(value, "${env:", "${")
	if mcpExactEnvironmentReferencePattern.MatchString(value) {
		return value
	}
	// Do not copy a literal token from another application's config. For headers,
	// the referenced environment variable contains the entire header value.
	return "${" + fallback + "}"
}

func importMCPHeaderCredential(value, fallback string) string {
	value = strings.ReplaceAll(value, "${env:", "${")
	// Preserve an authentication scheme plus a reference, never literal secret
	// suffixes. Environment entries continue to require an exact reference.
	if scheme, reference, ok := strings.Cut(value, " "); ok &&
		(strings.EqualFold(scheme, "Bearer") || strings.EqualFold(scheme, "Basic")) &&
		mcpExactEnvironmentReferencePattern.MatchString(strings.TrimLeft(reference, " ")) {
		return scheme + " " + strings.TrimLeft(reference, " ")
	}
	return importMCPCredential(value, fallback)
}

func runMCPImport(cmd *cobra.Command, path string, yes bool) error {
	interactive := mcpInteractiveSession()
	if !interactive && (!yes || path == "") {
		return fmt.Errorf("non-interactive import requires a file (or - for stdin) and --yes")
	}
	p := newMCPPrompter(cmd)
	var data []byte
	var err error
	if path == "" {
		choice, err := p.choose("Import from file or paste configuration", "file", []string{"file", "paste"})
		if err != nil {
			if errors.Is(err, errMCPPromptCancelled) {
				return nil
			}
			return err
		}
		if choice == "paste" {
			data, err = runMCPImportPaste(cmd)
		} else {
			path, err = p.prompt("Import file path (JSON or TOML)", "")
		}
		if err != nil {
			if errors.Is(err, errMCPPromptCancelled) {
				return nil
			}
			return err
		}
	}
	if data == nil {
		var reader io.Reader = cmd.InOrStdin()
		if path != "-" {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("cannot open import file")
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("import source must be a regular file")
			}
			reader = f
		}
		data, err = io.ReadAll(io.LimitReader(reader, mcpImportLimit+1))
		if err != nil {
			return fmt.Errorf("cannot read import source")
		}
	}
	servers, err := parseMCPImport(data)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	name := names[0]
	if len(names) > 1 {
		if !interactive {
			return fmt.Errorf("non-interactive import requires exactly one server")
		}
		name, err = p.choose("Select one connection to import", name, names)
		if err != nil {
			if errors.Is(err, errMCPPromptCancelled) {
				return nil
			}
			return err
		}
	}
	server, err := importedMCPConnection(servers[name])
	if err != nil {
		return err
	}
	if interactive {
		name, err = p.prompt("Connection name for imported server", name)
		if err != nil {
			if errors.Is(err, errMCPPromptCancelled) {
				return nil
			}
			return err
		}
	}
	if validateMCPServerName(name) != nil {
		return fmt.Errorf("invalid imported connection name")
	}
	configPath, cfg, err := loadWritableMCPConfig()
	if err != nil {
		return err
	}
	if _, exists := cfg.MCPServers[name]; exists {
		return fmt.Errorf("connection name already exists; import never overwrites it")
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Import preview: disabled, zero tools, no permissions copied. No server will be started.")
	printMCPConnectionPreview(cmd.ErrOrStderr(), server)
	fmt.Fprintln(cmd.ErrOrStderr(), "Literal env/header values are NOT copied. Set these environment references before connecting:")
	for _, entry := range server.Env {
		fmt.Fprintln(cmd.ErrOrStderr(), sanitizeMCPText(entry, 160))
	}
	for _, key := range mcpHeaderNames(server.Headers) {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", key, sanitizeMCPText(server.Headers[key], 160))
	}
	if interactive {
		ok, err := p.confirm("Save disabled connection only")
		if err != nil || !ok {
			return err
		}
	}
	if ctx := cmd.Context(); ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	draft, err := cloneAppConfig(cfg)
	if err != nil {
		return err
	}
	if draft.MCPServers == nil {
		draft.MCPServers = make(map[string]MCPServerConfig)
	}
	draft.MCPServers[name] = server
	prepareMCPManagerSave(draft, name)
	if err := saveConfig(configPath, draft); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported disabled connection %q. Use Edit connection to discover, select tools and enable it.\n", name)
	return nil
}

type mcpImportPasteModel struct {
	input     textarea.Model
	confirmed bool
}

func (m mcpImportPasteModel) Init() tea.Cmd { return nil }
func (m mcpImportPasteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+s":
			m.confirmed = true
			return m, tea.Quit
		case "esc", "ctrl+c", "ctrl+d":
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(msg)
	return m, command
}
func (m mcpImportPasteModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("Paste MCP configuration (JSON / TOML)\nContents hidden: %d characters. Nothing runs on paste.\nCtrl-S imports for preview. Esc cancels.\n", len(m.input.Value())))
}
func runMCPImportPaste(cmd *cobra.Command) ([]byte, error) {
	input := textarea.New()
	input.CharLimit = mcpImportLimit
	input.SetHeight(5)
	_ = input.Focus()
	result, err := runMCPModel(cmd.Context(), mcpImportPasteModel{input: input}, cmd.InOrStdin(), cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}
	m := result.(mcpImportPasteModel)
	if !m.confirmed {
		return nil, errMCPPromptCancelled
	}
	return []byte(m.input.Value()), nil
}
