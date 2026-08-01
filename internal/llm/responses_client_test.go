package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

func TestNewOpenAIResponsesClient_URLNormalization(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
		wantURL  string
	}{
		{
			name:     "base URL without trailing slash",
			inputURL: "https://api.example.com/v1",
			wantURL:  "https://api.example.com/v1/responses",
		},
		{
			name:     "base URL with trailing slash",
			inputURL: "https://api.example.com/v1/",
			wantURL:  "https://api.example.com/v1/responses",
		},
		{
			name:     "full URL already has /responses",
			inputURL: "https://api.example.com/v1/responses",
			wantURL:  "https://api.example.com/v1/responses",
		},
		{
			name:     "full URL with trailing slash",
			inputURL: "https://api.example.com/v1/responses/",
			wantURL:  "https://api.example.com/v1/responses",
		},
		{
			name:     "bare host",
			inputURL: "https://api.example.com",
			wantURL:  "https://api.example.com/responses",
		},
		{
			name:     "bare host with trailing slash",
			inputURL: "https://api.example.com/",
			wantURL:  "https://api.example.com/responses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewOpenAIResponsesClient(ClientConfig{URL: tt.inputURL})
			if client.cfg.URL != tt.wantURL {
				t.Errorf("got URL %q, want %q", client.cfg.URL, tt.wantURL)
			}
		})
	}
}

func TestBuildResponsesParams_SystemToInstructions(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a code reviewer."},
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "Review this code."},
		},
	}

	params := client.buildResponsesParams("gpt-5.4", req)

	if !params.Instructions.Valid() {
		t.Fatal("expected Instructions to be set when system messages are present")
	}
	got := params.Instructions.Value
	want := "You are a code reviewer.\n\nBe concise."
	if got != want {
		t.Errorf("Instructions = %q, want %q", got, want)
	}
	// system messages should NOT also appear as input items.
	if len(params.Input.OfInputItemList) != 1 {
		t.Errorf("expected 1 input item (the user message), got %d", len(params.Input.OfInputItemList))
	}
}

func TestBuildResponsesParams_NoInstructionsWhenNoSystem(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	req := ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	params := client.buildResponsesParams("gpt-5.4", req)
	if params.Instructions.Valid() {
		got := params.Instructions.Value
		t.Errorf("expected Instructions to be unset, got %q", got)
	}
	if params.PromptCacheKey.Valid() {
		got := params.PromptCacheKey.Value
		t.Errorf("expected PromptCacheKey to be unset when req.SessionID is empty, got %q", got)
	}
}

func TestBuildResponsesParams_ToolCallItems(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	req := ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "list files"},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{
						ID:       "call_abc",
						Type:     "function",
						Function: FunctionCall{Name: "list_files", Arguments: `{"path":"/tmp"}`},
					},
				},
			},
			{Role: "tool", Content: `[ "file1.go" ]`, ToolCallID: "call_abc"},
		},
	}

	params := client.buildResponsesParams("gpt-5.4", req)

	items := params.Input.OfInputItemList
	// Expect: user message, function_call, function_call_output (the empty
	// assistant text content is dropped because it's "").
	if len(items) != 3 {
		t.Fatalf("expected 3 input items, got %d", len(items))
	}

	if items[0].OfMessage == nil {
		t.Errorf("items[0] should be a user message")
	}
	if items[1].OfFunctionCall == nil {
		t.Fatalf("items[1] should be a function_call")
	}
	if items[1].OfFunctionCall.CallID != "call_abc" {
		t.Errorf("function_call.CallID = %q, want %q", items[1].OfFunctionCall.CallID, "call_abc")
	}
	if items[1].OfFunctionCall.Name != "list_files" {
		t.Errorf("function_call.Name = %q, want %q", items[1].OfFunctionCall.Name, "list_files")
	}
	if items[1].OfFunctionCall.Arguments != `{"path":"/tmp"}` {
		t.Errorf("function_call.Arguments = %q", items[1].OfFunctionCall.Arguments)
	}

	if items[2].OfFunctionCallOutput == nil {
		t.Fatalf("items[2] should be a function_call_output")
	}
	if items[2].OfFunctionCallOutput.CallID != "call_abc" {
		t.Errorf("function_call_output.CallID = %q, want %q", items[2].OfFunctionCallOutput.CallID, "call_abc")
	}
}

