package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
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

func TestBuildAnthropicParams_CacheControl(t *testing.T) {
	client := NewAnthropicClient(ClientConfig{URL: "https://api.anthropic.com"})

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a code reviewer."},
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "Review this code."},
		},
		Tools: []ToolDef{
			{Type: "function", Function: FunctionDef{Name: "tool_a", Description: "first tool", Parameters: map[string]any{"type": "object"}}},
			{Type: "function", Function: FunctionDef{Name: "tool_b", Description: "second tool", Parameters: map[string]any{"type": "object"}}},
		},
	}

	params, err := client.buildAnthropicParams("claude-sonnet-4-20250514", req)
	if err != nil {
		t.Fatalf("buildAnthropicParams: %v", err)
	}

	t.Run("last system block has cache control", func(t *testing.T) {
		if len(params.System) < 2 {
			t.Fatalf("expected at least 2 system blocks, got %d", len(params.System))
		}
		last := params.System[len(params.System)-1]
		if last.CacheControl.Type != "ephemeral" {
			t.Errorf("last system block CacheControl.Type = %q, want %q", last.CacheControl.Type, "ephemeral")
		}
	})

	t.Run("non-last system block has no cache control", func(t *testing.T) {
		first := params.System[0]
		if first.CacheControl.Type != "" {
			t.Errorf("first system block CacheControl.Type = %q, want empty", first.CacheControl.Type)
		}
	})

	t.Run("last tool has cache control", func(t *testing.T) {
		if len(params.Tools) < 2 {
			t.Fatalf("expected at least 2 tools, got %d", len(params.Tools))
		}
		last := params.Tools[len(params.Tools)-1]
		if last.OfTool == nil {
			t.Fatal("last tool OfTool is nil")
		}
		if last.OfTool.CacheControl.Type != "ephemeral" {
			t.Errorf("last tool CacheControl.Type = %q, want %q", last.OfTool.CacheControl.Type, "ephemeral")
		}
	})

	t.Run("non-last tool has no cache control", func(t *testing.T) {
		first := params.Tools[0]
		if first.OfTool == nil {
			t.Fatal("first tool OfTool is nil")
		}
		if first.OfTool.CacheControl.Type != "" {
			t.Errorf("first tool CacheControl.Type = %q, want empty", first.OfTool.CacheControl.Type)
		}
	})

	t.Run("top-level CacheControl is not set", func(t *testing.T) {
		if params.CacheControl.Type != "" {
			t.Errorf("params.CacheControl.Type = %q, want empty", params.CacheControl.Type)
		}
	})
}

func TestBuildAnthropicParams_CacheControl_NoTools(t *testing.T) {
	client := NewAnthropicClient(ClientConfig{URL: "https://api.anthropic.com"})

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a planner."},
			{Role: "user", Content: "Plan the review."},
		},
	}

	params, err := client.buildAnthropicParams("claude-sonnet-4-20250514", req)
	if err != nil {
		t.Fatalf("buildAnthropicParams: %v", err)
	}

	if len(params.System) == 0 {
		t.Fatal("expected system blocks")
	}
	last := params.System[len(params.System)-1]
	if last.CacheControl.Type != "ephemeral" {
		t.Errorf("system CacheControl.Type = %q, want %q", last.CacheControl.Type, "ephemeral")
	}
	if len(params.Tools) != 0 {
		t.Errorf("expected no tools, got %d", len(params.Tools))
	}
}

func TestBuildAnthropicParams_CacheControl_NoSystem(t *testing.T) {
	client := NewAnthropicClient(ClientConfig{URL: "https://api.anthropic.com"})

	req := ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []ToolDef{
			{Type: "function", Function: FunctionDef{Name: "tool_a", Description: "a tool", Parameters: map[string]any{"type": "object"}}},
		},
	}

	params, err := client.buildAnthropicParams("claude-sonnet-4-20250514", req)
	if err != nil {
		t.Fatalf("buildAnthropicParams: %v", err)
	}

	if len(params.System) != 0 {
		t.Errorf("expected no system blocks, got %d", len(params.System))
	}
	if len(params.Tools) == 0 {
		t.Fatal("expected tools")
	}
	if params.Tools[0].OfTool.CacheControl.Type != "ephemeral" {
		t.Errorf("tool CacheControl.Type = %q, want %q", params.Tools[0].OfTool.CacheControl.Type, "ephemeral")
	}
}

