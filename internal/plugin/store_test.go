// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testManifest(id, version string) Manifest {
	return Manifest{SchemaVersion: 1, ID: id, Name: "Example plugin", Version: version, Rules: []string{"rules.json"}}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func testPackage(t *testing.T, id, version string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestJSON(t, filepath.Join(dir, ManifestFilename), testManifest(id, version))
	writeTestJSON(t, filepath.Join(dir, "rules.json"), map[string]any{"default_rule": "Review carefully"})
	return dir
}

func requireError(t *testing.T, err error, part string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), part) {
		t.Fatalf("expected error containing %q, got %v", part, err)
	}
}

func TestEmptyStoreReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	store := NewStore(root)
	items, err := store.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("List = %v, %v", items, err)
	}
	enabled, err := store.Enabled()
	if err != nil || len(enabled) != 0 {
		t.Fatalf("Enabled = %v, %v", enabled, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("read created store directory: %v", err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	defaultStore, err := DefaultStore()
	if err != nil || defaultStore.root != filepath.Join(home, ".opencodereview", "plugins") {
		t.Fatalf("DefaultStore = %v, %v", defaultStore, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, err := DefaultStore(); err == nil {
		t.Fatal("expected missing home error")
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	source := testPackage(t, "example", "1.0.0")
	item, err := store.Install(ctx, source, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.Enabled || item.Source != source || item.Dir == source || !filepath.IsAbs(item.Dir) {
		t.Fatalf("unexpected install: %+v", item)
	}
	writeTestJSON(t, filepath.Join(source, "rules.json"), map[string]string{"default_rule": "New rule"})
	data, err := os.ReadFile(filepath.Join(item.Dir, "rules.json"))
	if err != nil || strings.Contains(string(data), "New rule") {
		t.Fatalf("installed snapshot changed: %s, %v", data, err)
	}
	_, err = store.Install(ctx, source, true)
	requireError(t, err, "already installed")
	activated, err := store.SetEnabledWithResult("example", true)
	if err != nil || !activated.Enabled || activated.Dir != item.Dir {
		t.Fatalf("activation result = %+v, %v", activated, err)
	}
	enabled, err := store.Enabled()
	if err != nil || len(enabled) != 1 || !enabled[0].Enabled {
		t.Fatalf("Enabled = %v, %v", enabled, err)
	}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "2.0.0"))
	updated, err := store.Update(ctx, "example")
	if err != nil || updated.Manifest.Version != "2.0.0" || !updated.Enabled || updated.Dir == item.Dir {
		t.Fatalf("Update = %+v, %v", updated, err)
	}
	if _, err := os.Stat(item.Dir); err != nil {
		t.Fatalf("update deleted snapshot held by an active review: %v", err)
	}
	deactivated, err := store.SetEnabledWithResult("example", false)
	if err != nil || deactivated.Enabled || deactivated.Dir != updated.Dir {
		t.Fatalf("deactivation result = %+v, %v", deactivated, err)
	}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "3.0.0"))
	updated, err = store.Update(ctx, "example")
	if err != nil || updated.Enabled {
		t.Fatalf("disabled state lost by Update: %+v, %v", updated, err)
	}
	if err := store.Remove("example"); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("List after remove = %v, %v", items, err)
	}
	if _, err := os.Stat(updated.Dir); err != nil {
		t.Fatalf("remove deleted snapshot held by an active review: %v", err)
	}
	requireError(t, store.Remove("example"), "not installed")
	requireError(t, store.SetEnabled("example", true), "not installed")
	_, err = store.Update(ctx, "example")
	requireError(t, err, "not installed")
}

func TestDeterministicEnabledListAndCorruption(t *testing.T) {
	store := NewStore(t.TempDir())
	ctx := context.Background()
	for _, id := range []string{"zebra", "alpha"} {
		if _, err := store.Install(ctx, testPackage(t, id, "1"), true); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.Enabled()
	if err != nil || items[0].Manifest.ID != "alpha" || items[1].Manifest.ID != "zebra" {
		t.Fatalf("Enabled = %v, %v", items, err)
	}
	writeTestJSON(t, filepath.Join(items[0].Dir, ManifestFilename), testManifest("alpha", "other"))
	_, err = store.Enabled()
	requireError(t, err, "manifest changed")
	requireError(t, store.SetEnabled("alpha", true), "manifest changed")
	if err := store.SetEnabled("alpha", false); err != nil {
		t.Fatal(err)
	}
	items, err = store.Enabled()
	if err != nil || len(items) != 1 || items[0].Manifest.ID != "zebra" {
		t.Fatalf("disabled damaged package blocked runtime: %v, %v", items, err)
	}
	if err := os.Remove(filepath.Join(items[0].Dir, "rules.json")); err != nil {
		t.Fatal(err)
	}
	_, err = store.List()
	if err == nil {
		t.Fatal("List accepted damaged packages")
	}
	requireError(t, store.SetEnabled("zebra", true), "existing regular file")
}

func TestFailedUpdatePreservesSnapshotAndState(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	source := testPackage(t, "example", "1")
	installed, err := store.Install(ctx, source, true)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("different", "2"))
	_, err = store.Update(ctx, "example")
	requireError(t, err, "does not match installed id")
	after, err := os.ReadFile(filepath.Join(store.root, "state.json"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("failed update changed state: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(store.root, "packages"))
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(installed.Dir) {
		t.Fatalf("failed update retained staging files: %v, %v", entries, err)
	}
}

func TestMutationsSerializedAcrossStores(t *testing.T) {
	root := t.TempDir()
	first, second := NewStore(root), NewStore(root)
	ctx := context.Background()
	unlock, err := first.lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err = second.Install(short, testPackage(t, "cancelled", "1"), false)
	requireError(t, err, "deadline exceeded")
	unlock()
	var group sync.WaitGroup
	errors := make(chan error, 2)
	for i, store := range []*Store{first, second} {
		id := []string{"alpha", "beta"}[i]
		source := testPackage(t, id, "1")
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.Install(ctx, source, false)
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := first.List()
	if err != nil || len(items) != 2 {
		t.Fatalf("serialized mutations lost a plugin: %v, %v", items, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".lock")); !os.IsNotExist(err) {
		t.Fatalf("lock left after mutation: %v", err)
	}
}

func TestInvalidStateAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	tests := map[string]string{
		"malformed":        "{",
		"schema":           `{"schema_version":2}`,
		"manifest":         `{"schema_version":1,"installed":{"example":{"manifest":{"schema_version":2,"id":"example"},"dir":"packages/example"}}}`,
		"id":               `{"schema_version":1,"installed":{"other":{"manifest":{"id":"example"}}}}`,
		"directory":        `{"schema_version":1,"installed":{"example":{"manifest":{"schema_version":1,"id":"example","name":"Example","version":"1","rules":["rules.json"]},"dir":"../outside"}}}`,
		"nested directory": `{"schema_version":1,"installed":{"example":{"manifest":{"schema_version":1,"id":"example","name":"Example","version":"1","rules":["rules.json"]},"dir":"packages/extra/example"}}}`,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			if err := os.WriteFile(filepath.Join(store.root, "state.json"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.List(); err == nil {
				t.Fatal("invalid state accepted")
			}
			if _, err := store.Install(ctx, testPackage(t, "example", "1"), false); err == nil {
				t.Fatal("install accepted invalid state")
			}
			if _, err := store.Update(ctx, "example"); err == nil {
				t.Fatal("update accepted invalid state")
			}
			if err := store.SetEnabled("example", false); err == nil {
				t.Fatal("enable accepted invalid state")
			}
			if err := store.Remove("example"); err == nil {
				t.Fatal("remove accepted invalid state")
			}
		})
	}
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := NewStore(filepath.Join(file, "child"))
	if _, err := bad.Install(ctx, "anything", false); err == nil {
		t.Fatal("expected lock creation error")
	}
	if _, err := bad.Update(ctx, "anything"); err == nil {
		t.Fatal("expected update lock error")
	}
	if err := bad.SetEnabled("anything", false); err == nil {
		t.Fatal("expected enable lock error")
	}
	if err := bad.Remove("anything"); err == nil {
		t.Fatal("expected remove lock error")
	}
	store := NewStore(t.TempDir())
	if err := os.Mkdir(filepath.Join(store.root, "state.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("directory accepted as state file")
	}
	if err := store.writeState(state{SchemaVersion: 1}); err == nil {
		t.Fatal("expected atomic rename failure")
	}
	entries, err := os.ReadDir(store.root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("write failure leaked temporary state: %v, %v", entries, err)
	}
}

func TestMCPOnlySnapshotDoesNotStartProcesses(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "server", "start.sh"), []byte("#!/bin/sh\ntouch started\n"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, ID: "mcp-only", Name: "MCP plugin", Version: "1", MCPServers: map[string]MCPServer{"tools": {Command: "sh", Args: []string{"${plugin_dir}/server/start.sh"}}}}
	writeTestJSON(t, filepath.Join(source, ManifestFilename), manifest)
	store := NewStore(t.TempDir())
	item, err := store.Install(context.Background(), source, true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(item.Dir, "server", "start.sh"))
	if err != nil || string(data) != "#!/bin/sh\ntouch started\n" {
		t.Fatalf("MCP executable was not copied: %s, %v", data, err)
	}
	for _, dir := range []string{source, item.Dir} {
		if _, err := os.Stat(filepath.Join(dir, "started")); !os.IsNotExist(err) {
			t.Fatalf("installation started MCP server in %s: %v", dir, err)
		}
	}
	items, err := store.Enabled()
	if err != nil || len(items) != 1 || len(items[0].Manifest.Rules) != 0 {
		t.Fatalf("MCP-only enabled snapshot = %v, %v", items, err)
	}
}

func TestOversizedStateWritePreservesRegistry(t *testing.T) {
	store := NewStore(t.TempDir())
	value := state{SchemaVersion: 1, Installed: make(map[string]Installed)}
	if err := store.writeState(value); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	value.Installed["huge"] = Installed{Source: strings.Repeat("x", maxStateBytes)}
	requireError(t, store.writeState(value), "byte limit")
	after, err := os.ReadFile(filepath.Join(store.root, "state.json"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("oversized state write changed registry: %v", err)
	}
}
