// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type fakeClient struct {
	responses []*llm.ChatResponse
	requests  []llm.ChatRequest
	calls     int
}

func (f *fakeClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.requests = append(f.requests, req)
	if f.calls >= len(f.responses) {
		content := ""
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
			Model:   "fake",
		}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func taskDoneResponseWithArguments(arguments string) *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: arguments,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}
}

func taskDoneResponse() *llm.ChatResponse {
	return taskDoneResponseWithArguments(`{}`)
}

func fileReadToolCallResponse(callID, args string) *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   callID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "file_read",
						Arguments: args,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 10},
	}
}

type fakeFileReadProvider struct {
	result string
}

func (f *fakeFileReadProvider) Tool() tool.Tool { return tool.FileRead }
func (f *fakeFileReadProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	return f.result, nil
}

func newTestDeps(client llm.LLMClient) Deps {
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: "package main\n"})
	return Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: tool.NewCommentCollector(),
		Session:          session.New("/tmp/test-repo", "main", "fake", session.SessionOptions{}),
	}
}

func TestRunPerFile_TaskDoneImmediately(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review this file")}
	completed, _, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	if client.calls != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.calls)
	}
	if runner.TotalInputTokens() != 10 {
		t.Errorf("TotalInputTokens = %d, want 10", runner.TotalInputTokens())
	}
	if runner.TotalOutputTokens() != 5 {
		t.Errorf("TotalOutputTokens = %d, want 5", runner.TotalOutputTokens())
	}
}

func TestRunPerFile_UsesCompletionTokenLimit(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.Template.MaxTokens = 200000
	deps.Template.MaxCompletionTokens = 58888
	runner := NewRunner(deps)

	_, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if got := client.requests[0].MaxTokens; got != 58888 {
		t.Fatalf("request MaxTokens = %d, want 58888", got)
	}
}

func TestRunPerFile_TaskDoneExplicitDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"DONE"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done DONE to complete RunPerFile")
	}
}

func TestRunPerFile_TaskDoneFailed(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		taskDoneResponseWithArguments(`{"state":"FAILED"}`),
	}}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err == nil || !strings.Contains(err.Error(), "task_done reported FAILED") {
		t.Fatalf("expected task_done FAILED error, got %v", err)
	}
	if completed {
		t.Fatal("task_done FAILED must not complete RunPerFile")
	}
	if client.calls != 1 {
		t.Fatalf("expected terminal failure after 1 LLM call, got %d", client.calls)
	}
}

