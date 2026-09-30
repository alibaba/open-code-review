// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const defaultNoPlanGuidance = "(no pre-scan plan; review the entire file as usual)"

// PlanFallbackGuidance returns the prompt used whenever planning is skipped or fails.
func (t ScanTemplate) PlanFallbackGuidance() string {
	if t.NoPlanGuidance != "" {
		return t.NoPlanGuidance
	}
	return defaultNoPlanGuidance
}

// scanPromptOverride deliberately excludes runtime limits and tool configuration.
// Omitted PLAN_TASK retains the default; an explicit null disables planning.
type scanPromptOverride struct {
	MainTask       LlmConversation `json:"MAIN_TASK"`
	PlanTask       json.RawMessage `json:"PLAN_TASK"`
	NoPlanGuidance string          `json:"NO_PLAN_GUIDANCE"`
}

// LoadScan loads the default scan configuration and, when path is nonempty,
// replaces its prompts using a strictly validated, prompt-only JSON file.
func LoadScan(path string) (*ScanTemplate, error) {
	tpl, err := LoadScanDefault()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return tpl, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scan prompt override: %w", err)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("scan prompt override must be valid UTF-8")
	}
	// Validate syntax and nesting depth before recursively inspecting fields.
	if !json.Valid(data) {
		return nil, fmt.Errorf("scan prompt override must contain exactly one valid JSON value")
	}
	if err := rejectDuplicatePromptFields(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return nil, fmt.Errorf("invalid scan prompt override: %w", err)
	}
	var override scanPromptOverride
	if err := decodePromptJSON(data, &override); err != nil {
		return nil, fmt.Errorf("invalid scan prompt override: %w", err)
	}
	mainPlaceholders := []string{
		"current_file_path", "file_content", "system_rule", "requirement_background",
		"plan_guidance", "current_system_date_time", "change_files",
	}
	if err := validateScanPromptMessages(override.MainTask, mainPlaceholders, mainPlaceholders[:5]); err != nil {
		return nil, fmt.Errorf("MAIN_TASK: %w", err)
	}
	if strings.TrimSpace(override.NoPlanGuidance) == "" {
		return nil, fmt.Errorf("NO_PLAN_GUIDANCE must not be empty")
	}
	if strings.Contains(override.NoPlanGuidance, "{{") || strings.Contains(override.NoPlanGuidance, "}}") {
		return nil, fmt.Errorf("NO_PLAN_GUIDANCE must be literal text without placeholders")
	}
	if len(override.PlanTask) > 0 {
		var plan *LlmConversation
		if err := decodePromptJSON(override.PlanTask, &plan); err != nil {
			return nil, fmt.Errorf("PLAN_TASK: %w", err)
		}
		if plan != nil {
			placeholders := []string{"current_file_path", "file_content", "system_rule", "current_system_date_time"}
			if err := validateScanPromptMessages(*plan, placeholders, placeholders[:3]); err != nil {
				return nil, fmt.Errorf("PLAN_TASK: %w", err)
			}
		}
		tpl.PlanTask = plan
	}
	tpl.MainTask = override.MainTask
	tpl.NoPlanGuidance = override.NoPlanGuidance

	// Hash the effective prompts, rather than the file path or JSON formatting.
	// This prevents checkpoints from a different review contract being reused.
	prompts := struct {
		MainTask       LlmConversation
		PlanTask       *LlmConversation
		NoPlanGuidance string
	}{tpl.MainTask, tpl.PlanTask, tpl.NoPlanGuidance}
	canonical, err := json.Marshal(prompts)
	if err != nil {
		return nil, fmt.Errorf("encode scan prompts: %w", err)
	}
	tpl.PromptOverrideSHA256 = fmt.Sprintf("%x", sha256.Sum256(canonical))
	return tpl, nil
}

func decodePromptJSON(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

// Reject non-ASCII field names and duplicate keys, including ASCII case variants.
// encoding/json also accepts Unicode aliases for ASCII field names; rejecting
// them keeps duplicate detection consistent with the supported schema.
func rejectDuplicatePromptFields(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name := key.(string)
			for _, r := range name {
				if r >= utf8.RuneSelf {
					return fmt.Errorf("unknown field %q: field names must be ASCII", name)
				}
			}
			name = strings.ToLower(name)
			if seen[name] {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[name] = true
			if err := rejectDuplicatePromptFields(dec); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for dec.More() {
			if err := rejectDuplicatePromptFields(dec); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = dec.Token()
	return err
}

func validateScanPromptMessages(conv LlmConversation, allowed, required []string) error {
	if len(conv.Messages) < 2 || conv.Messages[0].Role != "system" || conv.Messages[len(conv.Messages)-1].Role != "user" {
		return fmt.Errorf("messages must start with a system message and end with a user message")
	}
	seen := make(map[string]bool)
	for i, message := range conv.Messages {
		if message.Role != "system" && message.Role != "user" {
			return fmt.Errorf("messages[%d]: unsupported role %q", i, message.Role)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("messages[%d]: content must not be empty", i)
		}
		remaining := message.Content
		for {
			start := strings.Index(remaining, "{{")
			if start < 0 {
				if strings.Contains(remaining, "}}") {
					return fmt.Errorf("messages[%d]: malformed placeholder", i)
				}
				break
			}
			end := strings.Index(remaining[start+2:], "}}")
			if end < 0 || strings.Contains(remaining[:start], "}}") {
				return fmt.Errorf("messages[%d]: malformed placeholder", i)
			}
			name := remaining[start+2 : start+2+end]
			known := false
			for _, candidate := range allowed {
				if name == candidate {
					known = true
					break
				}
			}
			if !known {
				return fmt.Errorf("messages[%d]: unsupported placeholder %q", i, name)
			}
			seen[name] = true
			remaining = remaining[start+2+end+2:]
		}
	}
	for _, name := range required {
		if !seen[name] {
			return fmt.Errorf("missing required placeholder {{%s}}", name)
		}
	}
	return nil
}
