// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package toolsconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	tools, err := LoadDefault()
	if err != nil {
		t.Fatalf("Load default tools: %v", err)
	}
	expected := []struct {
		name string
		plan bool
	}{
		{"task_done", false},
		{"code_comment", false},
		{"code_search", true},
		{"file_read", false},
		{"file_read_diff", true},
		{"file_find", true},
	}
	if len(tools) != len(expected) {
		t.Fatalf("got %d tools, want %d", len(tools), len(expected))
	}
	for index, want := range expected {
		entry := tools[index]
		if entry.Name != want.name || entry.PlanTask != want.plan || !entry.MainTask {
			t.Errorf("tool %d: got %s (plan=%v, main=%v), want %s (plan=%v, main=true)", index, entry.Name, entry.PlanTask, entry.MainTask, want.name, want.plan)
		}
		var definition struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  struct {
				Type       string                     `json:"type"`
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"parameters"`
		}
		if err := json.Unmarshal(entry.Definition, &definition); err != nil {
			t.Fatalf("parse %s: %v", entry.Name, err)
		}
		if definition.Name != entry.Name || definition.Description == "" {
			t.Errorf("%s: invalid name or missing description", entry.Name)
		}
		if definition.Parameters.Type != "object" || len(definition.Parameters.Properties) == 0 {
			t.Errorf("%s: expected object schema with properties", entry.Name)
		}
		for _, required := range definition.Parameters.Required {
			if _, exists := definition.Parameters.Properties[required]; !exists {
				t.Errorf("%s: required field %q missing from properties", entry.Name, required)
			}
		}
	}
}

func TestLoadDefaultInvalidJSON(t *testing.T) {
	original := defaultToolsJSON
	t.Cleanup(func() { defaultToolsJSON = original })
	defaultToolsJSON = []byte("not json")
	_, err := LoadDefault()
	if err == nil || !strings.Contains(err.Error(), "unmarshal embedded tools") {
		t.Fatalf("got %v, want embedded tools parse error", err)
	}
}

func TestLoadDefaultIndependentCopies(t *testing.T) {
	first, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	original := string(first[0].Definition)
	first[0].Name = "changed"
	first[0].Definition[0] = '!'
	second, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Name != "task_done" || string(second[0].Definition) != original {
		t.Fatal("mutating a loaded definition changed the embedded defaults")
	}
}

func TestToolDefsByPhase(t *testing.T) {
	def := json.RawMessage(`{"name": "test"}`)
	tests := []struct {
		name     string
		entry    ToolConfigEntry
		planOnly bool
		wantOk   bool
	}{
		{"plan_task and planOnly=true", ToolConfigEntry{PlanTask: true, MainTask: false, Definition: def}, true, true},
		{"plan_task and planOnly=false", ToolConfigEntry{PlanTask: true, MainTask: false, Definition: def}, false, false},
		{"main_task and planOnly=false", ToolConfigEntry{PlanTask: false, MainTask: true, Definition: def}, false, true},
		{"main_task and planOnly=true", ToolConfigEntry{PlanTask: false, MainTask: true, Definition: def}, true, false},
		{"both and planOnly=true", ToolConfigEntry{PlanTask: true, MainTask: true, Definition: def}, true, true},
		{"both and planOnly=false", ToolConfigEntry{PlanTask: true, MainTask: true, Definition: def}, false, true},
		{"neither", ToolConfigEntry{PlanTask: false, MainTask: false, Definition: def}, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.entry.ToolDefsByPhase(tc.planOnly)
			if ok != tc.wantOk {
				t.Errorf("ToolDefsByPhase(planOnly=%v) ok=%v, want %v", tc.planOnly, ok, tc.wantOk)
			}
			if tc.wantOk && got == nil {
				t.Error("expected non-nil definition when ok=true")
			}
			if !tc.wantOk && got != nil {
				t.Error("expected nil definition when ok=false")
			}
		})
	}
}
