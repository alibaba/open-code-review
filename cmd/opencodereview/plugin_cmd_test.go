// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/plugin"
)

func writePluginCommandFixture(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := plugin.Manifest{
		SchemaVersion: 1, ID: "test-review", Name: "Test Review", Version: version,
		Description: "Review error handling", Rules: []string{"rules.json"},
		MCPServers: map[string]plugin.MCPServer{"notes": {
			Command: "nonexistent-plugin-test-command", Args: []string{"secret-argument"}, Env: []string{"TOKEN=secret-token"},
		}},
		Review: &plugin.ReviewContribution{Protocol: "stdio-v1", Command: "private-workflow-command", Args: []string{"workflow-secret"}, Stages: []string{"inspect"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{
		"ocr-plugin.json": data,
		"rules.json":      []byte(`{"rules":[{"path":"**/*.go","rule":"Check returned errors."}]}`),
	} {
		if err := os.WriteFile(filepath.Join(dir, path), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func pluginCommandOK(t *testing.T, args ...string) string {
	t.Helper()
	output, err := runPluginCommand(context.Background(), args)
	if err != nil {
		t.Fatalf("plugin %v: %v\n%s", args, err, output)
	}
	return output
}

func TestPluginCommandLifecycleAndCredentialRedaction(t *testing.T) {
	freshOCRHome(t)
	t.Setenv("PATH", "")
	dir := filepath.Join(t.TempDir(), "package with spaces")
	writePluginCommandFixture(t, dir, "1.0.0")
	preview := pluginCommandOK(t, "install", dir, "--dry-run", "--json")
	if strings.Contains(preview, "workflow-secret") || strings.Contains(preview, "private-workflow-command") || !strings.Contains(preview, "stdio-v1") {
		t.Fatalf("preview omitted protocol or exposed workflow execution: %s", preview)
	}
	if out := pluginCommandOK(t, "list", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("new store = %s", out)
	}
	if out := pluginCommandOK(t, "list"); !strings.Contains(out, "No plugins installed") {
		t.Fatalf("empty guidance missing: %s", out)
	}
	if out := pluginCommandOK(t, "install", dir); !strings.Contains(out, "disabled") || !strings.Contains(out, "ocr plugin enable test-review") {
		t.Fatalf("install did not explain activation: %s", out)
	}
	for _, args := range [][]string{{"show", "test-review"}, {"show", "test-review", "--json"}, {"list", "--json"}} {
		out := pluginCommandOK(t, args...)
		if strings.Contains(out, "secret-token") || strings.Contains(out, "secret-argument") || strings.Contains(out, "workflow-secret") || strings.Contains(out, "private-workflow-command") || !strings.Contains(out, "notes") {
			t.Fatalf("management output leaked credentials or omitted capabilities: %s", out)
		}
	}
	var info pluginInfo
	if err := json.Unmarshal([]byte(pluginCommandOK(t, "show", "test-review", "--json")), &info); err != nil || info.Review == nil || info.Review.Protocol != "stdio-v1" || len(info.Review.Stages) != 1 {
		t.Fatalf("management output omitted workflow capabilities: %+v, %v", info, err)
	}
	pluginCommandOK(t, "enable", "test-review", "--json")
	var infos []pluginInfo
	if err := json.Unmarshal([]byte(pluginCommandOK(t, "list", "--json")), &infos); err != nil || len(infos) != 1 || !infos[0].Enabled {
		t.Fatalf("enabled list = %+v, %v", infos, err)
	}
	if out := pluginCommandOK(t, "list"); !strings.Contains(out, "enabled") || !strings.Contains(out, "1 rule packs, 1 MCP servers") {
		t.Fatalf("list omitted state/capabilities: %s", out)
	}
	writePluginCommandFixture(t, dir, "1.1.0")
	var updated pluginInfo
	if err := json.Unmarshal([]byte(pluginCommandOK(t, "update", "test-review", "--json")), &updated); err != nil || updated.Version != "1.1.0" || !updated.Enabled {
		t.Fatalf("update lost source or enabled state: %+v, %v", updated, err)
	}
	pluginCommandOK(t, "disable", "test-review")
	if out := pluginCommandOK(t, "show", "test-review"); !strings.Contains(out, "disabled") {
		t.Fatalf("disable did not take effect: %s", out)
	}
	pluginCommandOK(t, "remove", "test-review", "--json")
	if _, err := runPluginCommand(context.Background(), []string{"show", "test-review"}); err == nil {
		t.Fatal("removed plugin is still available")
	}
}

func TestPluginCommandErrorsAreReturnedToCLIAndTUI(t *testing.T) {
	freshOCRHome(t)
	for _, args := range [][]string{
		{"install"}, {"install", filepath.Join(t.TempDir(), "missing")},
		{"show", "missing"}, {"update", "missing"}, {"enable", "missing"}, {"disable", "missing"}, {"remove", "missing"},
		{"market", "add", "missing-source"}, {"market", "add", "test", filepath.Join(t.TempDir(), "missing.json")},
		{"market", "refresh", "missing"}, {"market", "remove", "missing"}, {"unrecognized"},
	} {
		t.Run(strings.Join(args[:min(2, len(args))], "_"), func(t *testing.T) {
			if _, err := runPluginCommand(context.Background(), args); err == nil {
				t.Fatalf("plugin %v silently succeeded", args)
			}
		})
	}
	for _, args := range [][]string{{"--help"}} {
		if out := pluginCommandOK(t, args...); !strings.Contains(out, "Available Commands:") {
			t.Fatalf("help omitted commands: %s", out)
		}
	}
}

func TestPluginRetiredReviewManagementRequiresUpdate(t *testing.T) {
	freshOCRHome(t)
	t.Setenv("PATH", "")
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, plugin.ManifestFilename)
	legacy := `{"schema_version":1,"id":"legacy-review","name":"Legacy review","version":"1","review":{"engine":"old","prompt":"old.md"}}`
	if err := os.WriteFile(manifestPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"install", dir, "--dry-run"}, {"install", dir}} {
		out := pluginCommandOK(t, args...)
		if !strings.Contains(out, "update required") || strings.Contains(out, "Run once:") || strings.Contains(out, "Review stages:") {
			t.Fatalf("retired review advertised as runnable: %s", out)
		}
	}
	for _, args := range [][]string{{"show", "legacy-review"}, {"list"}} {
		out := pluginCommandOK(t, args...)
		if !strings.Contains(out, "update required") || strings.Contains(out, "Run once:") || strings.Contains(out, "explicit selection") {
			t.Fatalf("retired review advertised as runnable: %s", out)
		}
	}
	for _, args := range [][]string{{"show", "legacy-review", "--json"}, {"list", "--json"}, {"install", dir, "--dry-run", "--json"}} {
		out := pluginCommandOK(t, args...)
		if !strings.Contains(out, `"update_required": true`) {
			t.Fatalf("JSON omitted retired review status: %s", out)
		}
	}
	writePluginCommandFixture(t, dir, "2")
	pluginCommandOK(t, "install", dir)
	if out := pluginCommandOK(t, "show", "test-review"); !strings.Contains(out, "Run once: ocr review --review-plugin test-review") || strings.Contains(out, "update required") {
		t.Fatalf("current review lost runnable hint: %s", out)
	}
	if out := pluginCommandOK(t, "list"); !strings.Contains(out, "review policy (explicit selection)") {
		t.Fatalf("current review lost capability: %s", out)
	}
	if out := pluginCommandOK(t, "install", dir, "--dry-run"); !strings.Contains(out, "Review stages: inspect") || strings.Contains(out, "update required") {
		t.Fatalf("current review lost stage preview: %s", out)
	}
}

func TestPluginManagementTextStripsTerminalControl(t *testing.T) {
	if got := pluginText("hello\x1b[31m\n\tworld\x1b[0m"); got != "hello world" {
		t.Fatalf("unsafe plugin description: %q", got)
	}
}

func TestPluginManagementOutputRedactsSourceQueries(t *testing.T) {
	info := pluginManagementInfo(plugin.Installed{Source: "https://example.invalid/plugin.zip?token=source-secret"})
	if info.Source != "https://example.invalid/plugin.zip" {
		t.Fatalf("unsafe source: %s", info.Source)
	}
}

func TestPluginUpdateExecutionConfirmation(t *testing.T) {
	freshOCRHome(t)
	t.Setenv("PATH", "")
	source := t.TempDir()
	writePluginCommandFixture(t, source, "1")
	pluginCommandOK(t, "install", source, "--enable")
	manifestPath := filepath.Join(source, plugin.ManifestFilename)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	server := manifest.MCPServers["notes"]
	server.Args = []string{"new-secret-argument"}
	manifest.MCPServers["notes"] = server
	manifest.Version = "2"
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"update", "test-review"}, {"update", "test-review", "--json"}} {
		out, err := runPluginCommand(context.Background(), args)
		if err == nil || !strings.Contains(err.Error(), "--accept-permissions") {
			t.Fatalf("update = %s, %v", out, err)
		}
		if strings.Contains(out+err.Error(), "secret-argument") || strings.Contains(out+err.Error(), "secret-token") {
			t.Fatalf("changed values leaked: %s, %v", out, err)
		}
		if len(args) == 3 {
			var response map[string]any
			if json.Unmarshal([]byte(out), &response) != nil || !strings.Contains(out, "PLUGIN_EXECUTION_CHANGED") {
				t.Fatalf("change response = %s", out)
			}
		}
	}
	var info pluginInfo
	if err := json.Unmarshal([]byte(pluginCommandOK(t, "show", "test-review", "--json")), &info); err != nil || info.Version != "1" || !info.Enabled {
		t.Fatalf("rejected update = %+v, %v", info, err)
	}
	if err := json.Unmarshal([]byte(pluginCommandOK(t, "update", "test-review", "--accept-permissions", "--json")), &info); err != nil || info.Version != "2" || !info.Enabled || info.ContentSHA256 == "" || info.ArchiveSHA256 != "" {
		t.Fatalf("accepted update = %+v, %v", info, err)
	}
}
