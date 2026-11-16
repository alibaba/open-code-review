// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	copilot "github.com/github/copilot-sdk/go"
)

func TestCopilotTransportLimits(t *testing.T) {
	for _, tc := range []struct {
		path, body, key string
		want            int
	}{
		{"/responses", `{"input":[],"max_output_tokens":4096}`, "max_output_tokens", 128},
		{"/v1/messages", `{"messages":[],"max_tokens":4096}`, "max_tokens", 128},
		{"/chat/completions", `{"messages":[],"max_tokens":4096}`, "max_completion_tokens", 128},
		{"/responses", `{"type":"response.create","input":[],"max_output_tokens":64}`, "max_output_tokens", 64},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			transport := &copilotTransport{}
			transport.begin(t.Context(), 128)
			data, _, inference, err := transport.prepare([]byte(tc.body), tc.path)
			var body map[string]json.RawMessage
			if err != nil || !inference || json.Unmarshal(data, &body) != nil {
				t.Fatalf("prepare = %s, %v", data, err)
			}
			var limit int
			if json.Unmarshal(body[tc.key], &limit) != nil || limit != tc.want {
				t.Fatalf("limit = %s, want %d", data, tc.want)
			}
			transport.markResponse()
			if _, _, _, err := transport.prepare([]byte(tc.body), tc.path); err == nil {
				t.Fatal("unbudgeted model continuation was allowed")
			}
			transport.begin(t.Context(), 256)
			if _, _, _, err := transport.prepare([]byte(tc.body), tc.path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCopilotTransportInvalidRequestsAndMetadata(t *testing.T) {
	transport := &copilotTransport{}
	if _, _, _, err := transport.prepare([]byte(`{}`), "/responses"); err == nil {
		t.Fatal("inference without OCR request was allowed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	transport.begin(ctx, 128)
	for _, body := range []string{`invalid`, `null`, `[]`} {
		if _, _, _, err := transport.prepare([]byte(body), "/responses"); err == nil {
			t.Fatalf("invalid object accepted: %s", body)
		}
	}
	cancel()
	if _, _, _, err := transport.prepare([]byte(`{}`), "/responses"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inference = %v", err)
	}
	data, round, inference, err := transport.prepare([]byte(`{"routing":true}`), "/auto")
	if string(data) != `{"routing":true}` || round != nil || inference || err != nil {
		t.Fatalf("metadata changed: %s, %v", data, err)
	}
	transport.begin(t.Context(), 0)
	data, _, _, err = transport.prepare([]byte(`{"max_output_tokens":256}`), "/responses")
	if string(data) != `{"max_output_tokens":256}` || err != nil {
		t.Fatalf("unlimited request changed: %s, %v", data, err)
	}
}

func TestCopilotTransportThinkingBudget(t *testing.T) {
	const manual = `{"type":"enabled","budget_tokens":4096,"display":"summarized"}`
	const adaptive = `{"type":"adaptive","display":"summarized"}`
	for _, tc := range []struct {
		name           string
		limit          int
		thinking, want string
	}{
		{"unchanged", 8192, manual, manual},
		{"fits", 6000, manual, manual},
		{"equal", 4096, manual, `{"type":"enabled","budget_tokens":4095,"display":"summarized"}`},
		{"reduced", 2048, manual, `{"type":"enabled","budget_tokens":2047,"display":"summarized"}`},
		{"minimum", 1025, manual, `{"type":"enabled","budget_tokens":1024,"display":"summarized"}`},
		{"no-room", 1024, manual, ""},
		{"small", 128, manual, ""},
		{"adaptive", 128, adaptive, adaptive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &copilotTransport{}
			transport.begin(t.Context(), tc.limit)
			request := []byte(`{"max_tokens":8192,"thinking":` + tc.thinking + `,"messages":[]}`)
			data, _, _, err := transport.prepare(request, "/v1/messages")
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				MaxTokens int             `json:"max_tokens"`
				Thinking  json.RawMessage `json:"thinking"`
			}
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			if body.MaxTokens != tc.limit {
				t.Fatalf("max_tokens = %d, want %d", body.MaxTokens, tc.limit)
			}
			if tc.want == "" {
				if body.Thinking != nil {
					t.Fatalf("unexpected thinking: %s", body.Thinking)
				}
				return
			}
			var got, want map[string]any
			if err := json.Unmarshal(body.Thinking, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("thinking = %s, want %s", body.Thinking, tc.want)
			}
		})
	}
}

func TestCopilotClientLocalThinkingRequest(t *testing.T) {
	cliPath := os.Getenv("OCR_COPILOT_CLI_TEST_PATH")
	if cliPath == "" {
		t.Skip("set OCR_COPILOT_CLI_TEST_PATH for local CLI request inspection")
	}
	for _, tc := range []struct {
		model string
		limit int
	}{
		{"claude-sonnet-4.5", 8192}, {"claude-opus-4.1", 2048}, {"claude-sonnet-4.5", 128},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.model, tc.limit), func(t *testing.T) {
			requests := make(chan map[string]any, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}

				requests <- body
				thinking, _ := body["thinking"].(map[string]any)
				budget, _ := thinking["budget_tokens"].(float64)
				limit, _ := body["max_tokens"].(float64)
				if thinking["type"] == "enabled" && (budget < 1024 || budget >= limit) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"local validator: thinking budget must be >= 1024 and < max_tokens"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range []string{
					`{"type":"message_start","message":{"id":"msg_probe","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4.5","stop_reason":null,"usage":{"input_tokens":8,"output_tokens":0}}}`,
					`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
					`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
					`{"type":"content_block_stop","index":0}`,
					`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`,
					`{"type":"message_stop"}`,
				} {
					var data struct {
						Type string `json:"type"`
					}
					_ = json.Unmarshal([]byte(event), &data)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", data.Type, event)
				}
			}))
			defer server.Close()
			client := &CopilotClient{model: tc.model, cliPath: cliPath, timeout: 25 * time.Second,
				provider: &copilot.ProviderConfig{Type: "anthropic", BaseURL: server.URL, APIKey: "local-only"}}
			response, err := client.CompletionsWithCtx(t.Context(), ChatRequest{MaxTokens: tc.limit, Messages: []Message{NewTextMessage("user", "Say hello")}})
			select {
			case body := <-requests:
				t.Logf("CLI settings: max_tokens=%v thinking=%v", body["max_tokens"], body["thinking"])
				if body["max_tokens"] != float64(tc.limit) {
					t.Fatalf("output cap missing: %v", body["max_tokens"])
				}
				thinking, _ := body["thinking"].(map[string]any)
				if thinking["type"] == "enabled" {
					budget, ok := thinking["budget_tokens"].(float64)
					if !ok || budget < 1024 || budget >= float64(tc.limit) {
						t.Errorf("CLI request has incompatible thinking budget: %v", thinking)
						return
					}
				}
				if err != nil || response == nil || response.VisibleContent() != "Hello" {
					t.Errorf("valid control did not complete: response=%v error=%v", response, err)
				}
			default:
				t.Fatalf("CLI did not emit an Anthropic request: %v", err)
			}
		})
	}
}

