// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/stdout"
)

type progressBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *progressBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *progressBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func captureRequestProgress(t *testing.T, fn func()) string {
	t.Helper()
	var output progressBuffer
	defer stdout.Swap(&output)()
	fn()
	return output.String()
}

func TestRequestProgressStagesAndSafeIdentity(t *testing.T) {
	meta := testMeta()
	meta.Provider = "provider\n::warning::untrusted"
	meta.Model = "model\nname"
	meta.FilePath = "file\nname.go"
	meta.TaskType = "task\nname"
	output := captureRequestProgress(t, func() {
		progress := startRequestProgress(metaCtx(meta), meta.Model, "fallback")
		progress.stage("preflight")
		progress.stage("inference")
		progress.finish(nil)
	})
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 6 {
		t.Fatalf("want a request pair and two stage pairs, got %d lines:\n%s", len(lines), output)
	}
	wantEvents := []string{
		"LLM request started", "LLM stage started", "LLM stage finished",
		"LLM stage started", "LLM stage finished", "LLM request finished",
	}
	for i, event := range wantEvents {
		if !strings.Contains(lines[i], event) {
			t.Errorf("line %d = %q, want %q", i, lines[i], event)
		}
	}
	for _, value := range []string{meta.Provider, meta.Model, meta.FilePath, meta.TaskType} {
		if !strings.Contains(lines[0], strconv.Quote(value)) {
			t.Errorf("request identity missing escaped %q:\n%s", value, output)
		}
	}
	if strings.Count(output, `stage="preflight"`) != 2 || strings.Count(output, `stage="inference"`) != 2 {
		t.Errorf("stage identity missing from start/end pair:\n%s", output)
	}
	assertProgressDurations(t, output)
}

func TestRequestProgressOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "ok"},
		{name: "failed", err: errors.New("private provider error")},
		{name: "cancelled", err: fmt.Errorf("private context: %w", context.Canceled)},
		{name: "timeout", err: errors.Join(ErrRequestTimeout, context.DeadlineExceeded)},
		{name: "panicked", err: errRequestPanicked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureRequestProgress(t, func() {
				progress := startRequestProgress(context.Background(), "", "fallback-model")
				progress.stage("inference")
				progress.finish(tc.err)
			})
			if strings.Count(output, "status="+tc.name) != 2 {
				t.Errorf("stage and request must report %s:\n%s", tc.name, output)
			}
			if !strings.Contains(output, `model="fallback-model"`) {
				t.Errorf("configured model missing:\n%s", output)
			}
			if strings.Contains(output, "private") {
				t.Errorf("progress copied a private error message:\n%s", output)
			}
			assertProgressDurations(t, output)
		})
	}
}

func TestRequestProgressRespectsQuietAudience(t *testing.T) {
	output := captureRequestProgress(t, func() {
		defer stdout.Quiet()()
		progress := startRequestProgress(metaCtx(testMeta()), "selected-model", "fallback")
		progress.stage("preflight")
		progress.finish(errors.New("private provider error"))
	})
	if output != "" {
		t.Fatalf("quiet audience emitted progress: %q", output)
	}
}

func assertProgressDurations(t *testing.T, output string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if !strings.Contains(line, " finished ") {
			continue
		}
		_, elapsed, ok := strings.Cut(line, "elapsed=")
		if !ok {
			t.Errorf("finished event has no duration: %s", line)
			continue
		}
		fields := strings.Fields(elapsed)
		if len(fields) == 0 {
			t.Errorf("finished event has empty duration: %s", line)
			continue
		}
		duration, err := time.ParseDuration(fields[0])
		if err != nil || duration < 0 {
			t.Errorf("invalid duration in %q: %v", line, err)
		}
	}
}

func TestClientsReportOneLogicalRequest(t *testing.T) {
	for _, provider := range []struct {
		name string
		body string
		new  func(ClientConfig) LLMClient
	}{
		{name: "openai", body: openAIOKBody, new: func(cfg ClientConfig) LLMClient { return NewOpenAIClient(cfg) }},
		{name: "anthropic", body: anthropicOKBody, new: func(cfg ClientConfig) LLMClient { return NewAnthropicClient(cfg) }},
		{name: "responses", body: responsesOKBody, new: func(cfg ClientConfig) LLMClient { return NewOpenAIResponsesClient(cfg) }},
	} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%v", provider.name, fail), func(t *testing.T) {
				var output progressBuffer
				defer stdout.Swap(&output)()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					inFlight := output.String()
					if !strings.Contains(inFlight, "LLM request started") || strings.Contains(inFlight, "LLM request finished") {
						t.Errorf("request progress must start before the HTTP response:\n%s", inFlight)
					}
					w.Header().Set("Content-Type", "application/json")
					if fail {
						w.WriteHeader(http.StatusForbidden)
						_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"permission_error","message":"private provider error"}}`)
						return
					}
					_, _ = fmt.Fprint(w, provider.body)
				}))
				defer server.Close()
				client := provider.new(ClientConfig{URL: server.URL + "/v1", APIKey: "private-api-key", Model: "configured-model"})
				response, err := client.CompletionsWithCtx(metaCtx(testMeta()), ChatRequest{
					Messages: []Message{NewTextMessage("user", "private prompt")}, MaxTokens: 64,
				})
				if fail {
					if err == nil || response != nil {
						t.Fatalf("want request failure, got response=%+v err=%v", response, err)
					}
				} else if err != nil || response == nil {
					t.Fatalf("want response, got response=%+v err=%v", response, err)
				}
				progress := output.String()
				for _, event := range []string{"LLM request started", "LLM request finished"} {
					if strings.Count(progress, event) != 1 {
						t.Errorf("want one %s:\n%s", event, progress)
					}
				}
				status := "ok"
				if fail {
					status = "failed"
				}
				if !strings.Contains(progress, "status="+status) {
					t.Errorf("missing final status %s:\n%s", status, progress)
				}
				if strings.Contains(progress, "private") {
					t.Errorf("request progress disclosed content or credentials:\n%s", progress)
				}
				assertProgressDurations(t, progress)
			})
		}
	}
}
