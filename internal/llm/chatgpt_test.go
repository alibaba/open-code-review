// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgpt"
)

func TestChatGPTResolverRequiresManagedOAuth(t *testing.T) {
	clearAllEnv(t)
	preset, _ := LookupProvider(chatgpt.ProviderName)
	cfg := configFile{Provider: chatgpt.ProviderName, Model: "account-model"}
	path := writeConfigJSON(t, cfg)
	ep, err := ResolveEndpoint(path)
	if err != nil || !ep.ChatGPT || ep.Token != "" || ep.Protocol != ProtocolOpenAIResponses {
		t.Fatalf("endpoint: %+v %v", ep, err)
	}
	for _, entry := range []providerEntryConfig{{APIKey: "static"}, {APIKeyCmd: "echo secret"}, {URL: "https://other.test"}, {Protocol: "openai"}, {AuthHeader: "other"}, {ExtraHeaders: map[string]string{"Authorization": "secret"}}, {ExtraBody: map[string]any{"stream": false}}, {TimeoutSec: -1}} {
		cfg.Providers = map[string]providerEntryConfig{chatgpt.ProviderName: entry}
		if _, _, err := resolveChatGPTProvider(cfg, "", preset); err == nil {
			t.Fatalf("unsafe override accepted: %+v", entry)
		}
	}
	cfg.Providers = nil
	cfg.Model = ""
	if _, _, err := resolveChatGPTProvider(cfg, "", preset); err == nil {
		t.Fatal("empty model accepted")
	}
	cfg.Providers = map[string]providerEntryConfig{chatgpt.ProviderName: {Model: "configured", ExtraBody: map[string]any{"reasoning": map[string]any{"effort": "low"}}}}
	ep, _, err = resolveChatGPTProvider(cfg, "new-account-model", preset)
	if err != nil || ep.Model != "new-account-model" {
		t.Fatalf("override should not use a stale catalog: %+v %v", ep, err)
	}
	client := NewLLMClient(ep, nil, nil)
	if _, ok := client.(*OpenAIResponsesClient); !ok {
		t.Fatalf("wrong client: %T", client)
	}
}

func TestChatGPTStreamingToolRoundTrip(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != fmt.Sprintf("Bearer token-%d", call) {
			t.Error("wrong endpoint or token was not obtained for each request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["stream"] != true || body["store"] != false {
			t.Error("subscription inference requires streaming and disabled storage")
		}
		for _, field := range []string{"temperature", "max_output_tokens", "previous_response_id"} {
			if _, ok := body[field]; ok {
				t.Errorf("unsupported field %s", field)
			}
		}
		tools := body["tools"].([]any)
		if tools[0].(map[string]any)["type"] != "namespace" {
			t.Error("tools must be namespaced")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			_, _ = fmt.Fprint(w, "event: response.completed\ndata: "+`{"type":"response.completed","response":{"id":"resp_1","status":"completed","model":"account-model","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","namespace":"ocr","arguments":"{}"}],"usage":{"input_tokens":7,"output_tokens":2}}}`+"\n\n")
		} else {
			input, _ := json.Marshal(body["input"])
			if !strings.Contains(string(input), `"call_id":"call_1"`) || !strings.Contains(string(input), `"type":"function_call_output"`) || !strings.Contains(string(input), `"namespace":"ocr"`) {
				t.Errorf("tool history was not replayed: %s", input)
			}
			_, _ = fmt.Fprint(w, "event: response.output_text.delta\ndata: "+`{"type":"response.output_text.delta","delta":"partial"}`+"\n\nevent: response.completed\ndata: "+`{"type":"response.completed","response":{"id":"resp_2","status":"completed","model":"account-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Review complete."}]}]}}`+"\n\n")
		}
	}))
	defer server.Close()
	var tokenCalls atomic.Int32
	c := NewOpenAIResponsesClient(ClientConfig{URL: server.URL + "/v1", ChatGPT: true, tokenSource: func(context.Context) (string, error) { return fmt.Sprintf("token-%d", tokenCalls.Add(1)), nil }})
	temperature := 0.5
	req := ChatRequest{Model: "account-model", MaxTokens: 2048, Temperature: &temperature, Tools: []ToolDef{{Type: "function", Function: FunctionDef{Name: "read_file", Parameters: map[string]any{"type": "object"}}}}, Messages: []Message{{Role: "system", Content: "Review code"}, {Role: "user", Content: "Read a file"}}}
	first, err := c.CompletionsWithCtx(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ToolCalls()) != 1 || first.ToolCalls()[0].Function.Name != "read_file" {
		t.Fatal("completed tool call was lost")
	}
	req.Messages = append(req.Messages, NewToolCallMessage(first.VisibleContent(), first.ToolCalls(), first.Native(), first.ReasoningContent()), NewToolResultMessage("call_1", "file contents"))
	second, err := c.CompletionsWithCtx(context.Background(), req)
	if err != nil || second.Content() != "Review complete." {
		t.Fatalf("final response: %v %v", second, err)
	}
}