func TestRunPerFile_InvalidTaskDoneStateRetries(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
	}{
		{name: "unknown state", arguments: `{"state":"UNKNOWN"}`},
		{name: "empty state", arguments: `{"state":""}`},
		{name: "non-string state", arguments: `{"state":1}`},
		{name: "malformed arguments", arguments: `{"state":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{responses: []*llm.ChatResponse{
				taskDoneResponseWithArguments(tt.arguments),
				taskDoneResponseWithArguments(`{"state":"DONE"}`),
			}}
			runner := NewRunner(newTestDeps(client))

			completed, _, err := runner.RunPerFile(
				context.Background(),
				[]llm.Message{llm.NewTextMessage("user", "review this file")},
				"main.go",
			)
			if err != nil {
				t.Fatalf("RunPerFile: %v", err)
			}
			if !completed {
				t.Fatal("expected retry to complete with task_done DONE")
			}
			if client.calls != 2 {
				t.Fatalf("expected invalid state to be retried, got %d LLM calls", client.calls)
			}
		})
	}
}

func TestRunPerFile_ToolCallThenDone(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	if client.calls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", client.calls)
	}

	toolCalls := runner.ToolCalls()
	if toolCalls["file_read"] != 1 {
		t.Errorf("file_read calls = %d, want 1", toolCalls["file_read"])
	}
	if runner.TotalInputTokens() != 30 {
		t.Errorf("TotalInputTokens = %d, want 30", runner.TotalInputTokens())
	}
}

func TestRunPerFile_OpenAIReplaysCompleteAssistantToolTurn(t *testing.T) {
	tests := []struct {
		name              string
		contentJSON       string
		visibleContent    string
		reasoningFragment string
		wantReasoningJSON string
	}{
		{name: "visible content", contentJSON: `"I'll inspect"`, visibleContent: "I'll inspect", reasoningFragment: `,"reasoning_content":"private reasoning"`, wantReasoningJSON: `"private reasoning"`},
		{name: "whitespace and think tags", contentJSON: `"  <think>visible</think>  "`, visibleContent: "  <think>visible</think>  ", reasoningFragment: `,"reasoning_content":"private reasoning"`, wantReasoningJSON: `"private reasoning"`},
		{name: "empty string content", contentJSON: `""`, reasoningFragment: `,"reasoning_content":"private reasoning"`, wantReasoningJSON: `"private reasoning"`},
		{name: "null content", contentJSON: `null`, reasoningFragment: `,"reasoning_content":"private reasoning"`, wantReasoningJSON: `"private reasoning"`},
		{name: "empty reasoning is present", contentJSON: `null`, reasoningFragment: `,"reasoning_content":""`, wantReasoningJSON: `""`},
		{name: "absent reasoning stays absent", contentJSON: `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
				response := fmt.Sprintf(`{"id":"first","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":%s,"refusal":null,"annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":1,"title":"source","url":"https://example.com"}}],"audio":{"id":"audio-1","data":"response-only","expires_at":123,"transcript":"response-only"},"function_call":null%s,"vendor_state":{"nonce":"opaque"},"tool_calls":[{"id":"call_1","type":"function","extra_content":{"google":{"thought_signature":"gemini-sig-1"}},"function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":"tool_calls"}]}`, tt.contentJSON, tt.reasoningFragment)
				if call > 1 {
					response = `{"id":"second","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_2","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			})
			defer server.Close()

			client := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL() + "/v1", APIKey: "test", Model: "fake", AssistantReplay: llm.AssistantReplayNative})
			deps := newTestDeps(client)
			deps.Template.MaxToolRequestTimes = 2
			runner := NewRunner(deps)
			completed, _, err := runner.RunPerFile(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
			if err != nil {
				t.Fatalf("RunPerFile: %v", err)
			}
			if !completed {
				t.Fatal("expected task_done to complete RunPerFile")
			}
			requests := server.Requests()
			if len(requests) != 2 {
				t.Fatalf("provider requests = %d, want 2", len(requests))
			}

			messages := decodeOpenAIRequestMessages(t, requests[1])
			assistantIndex := -1
			toolIndex := -1
			for i, message := range messages {
				var role string
				if err := json.Unmarshal(message["role"], &role); err != nil {
					t.Fatalf("decode message %d role: %v", i, err)
				}
				if role == "assistant" && assistantIndex == -1 {
					assistantIndex = i
					gotReasoning, reasoningPresent := message["reasoning_content"]
					if tt.wantReasoningJSON == "" && reasoningPresent {
						t.Errorf("replayed absent reasoning_content = %s", gotReasoning)
					}
					if tt.wantReasoningJSON != "" && (!reasoningPresent || string(gotReasoning) != tt.wantReasoningJSON) {
						t.Errorf("replayed reasoning_content = %s, want %s (request=%s)", gotReasoning, tt.wantReasoningJSON, requests[1])
					}
					if got := string(message["content"]); got != tt.contentJSON {
						t.Errorf("replayed content JSON = %s, want exact %s", got, tt.contentJSON)
					}
					if tt.contentJSON != "null" {
						var got string
						if err := json.Unmarshal(message["content"], &got); err != nil {
							t.Fatalf("decode replayed content: %v", err)
						}
						if got != tt.visibleContent {
							t.Errorf("replayed content = %q, want exact %q", got, tt.visibleContent)
						}
					}
					if _, present := message["vendor_state"]; present {
						t.Errorf("replayed unknown vendor field without a request contract: %s", message["vendor_state"])
					}
					var toolCalls []struct {
						ExtraContent struct {
							Google struct {
								ThoughtSignature string `json:"thought_signature"`
							} `json:"google"`
						} `json:"extra_content"`
					}
					if err := json.Unmarshal(message["tool_calls"], &toolCalls); err != nil {
						t.Fatalf("decode replayed tool calls: %v", err)
					}
					if got := toolCalls[0].ExtraContent.Google.ThoughtSignature; got != "gemini-sig-1" {
						t.Errorf("replayed Gemini thought signature = %q, want exact signature", got)
					}
					if _, present := message["annotations"]; present {
						t.Errorf("replayed response-only annotations: %s", message["annotations"])
					}
					if got := string(message["audio"]); got != `{"id":"audio-1"}` {
						t.Errorf("replayed audio = %s, want request-safe ID only", got)
					}
					if _, present := message["refusal"]; present {
						t.Errorf("replayed null refusal instead of omitting it: %s", message["refusal"])
					}
					if _, present := message["function_call"]; present {
						t.Errorf("replayed null function_call instead of omitting it: %s", message["function_call"])
					}
				}
				if role == "tool" && toolIndex == -1 {
					toolIndex = i
				}
			}
			if assistantIndex == -1 || toolIndex == -1 || assistantIndex >= toolIndex {
				t.Fatalf("second request messages do not contain assistant turn before tool result: %s", bytes.TrimSpace(requests[1]))
			}
		})
	}
}

func TestRunPerFile_OpenAIStreamingReplaysLegacyFunctionCall(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
		if call == 1 {
			writeTestOpenAISSE(w,
				`{"id":"legacy-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","function_call":{"name":"file_","arguments":"{\"path\":"}},"finish_reason":null}]}`,
				`{"id":"legacy-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"function_call":{"name":"read","arguments":"\"main.go\"}"}},"finish_reason":null}]}`,
				`{"id":"legacy-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"function_call"}]}`,
			)
			return
		}
		writeTestOpenAISSE(w,
			`{"id":"legacy-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":null}]}`,
			`{"id":"legacy-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{
		URL: server.URL() + "/v1", APIKey: "test", Model: "fake",
		AssistantReplay: llm.AssistantReplayNative,
		ExtraBody:       map[string]any{"stream": true},
	})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	completed, _, err := NewRunner(deps).RunPerFile(
		context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	requests := server.Requests()
	if !completed || len(requests) != 2 {
		t.Fatalf("completed=%v requests=%d, want completion after legacy function retry", completed, len(requests))
	}
	messages := decodeOpenAIRequestMessages(t, requests[1])
	if got := string(messages[1]["function_call"]); got != `{"arguments":"{\"path\":\"main.go\"}","name":"file_read"}` {
		t.Fatalf("replayed legacy function_call = %s, want complete name and arguments", got)
	}
}

func TestRunPerFile_OpenAIReplaysAssistantOnNoToolRetry(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
		response := `{"id":"first","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"reason only"},"finish_reason":"stop"}]}`
		if call > 1 {
			response = `{"id":"second","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL() + "/v1", APIKey: "test", Model: "fake", AssistantReplay: llm.AssistantReplayNative})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	runner := NewRunner(deps)
	completed, _, err := runner.RunPerFile(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	requests := server.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(requests))
	}
	messages := decodeOpenAIRequestMessages(t, requests[1])
	if len(messages) != 3 {
		t.Fatalf("second request messages = %d, want initial user + assistant turn + retry user", len(messages))
	}
	var role string
	if err := json.Unmarshal(messages[1]["role"], &role); err != nil {
		t.Fatalf("decode replay role: %v", err)
	}
	if role != "assistant" {
		t.Fatalf("replay role = %q, want assistant", role)
	}
	if got := string(messages[1]["reasoning_content"]); got != `"reason only"` {
		t.Errorf("replayed reasoning_content = %s, want %q", got, "reason only")
	}
	if got := string(messages[1]["content"]); got == `"reason only"` {
		t.Errorf("reasoning-only response was flattened into content: %s", got)
	}
}

func TestRunPerFile_NoToolRetryHonorsCompressionLimit(t *testing.T) {
	reasoning := strings.Repeat("private reasoning ", 200)
	server := newRecordedOpenAIServer(t, func(call int, body []byte, w http.ResponseWriter) {
		if call > 1 {
			t.Errorf("provider received request %d after the replay turn exceeded the compression budget: %s", call, body)
		}
		response := fmt.Sprintf(`{"id":"first","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":%q},"finish_reason":"stop"}]}`, reasoning)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL() + "/v1", APIKey: "test", Model: "fake", AssistantReplay: llm.AssistantReplayNative})
	deps := newTestDeps(client)
	deps.Template.MaxTokens = 20
	deps.Template.MaxToolRequestTimes = 2
	deps.Template.MemoryCompressionTask = template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "Summarize {{context}}"}},
	}
	runner := NewRunner(deps)
	completed, stop, err := runner.RunPerFile(context.Background(), []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
	}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed || stop != StopCompression {
		t.Fatalf("completed=%v stop=%v, want StopCompression before retry", completed, stop)
	}
	if got := len(server.Requests()); got != 1 {
		t.Fatalf("provider requests = %d, want 1", got)
	}
}

func TestRunPerFile_EmptyNoToolResponseDoesNotReplayRoleOnlyAssistant(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
		response := `{"id":"first","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`
		if call > 1 {
			response = `{"id":"second","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	})
	defer server.Close()
	client := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL() + "/v1", APIKey: "test", Model: "fake", AssistantReplay: llm.AssistantReplayNative})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	runner := NewRunner(deps)
	completed, _, err := runner.RunPerFile(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	requests := server.Requests()
	if !completed || len(requests) != 2 {
		t.Fatalf("completed=%v requests=%d, want complete after two requests", completed, len(requests))
	}
	messages := decodeOpenAIRequestMessages(t, requests[1])
	if got := len(messages); got != 2 {
		t.Fatalf("empty no-tool replay messages = %d, want initial user + retry user", got)
	}
	var role string
	if err := json.Unmarshal(messages[1]["role"], &role); err != nil {
		t.Fatalf("decode retry role: %v", err)
	}
	if role != "user" {
		t.Fatalf("retry message role = %q, want user", role)
	}
}

func TestRunPerFile_OpenAIStreamingReplaysParallelToolTurn(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
		if call == 1 {
			writeTestOpenAISSE(w,
				`{"id":"stream-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"private ","reasoning_details":[{"index":0,"type":"reasoning.text","text":"indexed "},{"index":1,"type":"reasoning.encrypted","data":"ciphertext","signature":"sig-1"}],"vendor_state":{"nonce":"opaque"}},"finish_reason":null}]}`,
				`{"id":"stream-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"content":"  <think>inspect</think>  ","reasoning_content":"reasoning","reasoning_details":[{"index":0,"text":"reasoning"}],"tool_calls":[{"index":-1,"id":"call_1","type":"function","extra_content":{"google":{"thought_signature":"gemini-sig-1"}},"function":{"name":"file_read","arguments":"{\"path\":\""}},{"index":1,"id":"call_2","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":null}]}`,
				`{"id":"stream-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"vendor_trace":"trace-after-index-normalization","function":{"arguments":"main.go\"}"}}]},"finish_reason":null}]}`,
				`{"id":"stream-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		writeTestOpenAISSE(w,
			`{"id":"stream-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":null}]}`,
			`{"id":"stream-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{
		URL:             server.URL() + "/v1",
		APIKey:          "test",
		Model:           "fake",
		AssistantReplay: llm.AssistantReplayNative,
		ExtraBody:       map[string]any{"stream": true},
	})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	runner := NewRunner(deps)
	completed, _, err := runner.RunPerFile(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	requests := server.Requests()
	messages := decodeOpenAIRequestMessages(t, requests[1])
	if len(messages) != 4 {
		t.Fatalf("second request messages = %d, want user + assistant + 2 tool results", len(messages))
	}
	var assistant map[string]json.RawMessage
	for i := range messages {
		var role string
		_ = json.Unmarshal(messages[i]["role"], &role)
		if role == "assistant" {
			assistant = messages[i]
			if i != 1 {
				t.Fatalf("assistant replay index = %d, want 1", i)
			}
		}
	}
	if got := string(assistant["reasoning_content"]); got != `"private reasoning"` {
		t.Errorf("streamed reasoning_content = %s, want %q", got, "private reasoning")
	}
	if _, present := assistant["vendor_state"]; present {
		t.Errorf("streamed unknown vendor field must not be replayed: %s", assistant["vendor_state"])
	}
	var reasoningDetails []struct {
		Index     int    `json:"index"`
		Type      string `json:"type"`
		Text      string `json:"text"`
		Data      string `json:"data"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(assistant["reasoning_details"], &reasoningDetails); err != nil {
		t.Fatalf("decode streamed reasoning_details: %v", err)
	}
	if len(reasoningDetails) != 2 || reasoningDetails[0].Index != 0 ||
		reasoningDetails[0].Type != "reasoning.text" || reasoningDetails[0].Text != "indexed reasoning" ||
		reasoningDetails[1].Index != 1 || reasoningDetails[1].Type != "reasoning.encrypted" ||
		reasoningDetails[1].Data != "ciphertext" || reasoningDetails[1].Signature != "sig-1" {
		t.Errorf("streamed reasoning_details lost typed indexed state: %+v", reasoningDetails)
	}
	var streamedContent string
	if err := json.Unmarshal(assistant["content"], &streamedContent); err != nil {
		t.Fatalf("decode streamed visible content: %v", err)
	}
	if streamedContent != "  <think>inspect</think>  " {
		t.Errorf("streamed visible content = %q, want exact whitespace and think tags", streamedContent)
	}
	var calls []json.RawMessage
	if err := json.Unmarshal(assistant["tool_calls"], &calls); err != nil {
		t.Fatalf("decode replay tool_calls: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("replayed tool_calls = %d, want 2", len(calls))
	}
	for i, wantID := range []string{"call_1", "call_2"} {
		var call struct {
			ID           string `json:"id"`
			VendorTrace  string `json:"vendor_trace"`
			ExtraContent struct {
				Google struct {
					ThoughtSignature string `json:"thought_signature"`
				} `json:"google"`
			} `json:"extra_content"`
		}
		if err := json.Unmarshal(calls[i], &call); err != nil {
			t.Fatalf("decode call %d: %v", i, err)
		}
		if call.ID != wantID {
			t.Errorf("replayed call %d ID = %q, want %q", i, call.ID, wantID)
		}
		if i == 0 && call.ExtraContent.Google.ThoughtSignature != "gemini-sig-1" {
			t.Errorf("replayed call %d Gemini thought_signature = %q, want exact nested signature", i, call.ExtraContent.Google.ThoughtSignature)
		}
		if i == 0 && call.VendorTrace != "trace-after-index-normalization" {
			t.Errorf("replayed call %d vendor_trace = %q, want state from normalized index", i, call.VendorTrace)
		}
	}
	for i, wantID := range []string{"call_1", "call_2"} {
		var tool struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal(messages[i+2]["role"], &tool.Role); err != nil {
			t.Fatalf("decode tool %d role: %v", i, err)
		}
		if err := json.Unmarshal(messages[i+2]["tool_call_id"], &tool.ToolCallID); err != nil {
			t.Fatalf("decode tool %d ID: %v", i, err)
		}
		if tool.Role != "tool" || tool.ToolCallID != wantID {
			t.Errorf("tool %d = %+v, want tool result for %q", i, tool, wantID)
		}
	}
}

func TestRunPerFile_OpenAIStreamingReplaysMultiChunkRefusal(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
		if call == 1 {
			writeTestOpenAISSE(w,
				`{"id":"refusal-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","refusal":"cannot "},"finish_reason":null}]}`,
				`{"id":"refusal-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"refusal":"comply"},"finish_reason":null}]}`,
				`{"id":"refusal-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			)
			return
		}
		writeTestOpenAISSE(w,
			`{"id":"refusal-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":null}]}`,
			`{"id":"refusal-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{
		URL: server.URL() + "/v1", APIKey: "test", Model: "fake",
		AssistantReplay: llm.AssistantReplayNative,
		ExtraBody:       map[string]any{"stream": true},
	})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	completed, _, err := NewRunner(deps).RunPerFile(
		context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	requests := server.Requests()
	if !completed || len(requests) != 2 {
		t.Fatalf("completed=%v requests=%d, want completion after refusal retry", completed, len(requests))
	}
	messages := decodeOpenAIRequestMessages(t, requests[1])
	if got := string(messages[1]["refusal"]); got != `"cannot comply"` {
		t.Fatalf("replayed refusal = %s, want complete multi-chunk refusal", got)
	}
}

