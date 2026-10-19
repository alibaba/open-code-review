// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyMarketInstallationMigratesWithoutFetchingIndex(t *testing.T) {
	ctx := context.Background()
	archive := testZIP(t, "example", "2")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/package.zip" {
			t.Errorf("unexpected index request: %s", r.URL)
			w.WriteHeader(500)
			return
		}
		w.Write(archive)
	}))
	defer server.Close()
	for _, remote := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "https"}[remote], func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.client = server.Client()
			old, err := store.Install(ctx, testPackage(t, "example", "1"), true)
			if err != nil {
				t.Fatal(err)
			}
			source := testPackage(t, "example", "2")
			if remote {
				source = server.URL + "/package.zip?token=private"
			}
			value, err := store.readState()
			if err != nil {
				t.Fatal(err)
			}
			item := value.Installed["example"]
			item.Source, item.PluginID = "example@team", ""
			value.Installed["example"] = item
			stored := map[string]any{
				"schema_version": 1, "installed": value.Installed,
				"markets": map[string]any{"team": map[string]any{
					"name": "team", "schema_version": 1, "source": server.URL + "/index.json",
					"plugins": []map[string]string{{"id": "example", "source": source, "version": "2"}},
				}},
			}
			data, _ := json.Marshal(stored)
			statePath := filepath.Join(store.root, "state.json")
			if err := os.WriteFile(statePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			items, err := store.List()
			if err != nil || len(items) != 1 || items[0].Source != source || items[0].Dir != old.Dir || !items[0].Enabled {
				t.Fatalf("migrated listing = %+v, %v", items, err)
			}
			after, err := os.ReadFile(statePath)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatalf("listing wrote state: %v", err)
			}
			if err := store.SetEnabled("example", false); err != nil {
				t.Fatal(err)
			}
			after, err = os.ReadFile(statePath)
			if err != nil || bytes.Contains(after, []byte(`"markets"`)) || bytes.Contains(after, []byte("example@team")) {
				t.Fatalf("legacy cache persisted: %s, %v", after, err)
			}
			updated, err := store.Update(ctx, "example")
			if err != nil || updated.Manifest.Version != "2" || updated.Source != source || updated.Enabled {
				t.Fatalf("migrated update = %+v, %v", updated, err)
			}
		})
	}
}

func TestLegacyUnavailableSourcesRemainManageable(t *testing.T) {
	for _, cache := range []string{`null`, `{}`, `{"team":{"plugins":[{"id":"other","source":"https://example.com/plugin.zip"}]}}`, `{"team":{"plugins":[{"id":"example","source":"relative/path"}]}}`, `{"team":{"plugins":[{"id":"example","source":"https://user@example.com/plugin.zip"}]}}`, `"obsolete"`} {
		t.Run(cache, func(t *testing.T) {
			store := NewStore(t.TempDir())
			_, err := store.Install(context.Background(), testPackage(t, "example", "1"), false)
			if err != nil {
				t.Fatal(err)
			}
			value, _ := store.readState()
			item := value.Installed["example"]
			item.Source = "example@team"
			value.Installed["example"] = item
			data, _ := json.Marshal(map[string]any{"schema_version": 1, "installed": value.Installed, "markets": json.RawMessage(cache)})
			if err := os.WriteFile(filepath.Join(store.root, "state.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := store.SetEnabled("example", true); err != nil {
				t.Fatal(err)
			}
			_, err = store.Update(context.Background(), "example")
			requireError(t, err, "use a plugin URL or explicit local path")
			if err := store.Remove("example"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDirectSourceValidation(t *testing.T) {
	for _, source := range []string{"", "example@team", "http://example.com/plugin.zip", "ftp://example.com/plugin.zip", "https://", "https://user@example.com/plugin.zip", "https://example.com/plugin.zip#part"} {
		if _, err := canonicalSource(source); err == nil {
			t.Errorf("accepted invalid source %q", source)
		}
	}
	path := filepath.Join(t.TempDir(), "example@team")
	if source, err := canonicalSource(path); err != nil || source != path {
		t.Fatalf("explicit local path = %q, %v", source, err)
	}
	if source, err := canonicalSource("HTTPS://example.com/plugin.zip"); err != nil || !strings.HasPrefix(source, "HTTPS:") {
		t.Fatalf("HTTPS source = %q, %v", source, err)
	}
}
