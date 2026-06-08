package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

	params := client.buildAnthropicParams("claude-sonnet-4-20250514", req)

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

	params := client.buildAnthropicParams("claude-sonnet-4-20250514", req)

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

	params := client.buildAnthropicParams("claude-sonnet-4-20250514", req)

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

// Verify the SDK constant is accessible (compile-time check).
var _ anthropic.CacheControlEphemeralParam = anthropic.NewCacheControlEphemeralParam()
