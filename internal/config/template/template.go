// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package template loads and validates task prompt templates for the code review agent.
package template

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Template holds the native agent task template configuration.
// Scan-mode fields live in ScanTemplate, not here.
type Template struct {
	MainTask                   LlmConversation  `json:"MAIN_TASK"`
	PlanTask                   *LlmConversation `json:"PLAN_TASK,omitempty"`
	MemoryCompressionTask      LlmConversation  `json:"MEMORY_COMPRESSION_TASK"`
	MaxTokens                  int              `json:"MAX_TOKENS"`
	MaxCompletionTokens        int              `json:"MAX_COMPLETION_TOKENS"`
	MaxToolRequestTimes        int              `json:"MAX_TOOL_REQUEST_TIMES"`
	PlanModeLineThreshold      int              `json:"PLAN_MODE_LINE_THRESHOLD"`
	PlanModeGroupLineThreshold int              `json:"PLAN_MODE_GROUP_LINE_THRESHOLD"`
	MaxReviewRounds            int              `json:"MAX_REVIEW_ROUNDS"`
	ReLocationTask             *LlmConversation `json:"RE_LOCATION_TASK,omitempty"`
	ReviewFilterTask           *LlmConversation `json:"REVIEW_FILTER_TASK,omitempty"`
	GroupingTask               *LlmConversation `json:"GROUPING_TASK,omitempty"`
}

// ScanTemplate holds the full-file scan task template configuration loaded
// from scan_template.json. Kept entirely separate from Template so the two
// pipelines can evolve their prompts and budgets independently.
type ScanTemplate struct {
	MainTask              LlmConversation  `json:"MAIN_TASK"`
	PlanTask              *LlmConversation `json:"PLAN_TASK,omitempty"`
	MemoryCompressionTask LlmConversation  `json:"MEMORY_COMPRESSION_TASK"`
	ReLocationTask        *LlmConversation `json:"RE_LOCATION_TASK,omitempty"`
	MaxTokens             int              `json:"MAX_TOKENS"`
	MaxCompletionTokens   int              `json:"MAX_COMPLETION_TOKENS,omitempty"`
	ToolRequestWaitTimeMs int              `json:"TOOL_REQUEST_WAIT_TIME_MS"`
	MaxToolRequestTimes   int              `json:"MAX_TOOL_REQUEST_TIMES"`
	MaxSubtaskExecMinutes int              `json:"MAX_SUBTASK_EXECUTION_TIME_MINUTES"`
	MaxFileSizeBytes      int64            `json:"MAX_FILE_SIZE_BYTES,omitempty"`
	MaxTokensBudget       int64            `json:"MAX_TOKENS_BUDGET,omitempty"`
	BatchStrategy         string           `json:"BATCH_STRATEGY,omitempty"`
	BatchSize             int              `json:"BATCH_SIZE,omitempty"`
	DedupTask             *LlmConversation `json:"DEDUP_TASK,omitempty"`
	DedupMinComments      int              `json:"DEDUP_MIN_COMMENTS,omitempty"`
	ProjectSummaryTask    *LlmConversation `json:"PROJECT_SUMMARY_TASK,omitempty"`
}

// ReviewRounds returns the effective per-group round count, never below 1.
func (t Template) ReviewRounds() int {
	if t.MaxReviewRounds < 1 {
		return 1
	}
	return t.MaxReviewRounds
}

// PlanRequired reports whether a group warrants the plan phase based on its
// churn profile. Two thresholds cooperate: PlanModeLineThreshold gates on the
// largest single file (catches a big rewrite), while PlanModeGroupLineThreshold
// gates on the group's combined churn (catches several moderate files that
// together need structured guidance). The group threshold is deliberately larger
// to avoid making the plan phase unconditional.
func (t Template) PlanRequired(fileCount int, totalChanged, maxFileChanged int64) bool {
	if t.PlanModeLineThreshold <= 0 {
		return true
	}
	if maxFileChanged >= int64(t.PlanModeLineThreshold) {
		return true
	}
	if fileCount >= 2 && t.PlanModeGroupLineThreshold > 0 &&
		totalChanged >= int64(t.PlanModeGroupLineThreshold) {
		return true
	}
	return false
}

// CompletionTokenLimit returns the output cap for LLM requests. Runtime
// prompt-limit overrides must not silently expand the model's output budget.
func (t Template) CompletionTokenLimit() int {
	if t.MaxCompletionTokens > 0 {
		return t.MaxCompletionTokens
	}
	return t.MaxTokens
}

// CompletionTokenLimit is the scan-template counterpart of Template's method.
func (t ScanTemplate) CompletionTokenLimit() int {
	if t.MaxCompletionTokens > 0 {
		return t.MaxCompletionTokens
	}
	return t.MaxTokens
}

//go:embed task_template.json prompts/*
var templateFS embed.FS

//go:embed scan_template.json
var defaultScanTemplate []byte

type manifestMessage struct {
	Role       string `json:"role"`
	PromptFile string `json:"prompt_file"`
}

type manifestConversation struct {
	Messages []manifestMessage `json:"messages"`
}

