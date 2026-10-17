// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

type UpdateOptions struct {
	AcceptPermissions bool
}

// ExecutionChange identifies changed declarations without exposing their values.
type ExecutionChange struct {
	Contribution string   `json:"contribution"`
	Fields       []string `json:"fields"`
}

type ExecutionChangesError struct {
	PluginID string            `json:"plugin_id"`
	Changes  []ExecutionChange `json:"changes"`
}

func (e *ExecutionChangesError) Error() string {
	changes := make([]string, 0, len(e.Changes))
	for _, change := range e.Changes {
		changes = append(changes, change.Contribution+" ("+strings.Join(change.Fields, ", ")+")")
	}
	return fmt.Sprintf("plugin %q execution declarations changed: %s; inspect the source, then repeat update with --accept-permissions to accept", e.PluginID, strings.Join(changes, "; "))
}

func executionChanges(old, next Manifest) []ExecutionChange {
	var changes []ExecutionChange
	names := make([]string, 0, len(next.MCPServers))
	for name := range next.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		before, exists := old.MCPServers[name]
		after := next.MCPServers[name]
		if !exists {
			changes = append(changes, ExecutionChange{"mcp_servers." + name, []string{"added"}})
			continue
		}
		if before.Type == "" {
			before.Type = "stdio"
		}
		if after.Type == "" {
			after.Type = "stdio"
		}
		var fields []string
		for _, field := range []struct {
			name string
			same bool
		}{
			{"type", before.Type == after.Type},
			{"command", before.Command == after.Command},
			{"args", slices.Equal(before.Args, after.Args)},
			{"env", slices.Equal(before.Env, after.Env)},
			{"url", before.URL == after.URL},
			{"headers", maps.Equal(before.Headers, after.Headers)},
			{"tools", slices.Equal(before.Tools, after.Tools)},
		} {
			if !field.same {
				fields = append(fields, field.name)
			}
		}
		if len(fields) > 0 {
			changes = append(changes, ExecutionChange{"mcp_servers." + name, fields})
		}
	}
	return changes
}