func TestRunPerFile_OpenAIStreamingToolOnlyReplayOmitsContent(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, body []byte, w http.ResponseWriter) {
		if call == 1 {
			writeTestOpenAISSE(w,
				`{"id":"tool-only-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"private","tool_calls":[{"index":0,"id":"call-read","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":null}]}`,
				`{"id":"tool-only-1","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		messages := decodeOpenAIRequestMessages(t, body)
		if _, present := messages[1]["content"]; present {
			http.Error(w, "text content is empty", http.StatusBadRequest)
			return
		}
		writeTestOpenAISSE(w,
			`{"id":"tool-only-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":null}]}`,
			`{"id":"tool-only-2","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{
		URL: server.URL() + "/v1", APIKey: "test", Model: "fake",
		AssistantReplay: llm.AssistantReplayNative,
		ExtraBody:       map[string]any{"stream": true},
	})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	completed, _, err := NewRunner(deps).RunPerFile(
		context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	requests := server.Requests()
	if !completed || len(requests) != 2 {
		t.Fatalf("completed=%v requests=%d, want completion after tool-only replay", completed, len(requests))
	}
	messages := decodeOpenAIRequestMessages(t, requests[1])
	if _, present := messages[1]["content"]; present {
		t.Fatalf("tool-only replay synthesized content: %s", messages[1]["content"])
	}
	if got := string(messages[1]["reasoning_content"]); got != `"private"` {
		t.Fatalf("tool-only replay reasoning_content = %s, want private", got)
	}
}

