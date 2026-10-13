// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/spf13/cobra"
)

type mcpWizardFailingReader struct{}

func (mcpWizardFailingReader) Read([]byte) (int, error) {
	return 0, errors.New("reader failed")
}

func mcpWizardCoverageTools() []ocrmcp.DiscoveredTool {
	return []ocrmcp.DiscoveredTool{
		{Name: "read", Description: "Read data", DefinitionSHA256: strings.Repeat("a", 64)},
		{Name: "write", Description: "Write data", DefinitionSHA256: strings.Repeat("b", 64)},
	}
}

func TestMCPWizardPromptFallbackCoverage(t *testing.T) {
	t.Run("command prompter uses command streams and context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), struct{}{}, "value")
		cmd := &cobra.Command{Use: "test"}
		in := strings.NewReader("answer\n")
		out := new(bytes.Buffer)
		cmd.SetContext(ctx)
		cmd.SetIn(in)
		cmd.SetErr(out)
		prompt := newMCPPrompter(cmd)
		if prompt.ctx != ctx || prompt.in != in || prompt.out != out {
			t.Fatalf("prompter did not retain command dependencies: %+v", prompt)
		}
		value, err := prompt.prompt("Label", "fallback")
		if err != nil || value != "answer" || !strings.Contains(out.String(), "Label [fallback]:") {
			t.Fatalf("prompt = %q, %v; output = %q", value, err, out.String())
		}
	})

	for _, tc := range []struct {
		name     string
		input    string
		fallback string
		want     string
		cancel   bool
	}{
		{name: "trim answer", input: "  chosen  \n", want: "chosen"},
		{name: "blank takes fallback", input: "\n", fallback: "default", want: "default"},
		{name: "case insensitive cancel", input: " CaNcEl \n", cancel: true},
		{name: "empty EOF cancels", input: "", fallback: "default", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			prompt := &mcpPrompter{ctx: context.Background(), in: strings.NewReader(tc.input), out: &out}
			got, err := prompt.prompt("Prompt", tc.fallback)
			if tc.cancel {
				if !errors.Is(err, errMCPPromptCancelled) {
					t.Fatalf("prompt error = %v, want cancellation", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("prompt = %q, %v; want %q", got, err, tc.want)
			}
		})
	}

	t.Run("reader error is wrapped", func(t *testing.T) {
		prompt := &mcpPrompter{ctx: context.Background(), in: mcpWizardFailingReader{}, out: io.Discard}
		if _, err := prompt.prompt("Prompt", ""); err == nil || !strings.Contains(err.Error(), "read MCP prompt") {
			t.Fatalf("prompt error = %v", err)
		}
	})

	t.Run("line reader handles CRLF partial EOF and errors", func(t *testing.T) {
		if got, err := readMCPPromptLine(strings.NewReader("a\r\nb")); err != nil || got != "a" {
			t.Fatalf("CRLF read = %q, %v", got, err)
		}
		if got, err := readMCPPromptLine(strings.NewReader("partial")); !errors.Is(err, io.EOF) || got != "partial" {
			t.Fatalf("partial read = %q, %v", got, err)
		}
		if got, err := readMCPPromptLine(mcpWizardFailingReader{}); err == nil || got != "" {
			t.Fatalf("failed read = %q, %v", got, err)
		}
	})

	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{name: "yes short", input: "y\n", want: true},
		{name: "yes long case insensitive", input: "YES\n", want: true},
		{name: "no", input: "n\n", want: false},
		{name: "cancel is denial", input: "cancel\n", want: false},
		{name: "EOF is denial", input: "", want: false},
	} {
		t.Run("confirm "+tc.name, func(t *testing.T) {
			prompt := &mcpPrompter{ctx: context.Background(), in: strings.NewReader(tc.input), out: io.Discard}
			got, err := prompt.confirm("Continue")
			if err != nil || got != tc.want {
				t.Fatalf("confirm = %v, %v; want %v", got, err, tc.want)
			}
		})
	}

	t.Run("confirm propagates reader error", func(t *testing.T) {
		prompt := &mcpPrompter{ctx: context.Background(), in: mcpWizardFailingReader{}, out: io.Discard}
		if _, err := prompt.confirm("Continue"); err == nil {
			t.Fatal("confirm accepted a reader failure")
		}
	})
}