func TestAnthropicClient_UsesConfiguredXAPIKeyHeader(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "env-oauth-token")

	var gotXAPIKey string
	var gotAuthorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXAPIKey = r.Header.Get("X-Api-Key")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		URL:        server.URL + "/v1/messages",
		APIKey:     "sk-ant-api03-test",
		Model:      "claude-test",
		AuthHeader: "x-api-key",
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if gotXAPIKey != "sk-ant-api03-test" {
		t.Errorf("X-Api-Key = %q, want %q", gotXAPIKey, "sk-ant-api03-test")
	}
	if gotAuthorization != "" {
		t.Errorf("Authorization = %q, want empty", gotAuthorization)
	}
}

func TestAnthropicClient_UsesConfiguredAuthorizationHeader(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-api-key")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")

	var gotXAPIKey string
	var gotAuthorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXAPIKey = r.Header.Get("X-Api-Key")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		URL:        server.URL + "/v1/messages",
		APIKey:     "oauth-token",
		Model:      "claude-test",
		AuthHeader: "authorization",
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if gotAuthorization != "Bearer oauth-token" {
		t.Errorf("Authorization = %q, want %q", gotAuthorization, "Bearer oauth-token")
	}
	if gotXAPIKey != "" {
		t.Errorf("X-Api-Key = %q, want empty", gotXAPIKey)
	}
}

func TestAnthropicClient_DefaultsToAuthorizationHeader(t *testing.T) {
	var gotXAPIKey string
	var gotAuthorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXAPIKey = r.Header.Get("X-Api-Key")
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		URL:    server.URL + "/v1/messages",
		APIKey: "oauth-token",
		Model:  "claude-test",
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if gotAuthorization != "Bearer oauth-token" {
		t.Errorf("Authorization = %q, want %q", gotAuthorization, "Bearer oauth-token")
	}
	if gotXAPIKey != "" {
		t.Errorf("X-Api-Key = %q, want empty", gotXAPIKey)
	}
}

func TestAnthropicClient_ExtraHeadersSent(t *testing.T) {
	var gotCustomHeader string
	var gotOrgID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustomHeader = r.Header.Get("X-Custom-Header")
		gotOrgID = r.Header.Get("X-Org-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		URL:    server.URL + "/v1/messages",
		APIKey: "test-key",
		Model:  "claude-test",
		ExtraHeaders: map[string]string{
			"X-Custom-Header": "custom-val",
			"X-Org-ID":        "org-abc",
		},
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if gotCustomHeader != "custom-val" {
		t.Errorf("X-Custom-Header = %q, want %q", gotCustomHeader, "custom-val")
	}
	if gotOrgID != "org-abc" {
		t.Errorf("X-Org-ID = %q, want %q", gotOrgID, "org-abc")
	}
}

func TestOpenAIClient_ExtraHeadersSent(t *testing.T) {
	var gotCustomHeader string
	var gotOrgID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustomHeader = r.Header.Get("X-Custom-Header")
		gotOrgID = r.Header.Get("X-Org-ID")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"model":"gpt-test",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "gpt-test",
		ExtraHeaders: map[string]string{
			"X-Custom-Header": "custom-val",
			"X-Org-ID":        "org-abc",
		},
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if gotCustomHeader != "custom-val" {
		t.Errorf("X-Custom-Header = %q, want %q", gotCustomHeader, "custom-val")
	}
	if gotOrgID != "org-abc" {
		t.Errorf("X-Org-ID = %q, want %q", gotOrgID, "org-abc")
	}
}

func writeOpenAISSE(t *testing.T, w http.ResponseWriter, events ...string) {
	t.Helper()

	w.Header().Set("Content-Type", "text/event-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Error("response writer does not support flushing")
		return
	}

	for _, event := range events {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", event); err != nil {
			t.Errorf("write SSE event: %v", err)
			return
		}
		flusher.Flush()
	}

	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		t.Errorf("write SSE done event: %v", err)
		return
	}
	flusher.Flush()
}