func TestBuildResponsesParams_AssistantTextPlusToolCalls(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	req := ChatRequest{
		Messages: []Message{
			{
				Role:    "assistant",
				Content: "I'll list the files for you.",
				ToolCalls: []ToolCall{
					{ID: "call_1", Type: "function", Function: FunctionCall{Name: "ls", Arguments: "{}"}},
				},
			},
		},
	}

	params := client.buildResponsesParams("gpt-5.4", req)
	items := params.Input.OfInputItemList
	// assistant text item + function_call item.
	if len(items) != 2 {
		t.Fatalf("expected 2 input items (assistant text + function_call), got %d", len(items))
	}
	if items[0].OfMessage == nil {
		t.Errorf("items[0] should be the assistant text message")
	}
	if items[1].OfFunctionCall == nil {
		t.Errorf("items[1] should be the function_call")
	}
}

func TestBuildResponsesParams_Tools(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	req := ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools: []ToolDef{
			{
				Type: "function",
				Function: FunctionDef{
					Name:        "get_weather",
					Description: "Get current weather",
					Parameters:  map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
				},
			},
		},
	}

	params := client.buildResponsesParams("gpt-5.4", req)

	if len(params.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(params.Tools))
	}
	if params.Tools[0].OfFunction == nil {
		t.Fatal("expected OfFunction tool")
	}
	ft := params.Tools[0].OfFunction
	if ft.Name != "get_weather" {
		t.Errorf("tool Name = %q, want %q", ft.Name, "get_weather")
	}
	// Strict MUST be false (the plan mandates non-strict so the model can
	// fill in arguments our loose JSON schemas don't enumerate).
	if !ft.Strict.Valid() {
		t.Fatal("tool Strict should be explicitly set")
	}
	strict := ft.Strict.Value
	if strict {
		t.Errorf("tool Strict = true, want false (non-strict by default)")
	}
}

func TestBuildResponsesParams_StoreAndCacheKey(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	t.Run("store is always false", func(t *testing.T) {
		req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}
		params := client.buildResponsesParams("gpt-5.4", req)
		if !params.Store.Valid() {
			t.Fatal("expected Store to be set")
		}
		store := params.Store.Value
		if store {
			t.Errorf("Store = true, want false (stateless, privacy-preserving)")
		}
	})

	t.Run("PromptCacheKey passes through req.SessionID", func(t *testing.T) {
		req := ChatRequest{
			Messages:  []Message{{Role: "user", Content: "hi"}},
			SessionID: "abc123",
		}
		params := client.buildResponsesParams("gpt-5.4", req)
		if !params.PromptCacheKey.Valid() {
			t.Fatal("expected PromptCacheKey to be set when req.SessionID is non-empty")
		}
		if got := params.PromptCacheKey.Value; got != "abc123" {
			t.Errorf("PromptCacheKey = %q, want %q", got, "abc123")
		}
	})

	t.Run("PromptCacheKey unset when req.SessionID empty", func(t *testing.T) {
		req := ChatRequest{
			Messages: []Message{{Role: "system", Content: "system prompt"}},
		}
		params := client.buildResponsesParams("gpt-5.4", req)
		if params.PromptCacheKey.Valid() {
			t.Errorf("expected PromptCacheKey unset when req.SessionID empty, got %q", params.PromptCacheKey.Value)
		}
	})
}

func TestBuildResponsesParams_MaxTokensAndTemperature(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})

	t.Run("MaxTokens>0 sets MaxOutputTokens", func(t *testing.T) {
		req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 512}
		params := client.buildResponsesParams("gpt-5.4", req)
		if !params.MaxOutputTokens.Valid() {
			t.Fatal("expected MaxOutputTokens to be set")
		}
		v := params.MaxOutputTokens.Value
		if v != 512 {
			t.Errorf("MaxOutputTokens = %d, want 512", v)
		}
	})

	t.Run("MaxTokens=0 leaves MaxOutputTokens unset (no Anthropic-style 8192 floor)", func(t *testing.T) {
		req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}
		params := client.buildResponsesParams("gpt-5.4", req)
		if params.MaxOutputTokens.Valid() {
			v := params.MaxOutputTokens.Value
			t.Errorf("MaxOutputTokens = %d, want unset (no default floor)", v)
		}
	})

	temp := 0.7
	req := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}, Temperature: &temp}
	params := client.buildResponsesParams("gpt-5.4", req)
	if !params.Temperature.Valid() {
		t.Fatal("expected Temperature to be set")
	}
	gotTemp := params.Temperature.Value
	if gotTemp != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", gotTemp)
	}
}

// --- mapResponsesResponse tests ---

