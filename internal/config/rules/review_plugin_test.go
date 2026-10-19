// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/plugin"
)

func TestReviewPluginSelectionLoadsOnlyEnabledAndSelectedSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name         string
		brokenID     string
		otherEnabled bool
		wantError    bool
	}{
		{name: "valid disabled packages"},
		{name: "broken disabled package", brokenID: "other"},
		{name: "broken enabled package", brokenID: "other", otherEnabled: true, wantError: true},
		{name: "broken selected package", brokenID: "workflow", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := pluginRuleTestHome(t)
			source := t.TempDir()
			writePluginRuleFile(t, source, plugin.ManifestFilename, `{"schema_version":1,"id":"workflow","name":"Workflow","version":"1","review":{"protocol":"stdio-v1","command":"example-reviewer","stages":["inspect"]}}`)
			selected, err := store.Install(context.Background(), source, false)
			if err != nil {
				t.Fatal(err)
			}
			other, _ := installRulePlugin(t, store, "other", []string{"rules.json"}, map[string]string{"rules.json": `{"rules":[]}`}, tc.otherEnabled)
			if tc.brokenID != "" {
				broken := other
				if tc.brokenID == "workflow" {
					broken = selected
				}
				if err := os.Remove(filepath.Join(broken.Dir, plugin.ManifestFilename)); err != nil {
					t.Fatal(err)
				}
			}
			resolver, _, err := NewResolver(t.TempDir(), "", ResolverOptions{ReviewPlugin: "workflow"})
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), tc.brokenID) {
					t.Fatalf("damaged active package error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			loaded := resolver.(*pluginResolver)
			if loaded.ReviewPlugin() == nil || loaded.ReviewPlugin().Manifest.ID != "workflow" || len(loaded.EnabledPlugins()) != 1 {
				t.Fatalf("selected workflow snapshot = %+v", loaded.EnabledPlugins())
			}
		})
	}
}

func TestReviewPluginSelectionReportsMissingProjectLockEntry(t *testing.T) {
	store := pluginRuleTestHome(t)
	source := t.TempDir()
	writePluginRuleFile(t, source, plugin.ManifestFilename, `{"schema_version":1,"id":"workflow","name":"Workflow","version":"1","review":{"protocol":"stdio-v1","command":"example-reviewer","stages":["inspect"]}}`)
	if _, err := store.Install(context.Background(), source, false); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	writePluginRuleFile(t, repo, ".opencodereview/plugins.json", `{"schema_version":1,"plugins":[]}`)
	project, err := store.Project(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, err = NewResolver(repo, "", ResolverOptions{ReviewPlugin: "workflow"})
	if err == nil || !strings.Contains(err.Error(), `review plugin "workflow"`) || !strings.Contains(err.Error(), "project plugin lock") {
		t.Fatalf("missing project workflow error = %v", err)
	}
}

func TestReviewPluginIsInvocationScoped(t *testing.T) {
	store := pluginRuleTestHome(t)
	source := t.TempDir()
	writePluginRuleFile(t, source, plugin.ManifestFilename, `{"schema_version":1,"id":"example","name":"Behavior","version":"1","review":{"protocol":"stdio-v1","command":"example-reviewer","stages":["inspect"]}}`)
	writePluginRuleFile(t, source, "policy.md", "Selected example policy.")
	if _, err := store.Install(context.Background(), source, false); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	resolve := func(id string) *pluginResolver {
		t.Helper()
		r, _, err := NewResolver(repo, "", ResolverOptions{ReviewPlugin: id})
		if err != nil {
			t.Fatal(err)
		}
		return r.(*pluginResolver)
	}
	ordinary := resolve("")
	selected := resolve("example")
	if ordinary.ReviewPlugin() != nil || selected.ReviewPlugin() == nil {
		t.Fatal("selection lost")
	}
	if ordinary.Resolve("main.go") != selected.Resolve("main.go") {
		t.Fatal("policy replaced normal file rules")
	}
	if reflect.DeepEqual(ordinary.CanonicalConfig(), selected.CanonicalConfig()) {
		t.Fatal("selection missing from identity")
	}
	if len(selected.EnabledPlugins()) != 1 || !selected.EnabledPlugins()[0].Enabled {
		t.Fatal("missing per-run snapshot")
	}
	items, err := store.List()
	if err != nil || len(items) != 1 || items[0].Enabled {
		t.Fatalf("saved state changed: %+v %v", items, err)
	}
	if resolve("").ReviewPlugin() != nil {
		t.Fatal("selection persisted")
	}
	if err := store.SetEnabled("example", true); err != nil {
		t.Fatal(err)
	}
	if resolve("").ReviewPlugin() != nil {
		t.Fatal("global enable activated a review policy")
	}
	writePluginRuleFile(t, source, "policy.md", "Updated policy.")
	if _, err := store.Update(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	updated := resolve("example")
	before, _ := os.ReadFile(filepath.Join(selected.ReviewPlugin().Dir, "policy.md"))
	after, _ := os.ReadFile(filepath.Join(updated.ReviewPlugin().Dir, "policy.md"))
	if string(before) != "Selected example policy." || string(after) != "Updated policy." {
		t.Fatal("snapshot changed during run")
	}
	if reflect.DeepEqual(selected.CanonicalConfig(), updated.CanonicalConfig()) {
		t.Fatal("changed policy reused old identity")
	}
	installRulePlugin(t, store, "rules-only", []string{"rules.json"}, map[string]string{"rules.json": `{"rules":[]}`}, false)
	for _, id := range []string{"missing", "rules-only"} {
		if _, _, err := NewResolver(repo, "", ResolverOptions{ReviewPlugin: id}); err == nil {
			t.Fatalf("invalid review plugin %q accepted", id)
		}
	}
}
