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

func TestChatGPTPromptCacheAffinityHeaders(t *testing.T) {
	newRoutedClient := func(t *testing.T, cfg ClientConfig, handler http.HandlerFunc) *OpenAIResponsesClient {
		t.Helper()
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		target, _ := url.Parse(server.URL)
		c := NewOpenAIResponsesClient(cfg)
		c.chatGPTToken = func(context.Context) (string, error) { return "access-fixture", nil }
		c.sdk = openai.NewClient(option.WithBaseURL(cfg.URL), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: resolverRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			r := req.Clone(req.Context())
			u := *req.URL
			u.Scheme = target.Scheme
			u.Host = target.Host
			r.URL = &u
			return http.DefaultTransport.RoundTrip(r)
		})}))
		return c
	}

	okSSE := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"model\":\"gpt-6-luna\",\"output\":[{\"type\":\"message\",\"id\":\"m\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\",\"annotations\":[]}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":1,\"total_tokens\":6}}}\n\n")
	}

	t.Run("context session key populates routing headers across rounds", func(t *testing.T) {
		var seen [][]string
		var gotBody map[string]any
		c := newRoutedClient(t, ClientConfig{
			URL:               chatgptauth.Resource,
			ChatGPTPlan:       true,
			RequiresStreaming: true,
			SessionKey:        "fallback-client-key",
		}, func(w http.ResponseWriter, r *http.Request) {
			seen = append(seen, []string{r.Header.Get("session_id"), r.Header.Get("session-id")})
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			okSSE(w)
		})

		ctx := ContextWithSessionKey(context.Background(), "task-affinity-key")
		for range 2 {
			if _, err := c.CompletionsWithCtx(ctx, ChatRequest{
				Model:     "gpt-6-luna",
				Messages:  []Message{NewTextMessage("user", "ping")},
				SessionID: "subtask-uuid-1",
			}); err != nil {
				t.Fatalf("CompletionsWithCtx: %v", err)
			}
		}
		if len(seen) != 2 {
			t.Fatalf("requests = %d, want 2", len(seen))
		}
		for i, pair := range seen {
			if pair[0] != "task-affinity-key" || pair[1] != "task-affinity-key" {
				t.Errorf("turn %d headers = %q, want [task-affinity-key task-affinity-key]", i+1, pair)
			}
		}
		if got := gotBody["prompt_cache_key"]; got != "subtask-uuid-1" {
			t.Errorf("prompt_cache_key = %v, want %q", got, "subtask-uuid-1")
		}
	})

	t.Run("context key populates prompt_cache_key when request SessionID is empty", func(t *testing.T) {
		var gotBody map[string]any
		c := newRoutedClient(t, ClientConfig{
			URL:               chatgptauth.Resource,
			ChatGPTPlan:       true,
			RequiresStreaming: true,
		}, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			okSSE(w)
		})

		ctx := ContextWithSessionKey(context.Background(), "plan-affinity-key")
		if _, err := c.CompletionsWithCtx(ctx, ChatRequest{
			Model:    "gpt-6-luna",
			Messages: []Message{NewTextMessage("user", "ping")},
		}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if got := gotBody["prompt_cache_key"]; got != "plan-affinity-key" {
			t.Errorf("prompt_cache_key = %v, want %q", got, "plan-affinity-key")
		}
	})

	t.Run("request SessionID is used when context carries no key", func(t *testing.T) {
		var gotUnderscore, gotHyphen string
		c := newRoutedClient(t, ClientConfig{
			URL:               chatgptauth.Resource,
			ChatGPTPlan:       true,
			RequiresStreaming: true,
			SessionKey:        "fallback-client-key",
		}, func(w http.ResponseWriter, r *http.Request) {
			gotUnderscore = r.Header.Get("session_id")
			gotHyphen = r.Header.Get("session-id")
			okSSE(w)
		})

		if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
			Model:     "gpt-6-luna",
			Messages:  []Message{NewTextMessage("user", "ping")},
			SessionID: "conv-only-id",
		}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if gotUnderscore != "conv-only-id" || gotHyphen != "conv-only-id" {
			t.Errorf("headers = (%q, %q), want (%q, %q)", gotUnderscore, gotHyphen, "conv-only-id", "conv-only-id")
		}
	})

	t.Run("client SessionKey serves as fallback when both context and request keys are empty", func(t *testing.T) {
		var gotUnderscore, gotHyphen string
		c := newRoutedClient(t, ClientConfig{
			URL:               chatgptauth.Resource,
			ChatGPTPlan:       true,
			RequiresStreaming: true,
			SessionKey:        "fallback-client-key",
		}, func(w http.ResponseWriter, r *http.Request) {
			gotUnderscore = r.Header.Get("session_id")
			gotHyphen = r.Header.Get("session-id")
			okSSE(w)
		})

		if _, err := c.CompletionsWithCtx(context.Background(), ChatRequest{
			Model:    "gpt-6-luna",
			Messages: []Message{NewTextMessage("user", "ping")},
		}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if gotUnderscore != "fallback-client-key" || gotHyphen != "fallback-client-key" {
			t.Errorf("headers = (%q, %q), want fallback-client-key", gotUnderscore, gotHyphen)
		}
	})

	t.Run("explicit ExtraHeaders override default affinity headers", func(t *testing.T) {
		var gotUnderscore, gotHyphen string
		c := newRoutedClient(t, ClientConfig{
			URL:               chatgptauth.Resource,
			ChatGPTPlan:       true,
			RequiresStreaming: true,
			ExtraHeaders: map[string]string{
				"session_id": "{ocr_session_key}-custom",
				"session-id": "explicit-hyphen",
			},
		}, func(w http.ResponseWriter, r *http.Request) {
			gotUnderscore = r.Header.Get("session_id")
			gotHyphen = r.Header.Get("session-id")
			okSSE(w)
		})

		ctx := ContextWithSessionKey(context.Background(), "ctx-key")
		if _, err := c.CompletionsWithCtx(ctx, ChatRequest{
			Model:    "gpt-6-luna",
			Messages: []Message{NewTextMessage("user", "ping")},
		}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if gotUnderscore != "ctx-key-custom" || gotHyphen != "explicit-hyphen" {
			t.Errorf("headers = (%q, %q), want (ctx-key-custom, explicit-hyphen)", gotUnderscore, gotHyphen)
		}
	})

	t.Run("non-ChatGPTPlan requests do not add session affinity headers automatically", func(t *testing.T) {
		var hasUnderscore, hasHyphen bool
		c := newRoutedClient(t, ClientConfig{
			URL:        "https://api.openai.com/v1",
			APIKey:     "sk-test",
			SessionKey: "fallback-client-key",
		}, func(w http.ResponseWriter, r *http.Request) {
			_, hasUnderscore = r.Header[http.CanonicalHeaderKey("session_id")]
			_, hasHyphen = r.Header[http.CanonicalHeaderKey("session-id")]
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
		})

		ctx := ContextWithSessionKey(context.Background(), "task-key")
		if _, err := c.CompletionsWithCtx(ctx, ChatRequest{
			Model:     "gpt-5.4",
			Messages:  []Message{NewTextMessage("user", "ping")},
			SessionID: "req-id",
		}); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
		if hasUnderscore || hasHyphen {
			t.Errorf("unexpected automatic headers on standard OpenAI client: session_id=%v session-id=%v", hasUnderscore, hasHyphen)
		}
	})
}
