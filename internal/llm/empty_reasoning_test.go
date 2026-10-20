// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A thinking gateway requires reasoning_content on the next tool round even
// when the previous response carries an empty/null value. Absence is different:
// ordinary non-thinking gateways must not acquire a foreign reasoning field.
func TestOpenAIClient_ReplaysEmptyReasoningPresence(t *testing.T) {
	for _, priorReasoning := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, field := range []string{`"reasoning_content":""`, `"reasoning_content":null`, `"reasoning_content":"checked"`, ""} {
				t.Run(fmt.Sprintf("prior=%v/stream=%v/field=%s", priorReasoning, streaming, field), func(t *testing.T) {
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if requests.Add(1) == 2 {
							var body struct {
								Messages []map[string]any `json:"messages"`
							}
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Errorf("decode request: %v", err)
								return
							}
							expectedCount := 3
							if priorReasoning {
								expectedCount = 5
							}
							if len(body.Messages) != expectedCount {
								t.Errorf("messages = %d", len(body.Messages))
								return
							}
							value, present := body.Messages[len(body.Messages)-2]["reasoning_content"]
							expected := ""
							if field == `"reasoning_content":"checked"` {
								expected = "checked"
							}
							if present != (field != "" || priorReasoning) || (present && value != expected) {
								t.Errorf("reasoning value=%v present=%v, response field=%q", value, present, field)
								http.Error(w, "reasoning_content must be passed back", http.StatusBadRequest)
								return
							}
							if streaming {
								writeOpenAISSE(t, w, `{"id":"c","object":"chat.completion.chunk","created":1,"model":"fixture","choices":[{"index":0,"delta":{"role":"assistant","content":"finished"},"finish_reason":"stop"}]}`)
							} else {
								fmt.Fprint(w, `{"id":"c","object":"chat.completion","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"finished"},"finish_reason":"stop"}]}`)
							}
							return
						}
						prefix := field
						if prefix != "" {
							prefix += ","
						}
						if streaming {
							writeOpenAISSE(t, w,
								fmt.Sprintf(`{"id":"c","object":"chat.completion.chunk","created":1,"model":"fixture","choices":[{"index":0,"delta":{%s"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, prefix))
						} else {
							fmt.Fprintf(w, `{"id":"c","object":"chat.completion","model":"fixture","choices":[{"index":0,"message":{%s"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, prefix)
						}
					}))
					defer server.Close()
					client := NewOpenAIClient(ClientConfig{URL: server.URL + "/v1", APIKey: "fixture", Model: "fixture", ExtraBody: map[string]any{"stream": streaming}})
					user := Message{Role: "user", Content: "review"}
					messages := []Message{user}
					if priorReasoning {
						messages = []Message{{Role: "user", Content: "previous"}, NewToolCallMessage("previous", nil, NativeTurn{Family: "openai-chat-completions", Payload: ReasoningPayload("prior")}, "prior"), user}
					}
					first, err := client.CompletionsWithCtx(context.Background(), ChatRequest{Messages: messages})
					if err != nil {
						t.Fatal(err)
					}
					messages = append(messages, NewToolCallMessage(first.Content(), first.ToolCalls(), first.Native(), first.ReasoningContent()), NewToolResultMessage("call_1", "source"))
					second, err := client.CompletionsWithCtx(context.Background(), ChatRequest{Messages: messages})
					if err != nil {
						t.Fatal(err)
					}
					if second.Content() != "finished" || requests.Load() != 2 {
						t.Fatalf("result=%q requests=%d", second.Content(), requests.Load())
					}
				})
			}
		}
	}
}
