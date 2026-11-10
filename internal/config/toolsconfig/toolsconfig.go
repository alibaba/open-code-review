// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package toolsconfig loads the embedded tool definitions.
package toolsconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// ToolConfigEntry holds a single tool definition loaded from tools.json.
type ToolConfigEntry struct {
	Name       string          `json:"name"`
	PlanTask   bool            `json:"plan_task"`
	MainTask   bool            `json:"main_task"`
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

// ToolDefsByPhase returns the parsed tool definitions filtered by phase.
// planOnly=true returns only tools with plan_task:true.
// planOnly=false returns only tools with main_task:true.
func (t *ToolConfigEntry) ToolDefsByPhase(planOnly bool) (json.RawMessage, bool) {
	switch {
	case planOnly && t.PlanTask:
		return t.Definition, true
	case !planOnly && t.MainTask:
		return t.Definition, true
	default:
		return nil, false
	}
}
