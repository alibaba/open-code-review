package reviewbackend

import (
	"context"
	"fmt"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/session"
)

// ChatCompletionsBackend runs review via OpenAI/Anthropic chat completions and tool_calls.
type ChatCompletionsBackend struct {
	client llm.LLMClient
	ep     llm.ResolvedEndpoint
}

// NewChatCompletionsBackend creates a backend from a resolved LLM endpoint.
func NewChatCompletionsBackend(ep llm.ResolvedEndpoint) *ChatCompletionsBackend {
	return &ChatCompletionsBackend{
		client: llm.NewLLMClient(ep),
		ep:     ep,
	}
}

func (b *ChatCompletionsBackend) Kind() Kind { return KindChatCompletions }

func (b *ChatCompletionsBackend) Model() string { return b.ep.Model }

func (b *ChatCompletionsBackend) Source() string { return b.ep.Source }

func (b *ChatCompletionsBackend) Complete(ctx context.Context, req CompleteRequest) (*CompleteResponse, error) {
	model := req.Model
	if model == "" {
		model = b.ep.Model
	}
	start := time.Now()
	resp, err := b.client.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     model,
		Messages:  req.Messages,
		MaxTokens: req.MaxTokens,
	})
	_ = start
	if err != nil {
		return nil, err
	}
	outModel := model
	if resp.Model != "" {
		outModel = resp.Model
	}
	return &CompleteResponse{
		Content: resp.Content(),
		Model:   outModel,
		Usage:   resp.Usage,
		Raw:     resp,
	}, nil
}

// ReviewFile runs the chat-completions tool loop until task_done or limits are hit.
func (b *ChatCompletionsBackend) ReviewFile(ctx context.Context, req ReviewFileRequest, exec ToolExecutor, hooks *ReviewHooks) error {
	if hooks == nil {
		hooks = &ReviewHooks{}
	}
	logf := hooks.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	model := req.Model
	if model == "" {
		model = b.ep.Model
	}

	messages := append([]llm.Message(nil), req.Messages...)
	toolReqCount := req.MaxToolRounds
	if toolReqCount <= 0 {
		toolReqCount = 1
	}

	const maxConsecutiveEmptyRounds = 3
	consecutiveEmptyRounds := 0

	for toolReqCount > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		toolReqCount--

		var rec *session.TaskRecord
		if hooks.AppendTaskRecord != nil {
			rec = hooks.AppendTaskRecord(session.MainTask, append([]llm.Message(nil), messages...))
		}

		start := time.Now()
		resp, err := b.client.CompletionsWithCtx(ctx, llm.ChatRequest{
			Model:     model,
			Messages:  messages,
			Tools:     req.Tools,
			MaxTokens: req.MaxTokens,
		})
		duration := time.Since(start)

		if err != nil {
			if hooks.SetError != nil && rec != nil {
				hooks.SetError(rec, err, duration.Milliseconds())
			}
			if hooks.RecordLLMRequest != nil {
				hooks.RecordLLMRequest(duration.Milliseconds(), 0, "error")
			}
			return fmt.Errorf("LLM completion error: %w", err)
		}

		if hooks.SetResponse != nil && rec != nil {
			hooks.SetResponse(rec, resp, duration.Milliseconds())
		}
		totalTokens := int64(0)
		if resp.Usage != nil {
			totalTokens = resp.Usage.TotalTokens
			if hooks.RecordUsage != nil {
				hooks.RecordUsage(resp.Usage)
			}
		}
		if hooks.RecordLLMRequest != nil {
			hooks.RecordLLMRequest(duration.Milliseconds(), totalTokens, "ok")
		}

		content := resp.Content()
		calls := resp.ToolCalls()

		if len(calls) == 0 {
			logf("[ocr] No tool calls parsed for %s, retrying...\n", req.FilePath)
			messages = append(messages, llm.NewTextMessage("user", "You did not successfully call any tools. Please try again or use task_done if finished."))
			if content != "" {
				messages = append(messages[:len(messages)-1], llm.NewTextMessage("assistant", content), messages[len(messages)-1])
			}
			continue
		}

		var results []ToolRoundResult
		taskCompleted := false
		hasValidResult := false

		for _, call := range calls {
			out := exec(ctx, ToolCallInput{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			})
			if out.Completed {
				results = append(results, ToolRoundResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     "Task completed successfully.",
				})
				taskCompleted = true
			} else if out.Result != "" {
				results = append(results, ToolRoundResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     out.Result,
				})
				hasValidResult = true
			} else {
				results = append(results, ToolRoundResult{
					ToolCallID: call.ID,
					Name:       call.Function.Name,
					Result:     "Error: Tool execution returned no result.",
				})
			}
		}

		if taskCompleted {
			break
		}
		if !hasValidResult {
			consecutiveEmptyRounds++
			if consecutiveEmptyRounds >= maxConsecutiveEmptyRounds {
				logf("[ocr] Too many empty retries for %s, stopping.\n", req.FilePath)
				break
			}
			logf("[ocr] No valid tool results for %s, retrying...\n", req.FilePath)
		} else {
			consecutiveEmptyRounds = 0
		}

		if hooks.AppendRound != nil {
			if !hooks.AppendRound(content, calls, results, &messages) {
				logf("[ocr] Context compression exceeded threshold for %s, stopping.\n", req.FilePath)
				break
			}
		} else {
			appendRoundMessages(content, calls, results, &messages)
		}
	}

	if toolReqCount <= 0 {
		logf("[ocr] Max tool requests reached for %s.\n", req.FilePath)
	}

	return nil
}

func appendRoundMessages(assistantContent string, toolCalls []llm.ToolCall, results []ToolRoundResult, messages *[]llm.Message) {
	if len(toolCalls) > 0 {
		*messages = append(*messages, llm.NewToolCallMessage(assistantContent, toolCalls))
	} else if assistantContent != "" {
		*messages = append(*messages, llm.NewTextMessage("assistant", assistantContent))
	}
	for _, r := range results {
		*messages = append(*messages, llm.NewToolResultMessage(r.ToolCallID, r.Result))
	}
}
