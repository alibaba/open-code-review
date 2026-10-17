// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateExecutionAcceptance(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			ctx := context.Background()
			store := NewStore(t.TempDir())
			source := testPackage(t, "example", "1")
			old, err := store.Install(ctx, source, enabled)
			if err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(store.root, "state.json")
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatal(err)
			}
			next := testManifest("example", "2")
			next.MCPServers = map[string]MCPServer{"notes": {Command: "never-execute-me", Env: []string{"TOKEN=secret-token"}}}
			writeTestJSON(t, filepath.Join(source, ManifestFilename), next)
			_, err = store.Update(ctx, "example")
			var changed *ExecutionChangesError
			if !errors.As(err, &changed) || len(changed.Changes) != 1 || changed.Changes[0].Contribution != "mcp_servers.notes" || strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("update error = %v", err)
			}
			after, err := os.ReadFile(statePath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected update changed state: %v", err)
			}
			entries, err := os.ReadDir(filepath.Join(store.root, "packages"))
			if err != nil || len(entries) != 1 {
				t.Fatalf("rejected update created a snapshot: %v, %v", entries, err)
			}
			updated, err := store.UpdateWithOptions(ctx, "example", UpdateOptions{AcceptPermissions: true})
			if err != nil || updated.Manifest.Version != "2" || updated.Enabled != enabled || updated.Dir == old.Dir {
				t.Fatalf("accepted update = %+v, %v", updated, err)
			}
			if _, err := store.Update(ctx, "example"); err != nil {
				t.Fatalf("unchanged declarations asked again: %v", err)
			}
			writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "3"))
			if _, err := store.Update(ctx, "example"); err != nil {
				t.Fatalf("removing execution required acceptance: %v", err)
			}
		})
	}
}

func TestExecutionDeclarationChanges(t *testing.T) {
	for _, test := range []struct {
		field string
		edit  func(*MCPServer)
	}{
		{"type", func(s *MCPServer) { s.Type = "remote" }},
		{"command", func(s *MCPServer) { s.Command = "other" }},
		{"args", func(s *MCPServer) { s.Args = []string{"secret"} }},
		{"env", func(s *MCPServer) { s.Env = []string{"TOKEN=secret"} }},
		{"url", func(s *MCPServer) { s.URL = "https://example.com/new" }},
		{"headers", func(s *MCPServer) { s.Headers = map[string]string{"Authorization": "secret"} }},
		{"tools", func(s *MCPServer) { s.Tools = []string{"additional_tool"} }},
	} {
		t.Run(test.field, func(t *testing.T) {
			before := Manifest{MCPServers: map[string]MCPServer{"server": {Command: "original"}}}
			server := before.MCPServers["server"]
			test.edit(&server)
			after := Manifest{MCPServers: map[string]MCPServer{"server": server}}
			changes := executionChanges(before, after)
			if len(changes) != 1 || len(changes[0].Fields) != 1 || changes[0].Fields[0] != test.field {
				t.Fatalf("changes = %+v", changes)
			}
		})
	}
	before := Manifest{MCPServers: map[string]MCPServer{"server": {Command: "same"}}}
	after := Manifest{MCPServers: map[string]MCPServer{"server": {Type: "stdio", Command: "same", Args: []string{}, Headers: map[string]string{}}}}
	if changes := executionChanges(before, after); len(changes) != 0 {
		t.Fatalf("equivalent defaults = %+v", changes)
	}
	for _, field := range []string{"added", "protocol", "command", "args", "stages"} {
		t.Run("review/"+field, func(t *testing.T) {
			before := Manifest{Review: &ReviewContribution{Protocol: "stdio-v1", Command: "review", Stages: []string{"inspect"}}}
			copy := *before.Review
			after := Manifest{Review: &copy}
			switch field {
			case "added":
				before.Review = nil
			case "protocol":
				after.Review.Protocol = "stdio-v2"
			case "command":
				after.Review.Command = "other"
			case "args":
				after.Review.Args = []string{"secret"}
			case "stages":
				after.Review.Stages = []string{"inspect", "verify"}
			}
			changes := executionChanges(before, after)
			if len(changes) != 1 || len(changes[0].Fields) != 1 || changes[0].Fields[0] != field {
				t.Fatalf("changes = %+v", changes)
			}
			if changes := executionChanges(after, after); len(changes) != 0 {
				t.Fatalf("same workflow = %+v", changes)
			}
		})
	}
}
