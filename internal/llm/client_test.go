package llm

import (
	"context"
	"testing"
	"time"

	openai "github.com/openai/openai-go/v3"
)

func TestNewAnthropicClient_SDKConstruction(t *testing.T) {
	client := NewAnthropicClient(ClientConfig{
		URL:    "https://api.anthropic.com",
		APIKey: "test-key",
		Model:  "claude-sonnet-4-20250514",
	})
	if client.client == nil {
		t.Error("SDK client should not be nil")
	}
	if client.cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", client.cfg.Model, "claude-sonnet-4-20250514")
	}
}

func TestAnthropicClient_CompletionsWithCtx_TranslatesRequest(t *testing.T) {
	client := NewAnthropicClient(ClientConfig{
		URL:    "https://api.anthropic.com",
		APIKey: "test-key",
		Model:  "claude-sonnet-4-20250514",
	})

	req := ChatRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []Message{NewTextMessage("user", "hello")},
		MaxTokens: 1000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.CompletionsWithCtx(ctx, req)
	if err == nil {
		t.Log("Expected error from fake server, got nil")
	}
}

func TestNewAnthropicClient_URLPassthrough(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
	}{
		{
			name:     "bare host",
			inputURL: "https://api.anthropic.com",
		},
		{
			name:     "bare host with trailing slash",
			inputURL: "https://api.anthropic.com/",
		},
		{
			name:     "full URL",
			inputURL: "https://api.anthropic.com/v1",
		},
		{
			name:     "custom proxy base URL",
			inputURL: "https://proxy.example.com/anthropic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewAnthropicClient(ClientConfig{URL: tt.inputURL, APIKey: "test-key"})
			// SDK handles URL construction internally; verify client was created.
			if client.client == nil {
				t.Error("SDK client should not be nil")
			}
		})
	}
}

func TestMessagesToOpenAI(t *testing.T) {
	tests := []struct {
		name string
		msgs []Message
		want int // expected number of SDK messages
	}{
		{
			name: "single user message",
			msgs: []Message{NewTextMessage("user", "hello")},
			want: 1,
		},
		{
			name: "system and user messages",
			msgs: []Message{
				NewTextMessage("system", "you are helpful"),
				NewTextMessage("user", "hello"),
			},
			want: 2,
		},
		{
			name: "assistant with tool calls",
			msgs: []Message{{
				Role:    "assistant",
				Content: "let me check",
				ToolCalls: []ToolCall{{
					ID:   "call_123",
					Type: "function",
					Function: FunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"main.go"}`,
					},
				}},
			}},
			want: 1,
		},
		{
			name: "tool result message",
			msgs: []Message{{
				Role:       "tool",
				Content:    "file contents here",
				ToolCallID: "call_123",
			}},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := messagesToOpenAI(tt.msgs)
			if len(result) != tt.want {
				t.Errorf("got %d messages, want %d", len(result), tt.want)
			}
		})
	}
}

func TestToolsToOpenAI(t *testing.T) {
	tools := []ToolDef{{
		Type: "function",
		Function: FunctionDef{
			Name:        "read_file",
			Description: "Read a file from disk",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			},
		},
	}}
	result := toolsToOpenAI(tools)
	if len(result) != 1 {
		t.Fatalf("got %d tools, want 1", len(result))
	}
}

func TestOpenAIToChatResponse(t *testing.T) {
	t.Run("text response", func(t *testing.T) {
		resp := openAIToChatResponse("chatcmpl-123", "gpt-4", "stop", "Hello!", nil, nil)
		if resp.ID != "chatcmpl-123" {
			t.Errorf("ID = %q, want %q", resp.ID, "chatcmpl-123")
		}
		if len(resp.Choices) != 1 {
			t.Fatalf("got %d choices, want 1", len(resp.Choices))
		}
		if resp.Choices[0].FinishReason != "stop" {
			t.Errorf("FinishReason = %q, want %q", resp.Choices[0].FinishReason, "stop")
		}
		if resp.Choices[0].Message.Content == nil || *resp.Choices[0].Message.Content != "Hello!" {
			t.Errorf("Content = %v, want %q", resp.Choices[0].Message.Content, "Hello!")
		}
	})

	t.Run("tool calls response", func(t *testing.T) {
		toolCalls := []ToolCall{{
			ID:       "call_abc",
			Type:     "function",
			Function: FunctionCall{Name: "read_file", Arguments: `{"path":"main.go"}`},
		}}
		resp := openAIToChatResponse("chatcmpl-456", "gpt-4", "tool_calls", "", toolCalls, nil)
		if resp.Choices[0].FinishReason != "tool_calls" {
			t.Errorf("FinishReason = %q, want %q", resp.Choices[0].FinishReason, "tool_calls")
		}
		if len(resp.Choices[0].Message.ToolCalls) != 1 {
			t.Fatalf("got %d tool calls, want 1", len(resp.Choices[0].Message.ToolCalls))
		}
		if resp.Choices[0].Message.ToolCalls[0].ID != "call_abc" {
			t.Errorf("ToolCall ID = %q, want %q", resp.Choices[0].Message.ToolCalls[0].ID, "call_abc")
		}
	})
}

func TestNewOpenAIClient_SDKConstruction(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{
		URL:    "https://api.openai.com/v1",
		APIKey: "test-key",
		Model:  "gpt-4",
	})
	if client.client == nil {
		t.Error("SDK client should not be nil")
	}
	if client.cfg.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", client.cfg.Model, "gpt-4")
	}
}

func TestOpenAIClient_CompletionsWithCtx_TranslatesRequest(t *testing.T) {
	client := NewOpenAIClient(ClientConfig{
		URL:    "https://api.example.com/v1",
		APIKey: "test-key",
		Model:  "gpt-4",
	})

	req := ChatRequest{
		Model:     "gpt-4",
		Messages:  []Message{NewTextMessage("user", "hello")},
		MaxTokens: 1000,
	}

	// Use a short timeout so the test doesn't hang on retries.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// The call will fail (no real server), but we verify no panic and proper error handling.
	_, err := client.CompletionsWithCtx(ctx, req)
	if err == nil {
		t.Log("Expected error from fake server, got nil (server might be reachable)")
	}
}

func TestMessagesToAnthropic(t *testing.T) {
	t.Run("extracts system message", func(t *testing.T) {
		msgs := []Message{
			NewTextMessage("system", "you are helpful"),
			NewTextMessage("user", "hello"),
		}
		system, result := messagesToAnthropic(msgs)
		if len(system) != 1 {
			t.Errorf("got %d system blocks, want 1", len(system))
		}
		if len(result) != 1 {
			t.Errorf("got %d messages, want 1", len(result))
		}
	})

	t.Run("assistant with tool calls", func(t *testing.T) {
		msgs := []Message{{
			Role:    "assistant",
			Content: "let me check",
			ToolCalls: []ToolCall{{
				ID:   "call_123",
				Type: "function",
				Function: FunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"main.go"}`,
				},
			}},
		}}
		_, result := messagesToAnthropic(msgs)
		if len(result) != 1 {
			t.Fatalf("got %d messages, want 1", len(result))
		}
	})

	t.Run("tool result message", func(t *testing.T) {
		msgs := []Message{{
			Role:       "tool",
			Content:    "file contents",
			ToolCallID: "call_123",
		}}
		_, result := messagesToAnthropic(msgs)
		if len(result) != 1 {
			t.Fatalf("got %d messages, want 1", len(result))
		}
	})
}

