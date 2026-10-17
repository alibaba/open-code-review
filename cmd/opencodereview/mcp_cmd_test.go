// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/plugin"
)

func runMCPCommand(ctx context.Context, args ...string) (string, error) {
	var out bytes.Buffer
	command := newMCPCmd()
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs(args)
	err := command.ExecuteContext(ctx)
	return out.String(), err
}

func readMCPList(t *testing.T, repo string) mcpListResult {
	t.Helper()
	out, err := runMCPCommand(context.Background(), "list", "--repo", repo, "--json")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	var result mcpListResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.ConnectionStatus != "not_checked" {
		t.Fatalf("listing claims a connection check: %s", out)
	}
	return result
}

func saveMCPTestConfig(t *testing.T, cfg *Config) string {
	t.Helper()
	path, err := defaultConfigPath()
	if err == nil {
		err = saveConfig(path, cfg)
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func writeMCPListPackage(t *testing.T, directory string, servers map[string]plugin.MCPServer) {
	t.Helper()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(plugin.Manifest{SchemaVersion: 1, ID: "inventory", Name: "Inventory", Version: "1", MCPServers: servers})
	if err := os.WriteFile(filepath.Join(directory, plugin.ManifestFilename), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMCPListEmptyDoesNotCreateState(t *testing.T) {
	home := freshOCRHome(t)
	repo := t.TempDir()
	t.Chdir(repo)
	t.Setenv("PATH", "")
	for _, args := range [][]string{{"list"}, {"list", "--json"}, {"--help"}, {}} {
		if out, err := runMCPCommand(context.Background(), args...); err != nil || out == "" {
			t.Fatalf("mcp %v: %s / %v", args, out, err)
		}
	}
	result := readMCPList(t, repo)
	if result.ProjectScoped || result.Servers == nil || len(result.Servers) != 0 {
		t.Fatalf("empty inventory = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
		t.Fatalf("read created user state: %v", err)
	}
}

func TestMCPListCombinesSourcesWithoutExecutionOrSecrets(t *testing.T) {
	store := pluginRuntimeHome(t)
	marker := filepath.Join(t.TempDir(), "started")
	installed := installRuntimePlugin(t, store, marker, true)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(500)
	}))
	defer server.Close()
	local := installed.Manifest.MCPServers["tools"]
	configPath := saveMCPTestConfig(t, &Config{Provider: "unconfigured-provider", Llm: LlmConfig{AuthTokenCmd: "secret-auth-command"}, MCPServers: map[string]MCPServerConfig{
		"manual": {Command: local.Command, Args: local.Args, Env: local.Env, Setup: "secret-setup-command"},
		"remote": {Type: "remote", URL: server.URL + "?token=secret-query", Headers: map[string]string{"Authorization": "secret-header"}},
	}})
	source := t.TempDir()
	writeMCPListPackage(t, source, map[string]plugin.MCPServer{
		"local":  {Command: local.Command, Args: []string{"secret-argument"}, Env: []string{"TOKEN=secret-token"}},
		"remote": {Type: "remote", URL: server.URL + "?token=secret-query", Headers: map[string]string{"Authorization": "secret-header"}},
	})
	if _, err := store.Install(context.Background(), source, false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(configPath)
	repo := t.TempDir()
	result := readMCPList(t, repo)
	if result.ProjectScoped || len(result.Servers) != 5 {
		t.Fatalf("combined inventory = %+v", result)
	}
	for _, entry := range result.Servers {
		switch entry.Name {
		case "manual":
			if entry.Source != "user_config" || entry.Execution != "local_setup_and_process" || entry.Authorization != "user_configured" {
				t.Fatalf("manual metadata = %+v", entry)
			}
		case "remote":
			if entry.Transport != "remote" || entry.Execution != "remote_connection" {
				t.Fatalf("remote metadata = %+v", entry)
			}
		case "plugin/runtime/tools":
			if entry.State != "configured" || entry.Authorization != "enabled_by_user" || entry.Version != "1.0" {
				t.Fatalf("enabled metadata = %+v", entry)
			}
		default:
			if entry.Plugin != "inventory" || entry.State != "disabled" || entry.Authorization != "not_enabled" {
				t.Fatalf("disabled metadata = %+v", entry)
			}
		}
	}
	for _, args := range [][]string{{"list", "--repo", repo}, {"list", "--repo", repo, "--json"}} {
		out, err := runMCPCommand(context.Background(), args...)
		if err != nil || strings.Contains(out, "secret-") || strings.Contains(out, local.Command) || strings.Contains(out, server.URL) {
			t.Fatalf("listing leaked execution details: %s / %v", out, err)
		}
	}
	for _, source := range []string{installed.Source, source} {
		for _, suffix := range [][]string{{}, {"--json"}} {
			out := pluginCommandOK(t, append([]string{"install", source, "--dry-run"}, suffix...)...)
			if strings.Contains(out, "secret-") || strings.Contains(out, local.Command) {
				t.Fatalf("preview leaked execution details: %s", out)
			}
			if !strings.Contains(out, "local") {
				t.Fatalf("preview omitted execution summary: %s", out)
			}
		}
	}
	if out := pluginCommandOK(t, "enable", "inventory"); !strings.Contains(out, "local process") || !strings.Contains(out, "remote service") {
		t.Fatalf("activation omitted execution effects: %s", out)
	}
	if out := pluginCommandOK(t, "enable", "inventory", "--json"); !strings.Contains(out, `"mcp_execution"`) || strings.Contains(out, "secret-") {
		t.Fatalf("activation summary = %s", out)
	}
	if out := pluginCommandOK(t, "show", "inventory"); !strings.Contains(out, "user permissions") || !strings.Contains(out, "remote service") {
		t.Fatalf("show omitted execution effects: %s", out)
	}
	after, _ := os.ReadFile(configPath)
	if !bytes.Equal(before, after) || requests.Load() != 0 {
		t.Fatal("inspection changed user config or contacted a server")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("inspection started a local MCP process: %v", err)
	}
}

func TestPluginEnableSummaryOnlyInspectsTarget(t *testing.T) {
	store := pluginRuntimeHome(t)
	installed := installRuntimePlugin(t, store, filepath.Join(t.TempDir(), "started"), false)
	source := t.TempDir()
	writeMCPListPackage(t, source, map[string]plugin.MCPServer{"local": {Command: "not-started"}})
	broken, err := store.Install(context.Background(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(broken.Dir, plugin.ManifestFilename)); err != nil {
		t.Fatal(err)
	}
	if out := pluginCommandOK(t, "enable", installed.Manifest.ID); !strings.Contains(out, "local process") {
		t.Fatalf("target summary missing: %s", out)
	}
	enabled, err := store.Enabled()
	if err != nil || len(enabled) != 1 || enabled[0].Manifest.ID != installed.Manifest.ID {
		t.Fatalf("enabling the target was affected by an unrelated broken snapshot: %+v / %v", enabled, err)
	}
}

func TestMCPListProjectAuthorizationAndFrozenSelection(t *testing.T) {
	store := pluginRuntimeHome(t)
	installRuntimePlugin(t, store, filepath.Join(t.TempDir(), "started"), true)
	saveMCPTestConfig(t, &Config{MCPServers: map[string]MCPServerConfig{"manual": {Command: "not-started"}}})
	repo := t.TempDir()
	source := filepath.Join(repo, "package")
	writeMCPListPackage(t, source, map[string]plugin.MCPServer{"local": {Command: "not-started"}})
	pluginCommandOK(t, "add", source, "--repo", repo)
	if _, err := runMCPCommand(context.Background(), "list", "--repo", repo); err == nil || !strings.Contains(err.Error(), "ARTIFACT_MISSING") {
		t.Fatalf("missing snapshot silently fell back to global plugins: %v", err)
	}
	pluginCommandOK(t, "sync", "--frozen", "--offline", "--repo", repo)
	for _, trusted := range []bool{false, true, false} {
		if trusted {
			pluginCommandOK(t, "trust", "--repo", repo)
		} else {
			pluginCommandOK(t, "trust", "--revoke", "--repo", repo)
		}
		result := readMCPList(t, repo)
		if !result.ProjectScoped || len(result.Servers) != 2 || result.Servers[1].Plugin != "inventory" || result.Servers[1].Source != "project_plugin" {
			t.Fatalf("project inventory = %+v", result)
		}
		entry := result.Servers[1]
		if trusted && (entry.State != "configured" || entry.Authorization != "trusted_project_lock") {
			t.Fatalf("trusted inventory = %+v", entry)
		}
		if !trusted {
			if entry.State != "authorization_required" || entry.Authorization != "required" {
				t.Fatalf("untrusted inventory = %+v", entry)
			}
			if _, err := runPluginCommand(context.Background(), []string{"preflight", "--repo", repo}); err == nil {
				t.Fatal("listing authorized the project")
			}
		}
	}
	if out, err := runMCPCommand(context.Background(), "list", "--repo", repo); err != nil || !strings.Contains(out, "global plugin entries are excluded") {
		t.Fatalf("project selection not explained: %s / %v", out, err)
	}
	empty := t.TempDir()
	if err := os.Mkdir(filepath.Join(empty, ".opencodereview"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, ".opencodereview/plugins.json"), []byte(`{"schema_version":1,"plugins":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	pluginCommandOK(t, "lock", "--repo", empty)
	if result := readMCPList(t, empty); !result.ProjectScoped || len(result.Servers) != 1 || result.Servers[0].Source != "user_config" {
		t.Fatalf("empty project did not exclude global plugins: %+v", result)
	}
}

func TestMCPListKeepsIncompatibleGlobalPluginsInspectable(t *testing.T) {
	store := pluginRuntimeHome(t)
	previous := plugin.HostVersion
	t.Cleanup(func() { plugin.HostVersion = previous })
	plugin.HostVersion = "2.0.0"
	saveMCPTestConfig(t, &Config{MCPServers: map[string]MCPServerConfig{"manual": {Command: "not-started"}}})
	source := t.TempDir()
	writeMCPListPackage(t, source, map[string]plugin.MCPServer{"local": {Command: "not-started"}})
	manifestPath := filepath.Join(source, plugin.ManifestFilename)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Requires = &plugin.Requirements{MinOCRVersion: "2.0.0"}
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), source, true); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	for _, version := range []string{"1.0.0", "dev"} {
		plugin.HostVersion = version
		result := readMCPList(t, repo)
		if len(result.Servers) != 2 || result.Servers[0].State != "configured" || result.Servers[1].State != "incompatible" {
			t.Fatalf("incompatible plugin hid the inventory: %+v", result)
		}
	}
	if err := store.SetEnabled("inventory", false); err != nil {
		t.Fatal(err)
	}
	if result := readMCPList(t, repo); result.Servers[1].State != "disabled" {
		t.Fatalf("disabled state lost: %+v", result)
	}
}

func TestMCPListReportsConfigurationProblems(t *testing.T) {
	store := pluginRuntimeHome(t)
	installRuntimePlugin(t, store, filepath.Join(t.TempDir(), "started"), true)
	configPath := saveMCPTestConfig(t, &Config{MCPServers: map[string]MCPServerConfig{
		"plugin/runtime/tools": {Command: "manual-collision"},
		"no-url":               {Type: "remote"}, "no-command": {},
	}})
	repo := t.TempDir()
	result := readMCPList(t, repo)
	states := map[string]int{}
	for _, entry := range result.Servers {
		states[entry.State]++
	}
	if states["server_name_conflict"] != 2 || states["missing_url"] != 1 || states["missing_command"] != 1 {
		t.Fatalf("config diagnostics = %+v", result)
	}
	if err := store.SetEnabled("runtime", false); err != nil {
		t.Fatal(err)
	}
	for _, entry := range readMCPList(t, repo).Servers {
		if entry.State == "server_name_conflict" {
			t.Fatal("disabled plugin blocks a configured server")
		}
	}
	for _, args := range [][]string{{"list", "unexpected"}, {"unknown"}, {"list", "--repo", filepath.Join(repo, "missing")}} {
		if _, err := runMCPCommand(context.Background(), args...); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runMCPCommand(ctx, "list", "--repo", repo); err != context.Canceled {
		t.Fatalf("cancel = %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runMCPCommand(context.Background(), "list", "--repo", repo, "--json"); err == nil {
		t.Fatal("malformed config silently ignored")
	}
}
