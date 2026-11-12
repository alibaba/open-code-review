// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package toolsconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
)

func WithComparisonSides(definitions []llm.ToolDef) ([]llm.ToolDef, error) {
	result := append([]llm.ToolDef(nil), definitions...)
	for index, definition := range result {
		name := definition.Function.Name
		if name != "file_read" && name != "file_find" && name != "code_search" {
			continue
		}
		updated, err := comparisonDefinition(definition.Function)
		if err != nil {
			return nil, fmt.Errorf("add comparison sides to %s: %w", name, err)
		}
		result[index].Function = updated
	}
	return result, nil
}

func comparisonDefinition(function llm.FunctionDef) (llm.FunctionDef, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(function.RawDefinition, &fields); err != nil {
		return llm.FunctionDef{}, err
	}
	var parameters map[string]json.RawMessage
	if err := json.Unmarshal(fields["parameters"], &parameters); err != nil {
		return llm.FunctionDef{}, err
	}
	properties := bytes.TrimSpace(parameters["properties"])
	var propertyMap map[string]json.RawMessage
	if err := json.Unmarshal(properties, &propertyMap); err != nil {
		return llm.FunctionDef{}, err
	}
	if propertyMap == nil {
		return llm.FunctionDef{}, fmt.Errorf("parameters.properties must be an object")
	}
	if _, exists := propertyMap["side"]; exists {
		return llm.FunctionDef{}, fmt.Errorf("side is already defined")
	}
	updatedProperties := append([]byte(nil), properties[:len(properties)-1]...)
	if len(propertyMap) > 0 {
		updatedProperties = append(updatedProperties, ',')
	}
	updatedProperties = append(updatedProperties, []byte(`"side":{"type":"string","enum":["new","old"],"description":"The side of the current comparison to access. Defaults to new. Paths and line numbers belong to the selected side."}}`)...)
	parameters["properties"] = updatedProperties
	fields["parameters"], _ = json.Marshal(parameters)
	description := strings.ReplaceAll(function.Description,
		"This tool can only read file content from the modified version (after changes) in git diff.",
		"This tool reads either side of the current comparison.")
	description = strings.ReplaceAll(description,
		"This tool searches in the current version of files.",
		"This tool searches in the selected side of the current comparison.")
	description += "\n\nOmit side or use side=new for the reviewed new version. Use side=old for the frozen comparison base, not the current target branch tip. For renamed files, use the path on the selected side; paths are not automatically translated. An empty old tree contains no files."
	fields["description"], _ = json.Marshal(description)
	raw, err := json.Marshal(fields)
	if err != nil {
		return llm.FunctionDef{}, err
	}
	var updated llm.FunctionDef
	if err := json.Unmarshal(raw, &updated); err != nil {
		return llm.FunctionDef{}, err
	}
	updated.RawDefinition = raw
	return updated, nil
}