func TestMapResponsesResponse_TextOnly(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"resp_1",
		"object":"response",
		"model":"gpt-5.4",
		"status":"completed",
		"output":[
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello world"}]}
		],
		"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}
	}`
	sdkResp := unmarshalResponsesBody(t, body)

	resp := client.mapResponsesResponse(sdkResp)

	if resp.ID != "resp_1" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Model != "gpt-5.4" {
		t.Errorf("Model = %q", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	ch := resp.Choices[0]
	if ch.Message.Content == nil || *ch.Message.Content != "hello world" {
		got := "(nil)"
		if ch.Message.Content != nil {
			got = *ch.Message.Content
		}
		t.Errorf("Content = %q, want %q", got, "hello world")
	}
	if len(ch.Message.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(ch.Message.ToolCalls))
	}
	if ch.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", ch.FinishReason, "stop")
	}
}

func TestMapResponsesResponse_FunctionCalls(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"resp_2",
		"object":"response",
		"model":"gpt-5.4",
		"status":"completed",
		"output":[
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Calling tool"}]},
			{"type":"function_call","call_id":"call_xyz","name":"do_thing","arguments":"{\"x\":1}"}
		],
		"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}
	}`
	sdkResp := unmarshalResponsesBody(t, body)

	resp := client.mapResponsesResponse(sdkResp)

	ch := resp.Choices[0]
	if len(ch.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(ch.Message.ToolCalls))
	}
	tc := ch.Message.ToolCalls[0]
	// The loop pairs NewToolResultMessage(call.ID, ...) with this ID, so it
	// MUST be the SDK's CallID, not the generic item ID.
	if tc.ID != "call_xyz" {
		t.Errorf("ToolCall.ID = %q, want %q (must equal CallID for loop pairing)", tc.ID, "call_xyz")
	}
	if tc.Type != "function" {
		t.Errorf("ToolCall.Type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "do_thing" {
		t.Errorf("ToolCall.Function.Name = %q, want %q", tc.Function.Name, "do_thing")
	}
	if tc.Function.Arguments != `{"x":1}` {
		t.Errorf("ToolCall.Function.Arguments = %q, want %q", tc.Function.Arguments, `{"x":1}`)
	}
	// Status is "completed" but tool calls present → FinishReason must be "tool_calls".
	if ch.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q (tool_calls overrides status)", ch.FinishReason, "tool_calls")
	}
}

func TestMapResponsesResponse_StatusIncomplete(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"resp_3",
		"object":"response",
		"model":"gpt-5.4",
		"status":"incomplete",
		"output":[
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}
		],
		"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}
	}`
	sdkResp := unmarshalResponsesBody(t, body)

	resp := client.mapResponsesResponse(sdkResp)
	if resp.Choices[0].FinishReason != "length" {
		t.Errorf("FinishReason = %q, want %q for incomplete status", resp.Choices[0].FinishReason, "length")
	}
}

func TestMapResponsesResponse_StatusFailedAndCancelled(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
			body := `{
				"id":"resp_err",
				"object":"response",
				"model":"gpt-5.4",
				"status":"` + status + `",
				"output":[
					{"type":"message","role":"assistant","content":[{"type":"output_text","text":""}]}
				],
				"usage":{"input_tokens":5,"output_tokens":0,"total_tokens":5}
			}`
			sdkResp := unmarshalResponsesBody(t, body)

			resp := client.mapResponsesResponse(sdkResp)
			if resp.Choices[0].FinishReason != "error" {
				t.Errorf("FinishReason = %q, want %q for %s status", resp.Choices[0].FinishReason, "error", status)
			}
		})
	}
}

func TestMapResponsesResponse_Usage(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"resp_4",
		"object":"response",
		"model":"gpt-5.4",
		"status":"completed",
		"output":[
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}
		],
		"usage":{
			"input_tokens":100,
			"output_tokens":50,
			"total_tokens":150,
			"input_tokens_details":{"cached_tokens":80}
		}
	}`
	sdkResp := unmarshalResponsesBody(t, body)

	resp := client.mapResponsesResponse(sdkResp)
	u := resp.Usage
	if u == nil {
		t.Fatal("expected Usage to be non-nil")
	}
	if u.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", u.PromptTokens)
	}
	if u.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", u.CompletionTokens)
	}
	if u.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", u.TotalTokens)
	}
	if u.CacheReadTokens != 80 {
		t.Errorf("CacheReadTokens = %d, want 80 (from input_tokens_details.cached_tokens)", u.CacheReadTokens)
	}
}

func TestMapResponsesResponse_ReasoningAggregated(t *testing.T) {
	client := NewOpenAIResponsesClient(ClientConfig{URL: "https://api.openai.com/v1"})
	body := `{
		"id":"resp_5",
		"object":"response",
		"model":"o3",
		"status":"completed",
		"output":[
			{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"step one"},{"type":"summary_text","text":"step two"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}
		],
		"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}
	}`
	sdkResp := unmarshalResponsesBody(t, body)

	resp := client.mapResponsesResponse(sdkResp)
	// Both summary entries must be aggregated (not just the first).
	want := "step one\nstep two"
	if resp.Choices[0].Message.ReasoningContent != want {
		t.Errorf("ReasoningContent = %q, want %q", resp.Choices[0].Message.ReasoningContent, want)
	}
}

