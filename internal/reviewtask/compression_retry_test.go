// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/openai/openai-go/v3"
)

type compressionRetryProbeClient struct {
	runner       *Runner
	toolRounds   int
	mainRequests int
	mu           sync.Mutex
	inputs       [][sha256.Size]byte
	inputBytes   []int
	compression  func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)
}

func (c *compressionRetryProbeClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if req.SessionID != "" {
		// Complete the previous background job before allowing the next tool
		// round, so the failed-job slot is deterministically available again.
		c.runner.WaitBackground()
		c.mainRequests++
		if c.mainRequests > c.toolRounds {
			return taskDoneResponse(), nil
		}
		return fileReadToolCallResponse(fmt.Sprintf("read_%d", c.mainRequests), `{"path":"sample.go"}`), nil
	}
	encoded, err := json.Marshal(req.Messages)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.inputs = append(c.inputs, sha256.Sum256(encoded))
	c.inputBytes = append(c.inputBytes, len(encoded))
	c.mu.Unlock()
	if c.compression != nil {
		return c.compression(ctx, req)
	}
	request, _ := http.NewRequest(http.MethodPost, "http://example.invalid/v1/chat/completions", nil)
	return nil, &openai.Error{
		StatusCode: http.StatusBadRequest,
		Request:    request,
		Response:   &http.Response{StatusCode: http.StatusBadRequest},
		Code:       "context_length_exceeded",
		Message:    "compression input rejected",
	}
}

type compressionRetryProbeTool struct {
	results []string
	calls   int
}

func (p *compressionRetryProbeTool) Tool() tool.Tool { return tool.FileRead }

func (p *compressionRetryProbeTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	if p.calls >= len(p.results) {
		return "further tool output", nil
	}
	result := p.results[p.calls]
	p.calls++
	return result, nil
}

func compressionRetryProbeRunner(t *testing.T, client *compressionRetryProbeClient, results []string) *Runner {
	t.Helper()
	deps := Deps{
		LLMClient:        client,
		Model:            "fake",
		Template:         template.Template{MaxToolRequestTimes: 10},
		CommentCollector: tool.NewCommentCollector(),
	}
	deps.Template.MaxTokens = 1000
	deps.Template.MemoryCompressionTask = template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "Summarize: {{context}}"}},
	}
	deps.Tools = tool.NewRegistry()
	deps.Tools.Register(&compressionRetryProbeTool{results: results})
	deps.Session = session.New(t.TempDir(), "main", "fake", session.SessionOptions{})
	t.Cleanup(func() {
		if err := deps.Session.Finalize(); err != nil {
			t.Error(err)
		}
	})
	client.runner = NewRunner(deps)
	return client.runner
}

func compressionRetryProbeCounts(t *testing.T, client *compressionRetryProbeClient) (int, int) {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()
	unique := make(map[[sha256.Size]byte]struct{})
	for _, hash := range client.inputs {
		unique[hash] = struct{}{}
	}
	t.Logf("main_requests=%d compression_requests=%d unique_compression_inputs=%d duplicate_inputs=%d input_bytes=%v",
		client.mainRequests, len(client.inputs), len(unique), len(client.inputs)-len(unique), client.inputBytes)
	return len(client.inputs), len(unique)
}

