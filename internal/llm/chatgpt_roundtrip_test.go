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
	"net/url"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

func fixtureChatGPTClient(t *testing.T, handler http.HandlerFunc) *OpenAIResponsesClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	c := NewOpenAIResponsesClient(ClientConfig{URL: chatgptauth.Resource, ChatGPTPlan: true, RequiresStreaming: true})
	c.chatGPTToken = func(context.Context) (string, error) { return "access-fixture", nil }
	c.sdk = openai.NewClient(option.WithBaseURL(chatgptauth.Resource), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: resolverRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		r := req.Clone(req.Context())
		u := *req.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		r.URL = &u
		return http.DefaultTransport.RoundTrip(r)
	})}))
	return c
}
func TestChatGPTNativeToolHTTPRoundTrip(t *testing.T) {
	turns := 0
	c := fixtureChatGPTClient(t, func(w http.ResponseWriter, r *http.Request) {
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Error(err)
		}
		var body map[string]any
		json.Unmarshal(raw, &body)
		for _, key := range []string{"background", "conversation", "max_output_tokens", "max_tool_calls", "metadata", "moderation", "multi_agent", "prompt", "prompt_cache_retention", "safety_identifier", "temperature", "top_logprobs", "top_p", "truncation", "user", "previous_response_id"} {
			if _, ok := body[key]; ok {
				t.Error("unsupported field", key)
			}
		}
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer access-fixture" {
			t.Error(r.URL, r.Header)
		}
		if body["tools"].([]any)[0].(map[string]any)["type"] != "namespace" {
			t.Error(string(raw))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		turns++
		if turns == 1 {
			fmt.Fprint(w, "event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"reasoning\",\"id\":\"rs\",\"summary\":[],\"encrypted_content\":\"ciphertext\"}}\n\n")
			fmt.Fprint(w, "event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":1,\"item\":{\"type\":\"function_call\",\"namespace\":\"ocr\",\"name\":\"file_read\",\"call_id\":\"call\",\"id\":\"fc\",\"arguments\":\"{\\\"path\\\":\\\"main.go\\\"}\",\"status\":\"completed\"}}\n\n")
			fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"model\":\"gpt-6-luna\",\"output\":[],\"usage\":{\"input_tokens\":11,\"output_tokens\":12,\"total_tokens\":23,\"output_tokens_details\":{\"reasoning_tokens\":7}}}}\n\n")
		} else {
			if !strings.Contains(string(raw), `"namespace":"ocr"`) || !strings.Contains(string(raw), `"encrypted_content":"ciphertext"`) || !strings.Contains(string(raw), `"type":"function_call_output"`) || !strings.Contains(string(raw), "package main") {
				t.Error("replay lost", string(raw))
			}
			fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r2\",\"status\":\"completed\",\"model\":\"gpt-6-luna\",\"output\":[{\"type\":\"message\",\"id\":\"m\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"Reviewed\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":14,\"output_tokens\":3,\"total_tokens\":17}}}\n\n")
		}
	})
	messages := []Message{NewTextMessage("system", "review policy"), NewTextMessage("developer", "local tools only"), NewTextMessage("user", "review main.go")}
	tools := []ToolDef{{Type: "function", Function: FunctionDef{Name: "file_read", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}}
	req := ChatRequest{Model: "gpt-6-luna", Messages: messages, Tools: tools}
	first, err := c.CompletionsWithCtx(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	calls := first.ToolCalls()
	if len(calls) != 1 || calls[0].Function.Name != "file_read" || calls[0].ID != "call" {
		t.Fatal(calls)
	}
	if !strings.Contains(string(first.RawUsage), `"reasoning_tokens":7`) {
		t.Fatal(string(first.RawUsage))
	}
	req.Messages = append(req.Messages, NewToolCallMessage(first.VisibleContent(), calls, first.Native(), first.ReasoningContent()), NewToolResultMessage("call", "package main"))
	final, err := c.CompletionsWithCtx(context.Background(), req)
	if err != nil || final.Content() != "Reviewed" || turns != 2 {
		t.Fatal(final, err, turns)
	}
}
func TestChatGPTLateFailureAndWrongNamespace(t *testing.T) {
	for _, scenario := range []string{"late-failure", "wrong-status", "namespace", "incomplete-param"} {
		t.Run(scenario, func(t *testing.T) {
			c := fixtureChatGPTClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				switch scenario {
				case "late-failure":
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n")
					fmt.Fprint(w, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"subscription_sharing_usage_unavailable\",\"message\":\"unavailable\",\"param\":\"usage\"}}}\n\n")
				case "wrong-status":
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\",\"output\":[]}}\n\n")
				case "namespace":
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"function_call\",\"name\":\"file_read\",\"namespace\":\"unknown\",\"call_id\":\"call\",\"arguments\":\"{}\"}]}}\n\n")
				case "incomplete-param":
					fmt.Fprint(w, "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"content_filter\"}}}\n\n")
				}
			})
			_, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Model: "gpt-6-luna", Messages: []Message{NewTextMessage("user", "test")}})
			if err == nil {
				t.Fatal("failed stream accepted")
			}
			if scenario == "late-failure" && (!strings.Contains(err.Error(), "subscription_sharing_usage_unavailable") || !strings.Contains(err.Error(), "param=usage")) {
				t.Fatal(err)
			}
			if scenario == "incomplete-param" && !strings.Contains(err.Error(), "content_filter") {
				t.Fatal(err)
			}
		})
	}
}
func TestChatGPTFallbackCallAndCredentialFailure(t *testing.T) {
	c := NewOpenAIResponsesClient(ClientConfig{URL: chatgptauth.Resource, ChatGPTPlan: true, RequiresStreaming: true})
	c.chatGPTToken = func(context.Context) (string, error) { return "", errors.New("signed out") }
	if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("credential failure swallowed")
	}
	p := c.buildResponsesParams("gpt-6-luna", ChatRequest{Messages: []Message{NewToolCallMessage("", []ToolCall{{ID: "call", Function: FunctionCall{Name: "file_read", Arguments: "{}"}}}, NativeTurn{}, "")}})
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"namespace":"ocr"`) {
		t.Fatal(string(b))
	}
	old, oldModels := chatGPTCredentials, chatGPTModels
	defer func() { chatGPTCredentials, chatGPTModels = old, oldModels }()
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) {
		return &chatgptauth.Auth{ClientID: "fixture"}, nil
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		return []chatgptauth.Model{{Slug: "gpt-6-luna", DisplayName: "Luna"}}, nil
	}
	if m, err := ChatGPTModels(context.Background()); err != nil || len(m) != 1 {
		t.Fatal(m, err)
	}
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) { return nil, errors.New("fixture") }
	if _, err := ChatGPTModels(context.Background()); err == nil {
		t.Fatal("model credential failure")
	}
}
func TestChatGPTCapabilityStatus(t *testing.T) {
	var response responses.Response
	if err := response.UnmarshalJSON([]byte(`{"status":"incomplete"}`)); err != nil {
		t.Fatal(err)
	}
	if err := checkResponseStatus(&response); err != nil {
		t.Fatal("ordinary Responses incomplete behavior changed")
	}
}