// TestOpenAIResponsesClient_EndToEnd hits a fake server to make sure the
// request body carries store=false, prompt_cache_key, and instructions, and
// that the SDK path matches our cfg.URL contract.
func TestOpenAIResponsesClient_EndToEnd(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_e2e",
			"object":"response",
			"model":"gpt-5.4",
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],
			"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewOpenAIResponsesClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "gpt-5.4",
	})

	// The SDK base URL is server.URL/v1 and the SDK appends "responses".
	// cfg.URL is normalized to server.URL/v1/responses.
	if client.cfg.URL != server.URL+"/v1/responses" {
		t.Fatalf("cfg.URL = %q", client.cfg.URL)
	}

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "ping"},
		},
		SessionID: "test-session-id",
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	// SDK posts to /v1/responses.
	if gotPath != "/v1/responses" {
		t.Errorf("request path = %q, want /v1/responses", gotPath)
	}

	if v, _ := gotBody["store"].(bool); v {
		t.Errorf("request body store = %v, want false", v)
	}
	// store must be present (we always send it).
	storeVal, hasStore := gotBody["store"]
	if !hasStore {
		t.Error("request body missing store field")
	} else if storeVal != false {
		t.Errorf("request body store = %v, want false", storeVal)
	}
	if gotBody["instructions"] != "be brief" {
		t.Errorf("instructions = %v, want %q", gotBody["instructions"], "be brief")
	}
	cacheKey, ok := gotBody["prompt_cache_key"].(string)
	if !ok || cacheKey == "" {
		t.Errorf("expected non-empty prompt_cache_key, got %v", gotBody["prompt_cache_key"])
	}

	if resp.Content() != "pong" {
		t.Errorf("Content() = %q, want %q", resp.Content(), "pong")
	}
}

// TestOpenAIResponsesClient_ExtraBodyStreamDropped verifies that an
// extra_body.stream=true (valid for the Chat Completions client) is NOT
// forwarded to the Responses API. Forwarding it makes the API answer with SSE
// while Responses.New expects a JSON body, breaking every call. Other
// extra_body keys must still be forwarded.
func TestOpenAIResponsesClient_ExtraBodyStreamDropped(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_stream",
			"object":"response",
			"model":"gpt-5.4",
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
			"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	client := NewOpenAIResponsesClient(ClientConfig{
		URL:    server.URL + "/v1",
		APIKey: "test-key",
		Model:  "gpt-5.4",
		ExtraBody: map[string]any{
			"stream":               true,
			"keep_me":              "yes",
			"temperature_override": 0.1,
		},
	})

	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("CompletionsWithCtx: %v", err)
	}

	if _, present := gotBody["stream"]; present {
		t.Errorf("request body should NOT contain a stream field, got %v", gotBody["stream"])
	}
	if gotBody["keep_me"] != "yes" {
		t.Errorf("other extra_body keys must still be forwarded; keep_me = %v", gotBody["keep_me"])
	}
	if resp.Content() != "ok" {
		t.Errorf("Content() = %q, want %q", resp.Content(), "ok")
	}
}

// TestOpenAIResponsesClient_NonSuccessStatusReturnsError verifies that a
// response with HTTP 200 but a non-completed status is surfaced as an error
// rather than a normal ChatResponse. The Responses API returns 200 for
// failed/cancelled (terminal) and queued/in_progress (background) states, so
// the SDK reports a nil Go error; callers that branch on err must still fail.
func TestOpenAIResponsesClient_NonSuccessStatusReturnsError(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "queued", "in_progress"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id":"resp_` + status + `",
					"object":"response",
					"model":"gpt-5.4",
					"status":"` + status + `",
					"output":[],
					"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}
				}`))
			}))
			defer server.Close()

			client := NewOpenAIResponsesClient(ClientConfig{
				URL:    server.URL + "/v1",
				APIKey: "test-key",
				Model:  "gpt-5.4",
			})

			resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
				Messages: []Message{{Role: "user", Content: "hi"}},
			})
			if err == nil {
				t.Fatalf("expected error for status=%s, got nil (resp=%v)", status, resp)
			}
			if resp != nil {
				t.Errorf("expected nil ChatResponse for status=%s, got %v", status, resp)
			}
		})
	}
}

// --- helpers ---

func unmarshalResponsesBody(t *testing.T, body string) *responses.Response {
	t.Helper()
	var r responses.Response
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("unmarshal responses body: %v", err)
	}
	return &r
}

// Compile-time check that the client satisfies LLMClient.
var _ LLMClient = (*OpenAIResponsesClient)(nil)