func TestCompressionRetryRejectedInput(t *testing.T) {
	t.Run("main_loop_new_tool_rounds", func(t *testing.T) {
		client := &compressionRetryProbeClient{toolRounds: 4}
		runner := compressionRetryProbeRunner(t, client, []string{
			strings.Repeat("word ", 650), "second result", "third result", "fourth result",
		})
		completed, stop, err := runner.RunMainTask(context.Background(),
			[]llm.Message{llm.NewTextMessage("system", "system"), llm.NewTextMessage("user", "review")}, "sample.go")
		runner.WaitBackground()
		if err != nil || !completed || stop != llmloop.StopNone {
			t.Fatalf("main loop: completed=%v stop=%v err=%v", completed, stop, err)
		}
		calls, unique := compressionRetryProbeCounts(t, client)
		if calls != 4 || unique != calls {
			t.Fatalf("expected changed compression inputs for all four new rounds: calls=%d unique=%d", calls, unique)
		}
		t.Logf("completed=%v stop=%v; original history remained available to the model", completed, stop)
	})

	t.Run("main_loop_warning_threshold", func(t *testing.T) {
		client := &compressionRetryProbeClient{toolRounds: 4}
		runner := compressionRetryProbeRunner(t, client, []string{
			strings.Repeat("word ", 650), strings.Repeat("extra ", 200),
		})
		completed, stop, err := runner.RunMainTask(context.Background(),
			[]llm.Message{llm.NewTextMessage("system", "system"), llm.NewTextMessage("user", "review")}, "sample.go")
		runner.WaitBackground()
		if err != nil || completed || stop != llmloop.StopCompression {
			t.Fatalf("main loop: completed=%v stop=%v err=%v", completed, stop, err)
		}
		calls, _ := compressionRetryProbeCounts(t, client)
		if calls != 1 {
			t.Fatalf("synchronous fallback resubmitted rejected input: got %d requests, want 1", calls)
		}
		records := runner.deps.Session.GetOrCreateFileSession("sample.go").TaskRecords[session.MemoryCompressionTask]
		if len(records) != 1 || records[0].Error == "" {
			t.Fatalf("expected exactly the original failed request record, got %+v", records)
		}
		t.Logf("completed=%v stop=%v; the warning threshold already bounds this workflow", completed, stop)
	})
}

func TestCompressionRetryHTTPBoundary(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		retryDirective bool
		wantHTTP       int32
		wantCompleted  bool
	}{
		{"bad_request", 400, false, 1, false},
		{"request_too_large", 413, false, 1, false},
		{"rate_limit_recovers", 429, false, 2, true},
		{"provider_retry_directive", 400, true, 2, true},
		{"temporary_payload_limit", 413, false, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				n := requests.Add(1)
				t.Logf("compression HTTP request=%d bytes=%d sha256=%x", n, len(body), sha256.Sum256(body))
				w.Header().Set("Content-Type", "application/json")
				if !tc.wantCompleted || n == 1 {
					if tc.retryDirective {
						w.Header().Set("x-should-retry", "true")
					}
					if tc.wantCompleted {
						w.Header().Set("retry-after-ms", "1")
					}
					if tc.name == "temporary_payload_limit" {
						w.Header().Set("Retry-After", "0")
					}
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"controlled test rejection"}}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"id":"test","object":"chat.completion","model":"fake","choices":[{"index":0,"message":{"role":"assistant","content":"summary"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			sdk := llm.NewOpenAIClient(llm.ClientConfig{URL: server.URL + "/v1", APIKey: "test", Model: "fake"})
			client := &compressionRetryProbeClient{toolRounds: 2, compression: sdk.CompletionsWithCtx}
			runner := compressionRetryProbeRunner(t, client, []string{
				strings.Repeat("word ", 650), strings.Repeat("extra ", 200),
			})
			completed, stop, err := runner.RunMainTask(context.Background(),
				[]llm.Message{llm.NewTextMessage("system", "system"), llm.NewTextMessage("user", "review")}, "sample.go")
			runner.WaitBackground()
			wantStop := llmloop.StopCompression
			if tc.wantCompleted {
				wantStop = llmloop.StopNone
			}
			if err != nil || completed != tc.wantCompleted || stop != wantStop || requests.Load() != tc.wantHTTP {
				t.Fatalf("HTTP=%d completed=%v stop=%v err=%v; want HTTP=%d completed=%v stop=%v",
					requests.Load(), completed, stop, err, tc.wantHTTP, tc.wantCompleted, wantStop)
			}
		})
	}
}
