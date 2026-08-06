// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUnset_LoadErrors covers the load-config error branch of each unset helper:
// when the config path holds invalid JSON, loadOrCreateConfig fails to parse it
// and the helper must surface the wrapped error rather than proceed.
func TestUnset_LoadErrors(t *testing.T) {
	newBadConfig := func(t *testing.T) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
			t.Fatalf("write bad config: %v", err)
		}
		return path
	}

	t.Run("unsetActiveProvider", func(t *testing.T) {
		if err := unsetActiveProvider(newBadConfig(t)); err == nil {
			t.Fatal("expected load error, got nil")
		}
	})

	t.Run("unsetCustomProvider", func(t *testing.T) {
		if err := unsetCustomProvider(newBadConfig(t), "any"); err == nil {
			t.Fatal("expected load error, got nil")
		}
	})

	t.Run("unsetMCPServer", func(t *testing.T) {
		if err := unsetMCPServer(newBadConfig(t), "any"); err == nil {
			t.Fatal("expected load error, got nil")
		}
	})
}
