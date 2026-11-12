// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package toolsconfig

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestComparisonToolDefinitions(test *testing.T) {
	entries, err := LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	names := []string{"task_done", "code_comment", "code_search", "file_read", "file_read_diff", "file_find"}
	base, err := Resolve(entries, names)
	if err != nil {
		test.Fatal(err)
	}
	before, _ := json.Marshal(base)
	derived, err := WithComparisonSides(base)
	if err != nil {
		test.Fatal(err)
	}
	for index, original := range base {
		updated := derived[index].Function
		if original.Function.Name != updated.Name {
			test.Fatal("tool order changed")
		}
		if updated.Name != "file_read" && updated.Name != "file_find" && updated.Name != "code_search" {
			if !reflect.DeepEqual(original, derived[index]) {
				test.Fatalf("unrelated tool changed: %s", updated.Name)
			}
			continue
		}
		properties := updated.Parameters["properties"].(map[string]any)
		side := properties["side"].(map[string]any)
		if side["type"] != "string" || !reflect.DeepEqual(side["enum"], []any{"new", "old"}) {
			test.Fatalf("unexpected side: %#v", side)
		}
		if !reflect.DeepEqual(updated.Parameters["required"], original.Function.Parameters["required"]) {
			test.Fatal("required parameters changed")
		}
		var rawFunction llm.FunctionDef
		if err := json.Unmarshal(updated.RawDefinition, &rawFunction); err != nil {
			test.Fatal(err)
		}
		if !reflect.DeepEqual(rawFunction.Parameters, updated.Parameters) || rawFunction.Description != updated.Description {
			test.Fatal("raw and parsed definitions disagree")
		}
		var originalFields, updatedFields map[string]json.RawMessage
		json.Unmarshal(original.Function.RawDefinition, &originalFields)
		json.Unmarshal(updated.RawDefinition, &updatedFields)
		var oldParameters, newParameters map[string]json.RawMessage
		json.Unmarshal(originalFields["parameters"], &oldParameters)
		json.Unmarshal(updatedFields["parameters"], &newParameters)
		var oldProperties bytes.Buffer
		json.Compact(&oldProperties, oldParameters["properties"])
		if !bytes.HasPrefix(newParameters["properties"], bytes.TrimSuffix(oldProperties.Bytes(), []byte("}"))) {
			test.Fatal("original property order changed")
		}
		if strings.Contains(updated.Description, "can only read file content from the modified version") || strings.Contains(updated.Description, "searches in the current version of files") {
			test.Fatal("new-side-only description remains")
		}
		properties["side"] = "mutated"
	}
	after, _ := json.Marshal(base)
	if !bytes.Equal(before, after) {
		test.Fatal("comparison definitions mutated the base catalog")
	}
	again, err := Resolve(entries, names)
	if err != nil || !reflect.DeepEqual(again, base) {
		test.Fatal("later current-only resolution was changed")
	}
	if got, err := WithComparisonSides(nil); got != nil || err != nil {
		test.Fatalf("nil definitions: %v %v", got, err)
	}
}

func TestComparisonDefinitionErrors(test *testing.T) {
	for _, raw := range []string{"{", `{}`, `{"parameters":[]}`, `{"parameters":{"properties":null}}`, `{"parameters":{"properties":[]}}`, `{"parameters":{"properties":{"side":{}}}}`} {
		_, err := WithComparisonSides([]llm.ToolDef{{Function: llm.FunctionDef{Name: "file_read", RawDefinition: json.RawMessage(raw)}}})
		if err == nil {
			test.Fatalf("accepted invalid definition %s", raw)
		}
	}
	definition := llm.FunctionDef{Name: "file_read", RawDefinition: json.RawMessage(`{"name":"file_read","parameters":{"properties":{}}}`)}
	if _, err := comparisonDefinition(definition); err != nil {
		test.Fatalf("empty properties: %v", err)
	}
}
