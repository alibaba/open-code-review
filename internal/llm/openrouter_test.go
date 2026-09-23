// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveEndpoint_OpenRouter(t *testing.T) {
	for _, tt := range []struct {
		name, apiKey, envKey, model, wantToken, wantErr string
	}{
		{"configured key takes precedence", "config-key", "env-key", "anthropic/claude-sonnet-5", "config-key", ""},
		{"environment key", "", "env-key", "openai/gpt-5.5", "env-key", ""},
		{"model outside preset list", "config-key", "", "anthropic/claude-sonnet-4.6", "config-key", ""},
		{"routing suffix preserved", "config-key", "", "anthropic/claude-sonnet-5:floor", "config-key", ""},
		{"missing key", "", "", "anthropic/claude-sonnet-5", "", "no api_key"},
		{"model still required", "config-key", "", "", "", "no model configured"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearAllEnv(t)
			t.Setenv("OPENROUTER_API_KEY", tt.envKey)
			path, _ := writeResolverConfig(t, configFile{
				Provider: "openrouter",
				Providers: map[string]providerEntryConfig{
					"openrouter": {APIKey: tt.apiKey, Model: tt.model},
				},
			})
			ep, err := ResolveEndpoint(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEndpoint: %v", err)
			}
			if ep.Provider != "openrouter" || ep.URL != "https://openrouter.ai/api/v1" ||
				ep.Protocol != ProtocolOpenAIChatCompletions || ep.AuthHeader != "" ||
				ep.Token != tt.wantToken || ep.Model != tt.model {
				t.Fatalf("unexpected endpoint: %+v", ep)
			}
		})
	}
}

func TestOpenRouter_ToolCallingRequest(t *testing.T) {
	clearAllEnv(t)
	const model = "anthropic/claude-sonnet-5:floor"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat/completions" {
			t.Errorf("request = %s %s, want POST /api/v1/chat/completions", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-openrouter-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body struct {
			Model string    `json:"model"`
			Tools []ToolDef `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if body.Model != model || len(body.Tools) != 1 || body.Tools[0].Function.Name != "file_read" {
			t.Errorf("unexpected model or tools: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_read","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer server.Close()

	path, _ := writeResolverConfig(t, configFile{
		Provider: "openrouter",
		Providers: map[string]providerEntryConfig{
			"openrouter": {APIKey: "test-openrouter-key", Model: model, URL: server.URL + "/api/v1"},
		},
	})
	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	client := NewLLMClient(ep, nil, nil)
	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages:  []Message{{Role: "user", Content: "Review main.go"}},
		MaxTokens: 128,
		Tools: []ToolDef{{Type: "function", Function: FunctionDef{
			Name: "file_read", Description: "Read a source file",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		}}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 || calls[0].ID != "call_read" || calls[0].Function.Name != "file_read" || calls[0].Function.Arguments != `{"path":"main.go"}` {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
}
