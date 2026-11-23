// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package template loads and validates task prompt templates for the code review agent.
package template

import (
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
)

// Template holds the native agent task template configuration.
// Scan-mode fields live in ScanTemplate, not here.
type Template struct {
	MainTask                    LlmConversation  `json:"MAIN_TASK"`
	PlanTask                    *LlmConversation `json:"PLAN_TASK,omitempty"`
	MemoryCompressionTask       LlmConversation  `json:"MEMORY_COMPRESSION_TASK"`
	MaxTokens                   int              `json:"MAX_TOKENS"`
	MaxCompletionTokens         int              `json:"MAX_COMPLETION_TOKENS"`
	MaxToolRequestTimes         int              `json:"MAX_TOOL_REQUEST_TIMES"`
	PlanModeLineThreshold       int              `json:"PLAN_MODE_LINE_THRESHOLD"`
	PlanModeGroupLineThreshold  int              `json:"PLAN_MODE_GROUP_LINE_THRESHOLD"`
	GroupingMinFiles            int              `json:"GROUPING_MIN_FILES"`
	GroupingBundleLineThreshold int              `json:"GROUPING_BUNDLE_LINE_THRESHOLD"`
	MaxReviewRounds             int              `json:"MAX_REVIEW_ROUNDS"`
	ReLocationTask              *LlmConversation `json:"RE_LOCATION_TASK,omitempty"`
	ReviewFilterTask            *LlmConversation `json:"REVIEW_FILTER_TASK,omitempty"`
	GroupingTask                *LlmConversation `json:"GROUPING_TASK,omitempty"`
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
	MaxToolRequestTimes   int              `json:"MAX_TOOL_REQUEST_TIMES"`
	MaxFileSizeBytes      int64            `json:"MAX_FILE_SIZE_BYTES,omitempty"`
	PlanModeLineThreshold int              `json:"PLAN_MODE_LINE_THRESHOLD,omitempty"`
	MaxTokensBudget       int64            `json:"MAX_TOKENS_BUDGET,omitempty"`
	BatchStrategy         string           `json:"BATCH_STRATEGY,omitempty"`
	BatchSize             int              `json:"BATCH_SIZE,omitempty"`
	DedupTask             *LlmConversation `json:"DEDUP_TASK,omitempty"`
	DedupMinComments      int              `json:"DEDUP_MIN_COMMENTS,omitempty"`
	ProjectSummaryTask    *LlmConversation `json:"PROJECT_SUMMARY_TASK,omitempty"`
	// NoPlanGuidance overrides the "no pre-scan plan" sentinel substituted
	// for {{plan_guidance}} when planning is disabled or fails. Empty keeps
	// the built-in whole-file sentinel, so existing files stay valid.
	NoPlanGuidance string `json:"NO_PLAN_GUIDANCE,omitempty"`
}

// Canonical placeholder names the scan agent substitutes per task. The
// renderers in internal/scan iterate these same lists and Validate rejects
// any {{token}} outside them, so renderer and validator cannot drift apart.
// MEMORY_COMPRESSION_TASK and RE_LOCATION_TASK are consumed by shared paths
// and intentionally have no list here.
var (
	// ScanMainTokens fills MAIN_TASK prompts (see Agent.renderMessages).
	ScanMainTokens = []string{"plan_guidance", "current_system_date_time", "current_file_path", "system_rule", "change_files", "file_content", "requirement_background"}
	// ScanPlanTokens fills PLAN_TASK prompts (see Agent.maybeRunPlan).
	ScanPlanTokens = []string{"current_system_date_time", "current_file_path", "system_rule", "file_content"}
	// ScanSummaryTokens fills PROJECT_SUMMARY_TASK prompts.
	ScanSummaryTokens = []string{"comment_count", "file_count", "all_comments"}
	// ScanDedupTokens fills DEDUP_TASK prompts.
	ScanDedupTokens = []string{"batch_comments"}
)

// scanPlaceholderPattern matches {{token}} placeholders; single-brace
// spellings such as RE_LOCATION's {diff} are literal text, not placeholders.
var scanPlaceholderPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// maxScanTemplateFileBytes caps --scan-template inputs at 1 MiB, the same
// ceiling --background-file enforces.
const maxScanTemplateFileBytes = 1 << 20

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