func TestRunPerFile_CompressionRetainsOpenAIReplayEnvelope(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, body []byte, w http.ResponseWriter) {
		hasTools, err := openAIRequestHasTools(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !hasTools {
			_, _ = io.WriteString(w, `{"id":"compression","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"compressed summary"},"finish_reason":"stop"}]}`)
			return
		}

		if call == 1 {
			_, _ = io.WriteString(w, `{"id":"first","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"inspect","reasoning_content":"private reasoning","vendor_state":{"nonce":"opaque"},"tool_calls":[{"id":"call-1","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":"tool_calls"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"second","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL() + "/v1", APIKey: "test", Model: "fake", AssistantReplay: llm.AssistantReplayNative})
	deps := newTestDeps(client)
	deps.MainToolDefs = []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:       "file_read",
			Parameters: map[string]any{"type": "object"},
		},
	}}
	deps.Template.MaxTokens = 120
	deps.Template.MaxToolRequestTimes = 2
	deps.Template.MemoryCompressionTask = template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "Summarize {{context}}"}},
	}
	runner := NewRunner(deps)
	oldCalls := []llm.ToolCall{{ID: "old-call", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{}`}}}
	completed, _, err := runner.RunPerFile(context.Background(), []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
		llm.NewToolCallMessage(strings.Repeat("old assistant ", 300), oldCalls),
		llm.NewToolResultMessage("old-call", strings.Repeat("old result ", 300)),
	}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	var mainRequests [][]byte
	for _, body := range server.Requests() {
		hasTools, err := openAIRequestHasTools(body)
		if err != nil {
			t.Fatalf("decode recorded request: %v", err)
		}
		if hasTools {
			mainRequests = append(mainRequests, body)
		}
	}
	if !completed || len(mainRequests) != 2 {
		t.Fatalf("completed=%v main requests=%d, want completion after compressed replay", completed, len(mainRequests))
	}
	messages := decodeOpenAIRequestMessages(t, mainRequests[1])
	var replay map[string]json.RawMessage
	for _, message := range messages {
		if string(message["reasoning_content"]) == `"private reasoning"` {
			replay = message
			break
		}
	}
	if replay == nil {
		t.Fatalf("compressed active history lost OpenAI replay envelope: %s", mainRequests[1])
	}
	if _, present := replay["vendor_state"]; present {
		t.Fatalf("compressed replay echoed unknown vendor field: %s", replay["vendor_state"])
	}
}