func TestOpenAIClient_StreamOnlyGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		stream, ok := body["stream"].(bool)
		if !ok || !stream {
			t.Errorf("stream = %#v, want boolean true", body["stream"])
			http.Error(w, "streaming required", http.StatusBadRequest)
			return
		}
		if body["vendor_flag"] != "keep-me" {
			t.Errorf("vendor_flag = %#v, want %q", body["vendor_flag"], "keep-me")
			http.Error(w, "vendor flag required", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("X-Test-Header"); got != "streaming" {
			t.Errorf("X-Test-Header = %q, want %q", got, "streaming")
			http.Error(w, "test header required", http.StatusBadRequest)
			return
		}

		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{"role":"assistant","content":"hel"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "gpt-stream",
		ExtraBody: map[string]any{
			"stream":      true,
			"vendor_flag": "keep-me",
		},
		ExtraHeaders: map[string]string{
			"X-Test-Header": "streaming",
		},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if resp.ID != "chatcmpl-stream" {
		t.Errorf("ID = %q, want %q", resp.ID, "chatcmpl-stream")
	}
	if resp.Model != "gpt-stream" {
		t.Errorf("Model = %q, want %q", resp.Model, "gpt-stream")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices length = %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content == nil || *resp.Choices[0].Message.Content != "hello" {
		t.Errorf("content = %#v, want %q", resp.Choices[0].Message.Content, "hello")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish reason = %q, want %q", resp.Choices[0].FinishReason, "stop")
	}
}

func TestOpenAIClient_StreamingRequiresBooleanTrue(t *testing.T) {
	tests := []struct {
		name       string
		configured bool
		value      any
	}{
		{name: "missing"},
		{name: "boolean false", configured: true, value: false},
		{name: "string true", configured: true, value: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request body: %v", err)
					http.Error(w, "invalid request body", http.StatusBadRequest)
					return
				}

				got, exists := body["stream"]
				if exists != tt.configured {
					t.Errorf("stream presence = %t, want %t", exists, tt.configured)
				}
				if tt.configured {
					switch want := tt.value.(type) {
					case bool:
						gotBool, ok := got.(bool)
						if !ok || gotBool != want {
							t.Errorf("stream = %#v, want boolean %t", got, want)
						}
					case string:
						gotString, ok := got.(string)
						if !ok || gotString != want {
							t.Errorf("stream = %#v, want string %q", got, want)
						}
					}
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{
					"id":"chatcmpl-json",
					"object":"chat.completion",
					"model":"gpt-json",
					"choices":[{"index":0,"message":{"role":"assistant","content":"json"},"finish_reason":"stop"}]
				}`)
			}))
			defer server.Close()

			var extraBody map[string]any
			if tt.configured {
				extraBody = map[string]any{"stream": tt.value}
			}
			client := NewOpenAIClient(ClientConfig{
				URL:       server.URL + "/v1",
				APIKey:    "test-key",
				Model:     "gpt-json",
				ExtraBody: extraBody,
			})

			resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
				Messages: []Message{{Role: "user", Content: "ping"}},
			})
			if err != nil {
				t.Fatalf("CompletionsWithCtx: %v", err)
			}
			if got := resp.Content(); got != "json" {
				t.Errorf("content = %q, want %q", got, "json")
			}
		})
	}
}

func TestOpenAIClient_StreamingToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-tools","object":"chat.completion.chunk","created":1,"model":"gpt-tools","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_weather","type":"function","function":{"name":"get_","arguments":"{\"city\":"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tools","object":"chat.completion.chunk","created":1,"model":"gpt-tools","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"weather","arguments":"\"Paris\"}"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tools","object":"chat.completion.chunk","created":1,"model":"gpt-tools","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-tools",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "weather"}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	toolCalls := resp.ToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(toolCalls))
	}
	toolCall := toolCalls[0]
	if toolCall.ID != "call_weather" {
		t.Errorf("ToolCall ID = %q, want %q", toolCall.ID, "call_weather")
	}
	if toolCall.Type != "function" {
		t.Errorf("ToolCall type = %q, want %q", toolCall.Type, "function")
	}
	if toolCall.Function.Name != "get_weather" {
		t.Errorf("ToolCall function name = %q, want %q", toolCall.Function.Name, "get_weather")
	}
	if toolCall.Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("ToolCall arguments = %q, want %q", toolCall.Function.Arguments, `{"city":"Paris"}`)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices length = %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish reason = %q, want %q", resp.Choices[0].FinishReason, "tool_calls")
	}
}

func TestOpenAIClient_StreamingError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w, `{"error":{"message":"upstream stream failed","type":"server_error"}}`)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-stream",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if err == nil || !strings.Contains(err.Error(), "upstream stream failed") {
		t.Fatalf("error = %v, want error containing %q", err, "upstream stream failed")
	}
}

func TestOpenAIClient_StreamingCancellation(t *testing.T) {
	chunkWritten := make(chan struct{}, 1)
	handlerErr := make(chan error, 1)
	reportHandlerError := func(err error) {
		select {
		case handlerErr <- err:
		default:
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			reportHandlerError(errors.New("response writer does not support flushing"))
			return
		}
		if _, err := fmt.Fprint(w, `data: {"id":"chatcmpl-cancel","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"},"finish_reason":null}]}

`); err != nil {
			reportHandlerError(fmt.Errorf("write SSE event: %w", err))
			return
		}
		flusher.Flush()
		select {
		case chunkWritten <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-stream",
		ExtraBody: map[string]any{"stream": true},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		resp *ChatResponse
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		resp, err := client.CompletionsWithCtx(ctx, ChatRequest{
			Messages: []Message{{Role: "user", Content: "ping"}},
		})
		resultCh <- result{resp: resp, err: err}
	}()

	select {
	case <-chunkWritten:
		cancel()
	case err := <-handlerErr:
		cancel()
		t.Fatalf("stream handler: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for streamed chunk")
	}

	select {
	case got := <-resultCh:
		if got.resp != nil {
			t.Fatalf("response = %#v, want nil", got.resp)
		}
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("streaming cancellation deadlocked")
	}
}

func TestOpenAIClient_StreamingInconsistentChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-one","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{"role":"assistant","content":"one"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-two","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{"content":"two"},"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-stream",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if err == nil || !strings.Contains(err.Error(), "inconsistent chunks") {
		t.Fatalf("error = %v, want error containing %q", err, "inconsistent chunks")
	}
}

func TestOpenAIClient_StreamingUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		streamOptions, ok := body["stream_options"].(map[string]any)
		if !ok {
			t.Errorf("stream_options = %#v, want object", body["stream_options"])
			http.Error(w, "stream options required", http.StatusBadRequest)
			return
		}
		includeUsage, ok := streamOptions["include_usage"].(bool)
		if !ok || !includeUsage {
			t.Errorf("stream_options.include_usage = %#v, want boolean true", streamOptions["include_usage"])
			http.Error(w, "streaming usage required", http.StatusBadRequest)
			return
		}

		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1,"model":"gpt-usage","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}],"usage":null}`,
			`{"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1,"model":"gpt-usage","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}`,
			`{"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1,"model":"gpt-usage","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":4}}}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "gpt-usage",
		ExtraBody: map[string]any{
			"stream": true,
			"stream_options": map[string]any{
				"include_usage": true,
			},
		},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage = nil, want token usage")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 3 {
		t.Errorf("CompletionTokens = %d, want 3", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 13 {
		t.Errorf("TotalTokens = %d, want 13", resp.Usage.TotalTokens)
	}
	if resp.Usage.CacheReadTokens != 4 {
		t.Errorf("CacheReadTokens = %d, want 4", resp.Usage.CacheReadTokens)
	}
}

func TestOpenAIClient_StreamingReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-reasoning","object":"chat.completion.chunk","created":1,"model":"gpt-reasoning","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"first "},"finish_reason":null}]}`,
			`{"id":"chatcmpl-reasoning","object":"chat.completion.chunk","created":1,"model":"gpt-reasoning","choices":[{"index":0,"delta":{"reasoning_content":"second","content":"answer"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-reasoning","object":"chat.completion.chunk","created":1,"model":"gpt-reasoning","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-reasoning",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices length = %d, want 1", len(resp.Choices))
	}
	if got := resp.Choices[0].Message.ReasoningContent; got != "first second" {
		t.Errorf("ReasoningContent = %q, want %q", got, "first second")
	}
	if resp.Choices[0].Message.Content == nil || *resp.Choices[0].Message.Content != "answer" {
		t.Errorf("Content = %#v, want %q", resp.Choices[0].Message.Content, "answer")
	}
}

func TestOpenAIClient_StreamingIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-incomplete","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"},"finish_reason":null}]}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-stream",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if err == nil || !strings.Contains(err.Error(), "ended before choice 0 finished") {
		t.Fatalf("error = %v, want error containing %q", err, "ended before choice 0 finished")
	}
}

func TestOpenAIClient_StreamingNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOpenAISSE(t, w,
			`{"id":"chatcmpl-empty","object":"chat.completion.chunk","created":1,"model":"gpt-stream","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
		)
	}))
	defer server.Close()

	client := NewOpenAIClient(ClientConfig{
		URL:       server.URL + "/v1",
		APIKey:    "test-key",
		Model:     "gpt-stream",
		ExtraBody: map[string]any{"stream": true},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if resp != nil {
		t.Fatalf("response = %#v, want nil", resp)
	}
	if err == nil || !strings.Contains(err.Error(), "contained no choices") {
		t.Fatalf("error = %v, want error containing %q", err, "contained no choices")
	}
}

func TestAnthropicClient_NoExtraHeadersWhenEmpty(t *testing.T) {
	var customHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(ClientConfig{
		URL:    server.URL + "/v1/messages",
		APIKey: "test-key",
		Model:  "claude-test",
	})

	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "ping"}},
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	for k := range customHeaders {
		if k == "X-Custom-Header" || k == "X-Org-Id" {
			t.Errorf("unexpected custom header %q sent", k)
		}
	}
}

