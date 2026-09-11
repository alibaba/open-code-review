// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmresolve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/intent"
)

func testRequest() intent.LLMRequest {
	return intent.LLMRequest{
		System: "system prompt",
		User:   "user prompt",
		Tool: intent.ToolSchema{
			Name:        "submit_intent",
			Description: "submit",
			Parameters:  []byte(`{"type":"object"}`),
		},
	}
}

func TestNewClient(t *testing.T) {
	valid := Endpoint{URL: "http://x", Token: "k", Model: "m", Protocol: ProtocolOpenAI}
	if _, err := NewClient(valid); err != nil {
		t.Fatalf("openai: %v", err)
	}
	valid.Protocol = ProtocolAnthropic
	if _, err := NewClient(valid); err != nil {
		t.Fatalf("anthropic: %v", err)
	}

	bad := []Endpoint{
		{URL: "http://x", Token: "k", Model: "m", Protocol: ProtocolOpenAIResponses},
		{URL: "http://x", Token: "k", Model: "m", Protocol: ProtocolAnthropicBedrock},
		{URL: "http://x", Token: "k", Model: "m", Protocol: "bogus"},
		{URL: "http://x", Token: "k", Protocol: ProtocolOpenAI},
		{URL: "http://x", Model: "m", Protocol: ProtocolOpenAI},
		{Token: "k", Model: "m", Protocol: ProtocolOpenAI},
	}
	for i, ep := range bad {
		if _, err := NewClient(ep); err == nil {
			t.Fatalf("case %d: expected an error", i)
		}
	}

	c, err := NewClient(Endpoint{URL: "http://x", Token: "k", Model: "m", Protocol: ProtocolOpenAI})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Endpoint().Model != "m" {
		t.Fatalf("Endpoint() = %+v", c.Endpoint())
	}
}

func TestOpenAICallTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Errorf("authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Errorf("request body: %v", err)
		}
		if decoded["model"] != "m" {
			t.Errorf("model = %v", decoded["model"])
		}
		if decoded["tool_choice"] == nil {
			t.Error("tool_choice missing")
		}
		tools, _ := decoded["tools"].([]any)
		if len(tools) != 1 {
			t.Errorf("tools = %v", decoded["tools"])
		}
		io.WriteString(w, `{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"submit_intent","arguments":"{\"action\":\"scan\"}"}}]}}]}`)
	}))
	defer srv.Close()

	c, err := NewClient(Endpoint{URL: srv.URL + "/v1", Token: "key", Model: "m", Protocol: ProtocolOpenAI})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	call, err := c.CallTool(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if call.Name != "submit_intent" || !strings.Contains(string(call.Arguments), "scan") {
		t.Fatalf("call = %+v %s", call.Name, call.Arguments)
	}
}

func TestOpenAIErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"no choices", `{"choices":[]}`, 200},
		{"no tool calls", `{"choices":[{"message":{}}]}`, 200},
		{"two tool calls", `{"choices":[{"message":{"tool_calls":[{"function":{"name":"a"}},{"function":{"name":"b"}}]}}]}`, 200},
		{"endpoint error", `{"error":{"message":"bad key"}}`, 200},
		{"invalid json", `not json`, 200},
		{"http error", `{"error":"boom"}`, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			c, err := NewClient(Endpoint{URL: srv.URL, Token: "k", Model: "m", Protocol: ProtocolOpenAI})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := c.CallTool(context.Background(), testRequest()); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestAnthropicCallTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "key" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Errorf("request body: %v", err)
		}
		if decoded["max_tokens"] == nil || decoded["system"] == nil {
			t.Errorf("missing fields: %v", decoded)
		}
		io.WriteString(w, `{"content":[{"type":"text","text":"hi"},{"type":"tool_use","name":"submit_intent","input":{"action":"review","review":{"type":"workspace"}}}]}`)
	}))
	defer srv.Close()

	c, err := NewClient(Endpoint{URL: srv.URL, Token: "key", Model: "m", Protocol: ProtocolAnthropic, AuthHeader: "x-api-key"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	call, err := c.CallTool(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if call.Name != "submit_intent" || !strings.Contains(string(call.Arguments), "workspace") {
		t.Fatalf("call = %+v %s", call.Name, call.Arguments)
	}
}

func TestAnthropicAuthorizationHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Errorf("authorization = %q", got)
		}
		if r.Header.Get("x-api-key") != "" {
			t.Error("x-api-key should not be set")
		}
		io.WriteString(w, `{"content":[{"type":"tool_use","name":"submit_intent","input":{"action":"review","review":{"type":"workspace"}}}]}`)
	}))
	defer srv.Close()
	c, _ := NewClient(Endpoint{URL: srv.URL, Token: "key", Model: "m", Protocol: ProtocolAnthropic, AuthHeader: "authorization"})
	if _, err := c.CallTool(context.Background(), testRequest()); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
}

func TestAnthropicErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"no tool calls", `{"content":[{"type":"text","text":"hi"}]}`, 200},
		{"two tool calls", `{"content":[{"type":"tool_use","name":"a"},{"type":"tool_use","name":"b"}]}`, 200},
		{"endpoint error", `{"error":{"message":"bad"}}`, 200},
		{"invalid json", `nope`, 200},
		{"http error", `{}`, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			c, _ := NewClient(Endpoint{URL: srv.URL, Token: "k", Model: "m", Protocol: ProtocolAnthropic, AuthHeader: "x-api-key"})
			if _, err := c.CallTool(context.Background(), testRequest()); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestExtraHeadersAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Trace"); got != "1" {
			t.Errorf("X-Trace = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got == "evil" {
			t.Error("reserved User-Agent was overridden")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		json.Unmarshal(body, &decoded)
		if decoded["temperature"] != float64(0) {
			t.Errorf("extra body not merged: %v", decoded)
		}
		io.WriteString(w, `{"choices":[{"message":{"tool_calls":[{"function":{"name":"submit_intent","arguments":"{}"}}]}}]}`)
	}))
	defer srv.Close()
	c, _ := NewClient(Endpoint{
		URL: srv.URL, Token: "k", Model: "m", Protocol: ProtocolOpenAI,
		ExtraHeaders: map[string]string{"X-Trace": "1", "User-Agent": "evil", "Authorization": "evil"},
		ExtraBody:    map[string]any{"temperature": 0},
	})
	if _, err := c.CallTool(context.Background(), testRequest()); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
}

func TestCallToolUnsupportedProtocol(t *testing.T) {
	c := &Client{ep: Endpoint{Protocol: "bogus"}}
	if _, err := c.CallTool(context.Background(), testRequest()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestPostErrors(t *testing.T) {
	c := &Client{http: &http.Client{}}
	if _, err := c.post(context.Background(), "http://x", func() {}, nil); err == nil {
		t.Fatal("unmarshalable body should error")
	}
	if _, err := c.post(context.Background(), "://bad", map[string]any{}, nil); err == nil {
		t.Fatal("bad url should error")
	}
	if _, err := c.post(context.Background(), "http://127.0.0.1:1/", map[string]any{}, nil); err == nil {
		t.Log("unreachable endpoint unexpectedly reachable")
	}
}

func TestURLHelpers(t *testing.T) {
	if got := openAIChatURL("http://x/v1"); got != "http://x/v1/chat/completions" {
		t.Fatalf("openAIChatURL = %q", got)
	}
	if got := openAIChatURL("http://x/v1/chat/completions"); got != "http://x/v1/chat/completions" {
		t.Fatalf("openAIChatURL = %q", got)
	}
	if got := anthropicMessagesURL("http://x"); got != "http://x/v1/messages" {
		t.Fatalf("anthropicMessagesURL = %q", got)
	}
	if got := anthropicMessagesURL("http://x/v1/messages"); got != "http://x/v1/messages" {
		t.Fatalf("anthropicMessagesURL = %q", got)
	}
}

func TestApplyExtraHeaders(t *testing.T) {
	dst := map[string]string{"Authorization": "keep"}
	applyExtraHeaders(dst, map[string]string{"Authorization": "drop", "X-Trace": "1", "content-type": "drop"})
	if dst["Authorization"] != "keep" || dst["X-Trace"] != "1" {
		t.Fatalf("headers = %v", dst)
	}
	if _, ok := dst["content-type"]; ok {
		t.Fatalf("reserved header leaked: %v", dst)
	}
}
