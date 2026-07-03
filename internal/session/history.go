// Package session provides a session history mechanism for collecting conversation
// records during code review task execution. It organizes records by file path
// and request type (plan_task, main_task, memory_compression_task).
package session

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/model"
)

// TaskType identifies the kind of LLM request within a file subtask.
type TaskType string

const (
	PlanTask              TaskType = "plan_task"
	MainTask              TaskType = "main_task"
	MemoryCompressionTask TaskType = "memory_compression_task"
	ReLocationTask        TaskType = "re_location_task"
	ReviewFilterTask      TaskType = "review_filter_task"
)

const (
	ReviewModeWorkspace = "workspace"
	ReviewModeRange     = "range"
	ReviewModeCommit    = "commit"
	ReviewModeFullScan  = "full_scan"
)

// SessionHistory is the top-level container for an entire CR run.
// It is safe for concurrent use by multiple goroutines.
type SessionHistory struct {
	mu           sync.Mutex
	SessionID    string
	RepoDir      string
	GitBranch    string
	Model        string
	ReviewMode   string
	DiffFrom     string
	DiffTo       string
	DiffCommit   string
	ResumedFrom  string
	StartTime    time.Time
	EndTime      time.Time
	persist      *jsonlWriter
	FileSessions map[string]*FileSession
	llmFailures  int64
}

// FileSession represents the conversation records for a single file subtask.
type FileSession struct {
	mu          sync.Mutex
	FilePath    string
	TaskRecords map[TaskType][]*TaskRecord
	session     *SessionHistory // back-reference for JSONL persistence
}

// TaskRecord captures a single LLM request-response cycle within a file subtask.
type TaskRecord struct {
	Type            TaskType
	RequestNo       int           // sequential number within this task type
	RequestMessages []llm.Message // messages sent to LLM
	Response        *ResponseRecord
	ToolResults     []ToolResultRecord
	Duration        time.Duration
	Error           string
	fileSession     *FileSession // back-reference for JSONL persistence
}

// TokenUsage holds token usage for a single LLM request/response cycle.
// Uses actual token counts from the API response when available,
// falling back to local estimation via tiktoken.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// ResponseRecord holds the parsed LLM response.
type ResponseRecord struct {
	Content   string
	ToolCalls []llm.ToolCall
	Model     string
	Usage     *TokenUsage
}

// ToolResultRecord records the result of a tool call executed after the LLM response.
type ToolResultRecord struct {
	ToolName  string
	Arguments string
	Result    string
}

// SessionOptions holds optional metadata for a new session.
type SessionOptions struct {
	ReviewMode  string
	DiffFrom    string
	DiffTo      string
	DiffCommit  string
	ResumedFrom string
}

// New creates a new SessionHistory with the given repo directory.
func New(repoDir, gitBranch, model string, opts SessionOptions) *SessionHistory {
	sessionID := generateUUID()
	sh := &SessionHistory{
		SessionID:    sessionID,
		RepoDir:      repoDir,
		GitBranch:    gitBranch,
		Model:        model,
		ReviewMode:   opts.ReviewMode,
		DiffFrom:     opts.DiffFrom,
		DiffTo:       opts.DiffTo,
		DiffCommit:   opts.DiffCommit,
		ResumedFrom:  opts.ResumedFrom,
		StartTime:    time.Now(),
		FileSessions: make(map[string]*FileSession),
	}

	p, err := newJSONLWriter(sessionID, repoDir, gitBranch, model, opts)
	if err != nil {
		fmt.Printf("[ocr session] warning: failed to create session writer: %v\n", err)
	} else {
		sh.persist = p
		p.WriteSessionStart(sh.StartTime)
	}

	return sh
}

// GetOrCreateFileSession returns the FileSession for the given file path,
// creating one if it doesn't exist yet.
func (sh *SessionHistory) GetOrCreateFileSession(filePath string) *FileSession {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	fs, ok := sh.FileSessions[filePath]
	if !ok {
		fs = &FileSession{
			FilePath:    filePath,
			TaskRecords: make(map[TaskType][]*TaskRecord),
			session:     sh,
		}
		sh.FileSessions[filePath] = fs
	}
	return fs
}

// RecordReviewItemDone persists the file-level checkpoint used by resume.
func (sh *SessionHistory) RecordReviewItemDone(filePath, oldPath, newPath, fingerprint string, comments []model.LlmComment) {
	if sh == nil {
		return
	}
	if filePath == "" {
		filePath = newPath
	}
	if filePath != "" {
		sh.GetOrCreateFileSession(filePath)
	}
	if p := sh.persist; p != nil {
		p.WriteReviewItemDone(filePath, oldPath, newPath, fingerprint, comments)
	}
}

// RecordReviewItemReused records that this run reused a checkpoint from another session.
func (sh *SessionHistory) RecordReviewItemReused(filePath, oldPath, newPath, fingerprint, sourceSessionID string, comments []model.LlmComment) {
	if sh == nil {
		return
	}
	if filePath == "" {
		filePath = newPath
	}
	if filePath != "" {
		sh.GetOrCreateFileSession(filePath)
	}
	if p := sh.persist; p != nil {
		p.WriteReviewItemReused(filePath, oldPath, newPath, fingerprint, sourceSessionID, comments)
	}
}

// RecordReviewItemFailed persists an incomplete file-level checkpoint.
func (sh *SessionHistory) RecordReviewItemFailed(filePath, oldPath, newPath, fingerprint, errorMsg string) {
	if sh == nil {
		return
	}
	if filePath == "" {
		filePath = newPath
	}
	if filePath != "" {
		sh.GetOrCreateFileSession(filePath)
	}
	if p := sh.persist; p != nil {
		p.WriteReviewItemFailed(filePath, oldPath, newPath, fingerprint, errorMsg)
	}
}

