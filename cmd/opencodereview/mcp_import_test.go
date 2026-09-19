// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
)

func TestMCPImportFormatsAndCredentialConversion(t *testing.T) {
	for _, data := range []string{
		`{"mcpServers":{"docs":{"command":"npx","args":["-y","example"],"env":{"TOKEN":"SECRET","PASS":"${env:SOURCE}"},"enabled_tools":["everything"],"default_permission":"allow","setup":"touch BAD"}}}`,
		"[mcp_servers.docs]\ncommand = 'npx'\nargs = ['-y', 'example']\nenv = { TOKEN = 'SECRET', PASS = '${SOURCE}' }\nenabled_tools = ['everything']\ndefault_tools_approval_mode = 'approve'\n",
	} {
		servers, err := parseMCPImport([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		s, err := importedMCPConnection(servers["docs"])
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(s)
		if strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "everything") || s.Setup != "" || *s.Enabled || len(s.Tools) != 0 || s.DefaultPermission != ocrmcp.PermissionInherit {
			t.Fatal("unsafe import")
		}
		if strings.Join(s.Env, ",") != "PASS=${SOURCE},TOKEN=${TOKEN}" {
			t.Fatal(s.Env)
		}
	}
	for _, raw := range []string{
		`{"url":"https://example.test/mcp","bearer_token_env_var":"TOKEN"}`,
		`{"type":"http","url":"https://example.test/mcp","headers":{"Authorization":"SECRET"}}`,
		`{"type":"streamable-http","url":"https://example.test/mcp","http_headers":{"Authorization":"${env:TOKEN}"},"env_http_headers":{"X-Token":"OTHER"}}`,
		`{"command":"server","env_vars":["TOKEN"]}`,
	} {
		s, err := importedMCPConnection(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(s)
		if strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "env:") {
			t.Fatal("literal credential copied")
		}
	}
}

func TestMCPImportBearerEnvironmentTemplate(t *testing.T) {
	for _, value := range []string{"Bearer ${env:TOKEN}", "Bearer ${TOKEN}", "Bearer  ${env:TOKEN}", "Bearer  ${TOKEN}"} {
		raw, _ := json.Marshal(map[string]any{"url": "https://example.test/mcp", "headers": map[string]string{"Authorization": value}})
		s, err := importedMCPConnection(raw)
		if err != nil {
			t.Fatal(err)
		}
		if s.Headers["Authorization"] != "Bearer ${TOKEN}" {
			t.Fatalf("template lost: %q", s.Headers["Authorization"])
		}
	}
	if got := importMCPCredential("Bearer literal-secret", "FALLBACK"); got != "${FALLBACK}" {
		t.Fatal("literal copied", got)
	}
}

func TestMCPImportRejectsUnsupportedAndMalformed(t *testing.T) {
	for _, data := range []string{"", "bad TOML SECRET\n", "{ SECRET", `{}`, `{"mcpServers":{},"mcp_servers":{}}`, `{"mcpServers":[]}`, `{"mcpServers":{"bad name":{}}}`, strings.Repeat("x", mcpImportLimit+1)} {
		_, err := parseMCPImport([]byte(data))
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("unsafe parsing error", err)
		}
	}
	for _, raw := range []string{
		`null`, `[]`, `{"command":123}`, `{"command":"server","cwd":"/private"}`, `{"url":"https://example.test","oauth":{}}`,
		`{"type":"sse","url":"https://example.test"}`, `{"command":"server","url":"https://example.test"}`, `{"type":"stdio","command":"server","headers":{"X":"a"}}`,
		`{"url":"http://example.test"}`, `{"url":"https://user:SECRET@example.test"}`, `{"url":"https://example.test","headers":{"Host":"SECRET"}}`,
		`{"command":"server","env":{"BAD NAME":"SECRET"}}`, `{"command":"server","env_vars":["BAD NAME"]}`,
		`{"url":"https://example.test","headers":{"Authorization":"x"},"http_headers":{"authorization":"y"}}`,
		`{"url":"https://example.test","env_http_headers":{"X":"BAD NAME"}}`, `{"url":"https://example.test","bearer_token_env_var":"BAD NAME"}`,
		`{"url":"https://example.test","headers":{"Authorization":"x"},"bearer_token_env_var":"TOKEN"}`,
	} {
		_, err := importedMCPConnection(json.RawMessage(raw))
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("accepted unsupported configuration or leaked secret: %s: %v", raw, err)
		}
	}
}