func TestCopilotTransportHTTPAndCancellation(t *testing.T) {
	started := make(chan struct{})
	upstreamCancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["max_output_tokens"] != float64(128) {
			t.Errorf("body=%v, error=%v", body, err)
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(upstreamCancelled)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	transport := &copilotTransport{}
	transport.begin(ctx, 128)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/responses", strings.NewReader(`{"input":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	<-started
	cancel()
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("body cancellation = %v", err)
	}
	select {
	case <-upstreamCancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request was not cancelled")
	}
}

type copilotTestWebSocketWriter struct{}

func (copilotTestWebSocketWriter) SendText([]byte) error   { return nil }
func (copilotTestWebSocketWriter) SendBinary([]byte) error { return nil }

func TestCopilotWebSocketLimitAndControlFrames(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	frames := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		for range 2 {
			_, data, err := conn.Read(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			frames <- data
		}
	}))
	defer server.Close()
	transport := &copilotTransport{}
	transport.begin(ctx, 128)
	forwarder := copilot.NewCopilotWebSocketForwarder("ws"+strings.TrimPrefix(server.URL, "http")+"/responses", nil)
	handler := &copilotWebSocket{CopilotWebSocketForwarder: forwarder, transport: transport, path: "/responses"}
	if err := handler.Open(ctx, copilotTestWebSocketWriter{}); err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	for _, frame := range []string{`{"type":"response.create","input":[]}`, `{"type":"response.cancel"}`} {
		if err := handler.SendRequestMessage(ctx, copilot.NewTextMessage(frame)); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 2 {
		select {
		case data := <-frames:
			var body map[string]any
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			if index == 0 && body["max_output_tokens"] != float64(128) {
				t.Fatalf("WebSocket cap missing: %s", data)
			}
			if index == 1 && string(data) != `{"type":"response.cancel"}` {
				t.Fatalf("control frame changed: %s", data)
			}
		case <-ctx.Done():
			t.Fatal("WebSocket frame not forwarded")
		}
	}
}