func TestMCPConnectionWizardStdioSuccessCoverage(t *testing.T) {
	setupMCPTestHome(t, &Config{Language: "English"})
	setMCPTestInteractive(t, true)
	setMCPTestDiscovery(t, func(_ context.Context, name string, server MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		if name != "local" || server.Type != "stdio" || server.Command != "example-mcp" {
			t.Fatalf("discovery received %q %+v", name, server)
		}
		return mcpWizardCoverageTools(), nil
	})

	input := strings.Join([]string{
		"stdio",
		"example-mcp",
		`["--safe"]`,
		`["TOKEN=${MCP_TOKEN}"]`,
		"y",
		"1,write",
		"yes",
		"",
	}, "\n")
	cmd, stdout, stderr := newMCPTestCommand(input)
	if err := runMCPConnectionWizard(cmd, "local", false, mcpAddOptions{}); err != nil {
		t.Fatalf("runMCPConnectionWizard: %v\nstderr: %s", err, stderr.String())
	}
	cfg := loadMCPTestConfig(t)
	server := cfg.MCPServers["local"]
	if server.Type != "stdio" || server.Command != "example-mcp" || len(server.Args) != 1 || len(server.Env) != 1 {
		t.Fatalf("saved stdio server = %+v", server)
	}
	if got := strings.Join(server.Tools, ","); got != "read,write" {
		t.Fatalf("tools = %q", got)
	}
	for _, name := range server.Tools {
		if server.ToolPermissions[name] != ocrmcp.PermissionAsk || server.ToolDefinitionSHA256[name] == "" {
			t.Errorf("tool %q policy/fingerprint = %q/%q", name, server.ToolPermissions[name], server.ToolDefinitionSHA256[name])
		}
	}
	if server.Enabled == nil || !*server.Enabled || cfg.MCP == nil || cfg.MCP.Version != mcpConfigVersion {
		t.Fatalf("MCP defaults were not prepared: server=%+v global=%+v", server, cfg.MCP)
	}
	if !strings.Contains(stdout.String(), `Saved MCP server "local" with 2 enabled tool(s).`) {
		t.Errorf("stdout = %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "MCP_TOKEN") || !strings.Contains(stderr.String(), "Configuration to save") {
		t.Errorf("preview leaked a credential or omitted summary: %q", stderr.String())
	}
}

func TestMCPConnectionWizardRemoteEditCoverage(t *testing.T) {
	setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
		"remote": {
			Type: "remote", URL: "https://old.example/mcp", Setup: "legacy setup",
			Headers: map[string]string{"X-Old": "${OLD_TOKEN}"},
		},
	}})
	setMCPTestInteractive(t, true)
	setMCPTestDiscovery(t, func(_ context.Context, _ string, server MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		if !server.AllowInsecureHTTP || server.URL != "http://example.com/mcp" {
			t.Fatalf("remote confirmation was not applied before discovery: %+v", server)
		}
		return mcpWizardCoverageTools(), nil
	})

	input := strings.Join([]string{
		"", // keep remote transport
		"http://example.com/mcp",
		`{"Authorization":"Bearer ${MCP_TOKEN}"}`,
		"y", // permit non-loopback HTTP
		"y", // connect
		"2",
		"y", // save
		"",
	}, "\n")
	cmd, stdout, stderr := newMCPTestCommand(input)
	if err := runMCPConnectionWizard(cmd, "remote", true, mcpAddOptions{}); err != nil {
		t.Fatalf("edit remote wizard: %v\nstderr: %s", err, stderr.String())
	}
	server := loadMCPTestConfig(t).MCPServers["remote"]
	if server.Type != "remote" || server.URL != "http://example.com/mcp" || !server.AllowInsecureHTTP {
		t.Fatalf("saved remote server = %+v", server)
	}
	if server.Command != "" || server.Args != nil || server.Env != nil || server.Setup != "" {
		t.Fatalf("remote edit retained stdio/legacy state: %+v", server)
	}
	if len(server.Tools) != 1 || server.Tools[0] != "write" {
		t.Fatalf("remote tools = %v", server.Tools)
	}
	if !strings.Contains(stderr.String(), "Legacy setup command") || !strings.Contains(stdout.String(), "Saved MCP server") {
		t.Errorf("wizard output = %q / %q", stdout.String(), stderr.String())
	}
}