type templateManifest struct {
	MainTask                   manifestConversation  `json:"MAIN_TASK"`
	PlanTask                   *manifestConversation `json:"PLAN_TASK,omitempty"`
	MemoryCompressionTask      manifestConversation  `json:"MEMORY_COMPRESSION_TASK"`
	MaxTokens                  int                   `json:"MAX_TOKENS"`
	MaxCompletionTokens        int                   `json:"MAX_COMPLETION_TOKENS"`
	MaxToolRequestTimes        int                   `json:"MAX_TOOL_REQUEST_TIMES"`
	PlanModeLineThreshold      int                   `json:"PLAN_MODE_LINE_THRESHOLD"`
	PlanModeGroupLineThreshold int                   `json:"PLAN_MODE_GROUP_LINE_THRESHOLD"`
	MaxReviewRounds            int                   `json:"MAX_REVIEW_ROUNDS"`
	ReLocationTask             *manifestConversation `json:"RE_LOCATION_TASK,omitempty"`
	ReviewFilterTask           *manifestConversation `json:"REVIEW_FILTER_TASK,omitempty"`
	GroupingTask               *manifestConversation `json:"GROUPING_TASK,omitempty"`
}

func resolveConversation(m manifestConversation) (LlmConversation, error) {
	var conv LlmConversation
	conv.Messages = make([]ChatMessage, len(m.Messages))
	for i, mm := range m.Messages {
		data, err := templateFS.ReadFile("prompts/" + mm.PromptFile)
		if err != nil {
			return LlmConversation{}, fmt.Errorf("read prompt file %q: %w", mm.PromptFile, err)
		}
		conv.Messages[i] = ChatMessage{
			Role:    mm.Role,
			Content: strings.TrimRight(string(data), "\r\n"),
		}
	}
	return conv, nil
}

func resolveOptionalConversation(m *manifestConversation, name string) (*LlmConversation, error) {
	if m == nil {
		return nil, nil
	}
	conv, err := resolveConversation(*m)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &conv, nil
}

// LoadDefault parses the embedded task_template.json and resolves prompt file references.
func LoadDefault() (*Template, error) {
	data, err := templateFS.ReadFile("task_template.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded task_template.json: %w", err)
	}
	var m templateManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal task_template manifest: %w", err)
	}

	var tpl Template
	tpl.MaxTokens = m.MaxTokens
	tpl.MaxCompletionTokens = m.MaxCompletionTokens
	tpl.MaxToolRequestTimes = m.MaxToolRequestTimes
	tpl.PlanModeLineThreshold = m.PlanModeLineThreshold
	tpl.PlanModeGroupLineThreshold = m.PlanModeGroupLineThreshold
	tpl.MaxReviewRounds = m.MaxReviewRounds

	if tpl.MainTask, err = resolveConversation(m.MainTask); err != nil {
		return nil, fmt.Errorf("MAIN_TASK: %w", err)
	}
	if tpl.PlanTask, err = resolveOptionalConversation(m.PlanTask, "PLAN_TASK"); err != nil {
		return nil, err
	}
	if tpl.MemoryCompressionTask, err = resolveConversation(m.MemoryCompressionTask); err != nil {
		return nil, fmt.Errorf("MEMORY_COMPRESSION_TASK: %w", err)
	}
	if tpl.ReLocationTask, err = resolveOptionalConversation(m.ReLocationTask, "RE_LOCATION_TASK"); err != nil {
		return nil, err
	}
	if tpl.ReviewFilterTask, err = resolveOptionalConversation(m.ReviewFilterTask, "REVIEW_FILTER_TASK"); err != nil {
		return nil, err
	}
	if tpl.GroupingTask, err = resolveOptionalConversation(m.GroupingTask, "GROUPING_TASK"); err != nil {
		return nil, err
	}
	return &tpl, nil
}

// LoadScanDefault parses the embedded scan_template.json.
func LoadScanDefault() (*ScanTemplate, error) {
	var tpl ScanTemplate
	if err := json.Unmarshal(defaultScanTemplate, &tpl); err != nil {
		return nil, fmt.Errorf("unmarshal default scan template: %w", err)
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
// across MAIN_TASK, PLAN_TASK (if set), and MEMORY_COMPRESSION_TASK.
func (t *Template) ApplyLanguage(lang string) {
	instruction := "\n\nAlways respond in " + resolveLang(lang) + "."
	applyLanguage(&t.MainTask, instruction)
	if t.PlanTask != nil {
		applyLanguage(t.PlanTask, instruction)
	}
	applyLanguage(&t.MemoryCompressionTask, instruction)
}

// ApplyLanguage injects a language directive into all system-role messages
// of the scan template (MAIN_TASK, PLAN_TASK if set, DEDUP_TASK if set,
// and MEMORY_COMPRESSION_TASK).
func (t *ScanTemplate) ApplyLanguage(lang string) {
	instruction := "\n\nAlways respond in " + resolveLang(lang) + "."
	applyLanguage(&t.MainTask, instruction)
	if t.PlanTask != nil {
		applyLanguage(t.PlanTask, instruction)
	}
	if t.DedupTask != nil {
		applyLanguage(t.DedupTask, instruction)
	}
	if t.ProjectSummaryTask != nil {
		applyLanguage(t.ProjectSummaryTask, instruction)
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
	if t.MaxReviewRounds < 0 {
		return fmt.Errorf("max_review_rounds must not be negative")
	}
	if len(t.MainTask.Messages) == 0 {
		return fmt.Errorf("main_task.messages must not be empty")
	}
	return nil
}

// Validate checks that a ScanTemplate has the minimum fields populated.
func (t *ScanTemplate) Validate() error {
	if t.MaxTokens <= 0 {
		return fmt.Errorf("scan: max_tokens must be positive")
	}
	if t.MaxToolRequestTimes <= 0 {
		return fmt.Errorf("scan: max_tool_request_times must be positive")
	}
	if len(t.MainTask.Messages) == 0 {
		return fmt.Errorf("scan: main_task.messages must not be empty")
	}
	return nil
}

// LlmConversation is a preset prompt with settings.
type LlmConversation struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