func TestMCPImportCommandAtomicDisabledAndNoConnection(t *testing.T) {
	setupMCPTestHome(t, &Config{})
	setMCPTestInteractive(t, false)
	setMCPTestDiscovery(t, func(context.Context, string, MCPServerConfig) ([]ocrmcp.DiscoveredTool, error) {
		t.Fatal("import discovered tools")
		return nil, nil
	})
	data := `{"mcpServers":{"docs":{"command":"never-start","env":{"TOKEN":"SECRET"},"tools":["search"],"default_permission":"allow"}}}`
	cmd, out, stderr := newMCPTestCommand(data)
	if err := runMCPImport(cmd, "-", false); err == nil {
		t.Fatal("no noninteractive consent")
	}
	if err := runMCPImport(cmd, "", true); err == nil {
		t.Fatal("missing file accepted")
	}
	if err := runMCPImport(cmd, "-", true); err != nil {
		t.Fatal(err)
	}
	cfg := loadMCPTestConfig(t)
	s := cfg.MCPServers["docs"]
	if *s.Enabled || len(s.Tools) != 0 {
		t.Fatal("import activated tools")
	}
	if strings.Contains(out.String()+stderr.String(), "SECRET") {
		t.Fatal("secret in preview")
	}
	path, _ := resolveConfigPath()
	before, _ := os.ReadFile(path)
	cmd, _, _ = newMCPTestCommand(data)
	if runMCPImport(cmd, "-", true) == nil {
		t.Fatal("overwrote server")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed import wrote config")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows file modes do not represent ACLs; match the provider config tests.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("config not private")
	}
	cmd, _, _ = newMCPTestCommand(`{"mcpServers":{"a":{"command":"s"},"b":{"command":"s"}}}`)
	if runMCPImport(cmd, "-", true) == nil {
		t.Fatal("ambiguous noninteractive import")
	}
}

func TestMCPImportFileAndInteractiveCancellation(t *testing.T) {
	setupMCPTestHome(t, &Config{})
	setMCPTestInteractive(t, true)
	path := filepath.Join(t.TempDir(), "source.toml")
	if err := os.WriteFile(path, []byte("[mcp_servers.docs]\ncommand='server'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"cancel\n", "docs\nn\n"} {
		cmd, _, _ := newMCPTestCommand(input)
		if err := runMCPImport(cmd, path, false); err != nil {
			t.Fatal(err)
		}
		if len(loadMCPTestConfig(t).MCPServers) != 0 {
			t.Fatal("cancel saved")
		}
	}
	cmd, _, _ := newMCPTestCommand("file\n" + path + "\nrenamed\ny\n")
	if err := runMCPImport(cmd, "", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadMCPTestConfig(t).MCPServers["renamed"]; !ok {
		t.Fatal("rename lost")
	}
	cmd, _, _ = newMCPTestCommand("")
	if runMCPImport(cmd, path+"-missing", false) == nil || runMCPImport(cmd, filepath.Dir(path), false) == nil {
		t.Fatal("invalid file accepted")
	}
	cmd, _, _ = newMCPTestCommand("docs\ny\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	if runMCPImport(cmd, path, false) == nil {
		t.Fatal("cancelled context saved")
	}
	command := newMCPImportCommand()
	command.SetArgs([]string{path, "--yes"})
	setMCPTestInteractive(t, false)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestMCPImportPasteHidesText(t *testing.T) {
	input := textarea.New()
	input.CharLimit = mcpImportLimit
	_ = input.Focus()
	m := mcpImportPasteModel{input: input}
	if m.Init() != nil {
		t.Fatal("unexpected command")
	}
	next, _ := m.Update(tea.PasteMsg{Content: "{\nSECRET\n}"})
	m = next.(mcpImportPasteModel)
	if !strings.Contains(m.input.Value(), "SECRET") || strings.Contains(m.View().Content, "SECRET") {
		t.Fatal("paste lost or exposed")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !next.(mcpImportPasteModel).confirmed {
		t.Fatal("cannot submit")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(mcpImportPasteModel).confirmed {
		t.Fatal("cancel submitted")
	}
}