func TestMCPConnectionWizardFailureAndCancellationCoverage(t *testing.T) {
	tests := []struct {
		name        string
		initialName string
		editing     bool
		initial     *Config
		input       string
		discoverErr error
		wantErr     string
		cancelled   bool
		wantCalls   int
	}{
		{name: "prompted name then cancel", input: "local\ncancel\n", cancelled: true},
		{name: "invalid name", initialName: "bad name", wantErr: "invalid MCP server name"},
		{name: "edit missing", initialName: "missing", editing: true, wantErr: "not found"},
		{name: "add duplicate", initialName: "local", initial: &Config{MCPServers: map[string]MCPServerConfig{"local": {Type: "stdio", Command: "cmd"}}}, wantErr: "already exists"},
		{name: "invalid transport", initialName: "local", input: "bogus\n", wantErr: "invalid MCP server type"},
		{name: "bad arguments", initialName: "local", input: "stdio\ncmd\n[bad\n", wantErr: "parse arguments"},
		{name: "bad environment JSON", initialName: "local", input: "stdio\ncmd\n\n[bad\n", wantErr: "parse environment"},
		{name: "literal environment credential", initialName: "local", input: "stdio\ncmd\n\n[\"TOKEN=literal\"]\n", wantErr: "exact ${ENV_NAME} reference"},
		{name: "bad headers", initialName: "local", input: "remote\nhttps://example.com/mcp\n{bad\n", wantErr: "parse headers"},
		{name: "decline insecure HTTP", initialName: "local", input: "remote\nhttp://example.com/mcp\n\nn\n", cancelled: true},
		{name: "missing executable", initialName: "local", input: "stdio\n\n\n\n", wantErr: "command"},
		{name: "decline discovery", initialName: "local", input: "stdio\ncmd\n\n\nn\n", cancelled: true},
		{name: "discovery failure", initialName: "local", input: "stdio\ncmd\n\n\ny\n", discoverErr: errors.New("discovery failed"), wantErr: "discovery failed", wantCalls: 1},
		{name: "unknown selection", initialName: "local", input: "stdio\ncmd\n\n\ny\nmissing\n", wantErr: "was not returned", wantCalls: 1},
		{name: "decline save", initialName: "local", input: "stdio\ncmd\n\n\ny\n1\nn\n", cancelled: true, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.initial
			if cfg == nil {
				cfg = &Config{}
			}
			path := setupMCPTestHome(t, cfg)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			setMCPTestInteractive(t, true)
			calls := 0
			setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
				calls++
				if tc.discoverErr != nil {
					return nil, tc.discoverErr
				}
				return mcpWizardCoverageTools(), nil
			})
			cmd, stdout, _ := newMCPTestCommand(tc.input)
			err = runMCPConnectionWizard(cmd, tc.initialName, tc.editing, mcpAddOptions{})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("wizard error = %v, want %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("wizard error = %v", err)
			}
			if calls != tc.wantCalls {
				t.Fatalf("discovery calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.cancelled && !strings.Contains(stdout.String(), "Cancelled. No changes saved.") {
				t.Errorf("cancellation output = %q", stdout.String())
			}
			current, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(original, current) {
				t.Fatal("failed or cancelled wizard modified config")
			}
		})
	}
}

func TestMCPManagerUnknownAndCancelCoverage(t *testing.T) {
	setupMCPTestHome(t, &Config{})
	setMCPTestInteractive(t, true)
	cmd, _, stderr := newMCPTestCommand("unknown\nlocal\nquit\n")
	if err := runMCPManager(cmd); err != nil {
		t.Fatalf("runMCPManager: %v", err)
	}
	if !strings.Contains(stderr.String(), "MCP manager") || !strings.Contains(stderr.String(), `Unknown action "unknown"`) {
		t.Errorf("manager output = %q", stderr.String())
	}

	cmd, _, _ = newMCPTestCommand("cancel\n")
	if err := runMCPManager(cmd); err != nil {
		t.Fatalf("cancel manager: %v", err)
	}
}

