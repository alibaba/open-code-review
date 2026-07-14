package main

import (
	"os"
	"testing"
)

func TestSetConfigValueAuthHeaderNormalizesKnownValues(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.auth_header", " bearer "); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	if cfg.Llm.AuthHeader != "authorization" {
		t.Errorf("AuthHeader = %q, want %q", cfg.Llm.AuthHeader, "authorization")
	}
}

func TestSetConfigValueAuthHeaderRejectsCustomHeader(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.auth_header", " X-Custom-Auth "); err == nil {
		t.Fatal("expected error for unsupported auth_header, got nil")
	}
}

func TestSetConfigValueProvider(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "provider", "anthropic"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "anthropic")
	}
}

func TestSetConfigValueModel(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "model", "claude-opus-4-6"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-opus-4-6")
	}
}

func TestSetConfigValueModelWithProvider(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		Providers: map[string]ProviderEntry{
			"anthropic": {APIKey: "sk-test"},
		},
	}

	if err := setConfigValue(cfg, "model", "claude-opus-4-6"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Providers["anthropic"].Model != "claude-opus-4-6" {
		t.Errorf("entry Model = %q, want %q", cfg.Providers["anthropic"].Model, "claude-opus-4-6")
	}
	if cfg.Model != "" {
		t.Errorf("top-level Model = %q, want empty (should write to provider entry)", cfg.Model)
	}
}

func TestSetConfigValueProviderEntry(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.api_key", "sk-ant-test"); err != nil {
		t.Fatalf("setConfigValue api_key: %v", err)
	}
	if cfg.Providers["anthropic"].APIKey != "sk-ant-test" {
		t.Errorf("api_key = %q, want %q", cfg.Providers["anthropic"].APIKey, "sk-ant-test")
	}

	if err := setConfigValue(cfg, "providers.anthropic.model", "claude-opus-4-6"); err != nil {
		t.Fatalf("setConfigValue model: %v", err)
	}
	if cfg.Providers["anthropic"].Model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", cfg.Providers["anthropic"].Model, "claude-opus-4-6")
	}
}

func TestSetConfigValueProviderEntryNonPresetWritesCustomProvider(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.my-gateway.url", "https://gateway.internal.com/v1"); err != nil {
		t.Fatalf("setConfigValue url: %v", err)
	}

	if cfg.Providers != nil {
		if _, ok := cfg.Providers["my-gateway"]; ok {
			t.Fatal("non-preset providers.<name> should be stored in CustomProviders, not Providers")
		}
	}
	if cfg.CustomProviders["my-gateway"].URL != "https://gateway.internal.com/v1" {
		t.Errorf("custom provider URL = %q", cfg.CustomProviders["my-gateway"].URL)
	}
}

