package reviewbackend

import (
	"context"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
)

// Kind identifies how review tasks are executed.
type Kind string

const (
	KindChatCompletions Kind = "chat_completions"
	KindCursorAgent     Kind = "cursor_agent"
)

// CompleteRequest is a text-only completion (plan, filter, compression, llm test).
type CompleteRequest struct {
	Model     string
	Messages  []llm.Message
	MaxTokens int
}

// CompleteResponse is the result of a text-only completion.
type CompleteResponse struct {
	Content string
	Model   string
	Usage   *llm.UsageInfo
	Raw     *llm.ChatResponse
}

// ReviewFileRequest drives the main per-file review task with tools.
type ReviewFileRequest struct {
	Model         string
	Messages      []llm.Message
	Tools         []llm.ToolDef
	MaxTokens     int
	MaxToolRounds int
	FilePath      string
	// ToolsPrompt is human-readable tool guidance for Cursor MCP custom tools.
	ToolsPrompt string
}

// ToolCallInput is passed to the tool executor from any backend.
type ToolCallInput struct {
	ID        string
	Name      string
	Arguments string
}

// ToolCallOutput is returned by the tool executor.
type ToolCallOutput struct {
	Result    string
	Completed bool
}

// ToolExecutor runs a single tool call on behalf of the backend.
type ToolExecutor func(ctx context.Context, call ToolCallInput) ToolCallOutput

// ReviewHooks wires agent-level session, telemetry, and compression into a backend loop.
type ReviewHooks struct {
	// AppendTaskRecord must be called at the start of each review round, before exec
	// invokes tools, so tool results are recorded on the active task record.
	AppendTaskRecord func(taskType session.TaskType, messages []llm.Message) *session.TaskRecord
	SetResponse      func(rec *session.TaskRecord, resp *llm.ChatResponse, durationMs int64)
	SetError         func(rec *session.TaskRecord, err error, durationMs int64)
	RecordUsage      func(usage *llm.UsageInfo)
	RecordLLMRequest func(durationMs int64, totalTokens int64, status string)
	AppendRound      func(assistantContent string, calls []llm.ToolCall, results []ToolRoundResult, messages *[]llm.Message) bool
	Logf             func(format string, args ...any)
}

// ToolRoundResult is a single tool result within a review round.
type ToolRoundResult struct {
	ToolCallID string
	Name       string
	Result     string
}

// Backend executes review-related model work.
type Backend interface {
	Kind() Kind
	Model() string
	Source() string
	Complete(ctx context.Context, req CompleteRequest) (*CompleteResponse, error)
	ReviewFile(ctx context.Context, req ReviewFileRequest, exec ToolExecutor, hooks *ReviewHooks) error
}