func TestMCPToolsInteractiveAndJSONCoverage(t *testing.T) {
	t.Run("interactive selection and confirmation", func(t *testing.T) {
		setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
			"local": {
				Type: "stdio", Command: "cmd", Setup: "legacy",
				Tools:                []string{"read"},
				ToolPermissions:      map[string]ocrmcp.Permission{"read": ocrmcp.PermissionAllow},
				ToolDefinitionSHA256: map[string]string{"read": strings.Repeat("a", 64)},
			},
		}})
		setMCPTestInteractive(t, true)
		setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
			return mcpWizardCoverageTools(), nil
		})
		cmd, stdout, stderr := newMCPTestCommand("y\n2\ny\n")
		if err := runMCPTools(cmd, "local", mcpToolsOptions{}); err != nil {
			t.Fatalf("runMCPTools: %v", err)
		}
		server := loadMCPTestConfig(t).MCPServers["local"]
		if len(server.Tools) != 1 || server.Tools[0] != "write" || server.ToolPermissions["write"] != ocrmcp.PermissionAsk {
			t.Fatalf("saved tools = %+v", server)
		}
		if server.Setup != "" || !strings.Contains(stderr.String(), "Legacy setup command") || !strings.Contains(stdout.String(), "Removed the legacy setup") {
			t.Errorf("legacy setup/output = %+v / %q / %q", server, stdout.String(), stderr.String())
		}
	})

	t.Run("noninteractive JSON", func(t *testing.T) {
		setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
			"local": {Type: "stdio", Command: "cmd"},
		}})
		setMCPTestInteractive(t, false)
		setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
			return mcpWizardCoverageTools(), nil
		})
		cmd, stdout, _ := newMCPTestCommand("")
		if err := runMCPTools(cmd, "local", mcpToolsOptions{yes: true, json: true, enable: []string{"read"}}); err != nil {
			t.Fatalf("runMCPTools JSON: %v", err)
		}
		if !strings.Contains(stdout.String(), `"name": "local"`) || !strings.Contains(stdout.String(), `"read"`) {
			t.Errorf("JSON output = %q", stdout.String())
		}
	})
}

func TestMCPToolsFailureCoverage(t *testing.T) {
	t.Run("missing server", func(t *testing.T) {
		setupMCPTestHome(t, &Config{})
		cmd, _, _ := newMCPTestCommand("")
		if err := runMCPTools(cmd, "missing", mcpToolsOptions{yes: true}); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("missing server error = %v", err)
		}
	})

	t.Run("noninteractive selection requires flags", func(t *testing.T) {
		setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{"local": {Type: "stdio", Command: "cmd"}}})
		setMCPTestInteractive(t, false)
		cmd, _, _ := newMCPTestCommand("")
		if err := runMCPTools(cmd, "local", mcpToolsOptions{yes: true}); err == nil || !strings.Contains(err.Error(), "at least one") {
			t.Fatalf("selection error = %v", err)
		}
	})

	t.Run("discovery error", func(t *testing.T) {
		setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{"local": {Type: "stdio", Command: "cmd"}}})
		setMCPTestInteractive(t, false)
		setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
			return nil, errors.New("discovery failed")
		})
		cmd, _, _ := newMCPTestCommand("")
		if err := runMCPTools(cmd, "local", mcpToolsOptions{yes: true, enable: []string{"read"}}); err == nil || !strings.Contains(err.Error(), "discovery failed") {
			t.Fatalf("discovery error = %v", err)
		}
	})

	t.Run("interactive unknown selection and save denial", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			input string
			err   bool
		}{
			{name: "unknown", input: "y\nmissing\n", err: true},
			{name: "deny save", input: "y\n1\nn\n"},
			{name: "cancel selection", input: "y\ncancel\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{"local": {Type: "stdio", Command: "cmd"}}})
				setMCPTestInteractive(t, true)
				setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
					return mcpWizardCoverageTools(), nil
				})
				cmd, stdout, _ := newMCPTestCommand(tc.input)
				err := runMCPTools(cmd, "local", mcpToolsOptions{})
				if tc.err {
					if err == nil || !strings.Contains(err.Error(), "was not returned") {
						t.Fatalf("selection error = %v", err)
					}
				} else if err != nil || !strings.Contains(stdout.String(), "Cancelled") {
					t.Fatalf("cancellation = %v, %q", err, stdout.String())
				}
			})
		}
	})
}

