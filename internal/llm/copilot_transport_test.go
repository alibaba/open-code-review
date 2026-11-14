// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
