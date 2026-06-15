// Package template loads and validates task prompt templates for the code review agent.
package template

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Template holds the diff-review task template configuration loaded from
// task_template.json. Scan-mode fields live in ScanTemplate, not here.
type Template struct {
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
	ToolRequestWaitTimeMs int              `json:"TOOL_REQUEST_WAIT_TIME_MS"`
	MaxToolRequestTimes   int              `json:"MAX_TOOL_REQUEST_TIMES"`
	MaxSubtaskExecMinutes int              `json:"MAX_SUBTASK_EXECUTION_TIME_MINUTES"`
	// MaxFileSizeBytes is the per-file size cap (bytes) for enumeration.
	// Defaults to 2 MiB when ≤ 0.
	MaxFileSizeBytes int64 `json:"MAX_FILE_SIZE_BYTES,omitempty"`
	// MaxTokensBudget caps total token usage (input+output) for one scan.
	// Dispatch stops scheduling new batches once exceeded. 0 = unlimited.
	MaxTokensBudget int64 `json:"MAX_TOKENS_BUDGET,omitempty"`
	// BatchStrategy controls how files are grouped before per-batch dispatch.
	// Supported values: "none" (each file is its own batch — v1 behavior),
	// "by-language" (group by file extension), "by-directory" (group by
	// first-level subdirectory under repoDir). Empty / unknown → "none".
	BatchStrategy string `json:"BATCH_STRATEGY,omitempty"`
	// BatchSize caps the number of files in a single batch. ≤ 0 → no cap.
	// When a natural batch (e.g. all .go files) exceeds the cap it is sliced
	// into BatchSize-sized chunks while preserving the strategy ordering.
	BatchSize int `json:"BATCH_SIZE,omitempty"`
	// DedupTask, when non-nil, is invoked at the end of each batch to merge
	// near-duplicate comments produced within that batch.
	DedupTask *LlmConversation `json:"DEDUP_TASK,omitempty"`
	// DedupMinComments is the minimum number of comments a batch must
	// produce before DedupTask is invoked. ≤ 0 → 2 (don't dedup a single
	// comment; useless cost).
	DedupMinComments int `json:"DEDUP_MIN_COMMENTS,omitempty"`
	// ProjectSummaryTask, when non-nil, runs once after all batches finish
	// and produces a markdown summary of cross-file patterns / hotspots.
	ProjectSummaryTask *LlmConversation `json:"PROJECT_SUMMARY_TASK,omitempty"`
}

//go:embed task_template.json
var defaultTemplate []byte

//go:embed scan_template.json
var defaultScanTemplate []byte

// LoadDefault parses the embedded task_template.json.
func LoadDefault() (*Template, error) {
	var tpl Template
	if err := json.Unmarshal(defaultTemplate, &tpl); err != nil {
		return nil, fmt.Errorf("unmarshal default template: %w", err)
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
