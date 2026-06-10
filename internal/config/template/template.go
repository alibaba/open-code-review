// Package template loads and validates task prompt templates for the code review agent.
package template

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Template holds the native agent task template configuration.
// Mirrors NativeAgentTemplate from the Java implementation, loaded via JSON
// at runtime.
//
// Field grouping: the "scan-only fields" section is consumed exclusively by
// internal/scan; internal/agent (diff review) does not read those fields.
type Template struct {
	// Diff-review fields.
	MainTask              LlmConversation  `json:"MAIN_TASK"`
	PlanTask              *LlmConversation `json:"PLAN_TASK,omitempty"`
	MemoryCompressionTask LlmConversation  `json:"MEMORY_COMPRESSION_TASK"`
	MaxTokens             int              `json:"MAX_TOKENS"`
	ToolRequestWaitTimeMs int              `json:"TOOL_REQUEST_WAIT_TIME_MS"`
	MaxToolRequestTimes   int              `json:"MAX_TOOL_REQUEST_TIMES"`
	MaxSubtaskExecMinutes int              `json:"MAX_SUBTASK_EXECUTION_TIME_MINUTES"`
	PlanModeLineThreshold int              `json:"PLAN_MODE_LINE_THRESHOLD"`
	ReLocationTask        *LlmConversation `json:"RE_LOCATION_TASK,omitempty"`
	ReviewFilterTask      *LlmConversation `json:"REVIEW_FILTER_TASK,omitempty"`

	// Scan-only fields — consumed by internal/scan, never by internal/agent.
	FullScanTask                *LlmConversation `json:"FULL_SCAN_TASK,omitempty"`
	FullScanMaxToolRequestTimes int              `json:"FULL_SCAN_MAX_TOOL_REQUEST_TIMES,omitempty"`
}

//go:embed task_template.json
var defaultTemplate []byte

// LoadDefault parses the embedded task_template.json.
func LoadDefault() (*Template, error) {
	var tpl Template
	if err := json.Unmarshal(defaultTemplate, &tpl); err != nil {
		return nil, fmt.Errorf("unmarshal default template: %w", err)
	}
	return &tpl, nil
}

// applyLanguage appends instruction to all system-role messages in conv.
func applyLanguage(conv *LlmConversation, instruction string) {
	for i := range conv.Messages {
		if conv.Messages[i].Role == "system" {
			conv.Messages[i].Content += instruction
		}
	}
}

// resolveLang returns the resolved language name for the instruction.
func resolveLang(lang string) string {
	if lang == "" {
		return "English"
	}
	return lang
}

// ApplyLanguage injects a language directive into all system-role messages
// across MAIN_TASK, PLAN_TASK (if set), FULL_SCAN_TASK (if set), and
// MEMORY_COMPRESSION_TASK.
func (t *Template) ApplyLanguage(lang string) {
	instruction := "\n\nAlways respond in " + resolveLang(lang) + "."
	applyLanguage(&t.MainTask, instruction)
	if t.PlanTask != nil {
		applyLanguage(t.PlanTask, instruction)
	}
	if t.FullScanTask != nil {
		applyLanguage(t.FullScanTask, instruction)
	}
	applyLanguage(&t.MemoryCompressionTask, instruction)
}
func (t *Template) Validate() error {
	if t.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}
	if t.MaxToolRequestTimes <= 0 {
		return fmt.Errorf("max_tool_request_times must be positive")
	}
	if len(t.MainTask.Messages) == 0 {
		return fmt.Errorf("main_task.messages must not be empty")
	}
	return nil
}

// LlmConversation mirrors LlmConversation from the Java side — a preset prompt with settings.
type LlmConversation struct {
	Timeout  int           `json:"timeout"`
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
