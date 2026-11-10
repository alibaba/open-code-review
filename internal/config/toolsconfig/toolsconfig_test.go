// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package toolsconfig

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	tools, err := LoadDefault()
	if err != nil {
		t.Fatalf("Load default tools: %v", err)
	}
	expected := []string{"task_done", "code_comment", "code_search", "file_read", "file_read_diff", "file_find"}
	if len(tools) != len(expected) {
		t.Fatalf("got %d tools, want %d", len(tools), len(expected))
	}
	for index, want := range expected {
		entry := tools[index]
		if entry.Name != want {
			t.Errorf("tool %d: got %s, want %s", index, entry.Name, want)
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

func TestDefaultToolNamesAreUnique(test *testing.T) {
	entries, err := LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	seen := make(map[string]int, len(entries))
	for index, entry := range entries {
		if strings.TrimSpace(entry.Name) == "" {
			test.Errorf("tools.json entry %d has an empty name", index)
		}
		if previous, exists := seen[entry.Name]; exists {
			test.Errorf("tools.json entries %d and %d share the name %q", previous, index, entry.Name)
		} else {
			seen[entry.Name] = index
		}
	}
}

func TestDefaultToolNamesMatchDefinitions(test *testing.T) {
	entries, err := LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	for index, entry := range entries {
		var definition struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(entry.Definition, &definition); err != nil {
			test.Errorf("tools.json entry %d (%q): invalid definition: %v", index, entry.Name, err)
			continue
		}
		if definition.Name != entry.Name {
			test.Errorf("tools.json entry %d: name %q does not match definition.name %q", index, entry.Name, definition.Name)
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

func TestResolveOrderedDefinitions(test *testing.T) {
	entries, err := LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	names := []string{"file_find", "task_done", "file_read"}
	definitions, err := Resolve(entries, names)
	if err != nil {
		test.Fatal(err)
	}
	var actual []string
	for _, definition := range definitions {
		actual = append(actual, definition.Function.Name)
		if definition.Type != "function" {
			test.Errorf("unexpected tool type %q", definition.Type)
		}
		for _, entry := range entries {
			if entry.Name == definition.Function.Name && string(entry.Definition) != string(definition.Function.RawDefinition) {
				test.Errorf("raw definition changed for %s", entry.Name)
			}
		}
	}
	if !reflect.DeepEqual(actual, names) {
		test.Errorf("resolved names = %v, want %v", actual, names)
	}
	for _, empty := range [][]string{nil, {}} {
		definitions, err := Resolve(entries, empty)
		if err != nil || definitions != nil {
			test.Errorf("empty selection = %v, %v; want nil, nil", definitions, err)
		}
	}
}

func TestResolveDeduplicatesReferencesInFirstOccurrenceOrder(test *testing.T) {
	entries, err := LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	cases := []struct {
		name  string
		names []string
		want  []string
	}{
		{"adjacent", []string{"file_read", "file_read"}, []string{"file_read"}},
		{"separated", []string{"file_find", "task_done", "file_find", "file_read", "task_done"}, []string{"file_find", "task_done", "file_read"}},
		{"all repeated", []string{"code_search", "code_search", "code_search"}, []string{"code_search"}},
	}
	for _, scenario := range cases {
		test.Run(scenario.name, func(test *testing.T) {
			original := append([]string(nil), scenario.names...)
			definitions, err := Resolve(entries, scenario.names)
			if err != nil {
				test.Fatal(err)
			}
			var names []string
			for _, definition := range definitions {
				names = append(names, definition.Function.Name)
			}
			if !reflect.DeepEqual(names, scenario.want) {
				test.Errorf("resolved names = %v, want %v", names, scenario.want)
			}
			if !reflect.DeepEqual(scenario.names, original) {
				test.Errorf("input selection changed: got %v, want %v", scenario.names, original)
			}
		})
	}
}

func TestResolveInvalidConfiguration(test *testing.T) {
	valid := ToolConfigEntry{Name: "read", Definition: json.RawMessage(`{"name":"read","description":"Read a file","parameters":{"type":"object"}}`)}
	cases := []struct {
		name    string
		entries []ToolConfigEntry
		names   []string
		want    string
	}{
		{"unknown", []ToolConfigEntry{valid}, []string{"missing"}, `unknown tool reference "missing"`},
		{"empty reference", []ToolConfigEntry{valid}, []string{""}, `unknown tool reference ""`},
		{"unknown after duplicates", []ToolConfigEntry{valid}, []string{"read", "read", "missing"}, `unknown tool reference "missing"`},
		{"duplicate definition", []ToolConfigEntry{valid, valid}, nil, `duplicate tool definition "read"`},
		{"empty name", []ToolConfigEntry{{Name: "  "}}, nil, "tool name must not be empty"},
		{"malformed definition", []ToolConfigEntry{{Name: "read", Definition: json.RawMessage(`{invalid}`)}}, nil, `parse tool "read"`},
		{"mismatched name", []ToolConfigEntry{{Name: "other", Definition: valid.Definition}}, nil, "mismatched definition name"},
		{"null definition", []ToolConfigEntry{{Name: "read", Definition: json.RawMessage(`null`)}}, nil, "mismatched definition name"},
		{"missing description", []ToolConfigEntry{{Name: "read", Definition: json.RawMessage(`{"name":"read","parameters":{"type":"object"}}`)}}, nil, "requires a description and object parameters"},
		{"missing parameters", []ToolConfigEntry{{Name: "read", Definition: json.RawMessage(`{"name":"read","description":"Read"}`)}}, nil, "requires a description and object parameters"},
		{"invalid parameter type", []ToolConfigEntry{{Name: "read", Definition: json.RawMessage(`{"name":"read","description":"Read","parameters":{"type":"array"}}`)}}, nil, "requires a description and object parameters"},
	}
	for _, scenario := range cases {
		test.Run(scenario.name, func(test *testing.T) {
			definitions, err := Resolve(scenario.entries, scenario.names)
			if definitions != nil || err == nil || !strings.Contains(err.Error(), scenario.want) {
				test.Fatalf("Resolve = %v, %v; want no definitions and %q", definitions, err, scenario.want)
			}
		})
	}
}
