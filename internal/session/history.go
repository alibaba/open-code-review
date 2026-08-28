// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package session provides a session history mechanism for collecting conversation
// records during code review task execution. It organizes records by file path
// and request type (plan_task, main_task, memory_compression_task).
package session

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

// TaskType identifies the kind of LLM request within a file subtask.
type TaskType string

const (
	PlanTask              TaskType = "plan_task"
	MainTask              TaskType = "main_task"
	MemoryCompressionTask TaskType = "memory_compression_task"
	ReLocationTask        TaskType = "re_location_task"
	ReviewFilterTask      TaskType = "review_filter_task"
	GroupingTask          TaskType = "grouping_task"
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
	ScanPaths    []string
	ResumedFrom  string
	StartTime    time.Time
	EndTime      time.Time
	persist      *jsonlWriter
	FileSessions map[string]*FileSession
	llmFailures  int64

	// manifest is the run's coverage accumulator, sharing the session ID as its
	// run_id. It is only created when SessionOptions.Operation is non-empty (the
	// review path opts in; scan stays legacy with a nil builder). It is nil for
	// legacy/scan sessions, so all access must be nil-safe.
	manifest *ManifestBuilder
	// finalManifest is the frozen manifest handed back by the agent before
	// Finalize, embedded into session_end and exposed to the CLI. Nil for
	// legacy/scan runs.
	finalManifest *RunManifest
	// persistInitErr records a failure to create the JSONL writer. The run may
	// still produce a manifest for CLI output, but Finalize must report that the
	// persisted-session outlet was never available.
	persistInitErr error
	// finalizeOnce ensures session_end is written exactly once even if several
	// run paths (error, skip, normal) — possibly concurrently — reach Finalize.
	finalizeOnce sync.Once
	// finalizeErr caches the result of that single write attempt so every caller,
	// including any retry after the first, observes the same delivery outcome
	// rather than a later call falsely reporting success. Read only after
	// finalizeOnce.Do returns, which establishes the happens-before.
	finalizeErr error
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
	Content          string
	ToolCalls        []llm.ToolCall
	Model            string
	Usage            *TokenUsage
	ReasoningContent string
	Native           llm.NativeTurn
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
	ScanPaths   []string
	ResumedFrom string

	// Operation opts this session into a run manifest. When non-empty (e.g.
	// "review") New creates a ManifestBuilder with this operation and the session
	// ID as its run_id. Empty (the scan/legacy default) leaves the builder nil.
	Operation string
}

// ResumeInfo summarizes file-level reuse for a resumed run.
type ResumeInfo struct {
	ResumedFrom   string `json:"resumed_from"`
	ReusedFiles   int64  `json:"reused_files"`
	RerunFiles    int64  `json:"rerun_files"`
	PreviousModel string `json:"previous_model,omitempty"`
	CurrentModel  string `json:"current_model,omitempty"`
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
		ScanPaths:    append([]string(nil), opts.ScanPaths...),
		ResumedFrom:  opts.ResumedFrom,
		StartTime:    time.Now(),
		FileSessions: make(map[string]*FileSession),
	}

	p, err := newJSONLWriter(sessionID, repoDir, gitBranch, model, opts)
	if err != nil {
		// Do not print here: New runs before JSON output is silenced, so writing a
		// warning to stdout would corrupt the command's machine-readable output.
		// Finalize returns this cached delivery error to the command layer.
		sh.persistInitErr = fmt.Errorf("create session writer: %w", err)
	} else {
		sh.persist = p
		p.WriteSessionStart(sh.StartTime)
	}

	if opts.Operation != "" {
		sh.manifest = NewManifestBuilder(sessionID, opts.Operation)
	}

	return sh
}

// Manifest returns the run's coverage builder, or nil for legacy/scan sessions
// that did not opt in via SessionOptions.Operation. Callers must be nil-safe.
func (sh *SessionHistory) Manifest() *ManifestBuilder {
	if sh == nil {
		return nil
	}
	return sh.manifest
}