// GroupingStrategy is how a change set gets partitioned into review groups.
type GroupingStrategy int

const (
	// GroupingViaLLM runs the GROUPING_TASK call to partition files semantically.
	GroupingViaLLM GroupingStrategy = iota
	// GroupingBundleAll puts every file in one group, with no LLM call.
	GroupingBundleAll
	// GroupingPerFile gives every file its own group, with no LLM call.
	GroupingPerFile
)

// String names the strategy for logs and telemetry attributes. An unhandled
// value reads as "unknown" rather than falling through to a real strategy name,
// so a future addition that forgets its case shows up as an anomaly in
// telemetry instead of silently inflating one of the existing buckets.
func (s GroupingStrategy) String() string {
	switch s {
	case GroupingBundleAll:
		return "bundle_all"
	case GroupingPerFile:
		return "per_file"
	case GroupingViaLLM:
		return "llm"
	default:
		return "unknown"
	}
}

// GroupingPlan picks how to partition a change set. Two thresholds cooperate,
// and they answer different questions. GroupingMinFiles asks whether the
// partition is worth computing at all: below it the space of sensible
// partitions is tiny, so the LLM call buys no information. Only then does
// GroupingBundleLineThreshold ask whether the files can share one review —
// every one of a bundled group's MAX_REVIEW_ROUNDS rounds has to cover the
// whole set's churn, so past a ceiling a single round's attention is spread too
// thin and each file is better off in a subtask of its own. File count is
// deliberately not re-examined at the second step: once the count is low, churn
// alone decides.
//
// Only totalChanged gates the bundle, with no companion per-file threshold:
// the largest single file's churn never exceeds the total, so a same-valued
// per-file gate could not reject anything the total already admits.
//
// Either threshold at or below 0 disables its own step: GroupingMinFiles keeps
// grouping unconditional, GroupingBundleLineThreshold never bundles.
func (t Template) GroupingPlan(fileCount int, totalChanged int64) GroupingStrategy {
	if t.GroupingMinFiles <= 0 || fileCount >= t.GroupingMinFiles {
		return GroupingViaLLM
	}
	if t.GroupingBundleLineThreshold > 0 && totalChanged < int64(t.GroupingBundleLineThreshold) {
		return GroupingBundleAll
	}
	return GroupingPerFile
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

//go:embed task_template.json behavior_template.json prompts/*
var templateFS embed.FS

//go:embed scan_template.json
var defaultScanTemplate []byte

type manifestMessage struct {
	Role       string `json:"role"`
	PromptFile string `json:"prompt_file"`
}

type manifestConversation struct {
	Messages       []manifestMessage `json:"messages"`
	Tools          []string          `json:"tools,omitempty"`
	ToolReferences []string          `json:"tool_references,omitempty"`
}

type templateManifest struct {
	MainTask                    manifestConversation  `json:"MAIN_TASK"`
	PlanTask                    *manifestConversation `json:"PLAN_TASK,omitempty"`
	MemoryCompressionTask       manifestConversation  `json:"MEMORY_COMPRESSION_TASK"`
	MaxTokens                   int                   `json:"MAX_TOKENS"`
	MaxCompletionTokens         int                   `json:"MAX_COMPLETION_TOKENS"`
	MaxToolRequestTimes         int                   `json:"MAX_TOOL_REQUEST_TIMES"`
	PlanModeLineThreshold       int                   `json:"PLAN_MODE_LINE_THRESHOLD"`
	PlanModeGroupLineThreshold  int                   `json:"PLAN_MODE_GROUP_LINE_THRESHOLD"`
	GroupingMinFiles            int                   `json:"GROUPING_MIN_FILES"`
	GroupingBundleLineThreshold int                   `json:"GROUPING_BUNDLE_LINE_THRESHOLD"`
	MaxReviewRounds             int                   `json:"MAX_REVIEW_ROUNDS"`
	ReLocationTask              *manifestConversation `json:"RE_LOCATION_TASK,omitempty"`
	ReviewFilterTask            *manifestConversation `json:"REVIEW_FILTER_TASK,omitempty"`
	GroupingTask                *manifestConversation `json:"GROUPING_TASK,omitempty"`
}

func resolveConversation(m manifestConversation) (LlmConversation, error) {
	conv := LlmConversation{Tools: m.Tools, ToolReferences: m.ToolReferences}
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
	tpl.GroupingMinFiles = m.GroupingMinFiles
	tpl.GroupingBundleLineThreshold = m.GroupingBundleLineThreshold
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

// LoadScanFile parses a scan template override file into a ScanTemplate and
// validates it, so file overrides share the embedded default's enforcement
// point. Every error names the path.
func LoadScanFile(path string) (*ScanTemplate, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read scan template file %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("scan template file %q is a directory, not a file", path)
	}
	if info.Size() > maxScanTemplateFileBytes {
		return nil, fmt.Errorf("scan template file %q is %d bytes, exceeding the maximum of %d bytes",
			path, info.Size(), maxScanTemplateFileBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scan template file %q: %w", path, err)
	}
	var tpl ScanTemplate
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return nil, fmt.Errorf("unmarshal scan template file %q: %w", path, err)
	}
	if err := tpl.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scan template file %q: %w", path, err)
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
	return validateToolBindings(map[string]*LlmConversation{
		"MAIN_TASK":               &t.MainTask,
		"PLAN_TASK":               t.PlanTask,
		"MEMORY_COMPRESSION_TASK": &t.MemoryCompressionTask,
		"RE_LOCATION_TASK":        t.ReLocationTask,
		"REVIEW_FILTER_TASK":      t.ReviewFilterTask,
		"GROUPING_TASK":           t.GroupingTask,
	}, "PLAN_TASK")
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
	if err := validateToolBindings(map[string]*LlmConversation{
		"MAIN_TASK":               &t.MainTask,
		"PLAN_TASK":               t.PlanTask,
		"MEMORY_COMPRESSION_TASK": &t.MemoryCompressionTask,
		"RE_LOCATION_TASK":        t.ReLocationTask,
		"DEDUP_TASK":              t.DedupTask,
		"PROJECT_SUMMARY_TASK":    t.ProjectSummaryTask,
	}, ""); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if err := validateScanPlaceholders(t); err != nil {
		return err
	}
	return nil
}

// validateScanPlaceholders enforces the scan prompt contract: at least one
// MAIN_TASK user-role message must carry {{file_content}} (the file under
// review must reach the model), and every {{token}} in the four
// scan-rendered tasks must belong to that task's canonical known set — an
// override introducing a token the renderer never fills would otherwise
// leak a literal {{...}} into the prompt.
func validateScanPlaceholders(t *ScanTemplate) error {
	found := false
	for _, m := range t.MainTask.Messages {
		if m.Role == "user" && strings.Contains(m.Content, "{{file_content}}") {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("scan: MAIN_TASK must include {{file_content}} in at least one user-role message")
	}
	tasks := []struct {
		name  string
		conv  *LlmConversation
		known []string
	}{
		{"MAIN_TASK", &t.MainTask, ScanMainTokens},
		{"PLAN_TASK", t.PlanTask, ScanPlanTokens},
		{"DEDUP_TASK", t.DedupTask, ScanDedupTokens},
		{"PROJECT_SUMMARY_TASK", t.ProjectSummaryTask, ScanSummaryTokens},
	}
	for _, task := range tasks {
		if task.conv == nil {
			continue
		}
		for i, m := range task.conv.Messages {
			for _, match := range scanPlaceholderPattern.FindAllStringSubmatch(m.Content, -1) {
				if !slices.Contains(task.known, match[1]) {
					return fmt.Errorf("scan: %s message %d: unknown placeholder {{%s}}", task.name, i, match[1])
				}
			}
		}
	}
	return nil
}

func validateToolBindings(conversations map[string]*LlmConversation, referenceTask string) error {
	for _, name := range slices.Sorted(maps.Keys(conversations)) {
		conversation := conversations[name]
		if conversation == nil {
			continue
		}
		if name != "MAIN_TASK" && len(conversation.Tools) > 0 {
			return fmt.Errorf("%s.tools is not supported by this task", name)
		}
		if name != referenceTask && len(conversation.ToolReferences) > 0 {
			return fmt.Errorf("%s.tool_references is not supported by this task", name)
		}
	}
	return nil
}

// LlmConversation is a preset prompt with settings.
type LlmConversation struct {
	Messages       []ChatMessage `json:"messages"`
	Tools          []string      `json:"tools,omitempty"`
	ToolReferences []string      `json:"tool_references,omitempty"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
