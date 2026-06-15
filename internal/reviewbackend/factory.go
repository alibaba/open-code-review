package reviewbackend

import (
	"context"
	"fmt"

	"github.com/open-code-review/open-code-review/internal/llm"
)

// New creates a Backend from resolved configuration.
func New(ctx context.Context, resolved ResolvedBackend, repoDir string) (Backend, error) {
	switch resolved.Kind {
	case KindCursorAgent:
		return NewCursorAgentBackend(ctx, resolved.Cursor, repoDir)
	case KindChatCompletions:
		return NewChatCompletionsBackend(resolved.Endpoint), nil
	default:
		return nil, fmt.Errorf("unsupported backend kind %q", resolved.Kind)
	}
}

// TextClient returns an llm.LLMClient that delegates text completions to the backend.
func TextClient(b Backend) llm.LLMClient {
	if cc, ok := b.(*ChatCompletionsBackend); ok {
		return cc.client
	}
	return &completeAdapter{backend: b}
}

type completeAdapter struct {
	backend Backend
}

func (a *completeAdapter) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = a.backend.Model()
	}
	resp, err := a.backend.Complete(ctx, CompleteRequest{
		Model:     model,
		Messages:  req.Messages,
		MaxTokens: req.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	if resp.Raw != nil {
		return resp.Raw, nil
	}
	content := resp.Content
	return &llm.ChatResponse{
		Model: resp.Model,
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Role:    "assistant",
				Content: &content,
			},
			FinishReason: "stop",
		}},
		Usage: resp.Usage,
	}, nil
}
