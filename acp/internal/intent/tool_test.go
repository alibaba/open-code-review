// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"encoding/json"
	"testing"
)

func TestSubmitIntentToolSchema(t *testing.T) {
	tool := SubmitIntentTool()
	if tool.Name != submitIntentToolName || tool.Description == "" {
		t.Fatalf("tool = %+v", tool)
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	want := map[string]bool{
		"action": true, "review": true, "scan": true, "extra": true,
		"question": true, "missing": true, "reason": true, "hint": true,
	}
	if len(schema.Properties) != len(want) {
		t.Fatalf("properties = %v, want exactly the allowed set", schema.Properties)
	}
	for k := range schema.Properties {
		if !want[k] {
			t.Fatalf("schema exposes unexpected property %q", k)
		}
	}
	// No adapter-owned or infrastructure flag is expressible.
	for _, forbidden := range []string{"format", "audience", "color", "output", "repo", "rule", "model", "provider", "ocr_binary", "binary", "staged", "resume"} {
		if _, ok := schema.Properties[forbidden]; ok {
			t.Fatalf("schema exposes forbidden property %q", forbidden)
		}
	}
	if len(schema.Required) != 1 || schema.Required[0] != "action" {
		t.Fatalf("required = %v, want [action]", schema.Required)
	}
}

func TestBuildUserPromptWithoutState(t *testing.T) {
	if got := buildUserPrompt("hello", nil); got != "User request:\nhello" {
		t.Fatalf("prompt = %q", got)
	}
}
