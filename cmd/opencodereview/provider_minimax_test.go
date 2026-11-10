// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestMiniMaxModelSelectionAndRequest(t *testing.T) {
	for _, providerName := range []string{"minimax", "minimax-cn"} {
		for _, current := range []string{"", "MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed", "my-custom-model"} {
			t.Run(providerName+"/"+current, func(t *testing.T) {
				preset, ok := llm.LookupProvider(providerName)
				if !ok {
					t.Fatal("missing preset")
				}
				picker := newModelTUIConfig(modelTUIConfig{
					Provider: preset, ProviderName: providerName,
					RegistryModels: preset.Models, CurrentModel: current,
				})
				if got := picker.displayModels(); !reflect.DeepEqual(got, []string{"MiniMax-M3.1-Flash", "MiniMax-M3"}) {
					t.Fatalf("suggested models = %v", got)
				}
				want := current
				if want == "" {
					want = "MiniMax-M3.1-Flash"
				}
				selected := picker.selectedModel()
				if selected != want {
					t.Fatalf("selected model = %q, want %q", selected, want)
				}

				requests := make(chan map[string]any, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					if r.Header.Get("Authorization") != "Bearer test-key" {
						t.Error("missing fixture authorization")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests <- body
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"","reasoning_content":"preserved reasoning","tool_calls":[{"id":"call-1","type":"function","function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
				}))
				defer server.Close()

				configPath := filepath.Join(t.TempDir(), "config.json")
				cfg := map[string]any{
					"provider":  providerName,
					"model":     selected,
					"providers": map[string]any{providerName: map[string]any{"api_key": "test-key", "url": server.URL + "/v1"}},
				}
				data, err := json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
				ep, err := llm.ResolveEndpointWithOptions(configPath, llm.ResolveOptions{Provider: providerName})
				if err != nil {
					t.Fatal(err)
				}
				client := llm.NewLLMClient(ep, nil, nil)
				req := llm.ChatRequest{
					Messages:  []llm.Message{llm.NewTextMessage("user", "Review this change.")},
					Tools:     []llm.ToolDef{{Function: llm.FunctionDef{Name: "inspect", Parameters: map[string]any{"type": "object"}}}},
					MaxTokens: 4096,
				}
				response, err := client.CompletionsWithCtx(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				req.Messages = append(req.Messages,
					llm.NewToolCallMessage(response.VisibleContent(), response.ToolCalls(), response.Native(), response.ReasoningContent()),
					llm.NewToolResultMessage("call-1", "Review complete."),
				)
				if _, err := client.CompletionsWithCtx(context.Background(), req); err != nil {
					t.Fatal(err)
				}
				if len(requests) != 2 {
					t.Fatalf("request count = %d, want 2", len(requests))
				}
				bodies := []map[string]any{<-requests, <-requests}
				for _, body := range bodies {
					if body["model"] != want || body["max_completion_tokens"] != float64(4096) {
						t.Fatalf("unexpected model or output limit: %v", body)
					}
					for _, key := range []string{"thinking", "reasoning_effort", "service_tier", "reasoning_split"} {
						if _, exists := body[key]; exists {
							t.Errorf("default request unexpectedly sets %s", key)
						}
					}
				}
				messages := bodies[1]["messages"].([]any)
				assistant := messages[1].(map[string]any)
				if assistant["reasoning_content"] != "preserved reasoning" || len(assistant["tool_calls"].([]any)) != 1 {
					t.Fatalf("reasoning or tool calls lost on replay: %v", assistant)
				}
				if want == "MiniMax-M3.1-Flash" {
					for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
						ep.ExtraBody = map[string]any{"thinking": map[string]any{"type": "adaptive"}, "reasoning_effort": effort}
						if _, err := llm.NewLLMClient(ep, nil, nil).CompletionsWithCtx(context.Background(), req); err != nil {
							t.Fatal(err)
						}
						body := <-requests
						if body["reasoning_effort"] != effort || !reflect.DeepEqual(body["thinking"], ep.ExtraBody["thinking"]) {
							t.Fatalf("explicit thinking settings lost: %v", body)
						}
					}
				}
				if want == "MiniMax-M3" {
					ep.ExtraBody = map[string]any{"thinking": map[string]any{"type": "disabled"}}
					if _, err := llm.NewLLMClient(ep, nil, nil).CompletionsWithCtx(context.Background(), req); err != nil {
						t.Fatal(err)
					}
					if body := <-requests; !reflect.DeepEqual(body["thinking"], ep.ExtraBody["thinking"]) {
						t.Fatalf("explicit M3 thinking setting lost: %v", body)
					}
				}
			})
		}
	}
}
