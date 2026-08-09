// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"testing"

	"github.com/alibaba/open-code-review/internal/tool"
)

// silenceStderr redirects os.Stderr to /dev/null for the duration of fn so the
// MCP init warnings do not clutter test logs.
func silenceStderr(t *testing.T, fn func()) {
	t.Helper()
	orig := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	os.Stderr = devnull
	defer func() {
		os.Stderr = orig
		_ = devnull.Close()
	}()
	fn()
}

// TestInitMCPClients_ErrorBranches covers the connect/start/setup failure paths
// that end in a "skip this server" continue. Each server fails fast (refused
// connection, missing binary, non-zero setup) so no real MCP server is needed.
func TestInitMCPClients_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("remote connect failure skipped", func(t *testing.T) {
		reg := tool.NewRegistry()
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			// Port 1 is reserved and refuses connections immediately.
			"r": {Type: "remote", URL: "http://127.0.0.1:1/mcp"},
		}}
		var clients []interface{}
		silenceStderr(t, func() {
			for _, c := range initMCPClients(ctx, cfg, reg, t.TempDir(), "v") {
				clients = append(clients, c)
			}
		})
		if len(clients) != 0 {
			t.Errorf("got %d clients, want 0 (connect should fail)", len(clients))
		}
	})

	t.Run("stdio start failure skipped", func(t *testing.T) {
		reg := tool.NewRegistry()
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"s": {Type: "stdio", Command: "ocr-nonexistent-binary-xyz"},
		}}
		var n int
		silenceStderr(t, func() {
			n = len(initMCPClients(ctx, cfg, reg, t.TempDir(), "v"))
		})
		if n != 0 {
			t.Errorf("got %d clients, want 0 (start should fail)", n)
		}
	})

	t.Run("setup failure skips server", func(t *testing.T) {
		reg := tool.NewRegistry()
		cfg := &Config{MCPServers: map[string]MCPServerConfig{
			"s": {Type: "stdio", Command: "true", Setup: "exit 1"},
		}}
		var n int
		silenceStderr(t, func() {
			n = len(initMCPClients(ctx, cfg, reg, t.TempDir(), "v"))
		})
		if n != 0 {
			t.Errorf("got %d clients, want 0 (setup should fail and skip)", n)
		}
	})
}