// SetFinalManifest stores the frozen manifest the agent produced. It is embedded
// into session_end by Finalize and returned to the CLI via FinalManifest. Passing
// nil (legacy/scan, or a construction failure) leaves session_end in legacy form.
func (sh *SessionHistory) SetFinalManifest(m *RunManifest) {
	if sh == nil {
		return
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.finalManifest = m
}

// FinalManifest returns the frozen manifest stored for this run, or nil when the
// run produced none (legacy/scan, or a construction failure).
func (sh *SessionHistory) FinalManifest() *RunManifest {
	if sh == nil {
		return nil
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.finalManifest == nil {
		return nil
	}
	m := sh.finalManifest.cloned()
	return &m
}

// HasPersistence reports whether this session has a JSONL writer. A false value
// means no resumable session file exists, even though the run still has its own
// in-memory ID and may produce a CLI manifest.
func (sh *SessionHistory) HasPersistence() bool {
	if sh == nil {
		return false
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.persist != nil
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

// RecordResumeLineage persists the run's single resume_lineage event. A nil
// lineage is a non-resumed run and writes nothing.
func (sh *SessionHistory) RecordResumeLineage(l *ResumeLineage) {
	if sh == nil || l == nil {
		return
	}
	if p := sh.persist; p != nil {
		p.WriteResumeLineage(l)
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

// Finalize marks the session as complete, sets the end time, and persists the
// final summary record. When a frozen manifest was stored via SetFinalManifest
// it is embedded into session_end as run_manifest, which is the last physical
// record of the JSONL stream. It is idempotent — only the first call writes, and
// that single attempt's outcome is cached so every later call replays the same
// result instead of a retry falsely reporting success. The several run paths
// (normal, skipped, all-failed, run-level failure), even concurrently, can all
// call it safely. A persistence error is returned as a delivery error rather
// than swallowed; the frozen manifest is never rewritten because of it.
func (sh *SessionHistory) Finalize() error {
	sh.finalizeOnce.Do(func() {
		sh.mu.Lock()
		sh.EndTime = time.Now()
		p := sh.persist
		persistInitErr := sh.persistInitErr
		manifest := sh.finalManifest
		duration := sh.EndTime.Sub(sh.StartTime)
		filesReviewed := make([]string, 0, len(sh.FileSessions))
		if manifest != nil && manifest.SchemaVersion == ManifestSchemaVersion {
			filesReviewed = make([]string, 0, len(manifest.Coverage.Selected))
			for _, item := range manifest.Coverage.Selected {
				filesReviewed = append(filesReviewed, item.Path)
			}
		} else {
			// Legacy and scan sessions have no manifest, so retain the historical
			// all-FileSessions behavior for their summary record.
			for fp := range sh.FileSessions {
				filesReviewed = append(filesReviewed, fp)
			}
		}
		failures := atomic.LoadInt64(&sh.llmFailures)
		sh.mu.Unlock()

		if persistInitErr != nil {
			sh.finalizeErr = persistInitErr
			return
		}

		// The single write attempt happens outside the lock (disk I/O); its
		// result is cached in finalizeErr. sync.Once guarantees every other
		// caller blocks until this completes, then reads the same finalizeErr.
		if p != nil {
			sh.finalizeErr = p.WriteSessionEnd(duration, filesReviewed, failures, manifest)
		}
	})
	return sh.finalizeErr
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
			Role:             m.Role,
			Content:          m.Content,
			ToolCallID:       m.ToolCallID,
			ToolCalls:        append([]llm.ToolCall(nil), m.ToolCalls...),
			Native:           m.Native,
			ReasoningContent: m.ReasoningContent,
		}
	}
	return cp
}

// copyMessagesForJSON produces a JSON-friendly slice for persistence.
func copyMessagesForJSON(msgs []llm.Message) any {
	type msg struct {
		Role          string         `json:"role"`
		Content       any            `json:"content"`
		ToolCallID    string         `json:"tool_call_id,omitempty"`
		ToolCalls     []llm.ToolCall `json:"tool_calls,omitempty"`
		NativePayload any            `json:"native_payload,omitempty"`
	}
	out := make([]msg, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, msg{
			Role:          m.Role,
			Content:       m.Content,
			ToolCallID:    m.ToolCallID,
			ToolCalls:     m.ToolCalls,
			NativePayload: nativeTurnForJSON(m.Native),
		})
	}
	return out
}

func nativeTurnForJSON(n llm.NativeTurn) any {
	if n.Payload == nil {
		return nil
	}
	return map[string]any{"family": n.Family, "payload": n.Payload}
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
		Content:          content,
		ToolCalls:        choice.Message.ToolCalls,
		Model:            resp.Model,
		Usage:            usage,
		ReasoningContent: choice.Message.ReasoningContent,
		Native:           resp.Native(),
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
			p.WriteLLMResponse(fs.FilePath, tr.Type, content, choice.Message.ReasoningContent, toolCallsJSON, resp.Model, *usage, duration, nativeTurnForJSON(tr.Response.Native))
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
