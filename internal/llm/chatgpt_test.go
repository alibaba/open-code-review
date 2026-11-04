// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/openai/openai-go/v3/responses"
)

func TestChatGPTRequestNamespaceAndReplay(t *testing.T) {
	c := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1", ChatGPTPlan: true, RequiresStreaming: true, RejectsSamplingParams: true})
	temperature := 0.7
	params := c.buildResponsesParams("gpt-6-luna", ChatRequest{Messages: []Message{NewTextMessage("system", "policy"), NewTextMessage("user", "hello")}, Tools: []ToolDef{{Type: "function", Function: FunctionDef{Name: "file_read", Description: "Read", Parameters: map[string]any{"type": "object"}}}}, MaxTokens: 99, Temperature: &temperature, ToolChoice: "required"})
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal(b, &body)
	if body["instructions"] != "policy" || body["store"] != false {
		t.Fatal(string(b))
	}
	tools := body["tools"].([]any)
	namespace := tools[0].(map[string]any)
	if namespace["type"] != "namespace" || namespace["name"] != "ocr" {
		t.Fatal(string(b))
	}
	if namespace["tools"].([]any)[0].(map[string]any)["name"] != "file_read" {
		t.Fatal(string(b))
	}
	if body["max_output_tokens"] != nil || body["temperature"] != nil {
		t.Fatal(string(b))
	}
	var sdk responses.Response
	err = sdk.UnmarshalJSON([]byte(`{"id":"r","model":"gpt-6-luna","status":"completed","output":[{"type":"reasoning","id":"rs","summary":[],"encrypted_content":"opaque"},{"type":"function_call","namespace":"ocr","name":"file_read","call_id":"call","id":"fc","arguments":"{}","status":"completed"}],"usage":{"input_tokens":15,"output_tokens":4,"total_tokens":19,"output_tokens_details":{"reasoning_tokens":2}}}`))
	if err != nil {
		t.Fatal(err)
	}
	out := c.mapResponsesResponse(&sdk)
	if out.ToolCalls()[0].Function.Name != "file_read" || out.Usage == nil {
		t.Fatal(out)
	}
	replay := c.buildResponsesParams("gpt-6-luna", ChatRequest{Messages: []Message{NewTextMessage("user", "hello"), NewToolCallMessage("", out.ToolCalls(), out.Native(), ""), NewToolResultMessage("call", "done")}})
	b, _ = json.Marshal(replay)
	if !strings.Contains(string(b), `"namespace":"ocr"`) || !strings.Contains(string(b), `"encrypted_content":"opaque"`) || !strings.Contains(string(b), `"call_id":"call"`) {
		t.Fatal(string(b))
	}
}
func TestChatGPTStreamTerminal(t *testing.T) {
	for _, kind := range []string{"completed", "failed", "incomplete", "error", "interrupted"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["stream"] != true || body["store"] != false || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error(body, r.Header)
				}
				if body["reasoning"].(map[string]any)["effort"] != "low" {
					t.Error(body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
				if kind == "interrupted" {
					return
				}
				if kind == "error" {
					fmt.Fprint(w, "event: error\ndata: {\"type\":\"error\",\"code\":\"subscription_sharing_usage_limit_exceeded\",\"message\":\"limit\"}\n\n")
					return
				}
				fmt.Fprintf(w, "event: response.%s\ndata: {\"type\":\"response.%s\",\"response\":{\"id\":\"r\",\"model\":\"gpt-6-luna\",\"status\":%q,\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\",\"message\":\"limit\"},\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"id\":\"m\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\",\"annotations\":[]}],\"status\":\"completed\"}],\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n", kind, kind, kind)
			}))
			defer server.Close()
			c := NewOpenAIResponsesClient(ClientConfig{URL: chatgptauth.Resource, APIKey: "fixture", ChatGPTPlan: true, RequiresStreaming: true, RejectsSamplingParams: true, ExtraBody: map[string]any{"reasoning": map[string]any{"effort": "low"}}})
			c.chatGPTToken = func(context.Context) (string, error) { return "fixture", nil }
			target, _ := url.Parse(server.URL)
			c.sdk = openai.NewClient(option.WithBaseURL(chatgptauth.Resource), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: resolverRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				copy := req.Clone(req.Context())
				u := *req.URL
				u.Scheme = target.Scheme
				u.Host = target.Host
				copy.URL = &u
				return http.DefaultTransport.RoundTrip(copy)
			})}))
			out, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Model: "gpt-6-luna", Messages: []Message{NewTextMessage("user", "hello")}})
			if kind == "completed" {
				if err != nil || out.Content() != "ok" || out.Usage.TotalTokens != 3 {
					t.Fatal(out, err)
				}
			} else if err == nil {
				t.Fatal("non-completion accepted")
			} else if kind == "failed" && !strings.Contains(err.Error(), "subscription_sharing_usage_limit_exceeded") {
				t.Fatal(err)
			}
		})
	}
}
func TestChatGPTRejectsOverrides(t *testing.T) {
	for _, key := range []string{"temperature", "store", "stream", "previous_response_id", "input", "tools", "metadata", "reasoning.effort", "max_output_tokens", "background"} {
		c := NewOpenAIResponsesClient(ClientConfig{URL: chatgptauth.Resource, ChatGPTPlan: true, ExtraBody: map[string]any{key: true}})
		if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Model: "gpt-6-luna"}); err == nil {
			t.Fatal(key)
		}
	}
	c := NewOpenAIResponsesClient(ClientConfig{URL: "https://evil.invalid/v1", ChatGPTPlan: true})
	if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{Model: "gpt-6-luna"}); err == nil {
		t.Fatal("override accepted")
	}
}
func TestChatGPTResolveEndpoint(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	path, _ := writeResolverConfig(t, configFile{Provider: "chatgpt", Providers: map[string]providerEntryConfig{"chatgpt": {Model: "gpt-6-luna"}}})
	old := chatGPTCredentials
	oldModels := chatGPTModels
	defer func() { chatGPTCredentials = old; chatGPTModels = oldModels }()
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) {
		return &chatgptauth.Auth{ClientID: "oaiapp", AccessToken: "fixture"}, nil
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		return []chatgptauth.Model{{Slug: "gpt-6-luna", DisplayName: "Luna", Visibility: "list"}}, nil
	}
	ep, err := ResolveEndpointWithOptions(path, ResolveOptions{Model: "gpt-6-luna"})
	if err != nil || !ep.ChatGPTPlan || ep.URL != chatgptauth.Resource || ep.Token != "fixture" {
		t.Fatal(ep, err)
	}
	for _, entry := range []providerEntryConfig{{URL: "https://evil.invalid"}, {Protocol: "openai"}, {APIKey: "key"}, {APIKeyCmd: "exit 1"}, {ExtraHeaders: map[string]string{"Authorization": "bad"}}} {
		p, _ := writeResolverConfig(t, configFile{Provider: "chatgpt", Providers: map[string]providerEntryConfig{"chatgpt": entry}})
		if _, err := ResolveEndpoint(p); err == nil {
			t.Fatal(entry)
		}
	}
	chatGPTModels = func(context.Context, *chatgptauth.Auth) ([]chatgptauth.Model, error) {
		t.Fatal("explicit model must not query catalog")
		return nil, errors.New("catalog unavailable")
	}
	for _, slug := range []string{"unlisted", "unlisted[1m]"} {
		if ep, err := ResolveEndpointWithOptions(path, ResolveOptions{Model: slug}); err != nil || ep.Model != slug {
			t.Fatal("explicit model rejected or rewritten locally", ep, err)
		}
	}
	if ep, err := ResolveEndpoint(path); err != nil || ep.Model != "gpt-6-luna" {
		t.Fatal(ep, err)
	}
	chatGPTCredentials = func(context.Context, string) (*chatgptauth.Auth, error) { return nil, errors.New("signed out") }
	if _, err := ResolveEndpoint(path); err == nil {
		t.Fatal("missing creds accepted")
	}
}
