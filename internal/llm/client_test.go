package llm

import (
	"testing"

	openai "github.com/openai/openai-go/v3"
)

func TestNewOpenAIClient_URLNormalization(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
		wantURL  string
	}{
		{
			name:     "base URL without trailing slash",
			inputURL: "https://api.example.com/v1",
			wantURL:  "https://api.example.com/v1/chat/completions",
		},
		{
			name:     "base URL with trailing slash",
			inputURL: "https://api.example.com/v1/",
			wantURL:  "https://api.example.com/v1/chat/completions",
		},
		{
			name:     "full URL already has chat/completions",
			inputURL: "https://api.example.com/v1/chat/completions",
			wantURL:  "https://api.example.com/v1/chat/completions",
		},
		{
			name:     "full URL with trailing slash",
			inputURL: "https://api.example.com/v1/chat/completions/",
			wantURL:  "https://api.example.com/v1/chat/completions/",
		},
		{
			name:     "bare host",
			inputURL: "https://api.example.com",
			wantURL:  "https://api.example.com/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewOpenAIClient(ClientConfig{URL: tt.inputURL})
			if client.cfg.URL != tt.wantURL {
				t.Errorf("got URL %q, want %q", client.cfg.URL, tt.wantURL)
			}
		})
	}
}

func TestNewAnthropicClient_URLNormalization(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
		wantURL  string
	}{
		{
			name:     "bare host",
			inputURL: "https://api.anthropic.com",
			wantURL:  "https://api.anthropic.com/v1/messages",
		},
		{
			name:     "bare host with trailing slash",
			inputURL: "https://api.anthropic.com/",
			wantURL:  "https://api.anthropic.com/v1/messages",
		},
		{
			name:     "full URL already has /v1/messages",
			inputURL: "https://api.anthropic.com/v1/messages",
			wantURL:  "https://api.anthropic.com/v1/messages",
		},
		{
			name:     "full URL with trailing slash",
			inputURL: "https://api.anthropic.com/v1/messages/",
			wantURL:  "https://api.anthropic.com/v1/messages/",
		},
		{
			name:     "custom proxy base URL",
			inputURL: "https://proxy.example.com/anthropic",
			wantURL:  "https://proxy.example.com/anthropic/v1/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewAnthropicClient(ClientConfig{URL: tt.inputURL})
			if client.cfg.URL != tt.wantURL {
				t.Errorf("got URL %q, want %q", client.cfg.URL, tt.wantURL)
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

// Verify that the openai import is used (avoids unused import errors during compilation).
var _ = openai.ChatCompletionMessageParamUnion{}