func TestChatGPTStreamingFinalizedItemsWithoutTerminalOutput(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"reasoning_1","summary":[],"encrypted_content":"partial"}}`+"\n\n")
			_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"function_1","call_id":"call_1","name":"read_file","namespace":"ocr","arguments":"{\"path\":\"file.go\"}"}}`+"\n\n")
			_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"reasoning_1","summary":[],"encrypted_content":"final-encrypted"}}`+"\n\n")
		} else {
			input, _ := json.Marshal(body["input"])
			if !strings.Contains(string(input), `"encrypted_content":"final-encrypted"`) || strings.Contains(string(input), `"encrypted_content":"partial"`) || !strings.Contains(string(input), `"call_id":"call_1"`) {
				t.Errorf("final reasoning/tool history lost: %s", input)
			}
			_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_text.delta","delta":"partial"}`+"\n\n")
			_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"message_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Review complete."}]}}`+"\n\n")
		}
		_, _ = fmt.Fprint(w, "data: "+`{"type":"response.completed","response":{"id":"response_1","status":"completed","model":"account-model","output":[],"usage":{"input_tokens":7,"output_tokens":2}}}`+"\n\n")
	}))
	defer server.Close()
	c := NewOpenAIResponsesClient(ClientConfig{URL: server.URL, ChatGPT: true, tokenSource: func(context.Context) (string, error) { return "test-token", nil }})
	req := ChatRequest{Messages: []Message{{Role: "user", Content: "Read a file"}}}
	first, err := c.CompletionsWithCtx(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ToolCalls()) != 1 || first.ToolCalls()[0].Function.Arguments != `{"path":"file.go"}` {
		t.Fatal("finalized function call was discarded")
	}
	if first.Usage == nil || first.Usage.PromptTokens != 7 {
		t.Fatal("terminal response usage was discarded")
	}
	req.Messages = append(req.Messages, NewToolCallMessage(first.VisibleContent(), first.ToolCalls(), first.Native(), first.ReasoningContent()), NewToolResultMessage("call_1", "file contents"))
	second, err := c.CompletionsWithCtx(context.Background(), req)
	if err != nil || second.VisibleContent() != "Review complete." {
		t.Fatalf("finalized text lost: %+v %v", second, err)
	}
}

func TestChatGPTStreamingRejectsInvalidOutputOrdering(t *testing.T) {
	for _, index := range []int{-1, 2} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_item.done\",\"output_index\":%d,\"item\":{\"type\":\"message\"}}\n\n", index)
				_, _ = fmt.Fprint(w, "data: "+`{"type":"response.completed","response":{"id":"response_1","status":"completed","output":[]}}`+"\n\n")
			}))
			defer server.Close()
			c := NewOpenAIResponsesClient(ClientConfig{URL: server.URL, ChatGPT: true, tokenSource: func(context.Context) (string, error) { return "test-token", nil }})
			if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{}); err == nil {
				t.Fatal("invalid output ordering accepted")
			}
		})
	}
}

func TestChatGPTRejectsIncompleteStreamsAndQuotaRetry(t *testing.T) {
	for _, test := range []string{"eof", "failed", "incomplete", "invalid_completed", "empty_completed", "partial_item", "malformed", "quota", "ineligible", "invalid_user"} {
		t.Run(test, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				switch test {
				case "quota", "ineligible", "invalid_user":
					code := "subscription_sharing_usage_limit_exceeded"
					status := 429
					if test == "ineligible" {
						code = "subscription_sharing_user_not_eligible"
						status = 403
					}
					if test == "invalid_user" {
						code = "subscription_sharing_invalid_user"
						status = 401
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"cannot infer","type":"usage_error"}}`, code)
				default:
					w.Header().Set("Content-Type", "text/event-stream")
					if test == "eof" {
						_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_text.delta","delta":"partial"}`+"\n\n")
						return
					}
					if test == "malformed" {
						_, _ = fmt.Fprint(w, "data: invalid\n\n")
						return
					}
					if test == "empty_completed" || test == "partial_item" {
						if test == "partial_item" {
							_, _ = fmt.Fprint(w, "data: "+`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","arguments":"partial"}}`+"\n\n")
						}
						_, _ = fmt.Fprint(w, "data: "+`{"type":"response.completed","response":{"id":"response_1","status":"completed","output":[]}}`+"\n\n")
						return
					}
					if test == "invalid_completed" {
						_, _ = fmt.Fprint(w, "data: "+`{"type":"response.completed","response":{"status":"failed"}}`+"\n\n")
						return
					}
					_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.%s\"}\n\n", test)
				}
			}))
			defer server.Close()
			c := NewOpenAIResponsesClient(ClientConfig{URL: server.URL, ChatGPT: true, tokenSource: func(context.Context) (string, error) { return "test-token", nil }})
			resp, err := c.CompletionsWithCtx(context.Background(), ChatRequest{})
			if err == nil || resp != nil {
				t.Fatal("failed/incomplete stream was treated as success")
			}
			if attempts.Load() != 1 {
				t.Fatal("subscription error was retried automatically")
			}
			if test == "quota" && !strings.Contains(err.Error(), chatgpt.UsageURL) {
				t.Fatal("quota error needs a usage action")
			}
		})
	}
}

