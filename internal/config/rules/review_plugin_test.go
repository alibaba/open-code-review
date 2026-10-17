// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/plugin"
)

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