// Verify the SDK constant is accessible (compile-time check).
var _ anthropic.CacheControlEphemeralParam = anthropic.NewCacheControlEphemeralParam()

func TestCountTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"single word", "hello", 1},
		{"sentence", "The quick brown fox jumps over the lazy dog.", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountTokens(tt.text)
			if tt.want == 0 && got != 0 {
				t.Errorf("CountTokens(%q) = %d, want 0", tt.text, got)
			}
			if tt.want > 0 && got == 0 {
				t.Errorf("CountTokens(%q) = 0, expected > 0", tt.text)
			}
		})
	}
}

func TestCountTokensForModel(t *testing.T) {
	text := "Hello, world! This is a test."
	base := CountTokensForModel(text, "gpt-4")
	o1 := CountTokensForModel(text, "o1-mini")

	if base == 0 {
		t.Error("cl100k_base should produce non-zero tokens")
	}
	if o1 == 0 {
		t.Error("o200k_base should produce non-zero tokens")
	}

	if CountTokensForModel("", "gpt-4") != 0 {
		t.Error("empty text should return 0")
	}
}

func TestEncodingForModel(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"gpt-4", "cl100k_base"},
		{"claude-3-opus", "cl100k_base"},
		{"", "cl100k_base"},
		{"o1-preview", "o200k_base"},
		{"o3-mini", "o200k_base"},
		{"o4-mini", "o200k_base"},
		{"GPT-O1", "o200k_base"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := encodingForModel(tt.model)
			if got != tt.want {
				t.Errorf("encodingForModel(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestStripThinkTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no tags", "hello world", "hello world"},
		{"open only", "<think>partial", "partial"},
		{"close only", "partial</think>", "partial"},
		{"both tags", "<think>reasoning here</think>answer", "reasoning hereanswer"},
		{"multiple tags", "<think>a</think>b<think>c</think>d", "abcd"},
		{"empty", "", ""},
		{"tags only", "<think></think>", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkTags(tt.input)
			if got != tt.want {
				t.Errorf("stripThinkTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