func TestChatGPTTokenErrorsDoNotSendInference(t *testing.T) {
	for _, source := range []func(context.Context) (string, error){nil, func(context.Context) (string, error) { return "", errors.New("not signed in") }} {
		c := NewOpenAIResponsesClient(ClientConfig{URL: "https://unused.example.test", ChatGPT: true, tokenSource: source})
		if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{}); err == nil {
			t.Fatal("missing credentials accepted")
		}
	}
}

func TestChatGPTRequestUsesContextSessionKeyAndPlanScopes(t *testing.T) {
	t.Setenv("OPENAI_ORG_ID", "org-platform")
	t.Setenv("OPENAI_PROJECT_ID", "proj-platform")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("OpenAI-Organization") != "" || r.Header.Get("OpenAI-Project") != "" {
			t.Error("platform organization or project was sent with a plan token")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["prompt_cache_key"] != "file-session" {
			t.Errorf("prompt_cache_key = %v, want the request's session key", body["prompt_cache_key"])
		}
		input, _ := json.Marshal(body["input"])
		if !strings.Contains(string(input), `"namespace":"ocr"`) {
			t.Errorf("replayed call lost its tool namespace: %s", input)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: "+`{"type":"response.completed","response":{"id":"r","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}}`+"\n\n")
	}))
	defer server.Close()
	c := NewOpenAIResponsesClient(ClientConfig{URL: server.URL, ChatGPT: true, ExtraBody: map[string]any{"prompt_cache_key": SessionKeyTemplateVar},
		tokenSource: func(context.Context) (string, error) { return "test-token", nil }})
	req := ChatRequest{Messages: []Message{
		{Role: "user", Content: "Read a file"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Function: FunctionCall{Name: "read_file", Arguments: "{}"}}}},
		NewToolResultMessage("call_1", "contents"),
	}}
	if _, err := c.CompletionsWithCtx(ContextWithSessionKey(context.Background(), "file-session"), req); err != nil {
		t.Fatal(err)
	}
}

func TestChatGPTStreamFailureKeepsProviderReason(t *testing.T) {
	for _, test := range []struct{ event, want string }{
		{`{"type":"error","code":"subscription_sharing_usage_limit_exceeded","message":"limit reached","param":null,"sequence_number":1}`, chatgpt.UsageURL},
		{`{"type":"response.failed","response":{"id":"r","status":"failed","error":{"code":"server_error","message":"upstream down"}}}`, "server_error: upstream down"},
		{`{"type":"response.incomplete","response":{"id":"r","status":"incomplete","incomplete_details":{"reason":"content_filter"}}}`, "content_filter"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: "+test.event+"\n\n")
		}))
		c := NewOpenAIResponsesClient(ClientConfig{URL: server.URL, ChatGPT: true, tokenSource: func(context.Context) (string, error) { return "test-token", nil }})
		_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{})
		server.Close()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("stream failure %s: got %v, want it to mention %q", test.event, err, test.want)
		}
	}
}
