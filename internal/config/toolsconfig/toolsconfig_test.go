package toolsconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Default(t *testing.T) {
	tools, err := Load("")
	if err != nil {
		t.Fatalf("Load default tools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool from embedded config")
	}
	// Verify first tool has required fields
	first := tools[0]
	if first.Name == "" {
		t.Error("expected non-empty tool name")
	}
	if first.Definition == nil {
		t.Error("expected non-nil definition")
	}
}

func TestLoad_CustomFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "tools.json")
	data := `[
		{"name": "test_tool", "plan_task": true, "main_task": false, "definition": {"name": "test_tool"}}
	]`
	os.WriteFile(path, []byte(data), 0644)

	tools, err := Load(path)
	if err != nil {
		t.Fatalf("Load custom file: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "test_tool" {
		t.Errorf("expected name=test_tool, got %s", tools[0].Name)
	}
	if !tools[0].PlanTask {
		t.Error("expected PlanTask=true")
	}
	if tools[0].MainTask {
		t.Error("expected MainTask=false")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/tools.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "tools.json")
	os.WriteFile(path, []byte("not json"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
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