func TestMCPPermissionsInteractiveCoverage(t *testing.T) {
	t.Run("global", func(t *testing.T) {
		setupMCPTestHome(t, &Config{})
		setMCPTestInteractive(t, true)
		cmd, stdout, _ := newMCPTestCommand("allow\n120\ny\n")
		if err := runMCPPermissions(cmd, "", mcpPermissionsOptions{}); err != nil {
			t.Fatalf("global permissions: %v", err)
		}
		cfg := loadMCPTestConfig(t)
		if cfg.MCP == nil || cfg.MCP.DefaultPermission != ocrmcp.PermissionAllow || cfg.MCP.ApprovalTimeoutSeconds != 120 {
			t.Fatalf("global permissions = %+v", cfg.MCP)
		}
		if !strings.Contains(stdout.String(), "Saved global MCP permission allow") {
			t.Errorf("stdout = %q", stdout.String())
		}
	})

	t.Run("server tools", func(t *testing.T) {
		setupMCPTestHome(t, &Config{MCPServers: map[string]MCPServerConfig{
			"local": {
				Type: "stdio", Command: "cmd", Setup: "legacy", Tools: []string{"read", "write"},
				ToolDefinitionSHA256: map[string]string{
					"read": strings.Repeat("a", 64), "write": strings.Repeat("b", 64),
				},
			},
		}})
		setMCPTestInteractive(t, true)
		cmd, stdout, stderr := newMCPTestCommand("ask\nallow\ndeny\ny\n")
		if err := runMCPPermissions(cmd, "local", mcpPermissionsOptions{}); err != nil {
			t.Fatalf("server permissions: %v", err)
		}
		server := loadMCPTestConfig(t).MCPServers["local"]
		if server.DefaultPermission != ocrmcp.PermissionAsk || server.ToolPermissions["read"] != ocrmcp.PermissionAllow || server.ToolPermissions["write"] != ocrmcp.PermissionDeny {
			t.Fatalf("server permissions = %+v", server)
		}
		if server.Setup != "" || !strings.Contains(stderr.String(), "Legacy setup command") || !strings.Contains(stdout.String(), "Saved permissions") {
			t.Errorf("permission output = %q / %q", stdout.String(), stderr.String())
		}
	})
}

func TestMCPPermissionsFailureCoverage(t *testing.T) {
	serverConfig := func() *Config {
		return &Config{MCPServers: map[string]MCPServerConfig{
			"local": {
				Type: "stdio", Command: "cmd", Tools: []string{"read"},
				ToolPermissions:      map[string]ocrmcp.Permission{"read": ocrmcp.PermissionAsk},
				ToolDefinitionSHA256: map[string]string{"read": strings.Repeat("a", 64)},
			},
		}}
	}
	tests := []struct {
		name        string
		server      string
		opts        mcpPermissionsOptions
		interactive bool
		input       string
		want        string
		cancel      bool
	}{
		{name: "noninteractive global needs flags", interactive: false, want: "require flags"},
		{name: "noninteractive flags need yes", interactive: false, opts: mcpPermissionsOptions{defaultPermission: "ask"}, want: "require --yes"},
		{name: "tool needs server", interactive: false, opts: mcpPermissionsOptions{yes: true, toolPermissions: []string{"read=ask"}}, want: "requires an MCP server name"},
		{name: "invalid global permission", interactive: false, opts: mcpPermissionsOptions{yes: true, defaultPermission: "maybe"}, want: "invalid MCP permission"},
		{name: "global timeout too low", interactive: false, opts: mcpPermissionsOptions{yes: true, timeoutSeconds: -1}, want: "between 1 and 600"},
		{name: "global timeout too high", interactive: false, opts: mcpPermissionsOptions{yes: true, timeoutSeconds: 601}, want: "between 1 and 600"},
		{name: "interactive bad timeout", interactive: true, input: "ask\nnot-a-number\n", want: "must be an integer"},
		{name: "interactive cancel global", interactive: true, input: "cancel\n", cancel: true},
		{name: "missing server", server: "missing", interactive: false, opts: mcpPermissionsOptions{yes: true, defaultPermission: "ask"}, want: "not found"},
		{name: "server timeout rejected", server: "local", interactive: false, opts: mcpPermissionsOptions{yes: true, timeoutSeconds: 30}, want: "is global"},
		{name: "bad server default", server: "local", interactive: false, opts: mcpPermissionsOptions{yes: true, defaultPermission: "maybe"}, want: "invalid MCP permission"},
		{name: "bad tool flag", server: "local", interactive: false, opts: mcpPermissionsOptions{yes: true, toolPermissions: []string{"broken"}}, want: "expected exact TOOL=PERMISSION"},
		{name: "tool outside allowlist", server: "local", interactive: false, opts: mcpPermissionsOptions{yes: true, toolPermissions: []string{"write=ask"}}, want: "outside the explicit allowlist"},
		{name: "interactive cancel server", server: "local", interactive: true, input: "cancel\n", cancel: true},
		{name: "interactive deny save", server: "local", interactive: true, input: "ask\nask\nn\n", cancel: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupMCPTestHome(t, serverConfig())
			setMCPTestInteractive(t, tc.interactive)
			cmd, stdout, _ := newMCPTestCommand(tc.input)
			err := runMCPPermissions(cmd, tc.server, tc.opts)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("permission error = %v, want %q", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("permission flow = %v", err)
			}
			if tc.cancel && !strings.Contains(stdout.String(), "Cancelled") {
				t.Errorf("cancellation output = %q", stdout.String())
			}
		})
	}

	t.Run("inherit removes explicit permission", func(t *testing.T) {
		setupMCPTestHome(t, serverConfig())
		setMCPTestInteractive(t, false)
		cmd, _, _ := newMCPTestCommand("")
		if err := runMCPPermissions(cmd, "local", mcpPermissionsOptions{yes: true, toolPermissions: []string{"read=inherit"}}); err != nil {
			t.Fatal(err)
		}
		if got := loadMCPTestConfig(t).MCPServers["local"].ToolPermissions; got != nil {
			t.Fatalf("inherit retained explicit permissions: %v", got)
		}
	})
}