func writeTestOpenAISSE(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range chunks {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

type recordedOpenAIServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests [][]byte
}

func newRecordedOpenAIServer(t *testing.T, respond func(call int, body []byte, w http.ResponseWriter)) *recordedOpenAIServer {
	t.Helper()
	recorded := &recordedOpenAIServer{}
	recorded.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		recorded.mu.Lock()
		recorded.requests = append(recorded.requests, append([]byte(nil), body...))
		call := len(recorded.requests)
		recorded.mu.Unlock()
		respond(call, body, w)
	}))
	return recorded
}

func (s *recordedOpenAIServer) URL() string { return s.server.URL }
func (s *recordedOpenAIServer) Close()      { s.server.Close() }

func (s *recordedOpenAIServer) Requests() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	requests := make([][]byte, len(s.requests))
	for i, body := range s.requests {
		requests[i] = append([]byte(nil), body...)
	}
	return requests
}

func decodeOpenAIRequestMessages(t *testing.T, body []byte) []map[string]json.RawMessage {
	t.Helper()
	var request struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode OpenAI request: %v", err)
	}
	return request.Messages
}

func openAIRequestHasTools(body []byte) (bool, error) {
	var request struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return false, err
	}
	return len(request.Tools) > 0, nil
}

func TestRunPerFile_ContextCancelled(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunPerFile(ctx, msgs, "main.go")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
	if completed {
		t.Fatal("cancelled context should not complete RunPerFile")
	}
}