func TestSetConfigValueProviderEntryModelsJSON(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.models", `["llama-3-70b","llama-3-8b","llama-3-70b"]`); err != nil {
		t.Fatalf("setConfigValue models: %v", err)
	}

	got := cfg.CustomProviders["my-gateway"].Models
	want := []string{"llama-3-70b", "llama-3-8b"}
	if len(got) != len(want) {
		t.Fatalf("models length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetConfigValueProviderEntryModelsCommaSeparated(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.models", " llama-3-70b, llama-3-8b ,, llama-3-70b "); err != nil {
		t.Fatalf("setConfigValue models: %v", err)
	}

	got := cfg.CustomProviders["my-gateway"].Models
	want := []string{"llama-3-70b", "llama-3-8b"}
	if len(got) != len(want) {
		t.Fatalf("models length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetConfigValueProviderEntryModelsUnquotedBracketList(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.models", "[llama-3-70b,llama-3-8b]"); err != nil {
		t.Fatalf("setConfigValue models: %v", err)
	}

	got := cfg.CustomProviders["my-gateway"].Models
	want := []string{"llama-3-70b", "llama-3-8b"}
	if len(got) != len(want) {
		t.Fatalf("models length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetConfigValueProviderEntryProtocol(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.custom.protocol", "openai"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.CustomProviders["custom"].Protocol != "openai" {
		t.Errorf("protocol = %q, want %q", cfg.CustomProviders["custom"].Protocol, "openai")
	}

	if err := setConfigValue(cfg, "custom_providers.custom.protocol", "invalid"); err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

func TestSetConfigValueProviderEntryInvalidKey(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.unknown_field", "value"); err == nil {
		t.Fatal("expected error for unknown provider field")
	}
}

func TestSetConfigValueProviderEntryInvalidPath(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic", "value"); err == nil {
		t.Fatal("expected error for incomplete provider path")
	}
}

func TestSetConfigValueProviderEntryExtraBody(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.extra_body", `{"thinking":{"type":"disabled"}}`); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Providers["anthropic"].ExtraBody == nil {
		t.Fatal("extra_body should not be nil")
	}
	if _, ok := cfg.Providers["anthropic"].ExtraBody["thinking"]; !ok {
		t.Error("extra_body missing 'thinking' key")
	}
}

func TestSetConfigValueModelWithCustomProvider(t *testing.T) {
	cfg := &Config{
		Provider: "my-gateway",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway": {URL: "https://gw.example.com/v1", Protocol: "openai"},
		},
	}

	if err := setConfigValue(cfg, "model", "llama-3-70b"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.CustomProviders["my-gateway"].Model != "llama-3-70b" {
		t.Errorf("entry Model = %q, want %q", cfg.CustomProviders["my-gateway"].Model, "llama-3-70b")
	}
	if cfg.Model != "" {
		t.Errorf("top-level Model = %q, want empty (should write to custom provider entry)", cfg.Model)
	}
}

func TestSetConfigValueLlmExtraHeaders(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.extra_headers", "X-Custom=val1, X-Org=val2"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	if cfg.Llm.ExtraHeaders == nil {
		t.Fatal("ExtraHeaders should not be nil")
	}
	if v := cfg.Llm.ExtraHeaders["X-Custom"]; v != "val1" {
		t.Errorf("ExtraHeaders[\"X-Custom\"] = %q, want %q", v, "val1")
	}
	if v := cfg.Llm.ExtraHeaders["X-Org"]; v != "val2" {
		t.Errorf("ExtraHeaders[\"X-Org\"] = %q, want %q", v, "val2")
	}
}

func TestSetConfigValueLlmExtraHeadersInvalid(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.extra_headers", "no-equals-sign"); err == nil {
		t.Fatal("expected error for invalid extra headers, got nil")
	}
}

func TestSetConfigValueLlmExtraHeadersReservedRejected(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.extra_headers", "Authorization=bad"); err == nil {
		t.Fatal("expected error for reserved header, got nil")
	}
}

func TestSetConfigValueProviderExtraHeaders(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.extra_headers", "X-Custom=val1, X-Org=val2"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	entry := cfg.Providers["anthropic"]
	if entry.ExtraHeaders == nil {
		t.Fatal("ExtraHeaders should not be nil")
	}
	if v := entry.ExtraHeaders["X-Custom"]; v != "val1" {
		t.Errorf("ExtraHeaders[\"X-Custom\"] = %q, want %q", v, "val1")
	}
	if v := entry.ExtraHeaders["X-Org"]; v != "val2" {
		t.Errorf("ExtraHeaders[\"X-Org\"] = %q, want %q", v, "val2")
	}
}

func TestSetConfigValueProviderExtraHeadersInvalid(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "providers.anthropic.extra_headers", "=missing-key"); err == nil {
		t.Fatal("expected error for invalid extra headers, got nil")
	}
}

func TestSetConfigValueCustomProviderExtraHeaders(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "custom_providers.my-gateway.extra_headers", "X-Gateway=secret"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	entry := cfg.CustomProviders["my-gateway"]
	if entry.ExtraHeaders == nil {
		t.Fatal("ExtraHeaders should not be nil")
	}
	if v := entry.ExtraHeaders["X-Gateway"]; v != "secret" {
		t.Errorf("ExtraHeaders[\"X-Gateway\"] = %q, want %q", v, "secret")
	}
}

// --- unset tests ---

func TestParseConfigArgsUnset(t *testing.T) {
	action, err := parseConfigArgs([]string{"unset", "custom_providers.my-gateway"})
	if err != nil {
		t.Fatalf("parseConfigArgs: %v", err)
	}
	if action.subCmd != "unset" {
		t.Errorf("subCmd = %q, want %q", action.subCmd, "unset")
	}
	if action.key != "custom_providers.my-gateway" {
		t.Errorf("key = %q, want %q", action.key, "custom_providers.my-gateway")
	}
}