func TestMCPWizardParsingSelectionAndApplyCoverage(t *testing.T) {
	t.Run("parse lists", func(t *testing.T) {
		for _, tc := range []struct {
			value string
			want  []string
			err   bool
		}{
			{value: "", want: nil},
			{value: "   ", want: nil},
			{value: `["one","two"]`, want: []string{"one", "two"}},
			{value: " one, two ,three ", want: []string{"one", "two", "three"}},
			{value: "[bad", err: true},
			{value: `[1]`, err: true},
		} {
			got, err := parseMCPWizardList(tc.value)
			if tc.err {
				if err == nil {
					t.Errorf("parseMCPWizardList(%q) unexpectedly succeeded", tc.value)
				}
				continue
			}
			if err != nil || strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("parseMCPWizardList(%q) = %v, %v; want %v", tc.value, got, err, tc.want)
			}
		}
	})

	tools := mcpWizardCoverageTools()
	t.Run("select names numbers and duplicates", func(t *testing.T) {
		selected, err := selectMCPDiscoveredTools(tools, "read,2,read")
		if err != nil || len(selected) != 2 || selected[0].Name != "read" || selected[1].Name != "write" {
			t.Fatalf("selected = %v, %v", selected, err)
		}
		if selected, err = selectMCPDiscoveredTools(tools, ""); err != nil || selected != nil {
			t.Fatalf("blank selection = %v, %v", selected, err)
		}
	})
	for _, selection := range []string{"missing", "0", "3", "[bad"} {
		t.Run("reject "+selection, func(t *testing.T) {
			if _, err := selectMCPDiscoveredTools(tools, selection); err == nil {
				t.Fatalf("selection %q succeeded", selection)
			}
		})
	}

	discovered := map[string]ocrmcp.DiscoveredTool{"read": tools[0], "write": tools[1]}
	t.Run("apply repeated enable keeps one grant", func(t *testing.T) {
		server := MCPServerConfig{Tools: []string{"read"}}
		enable := make([]string, 1024)
		for index := range enable {
			enable[index] = "read"
		}
		if err := applyMCPToolChanges(&server, discovered, enable, nil); err != nil {
			t.Fatal(err)
		}
		if len(server.Tools) != 1 || server.Tools[0] != "read" || server.ToolPermissions["read"] != ocrmcp.PermissionAsk || server.ToolDefinitionSHA256["read"] != tools[0].DefinitionSHA256 {
			t.Fatalf("duplicate enable changed grant: %+v", server)
		}
	})
	t.Run("apply enable disable and sort", func(t *testing.T) {
		server := MCPServerConfig{
			Tools: []string{"old", "write"},
			ToolPermissions: map[string]ocrmcp.Permission{
				"old": ocrmcp.PermissionAllow, "write": ocrmcp.PermissionDeny,
			},
			ToolDefinitionSHA256: map[string]string{
				"old": strings.Repeat("c", 64), "write": tools[1].DefinitionSHA256,
			},
		}
		if err := applyMCPToolChanges(&server, discovered, []string{"read", "write"}, []string{"old", "old"}); err != nil {
			t.Fatal(err)
		}
		if strings.Join(server.Tools, ",") != "read,write" || server.ToolPermissions["read"] != ocrmcp.PermissionAsk || server.ToolPermissions["write"] != ocrmcp.PermissionDeny {
			t.Fatalf("applied server = %+v", server)
		}
	})

	t.Run("apply errors", func(t *testing.T) {
		for _, tc := range []struct {
			enable  []string
			disable []string
			want    string
		}{
			{enable: []string{"read"}, disable: []string{"read"}, want: "both enabled and disabled"},
			{enable: []string{"missing"}, want: "was not returned"},
		} {
			server := MCPServerConfig{}
			if err := applyMCPToolChanges(&server, discovered, tc.enable, tc.disable); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("apply error = %v, want %q", err, tc.want)
			}
		}
	})

	t.Run("apply disable cleans maps", func(t *testing.T) {
		server := MCPServerConfig{
			Tools:                []string{"read"},
			ToolPermissions:      map[string]ocrmcp.Permission{"read": ocrmcp.PermissionAsk},
			ToolDefinitionSHA256: map[string]string{"read": tools[0].DefinitionSHA256},
		}
		if err := applyMCPToolChanges(&server, discovered, nil, []string{"read"}); err != nil {
			t.Fatal(err)
		}
		if len(server.Tools) != 0 || server.ToolPermissions != nil || server.ToolDefinitionSHA256 != nil {
			t.Fatalf("disabled server retained state: %+v", server)
		}
	})

	t.Run("definition drift match missing and mismatch", func(t *testing.T) {
		server := MCPServerConfig{
			Tools: []string{"read", "write", "missing"},
			ToolPermissions: map[string]ocrmcp.Permission{
				"read": ocrmcp.PermissionAllow, "write": ocrmcp.PermissionAllow,
			},
			ToolDefinitionSHA256: map[string]string{
				"read": tools[0].DefinitionSHA256, "write": strings.Repeat("c", 64), "missing": strings.Repeat("d", 64),
			},
		}
		markMCPDefinitionDrift(&server, discovered)
		if server.ToolPermissions["read"] != ocrmcp.PermissionAllow || server.ToolDefinitionSHA256["read"] == "" {
			t.Errorf("matching definition changed: %+v", server)
		}
		for _, name := range []string{"write", "missing"} {
			if server.ToolPermissions[name] != ocrmcp.PermissionAsk || server.ToolDefinitionSHA256[name] != "" {
				t.Errorf("drift for %q = %+v", name, server)
			}
		}
	})
}

