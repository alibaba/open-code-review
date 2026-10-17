// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/plugin"
)

func TestProjectPluginsOverrideGlobalSetAndPreserveDeclarationOrder(t *testing.T) {
	store := pluginRuleTestHome(t)
	installRulePlugin(t, store, "global", []string{"rule.json"}, map[string]string{
		"rule.json": `{"rules":[{"path":"**/*.go","rule":"Global contribution"}]}`,
	}, true)
	repo := t.TempDir()
	project, err := store.Project(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"z-project", "a-project"} {
		source := filepath.Join(repo, id)
		m := plugin.Manifest{SchemaVersion: 1, ID: id, Name: id, Version: "1", Rules: []string{"rules.json"}}
		data, _ := json.Marshal(m)
		writePluginRuleFile(t, source, plugin.ManifestFilename, string(data))
		writePluginRuleFile(t, source, "rules.json", `{"rules":[{"path":"**/*.go","rule":"`+id+` contribution"}]}`)
		if _, err := project.Add(context.Background(), source, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := NewResolver(repo, "", ResolverOptions{}); err == nil || !strings.Contains(err.Error(), "ARTIFACT_MISSING") {
		t.Fatalf("missing project snapshots fell back to global: %v", err)
	}
	if _, err := project.Sync(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	resolver, _, err := NewResolver(repo, "", ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	detail := resolver.(DetailResolver).ResolveDetail("main.go")
	if len(detail.PluginRules) != 2 || detail.PluginRules[0].ID != "z-project" || detail.PluginRules[1].ID != "a-project" || strings.Contains(detail.Rule, "Global contribution") {
		t.Fatalf("project contributions = %+v", detail)
	}
	other, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil || !strings.Contains(other.Resolve("main.go"), "Global contribution") {
		t.Fatalf("legacy global behavior changed: %v", err)
	}
	writePluginRuleFile(t, repo, ".opencodereview/plugins.json", `{"schema_version":1,"plugins":[]}`)
	if _, err := project.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	empty, _, err := NewResolver(repo, "", ResolverOptions{})
	if err != nil || len(empty.(DetailResolver).ResolveDetail("main.go").PluginRules) != 0 {
		t.Fatalf("empty declaration loaded global plugins: %v", err)
	}
}

func pluginRuleTestHome(t *testing.T) *plugin.Store {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store, err := plugin.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writePluginRuleFile(t *testing.T, dir, file, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func installRulePlugin(t *testing.T, store *plugin.Store, id string, files []string, content map[string]string, enabled bool) (plugin.Installed, string) {
	t.Helper()
	dir := t.TempDir()
	manifest := plugin.Manifest{SchemaVersion: 1, ID: id, Name: id, Version: "1.0", Rules: files}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writePluginRuleFile(t, dir, plugin.ManifestFilename, string(data))
	for file, text := range content {
		writePluginRuleFile(t, dir, file, text)
	}
	installed, err := store.Install(context.Background(), dir, enabled)
	if err != nil {
		t.Fatal(err)
	}
	return installed, dir
}

func TestPluginRulesAdditiveAndDeterministic(t *testing.T) {
	store := pluginRuleTestHome(t)
	installRulePlugin(t, store, "z-pack", []string{"rule.json"}, map[string]string{
		"rule.json": `{"rules":[{"path":"**/*.go","rule":"Z plugin rule"}]}`,
	}, true)
	installRulePlugin(t, store, "a-pack", []string{"second/rule.json", "first/rule.json"}, map[string]string{
		"second/rule.json": `{"rules":[{"path":"**/*.go","rule":"../shared.md","merge_system_rule":true},{"path":"**/*.go","rule":"Never selected"}]}`,
		"first/rule.json":  `{"rules":[{"path":"**/*.{go,rs}","rule":"A second rule"}]}`,
		"shared.md":        "A first rule\n",
	}, true)
	repo := t.TempDir()
	writePluginRuleFile(t, repo, ".opencodereview/rule.json", `{"rules":[{"path":"**/*.go","rule":"Project rule"}],"exclude":["generated/**"]}`)
	r, filter, err := NewResolver(repo, "", ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	detail := r.(DetailResolver).ResolveDetail("src/main.go")
	if detail.Source != "project" || detail.Pattern != "**/*.go" || !strings.HasPrefix(detail.Rule, "Project rule") {
		t.Fatalf("base rule metadata changed: %+v", detail)
	}
	if len(detail.PluginRules) != 3 || detail.PluginRules[0].ID != "a-pack" || detail.PluginRules[0].File != "second/rule.json" || detail.PluginRules[2].ID != "z-pack" {
		t.Fatalf("plugin order = %+v", detail.PluginRules)
	}
	if strings.Index(detail.Rule, "A first rule") > strings.Index(detail.Rule, "A second rule") || strings.Index(detail.Rule, "A second rule") > strings.Index(detail.Rule, "Z plugin rule") || strings.Contains(detail.Rule, "Never selected") {
		t.Fatalf("unexpected additive rule: %s", detail.Rule)
	}
	if r.Resolve("src/main.go") != detail.Rule || filter == nil || !filter.IsUserExcluded("generated/a.go") {
		t.Fatal("rule and filter contracts changed")
	}
	if got := r.(DetailResolver).ResolveDetail("main.py"); len(got.PluginRules) != 0 || got.Source != "system" {
		t.Fatalf("unmatched file = %+v", got)
	}
}

func TestPluginRulesCanonicalIdentityAndDisable(t *testing.T) {
	store := pluginRuleTestHome(t)
	installed, source := installRulePlugin(t, store, "identity", []string{"rule.json"}, map[string]string{
		"rule.json": `{"rules":[{"path":"**/*.go","rule":"Version one"}]}`,
	}, false)
	repo := t.TempDir()
	load := func() Resolver {
		t.Helper()
		r, _, err := NewResolver(repo, "", ResolverOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	identity := func(r Resolver) string { return strings.Join(r.(canonicalConfigurer).CanonicalConfig(), "\x00") }
	disabled := identity(load())
	if err := store.SetEnabled("identity", true); err != nil {
		t.Fatal(err)
	}
	r := load()
	enabled := identity(r)
	if enabled == disabled || enabled != identity(r) {
		t.Fatal("enabled plugin is missing from deterministic identity")
	}
	writePluginRuleFile(t, installed.Dir, "rule.json", `{"rules":[{"path":"**/*.go","rule":"Changed content"}]}`)
	if identity(load()) == enabled {
		t.Fatal("changed plugin rule content did not change identity")
	}
	manifest := installed.Manifest
	manifest.Version = "2.0"
	manifest.MCPServers = map[string]plugin.MCPServer{"tools": {Command: "ocr-test-command"}}
	data, _ := json.Marshal(manifest)
	writePluginRuleFile(t, source, plugin.ManifestFilename, string(data))
	updated, err := store.UpdateWithOptions(context.Background(), "identity", plugin.UpdateOptions{AcceptPermissions: true})
	if err != nil {
		t.Fatal(err)
	}
	if identity(load()) == enabled || !strings.Contains(identity(load()), "ocr-test-command") {
		t.Fatal("plugin manifest changes did not participate in identity")
	}
	if err := store.SetEnabled("identity", false); err != nil {
		t.Fatal(err)
	}
	writePluginRuleFile(t, updated.Dir, "rule.json", "malformed")
	if identity(load()) != disabled {
		t.Fatal("disabled plugin changed rule identity")
	}
	if !strings.Contains(r.Resolve("src/main.go"), "Version one") {
		t.Fatal("loaded resolver did not preserve its rule snapshot")
	}
	if err := store.SetEnabled("identity", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewResolver(repo, "", ResolverOptions{}); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("corrupt enabled plugin error = %v", err)
	}
}

func TestPluginRulePackRejectsInvalidContributions(t *testing.T) {
	cases := []struct{ name, content, want string }{
		{"json", "{", "decode rules"},
		{"include", `{"include":["**/*.go"]}`, "rules only"},
		{"exclude", `{"exclude":["**/*.go"]}`, "rules only"},
		{"pattern", `{"rules":[{"path":"[","rule":"Invalid"}]}`, "invalid rule pattern"},
		{"missing reference", `{"rules":[{"path":"**","rule":"missing.md"}]}`, "read rule"},
		{"outside reference", `{"rules":[{"path":"**","rule":"../outside.md"}]}`, "read rule"},
		{"absolute reference", `{"rules":[{"path":"**","rule":"/outside.md"}]}`, "must be relative"},
		{"uri reference", `{"rules":[{"path":"**","rule":"file://outside.md"}]}`, "must be relative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := pluginRuleTestHome(t)
			installRulePlugin(t, store, "broken", []string{"rule.json"}, map[string]string{"rule.json": tc.content}, true)
			_, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPluginRuleLoadingErrors(t *testing.T) {
	store := pluginRuleTestHome(t)
	installed, _ := installRulePlugin(t, store, "missing", []string{"rule.json"}, map[string]string{"rule.json": `{}`}, true)
	if err := os.Remove(filepath.Join(installed.Dir, "rule.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewResolver(t.TempDir(), "", ResolverOptions{}); err == nil {
		t.Fatal("missing enabled rule pack was ignored")
	}
	for _, tc := range []struct{ name, dir, file string }{
		{"missing root", filepath.Join(t.TempDir(), "missing"), "rule.json"},
		{"missing pack", t.TempDir(), "rule.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadPluginRulePack(tc.dir, tc.file); err == nil {
				t.Fatal("expected load error")
			}
		})
	}
	dir := t.TempDir()
	writePluginRuleFile(t, dir, "outside.json", `{}`)
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPluginRulePack(child, "../outside.json"); err == nil {
		t.Fatal("outside rule pack was accepted")
	}
	writePluginRuleFile(t, dir, "rule.json", strings.Repeat(" ", 512*1024+1))
	if _, err := loadPluginRulePack(dir, "rule.json"); err == nil {
		t.Fatal("oversized rule pack was accepted")
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, _, err := NewResolver(t.TempDir(), "", ResolverOptions{}); err == nil {
		t.Fatal("missing plugin home was ignored")
	}
}

func TestPluginHostRequirementsBeforeRulesLoading(t *testing.T) {
	previous := plugin.HostVersion
	t.Cleanup(func() { plugin.HostVersion = previous })
	plugin.HostVersion = "1.2.3"
	store := pluginRuleTestHome(t)
	source := t.TempDir()
	manifest := plugin.Manifest{
		SchemaVersion: 1, ID: "compatible", Name: "Compatible", Version: "1",
		Rules: []string{"rules.json"}, Requires: &plugin.Requirements{MinOCRVersion: "1.2.0"},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writePluginRuleFile(t, source, plugin.ManifestFilename, string(data))
	writePluginRuleFile(t, source, "rules.json", `{"default_rule":"Test rule"}`)
	if _, err := store.Install(context.Background(), source, true); err != nil {
		t.Fatal(err)
	}
	plugin.HostVersion = "1.1.0"
	options := ResolverOptions{}
	if _, _, err := NewResolver(t.TempDir(), "", options); err == nil || !strings.Contains(err.Error(), "requires OCR") {
		t.Fatalf("incompatible runtime loading: %v", err)
	}
}
