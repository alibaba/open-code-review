// Package toolsconfig loads tool definitions from JSON config files.
package toolsconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
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

// Load parses the tools config file. When path is empty, falls back to
// the embedded default tools configuration.
func Load(path string) ([]ToolConfigEntry, error) {
	var data []byte
	var err error
	if path == "" {
		data = defaultToolsJSON
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read tools file %s: %w", path, err)
		}
	}
	var tools []ToolConfigEntry
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("unmarshal tools file: %w", err)
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