func TestMCPWizardPermissionFlagCoverage(t *testing.T) {
	permissions, err := parseMCPPermissionFlags([]string{" read = allow ", "write=deny", "read=ask", "other=inherit"})
	if err != nil {
		t.Fatal(err)
	}
	if permissions["read"] != ocrmcp.PermissionAsk || permissions["write"] != ocrmcp.PermissionDeny || permissions["other"] != ocrmcp.PermissionInherit {
		t.Fatalf("permissions = %v", permissions)
	}
	for _, value := range []string{"broken", "=ask", "read=unknown"} {
		if _, err := parseMCPPermissionFlags([]string{value}); err == nil {
			t.Errorf("permission flag %q succeeded", value)
		}
	}
}

func TestMCPWizardPreviewRedactionAndSummaryCoverage(t *testing.T) {
	t.Run("remote preview", func(t *testing.T) {
		var out bytes.Buffer
		printMCPConnectionPreview(&out, MCPServerConfig{
			Type: "remote", URL: "https://user:pass@example.com/mcp?token=secret#fragment",
			Headers: map[string]string{"X-Z": "hidden", "Authorization": "hidden"},
		})
		got := out.String()
		for _, secret := range []string{"user", "pass", "token=", "secret", "fragment"} {
			if strings.Contains(got, secret) {
				t.Errorf("remote preview leaked %q: %s", secret, got)
			}
		}
		if !strings.Contains(got, "https://example.com/mcp") || !strings.Contains(got, "Authorization, X-Z") {
			t.Errorf("remote preview = %q", got)
		}
	})

	t.Run("stdio preview", func(t *testing.T) {
		var out bytes.Buffer
		printMCPConnectionPreview(&out, MCPServerConfig{
			Type: "stdio", Command: "example-mcp",
			Args: []string{
				"--token=first", "--password", "second", "visible",
				"https://user:pass@example.com/mcp?x=third#fragment", "--secret",
			},
			Env: []string{"Z_TOKEN=hidden", "A_REGION=hidden"},
		})
		got := out.String()
		for _, secret := range []string{"first", "second", "third", "user:pass", "fragment"} {
			if strings.Contains(got, secret) {
				t.Errorf("stdio preview leaked %q: %s", secret, got)
			}
		}
		for _, visible := range []string{"example-mcp", "--token=<redacted>", "--password", "<redacted>", "visible", "A_REGION, Z_TOKEN"} {
			if !strings.Contains(got, visible) {
				t.Errorf("stdio preview omitted %q: %s", visible, got)
			}
		}
	})

	redacted := redactedMCPCommandArguments([]string{"--plain=value", "relative", "://bad"})
	if strings.Join(redacted, ",") != "--plain=value,relative,://bad" {
		t.Errorf("ordinary arguments changed: %v", redacted)
	}
	for _, value := range []string{"--TOKEN", "api-key", "passwd-file", "Authorization", "private-key-path"} {
		if !mcpSensitiveArgumentName(value) {
			t.Errorf("sensitive argument %q was not recognized", value)
		}
	}
	if mcpSensitiveArgumentName("--region") {
		t.Fatal("ordinary region argument was marked sensitive")
	}

	if got := mcpRemoteEndpointPreview("://bad"); got != "(invalid remote URL)" {
		t.Errorf("invalid endpoint preview = %q", got)
	}
	if got := mcpRemoteEndpointPreview("relative/path"); got != "(invalid remote URL)" {
		t.Errorf("relative endpoint preview = %q", got)
	}
	if got := mcpRemoteEndpointPreview("https://user:pass@example.com/mcp?q=secret#fragment"); got != "https://example.com/mcp" {
		t.Errorf("sanitized endpoint preview = %q", got)
	}

	t.Run("full summary", func(t *testing.T) {
		var out bytes.Buffer
		printMCPServerSummary(&out, mcpServerSummary{
			Name: "local", Transport: "stdio", Endpoint: "example-mcp", Status: mcpStatusError,
			Tools: []string{"read\x1b[31m", "write"}, DefaultPermission: ocrmcp.PermissionAsk,
			FingerprintStatus:        map[string]string{"read\x1b[31m": mcpFingerprintNeedsReview, "write": mcpFingerprintRecorded},
			ErrorCategory:            "invalid_server_policy",
			Environment:              []string{"TOKEN"},
			Headers:                  []string{"Authorization"},
			LegacyLiteralCredentials: true,
		})
		got := out.String()
		for _, want := range []string{
			"Server: local", "Transport: stdio", "Status: error", "Enabled tools: 2",
			"Error category: invalid_server_policy", "fingerprint=needs-review", "fingerprint=recorded",
			"Environment names: TOKEN", "Header names: Authorization", "deprecated literal values",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("summary omitted %q: %s", want, got)
			}
		}
		if strings.Contains(got, "\x1b") {
			t.Errorf("summary retained control sequence: %q", got)
		}
	})

	t.Run("minimal summary", func(t *testing.T) {
		var out bytes.Buffer
		printMCPServerSummary(&out, mcpServerSummary{Name: "empty", Transport: "remote", Status: mcpStatusReady})
		if strings.Contains(out.String(), "Error category") || strings.Contains(out.String(), "Environment names") || strings.Contains(out.String(), "Header names") {
			t.Errorf("minimal summary emitted optional sections: %q", out.String())
		}
	})
}
