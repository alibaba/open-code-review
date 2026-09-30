// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func scanOverrideExample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "scan", "bounded-template.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeScanOverride(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan-prompts.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func changeScanOverride(t *testing.T, key string, value any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(scanOverrideExample(t), &fields); err != nil {
		t.Fatal(err)
	}
	if value == nil {
		delete(fields, key)
	} else {
		fields[key] = value
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLoadScan_DefaultCompatibility(t *testing.T) {
	want, err := LoadScanDefault()
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadScan("")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("default changed: got=%+v, err=%v", got, err)
	}
	if got.PlanFallbackGuidance() != "(no pre-scan plan; review the entire file as usual)" {
		t.Fatal("default no-plan guidance changed")
	}
}

func TestLoadScan_PromptOnlyOverride(t *testing.T) {
	defaults, err := LoadScanDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, disablePlan := range []bool{false, true} {
		t.Run(map[bool]string{false: "inherit plan", true: "disable plan"}[disablePlan], func(t *testing.T) {
			data := scanOverrideExample(t)
			if !disablePlan {
				data = changeScanOverride(t, "PLAN_TASK", nil)
			}
			got, err := LoadScan(writeScanOverride(t, data))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.PromptOverrideSHA256) != 64 {
				t.Fatal("custom prompt identity missing")
			}
			if !strings.Contains(got.MainTask.Messages[0].Content, "selected") {
				t.Fatal("custom system prompt missing")
			}
			if strings.Contains(got.PlanFallbackGuidance(), "entire file") {
				t.Fatal("default no-plan scope leaked into the custom contract")
			}
			if disablePlan && got.PlanTask != nil {
				t.Fatal("PLAN_TASK null must disable planning")
			}
			if !disablePlan && !reflect.DeepEqual(got.PlanTask, defaults.PlanTask) {
				t.Fatal("omitting PLAN_TASK must preserve default planning")
			}
			// All settings and auxiliary tasks must remain identical to the control.
			got.MainTask = defaults.MainTask
			got.PlanTask = defaults.PlanTask
			got.NoPlanGuidance = defaults.NoPlanGuidance
			got.PromptOverrideSHA256 = ""
			if !reflect.DeepEqual(got, defaults) {
				t.Fatal("prompt selection changed runtime budgets or auxiliary tasks")
			}
		})
	}
}

func TestLoadScan_CustomPlanAndCanonicalIdentity(t *testing.T) {
	plan := LlmConversation{Messages: []ChatMessage{
		{Role: "system", Content: "Plan only the supplied function."},
		{Role: "user", Content: "{{current_file_path}} {{file_content}} {{system_rule}} {{current_system_date_time}}"},
	}}
	data := changeScanOverride(t, "PLAN_TASK", plan)
	got, err := LoadScan(writeScanOverride(t, data))
	if err != nil || !reflect.DeepEqual(got.PlanTask, &plan) {
		t.Fatalf("custom plan: got=%+v, err=%v", got, err)
	}
	var pretty strings.Builder
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(&pretty)
	enc.SetIndent("", "    ")
	if err := enc.Encode(fields); err != nil {
		t.Fatal(err)
	}
	same, err := LoadScan(writeScanOverride(t, []byte(pretty.String())))
	if err != nil || same.PromptOverrideSHA256 != got.PromptOverrideSHA256 {
		t.Fatalf("JSON formatting or file location changed identity: %v", err)
	}
	changed := strings.Replace(string(data), "Plan only", "Focus only", 1)
	other, err := LoadScan(writeScanOverride(t, []byte(changed)))
	if err != nil || other.PromptOverrideSHA256 == got.PromptOverrideSHA256 {
		t.Fatalf("changed plan must change identity: %v", err)
	}
}