func TestParseConfigArgsUnsetMissingKey(t *testing.T) {
	_, err := parseConfigArgs([]string{"unset"})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestUnsetCustomProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{
		Provider: "anthropic",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway": {URL: "https://gw.example.com/v1", Protocol: "openai", Model: "llama-3"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetCustomProvider(configPath, "my-gateway"); err != nil {
		t.Fatalf("unsetCustomProvider: %v", err)
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.CustomProviders != nil {
		t.Errorf("CustomProviders should be nil after deleting the only entry, got %v", cfg.CustomProviders)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q (should be untouched)", cfg.Provider, "anthropic")
	}
}

func TestUnsetActiveCustomProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{
		Provider: "my-gateway",
		Model:    "fallback-model",
		CustomProviders: map[string]ProviderEntry{
			"my-gateway":    {URL: "https://gw.example.com/v1", Protocol: "openai", Model: "llama-3"},
			"other-gateway": {URL: "https://other.example.com/v1", Protocol: "openai", Model: "other-model"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetCustomProvider(configPath, "my-gateway"); err != nil {
		t.Fatalf("unsetCustomProvider: %v", err)
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Provider != "" {
		t.Errorf("Provider = %q, want empty after deleting active provider", cfg.Provider)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty after deleting active provider", cfg.Model)
	}
	if _, exists := cfg.CustomProviders["my-gateway"]; exists {
		t.Error("my-gateway should have been deleted")
	}
	if _, exists := cfg.CustomProviders["other-gateway"]; !exists {
		t.Error("other-gateway should still exist")
	}
}

func TestUnsetInvalidKey(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"my-gateway", false},
		{"nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := dir + "/config.json"
			cfg := &Config{
				CustomProviders: map[string]ProviderEntry{
					"my-gateway": {URL: "https://gw.example.com/v1"},
				},
			}
			if err := saveConfig(configPath, cfg); err != nil {
				t.Fatalf("saveConfig: %v", err)
			}
			err := unsetCustomProvider(configPath, tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("unsetCustomProvider(%q): err=%v, wantErr=%v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestMergeModelLists(t *testing.T) {
	tests := []struct {
		name  string
		lists [][]string
		want  []string
	}{
		{"empty", nil, nil},
		{"single list", [][]string{{"a", "b"}}, []string{"a", "b"}},
		{"merge with dedup", [][]string{{"a", "b"}, {"b", "c"}}, []string{"a", "b", "c"}},
		{"three lists", [][]string{{"x"}, {"y"}, {"x", "z"}}, []string{"x", "y", "z"}},
		{"empty strings filtered", [][]string{{"a", "", "b"}}, []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeModelLists(tc.lists...)
			if len(got) != len(tc.want) {
				t.Fatalf("mergeModelLists() = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSetMCPServerValue_Command(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.command", "npx"); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	if cfg.MCPServers["my-server"].Command != "npx" {
		t.Errorf("Command = %q, want %q", cfg.MCPServers["my-server"].Command, "npx")
	}
}

func TestSetMCPServerValue_CommandEmpty(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.command", ""); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestSetMCPServerValue_Args(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.args", `["--port","8080"]`); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	args := cfg.MCPServers["my-server"].Args
	if len(args) != 2 || args[0] != "--port" || args[1] != "8080" {
		t.Errorf("Args = %v", args)
	}
}

func TestSetMCPServerValue_ArgsInvalidJSON(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.args", "not-json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSetMCPServerValue_Env(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.env", `["FOO=bar","BAZ=qux"]`); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	env := cfg.MCPServers["my-server"].Env
	if len(env) != 2 || env[0] != "FOO=bar" {
		t.Errorf("Env = %v", env)
	}
}

func TestSetMCPServerValue_EnvInvalidJSON(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.env", "not-json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSetMCPServerValue_EnvInvalidFormat(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.env", `["NOEQUALS"]`); err == nil {
		t.Fatal("expected error for env entry without KEY=VALUE format")
	}
}

func TestSetMCPServerValue_Tools(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.tools", `["search","read","search"]`); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	tools := cfg.MCPServers["my-server"].Tools
	if len(tools) != 2 || tools[0] != "search" || tools[1] != "read" {
		t.Errorf("Tools = %v (expected deduped)", tools)
	}
}

func TestSetMCPServerValue_ToolsInvalidJSON(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.tools", "not-json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSetMCPServerValue_ToolsEmptyName(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.tools", `["search",""]`); err == nil {
		t.Fatal("expected error for empty tool name")
	}
}

func TestSetMCPServerValue_Setup(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.setup", "init-script.sh"); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	if cfg.MCPServers["my-server"].Setup != "init-script.sh" {
		t.Errorf("Setup = %q", cfg.MCPServers["my-server"].Setup)
	}
}

func TestSetMCPServerValue_SetupTimeout(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.setup_timeout", "30"); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	if cfg.MCPServers["my-server"].SetupTimeout != 30 {
		t.Errorf("SetupTimeout = %d, want %d", cfg.MCPServers["my-server"].SetupTimeout, 30)
	}
}

func TestSetMCPServerValue_SetupTimeoutInvalid(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.setup_timeout", "not-a-number"); err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestSetMCPServerValue_SetupTimeoutNonPositive(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.setup_timeout", "0"); err == nil {
		t.Fatal("expected error for non-positive timeout")
	}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.setup_timeout", "-5"); err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestSetMCPServerValue_UnknownField(t *testing.T) {
	cfg := &Config{}
	if err := setMCPServerValue(cfg, "mcp_servers.my-server.unknown", "val"); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestSetMCPServerValue_InvalidKey(t *testing.T) {
	cfg := &Config{}
	tests := []string{
		"mcp_servers",
		"mcp_servers.",
		"mcp_servers..command",
		"mcp_servers.name",
	}
	for _, key := range tests {
		if err := setMCPServerValue(cfg, key, "val"); err == nil {
			t.Errorf("expected error for key %q", key)
		}
	}
}

func TestSetMCPServerValue_ExistingServer(t *testing.T) {
	cfg := &Config{
		MCPServers: map[string]MCPServerConfig{
			"srv": {Command: "old-cmd"},
		},
	}
	if err := setMCPServerValue(cfg, "mcp_servers.srv.command", "new-cmd"); err != nil {
		t.Fatalf("setMCPServerValue: %v", err)
	}
	if cfg.MCPServers["srv"].Command != "new-cmd" {
		t.Errorf("Command = %q, want %q", cfg.MCPServers["srv"].Command, "new-cmd")
	}
}

func TestUnsetMCPServer(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{
		MCPServers: map[string]MCPServerConfig{
			"srv1": {Command: "cmd1"},
			"srv2": {Command: "cmd2"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetMCPServer(configPath, "srv1"); err != nil {
		t.Fatalf("unsetMCPServer: %v", err)
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, exists := cfg.MCPServers["srv1"]; exists {
		t.Error("srv1 should have been deleted")
	}
	if _, exists := cfg.MCPServers["srv2"]; !exists {
		t.Error("srv2 should still exist")
	}
}

func TestUnsetMCPServer_LastEntry(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{
		MCPServers: map[string]MCPServerConfig{
			"only": {Command: "cmd"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetMCPServer(configPath, "only"); err != nil {
		t.Fatalf("unsetMCPServer: %v", err)
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.MCPServers != nil {
		t.Errorf("MCPServers should be nil after deleting last entry, got %v", cfg.MCPServers)
	}
}

func TestUnsetMCPServer_NotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/config.json"

	cfg := &Config{}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetMCPServer(configPath, "nonexistent"); err == nil {
		t.Fatal("expected error for nil MCPServers")
	}

	cfg = &Config{
		MCPServers: map[string]MCPServerConfig{
			"other": {Command: "cmd"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	if err := unsetMCPServer(configPath, "nonexistent"); err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestRunConfigUnset_UnknownPrefix(t *testing.T) {
	if err := runConfigUnset("providers.anthropic"); err == nil {
		t.Fatal("expected error for unsupported prefix")
	}
}

func TestSetConfigValueMCPServer(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "mcp_servers.my-server.command", "npx"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.MCPServers["my-server"].Command != "npx" {
		t.Errorf("Command = %q", cfg.MCPServers["my-server"].Command)
	}
}

func TestEnsureTelemetry(t *testing.T) {
	cfg := &Config{}
	if cfg.Telemetry != nil {
		t.Fatal("Telemetry should be nil initially")
	}
	cfg.ensureTelemetry()
	if cfg.Telemetry == nil {
		t.Fatal("Telemetry should be non-nil after ensureTelemetry()")
	}
	cfg.ensureTelemetry()
	if cfg.Telemetry == nil {
		t.Fatal("Telemetry should remain non-nil on second call")
	}
}

func TestSetConfigValueLlmURL(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.url", "https://example.com/v1"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Llm.URL != "https://example.com/v1" {
		t.Errorf("URL = %q", cfg.Llm.URL)
	}
}

func TestSetConfigValueLlmAuthToken(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.auth_token", "tok-123"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Llm.AuthToken != "tok-123" {
		t.Errorf("AuthToken = %q", cfg.Llm.AuthToken)
	}
}

func TestSetConfigValueLlmModel(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.model", "my-model"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Llm.Model != "my-model" {
		t.Errorf("Model = %q", cfg.Llm.Model)
	}
}

func TestSetConfigValueLlmUseAnthropic(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.use_anthropic", "false"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Llm.UseAnthropic == nil || *cfg.Llm.UseAnthropic != false {
		t.Errorf("UseAnthropic = %v", cfg.Llm.UseAnthropic)
	}
}

func TestSetConfigValueLlmUseAnthropicInvalid(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.use_anthropic", "notbool"); err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestSetConfigValueLanguage(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "language", "English"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Language != "English" {
		t.Errorf("Language = %q", cfg.Language)
	}
}

func TestSetConfigValueTelemetryEnabled(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "telemetry.enabled", "true"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Telemetry == nil || !cfg.Telemetry.Enabled {
		t.Error("Telemetry.Enabled should be true")
	}
}

func TestSetConfigValueTelemetryEnabledInvalid(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "telemetry.enabled", "notbool"); err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestSetConfigValueTelemetryExporter(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "telemetry.exporter", "otlp"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Telemetry.Exporter != "otlp" {
		t.Errorf("Exporter = %q", cfg.Telemetry.Exporter)
	}
}

func TestSetConfigValueTelemetryOTLPEndpoint(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "telemetry.otlp_endpoint", "localhost:4317"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Telemetry.OTLPEndpoint != "localhost:4317" {
		t.Errorf("OTLPEndpoint = %q", cfg.Telemetry.OTLPEndpoint)
	}
}

func TestSetConfigValueTelemetryContentLogging(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "telemetry.content_logging", "true"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if !cfg.Telemetry.ContentLog {
		t.Error("ContentLog should be true")
	}
}

func TestSetConfigValueTelemetryContentLoggingInvalid(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "telemetry.content_logging", "notbool"); err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestSetConfigValueLlmExtraBody(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.extra_body", `{"key":"val"}`); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Llm.ExtraBody == nil {
		t.Fatal("ExtraBody should not be nil")
	}
	if cfg.Llm.ExtraBody["key"] != "val" {
		t.Errorf("ExtraBody[\"key\"] = %v", cfg.Llm.ExtraBody["key"])
	}
}

func TestSetConfigValueLlmExtraBodyInvalid(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "llm.extra_body", "not-json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSetConfigValueUnknownKey(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "unknown.key", "val"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestSetConfigValueProviderClearsModel(t *testing.T) {
	cfg := &Config{Provider: "old-provider", Model: "old-model"}
	if err := setConfigValue(cfg, "provider", "new-provider"); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.Model != "" {
		t.Errorf("Model should be cleared on provider change, got %q", cfg.Model)
	}
}

func TestRunConfigUnset_InvalidKey(t *testing.T) {
	if err := runConfigUnset("provider"); err == nil {
		t.Fatal("expected error for non custom_providers key")
	}
	if err := runConfigUnset("custom_providers."); err == nil {
		t.Fatal("expected error for empty provider name")
	}
}

func TestRunConfig_EmptyArgs(t *testing.T) {
	err := runConfig(nil)
	if err != nil {
		t.Fatalf("runConfig with nil args should print usage, got error: %v", err)
	}
}

func TestRunConfig_ProviderWithArgs(t *testing.T) {
	err := runConfig([]string{"provider", "extra"})
	if err == nil {
		t.Fatal("expected error when provider has args")
	}
}

func TestRunConfig_ModelWithArgs(t *testing.T) {
	err := runConfig([]string{"model", "extra"})
	if err == nil {
		t.Fatal("expected error when model has args")
	}
}

func TestDeleteCustomProvider_NotFound(t *testing.T) {
	cfg := &Config{}
	_, err := deleteCustomProvider(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nil CustomProviders")
	}

	cfg.CustomProviders = map[string]ProviderEntry{"other": {}}
	_, err = deleteCustomProvider(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestActiveModelForProvider(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Config
		provider string
		entry    ProviderEntry
		want     string
	}{
		{"entry model", nil, "p", ProviderEntry{Model: "m1"}, "m1"},
		{"cfg model", &Config{Provider: "p", Model: "m2"}, "p", ProviderEntry{}, "m2"},
		{"entry takes precedence", &Config{Provider: "p", Model: "m2"}, "p", ProviderEntry{Model: "m1"}, "m1"},
		{"different provider", &Config{Provider: "other", Model: "m2"}, "p", ProviderEntry{}, ""},
		{"no model", &Config{Provider: "p"}, "p", ProviderEntry{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := activeModelForProvider(tc.cfg, tc.provider, tc.entry)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeModelList(t *testing.T) {
	tests := []struct {
		name   string
		models []string
		want   []string
	}{
		{"dedup", []string{"a", "b", "a"}, []string{"a", "b"}},
		{"trim spaces", []string{" a ", " b "}, []string{"a", "b"}},
		{"filter empty", []string{"a", "", "b"}, []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeModelList(tc.models)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseModelListValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{"empty", "", 0},
		{"json array", `["a","b"]`, 2},
		{"comma separated", "a,b,c", 3},
		{"bracket unquoted", "[a,b]", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseModelListValue(tc.value)
			if err != nil {
				t.Fatalf("parseModelListValue: %v", err)
			}
			if len(got) != tc.want {
				t.Errorf("got %d models, want %d: %v", len(got), tc.want, got)
			}
		})
	}
}

func TestResolveConfigPath_Default(t *testing.T) {
	t.Setenv("OCR_CONFIG_PATH", "")
	p, err := resolveConfigPath()
	if err != nil {
		t.Fatalf("resolveConfigPath: %v", err)
	}
	if p == "" {
		t.Fatal("expected non-empty default config path")
	}
}

func TestResolveConfigPath_Env(t *testing.T) {
	t.Setenv("OCR_CONFIG_PATH", "/tmp/test-config.json")
	p, err := resolveConfigPath()
	if err != nil {
		t.Fatalf("resolveConfigPath: %v", err)
	}
	if p != "/tmp/test-config.json" {
		t.Errorf("path = %q, want /tmp/test-config.json", p)
	}
}

func TestLoadOrCreateConfig_NewFile(t *testing.T) {
	cfg, err := loadOrCreateConfig(t.TempDir() + "/nonexistent.json")
	if err != nil {
		t.Fatalf("loadOrCreateConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoadOrCreateConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.json"
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadOrCreateConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadAppConfig_NotExist(t *testing.T) {
	cfg, err := LoadAppConfig(t.TempDir() + "/none.json")
	if err != nil {
		t.Fatalf("LoadAppConfig: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config for non-existent file")
	}
}

func TestLoadAppConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.json"
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadAppConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEnsureModelInList(t *testing.T) {
	models := []string{"test-model", "test-model-2", "bbb", "aaa", "test-model-3"}

	got := ensureModelInList(models, "test-model-3")
	if len(got) != len(models) {
		t.Fatalf("existing model should not reorder: got %v", got)
	}
	for i := range models {
		if got[i] != models[i] {
			t.Errorf("models[%d] = %q, want %q", i, got[i], models[i])
		}
	}

	got = ensureModelInList(models, "new-model")
	want := append(append([]string(nil), models...), "new-model")
	if len(got) != len(want) || got[len(got)-1] != "new-model" {
		t.Errorf("new model should append: got %v, want %v", got, want)
	}
}