// Finalize marks the session as complete, sets the end time, and persists
// the final summary record.
func (sh *SessionHistory) Finalize() {
	sh.mu.Lock()
	sh.EndTime = time.Now()
	p := sh.persist
	duration := sh.EndTime.Sub(sh.StartTime)
	filesReviewed := make([]string, 0, len(sh.FileSessions))
	for fp := range sh.FileSessions {
		filesReviewed = append(filesReviewed, fp)
	}
	failures := atomic.LoadInt64(&sh.llmFailures)
	sh.mu.Unlock()

	if p != nil {
		p.WriteSessionEnd(duration, filesReviewed, failures)
	}
}

// AppendTaskRecord adds a new task record to the file session for the given
// file path and task type. It auto-assigns the RequestNo based on existing records
// and writes an llm_request record to the JSONL stream.
func (fs *FileSession) AppendTaskRecord(taskType TaskType, messages []llm.Message) *TaskRecord {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	rec := &TaskRecord{
		Type:            taskType,
		RequestNo:       len(fs.TaskRecords[taskType]) + 1,
		RequestMessages: copyMessages(messages),
		fileSession:     fs,
	}
	fs.TaskRecords[taskType] = append(fs.TaskRecords[taskType], rec)

	if p := fs.session.persist; p != nil {
		p.WriteLLMRequest(fs.FilePath, taskType, rec.RequestNo, copyMessagesForJSON(messages))
	}

	return rec
}

// copyMessages returns a deep copy of a messages slice so that future mutations
// don't corrupt stored records.
func copyMessages(msgs []llm.Message) []llm.Message {
	cp := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		cp[i] = llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  append([]llm.ToolCall(nil), m.ToolCalls...),
		}
	}
	return cp
}

// copyMessagesForJSON produces a JSON-friendly slice for persistence.
func copyMessagesForJSON(msgs []llm.Message) any {
	type msg struct {
		Role       string `json:"role"`
		Content    any    `json:"content"`
		ToolCallID string `json:"tool_call_id,omitempty"`
	}
	out := make([]msg, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, msg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		})
	}
	return out
}

// SetResponse records the LLM response in the most recent TaskRecord of the given type.
// It uses actual token usage from the API response when available, falling back to
// local estimation via tiktoken, and writes an llm_response record to the JSONL stream.
func (tr *TaskRecord) SetResponse(resp *llm.ChatResponse, duration time.Duration) {
	if resp == nil || len(resp.Choices) == 0 {
		tr.SetError(fmt.Errorf("empty response"), duration)
		return
	}
	choice := resp.Choices[0]
	content := ""
	if choice.Message.Content != nil {
		content = *choice.Message.Content
	}

	var promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int
	if resp.Usage != nil {
		promptTokens = int(resp.Usage.PromptTokens)
		completionTokens = int(resp.Usage.CompletionTokens)
		cacheReadTokens = int(resp.Usage.CacheReadTokens)
		cacheWriteTokens = int(resp.Usage.CacheWriteTokens)
	} else {
		for _, m := range tr.RequestMessages {
			promptTokens += llm.CountTokens(m.ExtractText())
		}
		completionTokens = llm.CountTokens(content)
	}

	usage := &TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		CacheReadTokens:  cacheReadTokens,
		CacheWriteTokens: cacheWriteTokens,
	}

	tr.Response = &ResponseRecord{
		Content:   content,
		ToolCalls: choice.Message.ToolCalls,
		Model:     resp.Model,
		Usage:     usage,
	}
	tr.Duration = duration

	if fs := tr.fileSession; fs != nil {
		if p := fs.session.persist; p != nil {
			toolCallsJSON := make([]map[string]any, 0, len(choice.Message.ToolCalls))
			for _, tc := range choice.Message.ToolCalls {
				toolCallsJSON = append(toolCallsJSON, map[string]any{
					"id":        tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
			p.WriteLLMResponse(fs.FilePath, tr.Type, content, toolCallsJSON, resp.Model, *usage, duration)
		}
	}
}

// SetError records an error for this task record, writes an llm_error entry to
// the JSONL stream, and increments the session-level LLM failure counter.
func (tr *TaskRecord) SetError(err error, duration time.Duration) {
	tr.Error = err.Error()
	tr.Duration = duration

	if fs := tr.fileSession; fs != nil {
		if p := fs.session.persist; p != nil {
			p.WriteLLMError(fs.FilePath, tr.Type, tr.RequestNo, err.Error(), duration)
		}
		atomic.AddInt64(&fs.session.llmFailures, 1)
	}
}

// LLMFailures returns the total number of LLM request failures recorded during this session.
func (sh *SessionHistory) LLMFailures() int64 {
	return atomic.LoadInt64(&sh.llmFailures)
}

// AddToolResult appends a tool call result to this task record and writes a
// tool_call record to the JSONL stream.
func (tr *TaskRecord) AddToolResult(toolName, arguments, result string) {
	tr.ToolResults = append(tr.ToolResults, ToolResultRecord{
		ToolName:  toolName,
		Arguments: arguments,
		Result:    result,
	})

	if fs := tr.fileSession; fs != nil {
		if p := fs.session.persist; p != nil {
			p.WriteToolCall(fs.FilePath, tr.Type, toolName, arguments, result, true, 0)
		}
	}
}