func TestLoadScan_RejectsInvalidOverrides(t *testing.T) {
	valid := string(scanOverrideExample(t))
	tests := []struct {
		name string
		data string
	}{
		{"empty file", ""},
		{"invalid JSON", "{bad"},
		{"truncated object", "{\"MAIN_TASK\":"},
		{"truncated key", "{\"MAIN_TASK"},
		{"truncated array", "{\"MAIN_TASK\":{\"messages\":["},
		{"excessive nesting", strings.Repeat("[", 10001) + strings.Repeat("]", 10001)},
		{"null document", "null"},
		{"array document", "[]"},
		{"missing fields", "{}"},
		{"missing main", string(changeScanOverride(t, "MAIN_TASK", nil))},
		{"missing no-plan", string(changeScanOverride(t, "NO_PLAN_GUIDANCE", nil))},
		{"blank no-plan", string(changeScanOverride(t, "NO_PLAN_GUIDANCE", " \n "))},
		{"no-plan placeholder", string(changeScanOverride(t, "NO_PLAN_GUIDANCE", "{{file_content}}"))},
		{"no-plan closing delimiter", string(changeScanOverride(t, "NO_PLAN_GUIDANCE", "bad }}"))},
		{"unknown main placeholder", strings.Replace(valid, "{{file_content}}", "{{diff}}", 1)},
		{"unclosed placeholder", strings.Replace(valid, "{{file_content}}", "{{file_content", 1)},
		{"stray closing placeholder", strings.Replace(valid, "File:", "bad }} File:", 1)},
		{"trailing closing placeholder", strings.Replace(valid, "{{plan_guidance}}", "{{plan_guidance}} }}", 1)},
		{"wrong initial role", strings.Replace(valid, "\"role\": \"system\"", "\"role\": \"assistant\"", 1)},
		{"wrong final role", strings.Replace(valid, "\"role\": \"user\"", "\"role\": \"tool\"", 1)},
		{"blank content", strings.Replace(valid, "\"content\": \"Review only the function selected in the requirement background, using only the supplied source and review rules. Do not request other files or call context-acquisition tools. Report supported findings via code_comment, then call task_done.\"", "\"content\": \" \"", 1)},
		{"empty main", string(changeScanOverride(t, "MAIN_TASK", LlmConversation{}))},
		{"wrong main type", string(changeScanOverride(t, "MAIN_TASK", 42))},
		{"empty plan", string(changeScanOverride(t, "PLAN_TASK", LlmConversation{}))},
		{"wrong plan type", string(changeScanOverride(t, "PLAN_TASK", "plan"))},
		{"duplicate root field", strings.Replace(valid, "{", "{\"MAIN_TASK\": {},", 1)},
		{"case variant duplicate", strings.Replace(valid, "{", "{\"main_task\": {},", 1)},
		{"duplicate message field", strings.Replace(valid, "\"role\": \"system\"", "\"role\": \"system\", \"role\": \"system\"", 1)},
		{"unknown conversation field", strings.Replace(valid, "\"messages\":", "\"timeout\": 90, \"messages\":", 1)},
		{"unknown message field", strings.Replace(valid, "\"role\": \"system\"", "\"role\": \"system\", \"prompt_file\": \"file.md\"", 1)},
		{"second JSON value", valid + "{}"},
		{"trailing garbage", valid + "invalid"},
		{"invalid UTF-8", valid + string([]byte{0xff})},
	}
	for _, name := range []string{"current_file_path", "file_content", "system_rule", "requirement_background", "plan_guidance"} {
		tests = append(tests, struct{ name, data string }{"missing " + name, strings.ReplaceAll(valid, "{{"+name+"}}", "")})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl, err := LoadScan(writeScanOverride(t, []byte(tt.data)))
			if err == nil || tpl != nil {
				t.Fatalf("invalid override must fail without a fallback: tpl=%+v, err=%v", tpl, err)
			}
		})
	}
}

func TestLoadScan_RejectsBudgetsAndUnknownPlanFields(t *testing.T) {
	for _, field := range []string{
		"MAX_TOKENS", "MAX_COMPLETION_TOKENS", "MAX_TOOL_REQUEST_TIMES",
		"MAX_TOKENS_BUDGET", "MAX_FILE_SIZE_BYTES", "BATCH_SIZE", "tools",
	} {
		t.Run(field, func(t *testing.T) {
			tpl, err := LoadScan(writeScanOverride(t, changeScanOverride(t, field, 123)))
			if err == nil || tpl != nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("runtime setting must be rejected: tpl=%+v, err=%v", tpl, err)
			}
		})
	}
	plans := []any{
		map[string]any{"messages": []any{}, "timeout": 90},
		LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "plan"}, {Role: "user", Content: "{{plan_guidance}}"}}},
		LlmConversation{Messages: []ChatMessage{{Role: "system", Content: "plan"}, {Role: "assistant", Content: "example"}, {Role: "user", Content: "{{file_content}}"}}},
	}
	for _, plan := range plans {
		tpl, err := LoadScan(writeScanOverride(t, changeScanOverride(t, "PLAN_TASK", plan)))
		if err == nil || tpl != nil || !strings.Contains(err.Error(), "PLAN_TASK") {
			t.Fatalf("invalid plan must fail closed: tpl=%+v, err=%v", tpl, err)
		}
	}
}

func TestLoadScan_FileReadFailure(t *testing.T) {
	tpl, err := LoadScan(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || tpl != nil || !strings.Contains(err.Error(), "read scan prompt override") {
		t.Fatalf("missing override must fail closed: tpl=%+v, err=%v", tpl, err)
	}
}

func TestLoadScan_RejectsUnicodeFieldAliases(t *testing.T) {
	valid := string(scanOverrideExample(t))
	tests := []struct{ name, data string }{
		{"root long s alias", strings.Replace(valid, "{", "{\"MAIN_TA\\u017fK\": {},", 1)},
		{"root kelvin alias", strings.Replace(valid, "{", "{\"MAIN_TAS\\u212a\": {},", 1)},
		{"nested messages alias", strings.Replace(valid, "\"messages\":", "\"me\\u017f\\u017fages\": [], \"messages\":", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl, err := LoadScan(writeScanOverride(t, []byte(tt.data)))
			if err == nil || tpl != nil {
				t.Fatalf("Unicode field alias must be rejected: tpl=%+v, err=%v", tpl, err)
			}
		})
	}
	// Restrict field names, while keeping international prompt content valid.
	want := "Review the r\u00e9sum\u00e9."
	tpl, err := LoadScan(writeScanOverride(t, changeScanOverride(t, "NO_PLAN_GUIDANCE", want)))
	if err != nil || tpl.PlanFallbackGuidance() != want {
		t.Fatalf("Unicode prompt content must remain valid: %v", err)
	}
}
