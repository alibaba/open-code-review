// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestApplyProviderField_MaxInFlight(t *testing.T) {
	t.Run("valid values set the entry", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "max_in_flight", "providers.p.max_in_flight", "3"); err != nil {
			t.Fatalf("set 3: %v", err)
		}
		if e.MaxInFlight != 3 {
			t.Fatalf("MaxInFlight = %d, want 3", e.MaxInFlight)
		}
		if err := applyProviderField("p", &e, "max_in_flight", "providers.p.max_in_flight", "0"); err != nil {
			t.Fatalf("set 0 (disable): %v", err)
		}
		if e.MaxInFlight != 0 {
			t.Fatalf("MaxInFlight = %d, want 0", e.MaxInFlight)
		}
	})

	t.Run("negative and non-integer values are rejected", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "max_in_flight", "providers.p.max_in_flight", "-1"); err == nil {
			t.Fatal("negative accepted")
		}
		if err := applyProviderField("p", &e, "max_in_flight", "providers.p.max_in_flight", "two"); err == nil {
			t.Fatal("non-integer accepted")
		}
		if e.MaxInFlight != 0 {
			t.Fatalf("rejected writes must not apply, got %d", e.MaxInFlight)
		}
	})
}

func TestCloneProviderEntry_PreservesMaxInFlight(t *testing.T) {
	// The TUI's save/rollback paths round-trip entries through this clone; a
	// field missing here is silently dropped (see ProviderEntry's own doc).
	e := ProviderEntry{MaxInFlight: 4}
	if got := cloneProviderEntry(e).MaxInFlight; got != 4 {
		t.Fatalf("cloned MaxInFlight = %d, want 4", got)
	}
}

func TestConfigFileMaxInFlightRoundTrip(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(filepath.Join(home, ".opencodereview"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".opencodereview", "config.json")
	configJSON := `{
  "provider": "acme",
  "custom_providers": {
    "acme": {
      "url": "https://acme.example/v1/messages",
      "api_key": "sk-test",
      "protocol": "anthropic",
      "model": "claude-test",
      "max_in_flight": 2
    }
  }
}`
	if err := os.WriteFile(path, []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadAppConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.CustomProviders["acme"].MaxInFlight; got != 2 {
		t.Fatalf("cmd view MaxInFlight = %d, want 2", got)
	}

	// The resolver mirrors the file independently; it must map and expose the
	// limit on the resolved endpoint, and reject negatives.
	ep, err := llm.ResolveEndpointWithOptions(path, llm.ResolveOptions{Provider: "acme"})
	if err != nil {
		t.Fatalf("ResolveEndpointWithOptions: %v", err)
	}
	if ep.MaxInFlight != 2 {
		t.Fatalf("ResolvedEndpoint.MaxInFlight = %d, want 2", ep.MaxInFlight)
	}

	negJSON := strings.Replace(configJSON, `"max_in_flight": 2`, `"max_in_flight": -3`, 1)
	negPath := filepath.Join(dir, "neg.json")
	if err := os.WriteFile(negPath, []byte(negJSON), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := llm.ResolveEndpointWithOptions(negPath, llm.ResolveOptions{Provider: "acme"}); err == nil || !strings.Contains(err.Error(), "max_in_flight") {
		t.Fatalf("negative max_in_flight: err = %v, want config error naming max_in_flight", err)
	}
}
