// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package toolsconfig loads the embedded tool definitions.
package toolsconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
)

// ToolConfigEntry holds a single tool definition loaded from tools.json.
type ToolConfigEntry struct {
	Name       string          `json:"name"`
	Definition json.RawMessage `json:"definition"`
}

//go:embed tools.json
var defaultToolsJSON []byte

// LoadDefault parses the embedded tool definitions shipped with this build.
func LoadDefault() ([]ToolConfigEntry, error) {
	var tools []ToolConfigEntry
	if err := json.Unmarshal(defaultToolsJSON, &tools); err != nil {
		return nil, fmt.Errorf("unmarshal embedded tools: %w", err)
	}
	return tools, nil
}

func Resolve(entries []ToolConfigEntry, names []string) ([]llm.ToolDef, error) {
	catalog := make(map[string]llm.ToolDef, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.Name) == "" {
			return nil, fmt.Errorf("tool name must not be empty")
		}
		if _, exists := catalog[entry.Name]; exists {
			return nil, fmt.Errorf("duplicate tool definition %q", entry.Name)
		}
		var function llm.FunctionDef
		if err := json.Unmarshal(entry.Definition, &function); err != nil {
			return nil, fmt.Errorf("parse tool %q: %w", entry.Name, err)
		}
		if function.Name != entry.Name {
			return nil, fmt.Errorf("tool %q has mismatched definition name %q", entry.Name, function.Name)
		}
		if strings.TrimSpace(function.Description) == "" || function.Parameters["type"] != "object" {
			return nil, fmt.Errorf("tool %q requires a description and object parameters", entry.Name)
		}
		function.RawDefinition = entry.Definition
		catalog[entry.Name] = llm.ToolDef{Type: "function", Function: function}
	}
	var definitions []llm.ToolDef
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		definition, exists := catalog[name]
		if !exists {
			return nil, fmt.Errorf("unknown tool reference %q", name)
		}
		seen[name] = true
		definitions = append(definitions, definition)
	}
	return definitions, nil
}
