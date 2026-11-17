// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestTaskBudgetIncludesAnthropicCompatibleCache(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			var requests []llm.ChatRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request llm.ChatRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				requests = append(requests, request)
				name, arguments := "read_test_data", `{}`
				usage := `{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":40,"cache_creation_input_tokens":15}`
				if len(requests) > 1 {
					name, arguments = "submit_test_result", `{"value":42}`
					usage = `{"total_tokens":0}`
				}
				call, _ := json.Marshal(llm.ToolCall{ID: name, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: arguments}})
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "data: {\"id\":\"cache-test\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[%s]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"id\":\"cache-test\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[],\"usage\":%s}\n\ndata: [DONE]\n\n", call, usage)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"id":"cache-test","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[%s]},"finish_reason":"tool_calls"}],"usage":%s}`, call, usage)
			}))
			defer server.Close()

			client := llm.NewOpenAIClient(llm.ClientConfig{
				URL: server.URL + "/v1", APIKey: "test-key", Model: "test",
				ExtraBody: map[string]any{"stream": streaming},
			})
			task := testTask()
			runner := NewRunner(Deps{
				LLMClient: client, MaxTokensBudget: 150,
				Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{}),
			}, task)
			result, err := runner.Run(t.Context(), taskMessages(), "data")
			if err != nil || !result.Completed || result.StopReason != StopTokenBudget {
				t.Errorf("Run = %+v, %v; want completed token-budget grace round", result, err)
			}
			if len(requests) != 2 {
				t.Fatalf("requests = %d, want 2", len(requests))
			}
			last := requests[1]
			if len(last.Tools) != 1 || last.Tools[0].Function.Name != "submit_test_result" || last.Messages[len(last.Messages)-1].ExtractText() != "Submit your test result now." {
				t.Error("second request did not use the task's budget grace policy")
			}
			if runner.TotalInputTokens() != 155 || runner.TotalOutputTokens() != 20 || runner.TotalTokensUsed() != 175 || runner.TotalCacheReadTokens() != 40 || runner.TotalCacheWriteTokens() != 15 {
				t.Errorf("usage: input=%d output=%d total=%d cache read/write=%d/%d", runner.TotalInputTokens(), runner.TotalOutputTokens(), runner.TotalTokensUsed(), runner.TotalCacheReadTokens(), runner.TotalCacheWriteTokens())
			}
		})
	}
}
