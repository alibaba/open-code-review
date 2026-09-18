// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveEndpoint_CodingPlanCredentials(t *testing.T) {
	for _, provider := range []string{"dashscope-codingplan", "dashscope-codingplan-intl"} {
		for _, source := range []string{"environment", "config", "other plan"} {
			t.Run(provider+"/"+source, func(t *testing.T) {
				clearAllEnv(t)
				t.Setenv("OCR_LLM_TIMEOUT", "")
				t.Setenv("OCR_LLM_EXTRA_HEADERS", "")
				t.Setenv("DASHSCOPE_CODINGPLAN_KEY", "")
				t.Setenv("DASHSCOPE_CODINGPLAN_INTL_KEY", "")
				t.Setenv("DASHSCOPE_API_KEY", "pay-as-you-go-key")
				t.Setenv("DASHSCOPE_TOKENPLAN_KEY", "token-plan-key")
				p, _ := LookupProvider(provider)
				entry := providerEntryConfig{Model: "qwen3.7-plus"}
				wantToken := "sk-sp-env-key"
				if source == "other plan" {
					otherEnv := "DASHSCOPE_CODINGPLAN_INTL_KEY"
					if provider == "dashscope-codingplan-intl" {
						otherEnv = "DASHSCOPE_CODINGPLAN_KEY"
					}
					t.Setenv(otherEnv, "other-region-key")
				} else {
					t.Setenv(p.EnvVar, wantToken)
				}
				if source == "config" {
					wantToken = "sk-sp-config-key"
					entry.APIKey = wantToken
				}
				path, _ := writeResolverConfig(t, configFile{
					Provider:  provider,
					Providers: map[string]providerEntryConfig{provider: entry},
				})
				ep, err := ResolveEndpoint(path)
				if source == "other plan" {
					if err == nil || !strings.Contains(err.Error(), "no environment variable fallback found") {
						t.Fatalf("error = %v, want missing Coding Plan key", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("ResolveEndpoint: %v", err)
				}
				if ep.Provider != provider || ep.Model != entry.Model || ep.Token != wantToken || ep.URL != p.BaseURL || ep.Protocol != ProtocolOpenAIChatCompletions || ep.Source != "provider:"+provider {
					t.Fatalf("unexpected endpoint: %+v", ep)
				}
				for _, model := range []string{"qwen3-coder-next", "mimo-v2.5"} {
					override, err := ResolveEndpointWithOptions(path, ResolveOptions{Provider: provider, Model: model})
					if model == "mimo-v2.5" {
						if err == nil || !strings.Contains(err.Error(), "not available for provider") {
							t.Fatalf("unsupported model error = %v", err)
						}
					} else if err != nil || override.Model != model {
						t.Fatalf("model override = %q, error = %v", override.Model, err)
					}
				}
			})
		}
	}
}

func TestCodingPlan_ToolCallRoundTrip(t *testing.T) {
	for _, provider := range []string{"dashscope-codingplan", "dashscope-codingplan-intl"} {
		t.Run(provider, func(t *testing.T) {
			clearAllEnv(t)
			t.Setenv("OCR_LLM_TIMEOUT", "")
			t.Setenv("OCR_LLM_EXTRA_HEADERS", "")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-sp-test-key" {
					t.Errorf("unexpected request: %s %s, authorization = %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				var body struct {
					Model    string    `json:"model"`
					Messages []Message `json:"messages"`
					Tools    []ToolDef `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				if body.Model != "qwen3.7-plus" || len(body.Tools) != 1 || body.Tools[0].Function.Name != "file_read" {
					t.Errorf("unexpected model or tools: %+v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 {
					fmt.Fprint(w, `{"id":"codingplan-1","object":"chat.completion","model":"qwen3.7-plus","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_read","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":"tool_calls"}]}`)
					return
				}
				if len(body.Messages) != 3 {
					t.Errorf("messages = %v, want user, assistant, tool", body.Messages)
				} else {
					assistant, result := body.Messages[1], body.Messages[2]
					if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_read" || assistant.ToolCalls[0].Function.Arguments != `{"path":"main.go"}` {
						t.Errorf("unexpected assistant history: %+v", assistant)
					}
					if result.Role != "tool" || result.ToolCallID != "call_read" || result.ExtractText() != "package main" {
						t.Errorf("unexpected tool result: %+v", result)
					}
				}
				fmt.Fprint(w, `{"id":"codingplan-2","object":"chat.completion","model":"qwen3.7-plus","choices":[{"index":0,"message":{"role":"assistant","content":"Review complete."},"finish_reason":"stop"}],"usage":{"prompt_tokens":40,"completion_tokens":10,"total_tokens":50}}`)
			}))
			defer server.Close()
			path, _ := writeResolverConfig(t, configFile{
				Provider: provider,
				Providers: map[string]providerEntryConfig{
					provider: {APIKey: "sk-sp-test-key", Model: "qwen3.7-plus", URL: server.URL + "/v1"},
				},
			})
			ep, err := ResolveEndpoint(path)
			if err != nil {
				t.Fatalf("ResolveEndpoint: %v", err)
			}
			client := NewLLMClient(ep, nil, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req := ChatRequest{
				Messages: []Message{NewTextMessage("user", "Review main.go")},
				Tools: []ToolDef{{Type: "function", Function: FunctionDef{
					Name: "file_read", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
				}}},
			}
			resp, err := client.CompletionsWithCtx(ctx, req)
			if err != nil {
				t.Fatalf("first completion: %v", err)
			}
			toolCalls := resp.ToolCalls()
			if len(toolCalls) != 1 || toolCalls[0].Function.Name != "file_read" {
				t.Fatalf("unexpected tool calls: %v", toolCalls)
			}
			req.Messages = append(req.Messages,
				NewToolCallMessage(resp.VisibleContent(), toolCalls, resp.Native(), resp.ReasoningContent()),
				NewToolResultMessage(toolCalls[0].ID, "package main"),
			)
			resp, err = client.CompletionsWithCtx(ctx, req)
			if err != nil {
				t.Fatalf("second completion: %v", err)
			}
			if resp.Content() != "Review complete." || calls.Load() != 2 || resp.Usage == nil || resp.Usage.PromptTokens != 40 || resp.Usage.CompletionTokens != 10 {
				t.Fatalf("unexpected completion: %+v, calls = %d", resp, calls.Load())
			}
		})
	}
}
