// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

func TestAccumulateResponseStream_PreservesTerminalOutput(t *testing.T) {
	const item = `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"complete answer","annotations":[]}]}`
	for _, tc := range []struct {
		name   string
		status string
		prefix []string
	}{
		{name: "terminal only", status: "completed"},
		{name: "incomplete terminal", status: "incomplete"},
		{name: "partial item events", status: "completed", prefix: []string{
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"ciphertext","status":"completed"}}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bodies := append(tc.prefix, `{"type":"response.`+tc.status+`","response":{"id":"resp_terminal","status":"`+tc.status+`","output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"ciphertext","status":"completed"},`+item+`],"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14}}}`)
			resp, err := accumulateResponseStream(responseStreamEvents(t, bodies...))
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Output) != 2 || resp.OutputText() != "complete answer" {
				t.Fatalf("output = %s, want complete terminal output", resp.RawJSON())
			}
			if resp.Usage.TotalTokens != 14 {
				t.Errorf("total tokens = %d, want 14", resp.Usage.TotalTokens)
			}
			if string(resp.Status) != tc.status {
				t.Errorf("status = %s, want %s", resp.Status, tc.status)
			}
		})
	}
}

func TestAccumulateResponseStream_TerminalOutputDoesNotBypassStatus(t *testing.T) {
	for _, status := range []string{"failed", "cancelled", "queued", "in_progress", "vendor_done"} {
		t.Run(status, func(t *testing.T) {
			events := responseStreamEvents(t, `{"type":"response.completed","response":{"id":"resp_invalid","status":"`+status+`","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]}}`)
			if _, err := accumulateResponseStream(events); err == nil {
				t.Fatal("nonempty output must not make an unsuccessful status succeed")
			}
		})
	}
}

func TestOpenAIResponsesClient_StreamingTerminalOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if body["stream"] != true {
			t.Errorf("stream = %v, want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"id":"resp_terminal","object":"response","model":"fixture-model","status":"completed","output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"ciphertext","status":"completed"},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"inspect","arguments":"{\"path\":\"main.go\"}","status":"completed"}],"usage":{"input_tokens":10,"output_tokens":4,"total_tokens":14}}}`+"\n\n")
	}))
	defer server.Close()

	client := NewOpenAIResponsesClient(ClientConfig{
		URL: server.URL + "/v1", APIKey: "test-key", Model: "fixture-model", RequiresStreaming: true,
	})
	resp, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "inspect main.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Function.Name != "inspect" || calls[0].Function.Arguments != `{"path":"main.go"}` {
		t.Fatalf("tool calls = %+v, want terminal inspect call", calls)
	}
	payload, ok := resp.Native().Payload.([]responses.ResponseInputItemUnionParam)
	if !ok || len(payload) != 2 || payload[0].OfReasoning == nil || payload[1].OfFunctionCall == nil {
		t.Fatalf("native payload = %#v, want reasoning and function call", resp.Native().Payload)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"encrypted_content":"ciphertext"`) {
		t.Errorf("native payload lost encrypted reasoning: %s", raw)
	}
}