func TestRunPerFile_UnknownTool(t *testing.T) {
	content := ""
	unknownToolResp := &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_x",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "nonexistent_tool",
						Arguments: `{}`,
					},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 5},
	}
	client := &fakeClient{responses: []*llm.ChatResponse{unknownToolResp, taskDoneResponse()}}
	deps := newTestDeps(client)
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, _, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	if client.calls != 2 {
		t.Errorf("expected 2 calls, got %d", client.calls)
	}
}

// TestRunPerFile_NoToolRetryBoundedByCompression is a regression test: the
// no-tool retry path must share the compression bounds of tool rounds. A
// model that keeps answering without tool calls used to grow the
// conversation without any token check; the loop must instead stop with
// StopCompression once the conversation exceeds the warning threshold and
// compression cannot shrink it.
func TestRunPerFile_NoToolRetryBoundedByCompression(t *testing.T) {
	t_tempDir = t.TempDir()
	content := strings.Repeat("chatter ", 200)
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}}},
	}}
	// No MemoryCompressionTask: compression cannot shrink the conversation,
	// so crossing the warning threshold has to stop the loop.
	tpl := template.Template{MaxTokens: 100, MaxToolRequestTimes: 10}
	r := newTestRunner(client, tpl)

	completed, stop, err := r.RunPerFile(context.Background(), []llm.Message{msg("user", "review")}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("expected RunPerFile not to complete")
	}
	if stop != StopCompression {
		t.Errorf("stop = %v, want StopCompression", stop)
	}
	if client.calls != 1 {
		t.Errorf("LLM calls = %d, want 1 (loop must stop instead of retrying unbounded)", client.calls)
	}
}

func TestRunPerFile_MaxToolRequestsWithoutTaskDoneDoesNotComplete(t *testing.T) {
	content := ""
	client := &fakeClient{responses: []*llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
		Usage:   &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 5},
	}}}
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 1
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
	}
	if stop != StopMaxRounds {
		t.Fatalf("expected StopMaxRounds, got %v", stop)
	}
}

func TestRunPerFile_EmptyToolResultsStopWithEmptyRounds(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		fileReadToolCallResponse("call_2", `{"path":"main.go"}`),
		fileReadToolCallResponse("call_3", `{"path":"main.go"}`),
	}}
	deps := newTestDeps(client)
	reg := tool.NewRegistry()
	reg.Register(&fakeFileReadProvider{result: ""})
	deps.Tools = reg
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", "review")}
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
	}
	if stop != StopEmptyRounds {
		t.Fatalf("stop = %v, want StopEmptyRounds", stop)
	}
	if client.calls != 3 {
		t.Fatalf("LLM calls = %d, want 3 empty rounds", client.calls)
	}
}

