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
)

func TestPluginProjectCommandLifecycle(t *testing.T) {
	freshOCRHome(t)
	repo := t.TempDir()
	source := filepath.Join(repo, "plugins", "example")
	writePluginCommandFixture(t, source, "1")
	output := pluginCommandOK(t, "add", source, "--repo", repo, "--json")
	if strings.Contains(output, "secret-token") || strings.Contains(output, "secret-argument") || !strings.Contains(output, "./plugins/example") {
		t.Fatalf("project lock output = %s", output)
	}
	if out := pluginCommandOK(t, "list", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("project add changed global state: %s", out)
	}
	for _, args := range [][]string{
		{"sync", "--repo", repo},
		{"preflight", "--repo", repo},
	} {
		out, err := runPluginCommand(context.Background(), append(args, "--json"))
		if err == nil || !json.Valid([]byte(out)) || !strings.Contains(out, `"error"`) {
			t.Fatalf("structured project failure = %s, %v", out, err)
		}
	}
	pluginCommandOK(t, "sync", "--frozen", "--offline", "--repo", repo)
	out, err := runPluginCommand(context.Background(), []string{"preflight", "--repo", repo, "--json"})
	if err == nil || !strings.Contains(out, "PLUGIN_EXEC_NOT_AUTHORIZED") {
		t.Fatalf("untrusted project preflight = %s, %v", out, err)
	}
	pluginCommandOK(t, "trust", "--repo", repo, "--json")
	var fingerprint string
	if err := json.Unmarshal([]byte(pluginCommandOK(t, "preflight", "--fingerprint", "--json", "--repo", repo)), &fingerprint); err != nil || len(fingerprint) != 64 {
		t.Fatalf("preflight fingerprint = %q, %v", fingerprint, err)
	}
	if got := strings.TrimSpace(pluginCommandOK(t, "preflight", "--fingerprint", "--repo", repo)); got != fingerprint {
		t.Fatalf("text fingerprint differs: %q", got)
	}
	if out := pluginCommandOK(t, "preflight", "--repo", repo); !strings.Contains(out, "test-review 1") {
		t.Fatalf("trusted preflight = %s", out)
	}
	pluginCommandOK(t, "project", "--repo", repo)
	pluginCommandOK(t, "trust", "--revoke", "--repo", repo)
	if _, err := runPluginCommand(context.Background(), []string{"preflight", "--repo", repo}); err == nil {
		t.Fatal("revoked trust was ignored")
	}
	writePluginCommandFixture(t, source, "2")
	pluginCommandOK(t, "lock", "--repo", repo)
	pluginCommandOK(t, "sync", "--frozen", "--offline", "--repo", repo, "--json")
	if data, err := os.ReadFile(filepath.Join(repo, ".opencodereview", "plugins.lock.json")); err != nil || !strings.Contains(string(data), `"version": "2"`) {
		t.Fatalf("relock did not update version: %v", err)
	}
	pluginCommandOK(t, "trust", "--repo", repo)
	pluginCommandOK(t, "preflight", "--repo", repo, "--json")
	if got := strings.TrimSpace(pluginCommandOK(t, "preflight", "--fingerprint", "--repo", repo)); got == fingerprint {
		t.Fatal("relocked plugin retained fingerprint")
	}
}

func TestPluginProjectRepoFlagUsesCurrentDirectory(t *testing.T) {
	freshOCRHome(t)
	repo := t.TempDir()
	t.Chdir(repo)
	if err := os.MkdirAll(filepath.Join(repo, ".opencodereview"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".opencodereview", "plugins.json"), []byte(`{"schema_version":1,"plugins":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	implicit := pluginCommandOK(t, "lock", "--json")
	explicit := pluginCommandOK(t, "project", "--repo", repo, "--json")
	if implicit != explicit {
		t.Fatalf("default repo differs from explicit repo: %s / %s", implicit, explicit)
	}
	for _, args := range [][]string{{"project"}, {"project", "--repo", "."}} {
		if got := pluginCommandOK(t, append(args, "--json")...); got != explicit {
			t.Fatalf("relative repo changed project selection: %s", got)
		}
	}
	out, err := runPluginCommand(context.Background(), []string{"project", "--repo", filepath.Join(repo, "missing"), "--json"})
	if err == nil || !json.Valid([]byte(out)) || !strings.Contains(out, "PROJECT_PLUGIN_ERROR") {
		t.Fatalf("repo resolution lost structured error: %s / %v", out, err)
	}
}