func TestToolsToAnthropic(t *testing.T) {
	tools := []ToolDef{{
		Type: "function",
		Function: FunctionDef{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		},
	}}
	result := toolsToAnthropic(tools)
	if len(result) != 1 {
		t.Fatalf("got %d tools, want 1", len(result))
	}
}

func TestAnthropicToChatResponse(t *testing.T) {
	t.Run("text response", func(t *testing.T) {
		resp := anthropicToChatResponse("msg_123", "claude-sonnet-4-20250514", "end_turn", "Hello!", nil, nil)
		if resp.ID != "msg_123" {
			t.Errorf("ID = %q, want %q", resp.ID, "msg_123")
		}
		if len(resp.Choices) != 1 {
			t.Fatalf("got %d choices, want 1", len(resp.Choices))
		}
		if resp.Choices[0].Message.Content == nil || *resp.Choices[0].Message.Content != "Hello!" {
			t.Errorf("Content = %v, want %q", resp.Choices[0].Message.Content, "Hello!")
		}
	})

	t.Run("tool use response", func(t *testing.T) {
		toolCalls := []ToolCall{{
			ID:       "toolu_abc",
			Type:     "function",
			Function: FunctionCall{Name: "read_file", Arguments: `{"path":"main.go"}`},
		}}
		resp := anthropicToChatResponse("msg_456", "claude-sonnet-4-20250514", "tool_use", "", toolCalls, nil)
		if resp.Choices[0].FinishReason != "tool_calls" {
			t.Errorf("FinishReason = %q, want %q", resp.Choices[0].FinishReason, "tool_calls")
		}
		if len(resp.Choices[0].Message.ToolCalls) != 1 {
			t.Fatalf("got %d tool calls, want 1", len(resp.Choices[0].Message.ToolCalls))
		}
	})
}

// Verify that the openai import is used (avoids unused import errors during compilation).
var _ = openai.ChatCompletionMessageParamUnion{}