func TestRunPerFile_UncompressibleContextStopsWithCompression(t *testing.T) {
	emptySummary := ""
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"path":"main.go"}`),
		{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &emptySummary}}},
			Model:   "fake",
		},
	}}
	deps := newTestDeps(client)
	deps.Template.MaxTokens = 20
	deps.Template.MemoryCompressionTask = template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "Summarize: {{context}}"}},
	}
	runner := NewRunner(deps)

	msgs := []llm.Message{llm.NewTextMessage("user", strings.Repeat("word ", 100))}
	completed, stop, err := runner.RunPerFile(context.Background(), msgs, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed {
		t.Fatal("RunPerFile completed without task_done")
	}
	if stop != StopCompression {
		t.Fatalf("stop = %v, want StopCompression", stop)
	}
	if client.calls != 2 {
		t.Fatalf("LLM calls = %d, want one main call and one compression call", client.calls)
	}
}

func TestRunner_RecordWarning(t *testing.T) {
	deps := newTestDeps(&fakeClient{})
	runner := NewRunner(deps)

	runner.RecordWarning("token_limit", "a.go", "approaching token limit")
	runner.RecordWarning("parse_error", "b.go", "invalid JSON")

	warnings := runner.Warnings()
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(warnings))
	}
	if warnings[0].Type != "token_limit" {
		t.Errorf("Type = %q", warnings[0].Type)
	}
	if warnings[1].File != "b.go" {
		t.Errorf("File = %q", warnings[1].File)
	}
}

func TestRunner_RecordUsage(t *testing.T) {
	deps := newTestDeps(&fakeClient{})
	runner := NewRunner(deps)

	runner.RecordUsage(&llm.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		CacheReadTokens:  20,
		CacheWriteTokens: 10,
	})
	runner.RecordUsage(nil)

	if runner.TotalInputTokens() != 100 {
		t.Errorf("input = %d", runner.TotalInputTokens())
	}
	if runner.TotalOutputTokens() != 50 {
		t.Errorf("output = %d", runner.TotalOutputTokens())
	}
	if runner.TotalCacheReadTokens() != 20 {
		t.Errorf("cache read = %d", runner.TotalCacheReadTokens())
	}
	if runner.TotalCacheWriteTokens() != 10 {
		t.Errorf("cache write = %d", runner.TotalCacheWriteTokens())
	}
	if runner.TotalTokensUsed() != 150 {
		t.Errorf("total = %d", runner.TotalTokensUsed())
	}
}

// argsCapturingProvider records the args map Execute receives, so tests can
// assert the runner never hands tools a nil map.
type argsCapturingProvider struct {
	tool     tool.Tool
	gotArgs  map[string]any
	captured bool
}

func (p *argsCapturingProvider) Tool() tool.Tool { return p.tool }
func (p *argsCapturingProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	p.gotArgs = args
	p.captured = true
	return "ok", nil
}

func TestExecuteToolCall_ArgumentsEdgeCases(t *testing.T) {
	// Regression for #382: some OpenAI-compatible gateways emit
	// "arguments": null; json.Unmarshal("null", &m) leaves m nil, and the
	// code_comment path override then panicked with "assignment to entry
	// in nil map".
	tests := []struct {
		name           string
		toolName       string
		arguments      string
		wantContains   string // substring expected in cp.Data ("" = skip)
		wantComment    string // if non-empty, expect one collected comment with this path
		wantNonNilArgs bool   // dynamic tool: Execute must receive a non-nil args map
	}{
		{
			name:         "null args on code_comment (issue #382)",
			toolName:     "code_comment",
			arguments:    `null`,
			wantContains: "'comments' array is required",
		},
		{
			name:         "empty object on code_comment",
			toolName:     "code_comment",
			arguments:    `{}`,
			wantContains: "'comments' array is required",
		},
		{
			name:        "valid args keeps path override",
			toolName:    "code_comment",
			arguments:   `{"path":"hallucinated.go","comments":[{"content":"issue","existing_code":"foo"}]}`,
			wantComment: "file.go",
		},
		{
			name:         "empty string args",
			toolName:     "code_comment",
			arguments:    ``,
			wantContains: "Error parsing tool arguments",
		},
		{
			name:         "malformed json args",
			toolName:     "code_comment",
			arguments:    `{"comments":`,
			wantContains: "Error parsing tool arguments",
		},
		{
			name:           "null args on dynamic tool",
			toolName:       "dyn_echo",
			arguments:      `null`,
			wantNonNilArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := tool.NewCommentCollector()
			dyn := &argsCapturingProvider{tool: tool.Dynamic("dyn_echo")}
			reg := tool.NewRegistry()
			reg.Register(&tool.CodeCommentProvider{Collector: collector})
			reg.Register(dyn)
			reg.Freeze()

			r := NewRunner(Deps{
				Tools:            reg,
				CommentCollector: collector,
			})

			cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
				Function: llm.FunctionCall{
					Name:      tt.toolName,
					Arguments: tt.arguments,
				},
			}, nil, "")

			if tt.wantContains != "" && !strings.Contains(cp.Data, tt.wantContains) {
				t.Errorf("cp.Data = %q, want substring %q", cp.Data, tt.wantContains)
			}
			if tt.wantComment != "" {
				comments := collector.Comments()
				if len(comments) != 1 {
					t.Fatalf("expected 1 comment, got %d", len(comments))
				}
				if comments[0].Path != tt.wantComment {
					t.Errorf("comment path = %q, want %q", comments[0].Path, tt.wantComment)
				}
			}
			if tt.wantNonNilArgs {
				if !dyn.captured {
					t.Fatal("dynamic tool Execute was not called")
				}
				if dyn.gotArgs == nil {
					t.Error("dynamic tool Execute received nil args map, want non-nil empty map")
				}
			}
		})
	}
}

func TestExecuteToolCall_CodeCommentOverridesHallucinatedPath(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{
		Tools:            reg,
		CommentCollector: collector,
	})

	args := map[string]any{
		"path": "wrong.go",
		"comments": []any{
			map[string]any{
				"content":       "issue",
				"existing_code": "foo",
			},
		},
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}

	cp := r.executeToolCall(context.Background(), "correct.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      "code_comment",
			Arguments: string(argsJSON),
		},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("unexpected result: %+v", cp)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Path != "correct.go" {
		t.Errorf("path override: got %q, want %q", comments[0].Path, "correct.go")
	}
}

// TestRunPerFile_OpenAIDefaultRebuildsNormalizedAssistantHistory pins the
// default (flag-off) contract: without an assistant_replay opt-in the second
// request rebuilds the assistant turn from the normalized projection exactly
// as before — reasoning substituted into visible content, tool calls reduced
// to id/type/function, and no provider response fields echoed back.
func TestRunPerFile_OpenAIDefaultRebuildsNormalizedAssistantHistory(t *testing.T) {
	server := newRecordedOpenAIServer(t, func(call int, _ []byte, w http.ResponseWriter) {
		response := `{"id":"first","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"","reasoning_content":"private reasoning","annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":1,"title":"source","url":"https://example.com"}}],"audio":{"id":"audio-1","data":"response-only","expires_at":123,"transcript":"response-only"},"vendor_state":{"nonce":"opaque"},"tool_calls":[{"id":"call_1","type":"function","extra_content":{"google":{"thought_signature":"gemini-sig-1"}},"function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}]},"finish_reason":"tool_calls"}]}`
		if call > 1 {
			response = `{"id":"second","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_2","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	})
	defer server.Close()

	client := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL() + "/v1", APIKey: "test", Model: "fake"})
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 2
	runner := NewRunner(deps)
	completed, _, err := runner.RunPerFile(context.Background(), []llm.Message{llm.NewTextMessage("user", "review")}, "main.go")
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunPerFile")
	}
	requests := server.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(requests))
	}

	messages := decodeOpenAIRequestMessages(t, requests[1])
	assistantIndex := -1
	for i, message := range messages {
		var role string
		if err := json.Unmarshal(message["role"], &role); err != nil {
			t.Fatalf("decode message %d role: %v", i, err)
		}
		if role != "assistant" || assistantIndex != -1 {
			continue
		}
		assistantIndex = i
		for _, field := range []string{"reasoning_content", "vendor_state", "audio", "annotations", "refusal", "function_call"} {
			if value, present := message[field]; present {
				t.Errorf("default rebuild echoed response field %s = %s", field, value)
			}
		}
		if got := string(message["content"]); got != `"private reasoning"` {
			t.Errorf("rebuilt content = %s, want reasoning substitution %q", got, `"private reasoning"`)
		}
		var toolCalls []map[string]json.RawMessage
		if err := json.Unmarshal(message["tool_calls"], &toolCalls); err != nil {
			t.Fatalf("decode rebuilt tool calls: %v", err)
		}
		if len(toolCalls) != 1 {
			t.Fatalf("rebuilt tool calls = %d, want 1", len(toolCalls))
		}
		if _, present := toolCalls[0]["extra_content"]; present {
			t.Errorf("rebuilt tool call kept vendor extension: %s", message["tool_calls"])
		}
		if got := string(toolCalls[0]["id"]); got != `"call_1"` {
			t.Errorf("rebuilt tool call id = %s, want %q", got, "call_1")
		}
		var function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(toolCalls[0]["function"], &function); err != nil {
			t.Fatalf("decode rebuilt function: %v", err)
		}
		if function.Name != "file_read" || function.Arguments != `{"path":"main.go"}` {
			t.Errorf("rebuilt function = %+v, want file_read with original arguments", function)
		}
	}
	if assistantIndex == -1 {
		t.Fatalf("second request has no assistant message: %s", bytes.TrimSpace(requests[1]))
	}
}
