// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/openai/openai-go/v3"
)

type compressionRetryProbeClient struct {
	mu          sync.Mutex
	inputs      [][sha256.Size]byte
	inputBytes  []int
	compression func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)
}

func (c *compressionRetryProbeClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
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
	return nil, compressionRetryInputError()
}

func compressionRetryInputError() error {
	request, _ := http.NewRequest(http.MethodPost, "http://example.invalid/v1/chat/completions", nil)
	return &openai.Error{
		StatusCode: http.StatusBadRequest,
		Request:    request,
		Response:   &http.Response{StatusCode: http.StatusBadRequest},
		Code:       "context_length_exceeded",
		Message:    "compression input rejected",
	}
}

func compressionRetryProbeRunner(t *testing.T, client *compressionRetryProbeClient) *Runner {
	t.Helper()
	deps := Deps{
		LLMClient: client,
		Model:     "fake",
		Session:   session.New(t.TempDir(), "main", "fake", session.SessionOptions{}),
	}
	t.Cleanup(func() {
		if err := deps.Session.Finalize(); err != nil {
			t.Error(err)
		}
	})
	return NewRunner(deps, Task{
		MaxContextTokens: 1000,
		Compression: CompressionPolicy{
			Messages:        []MessageTemplate{{Role: "user", Content: "Summarize: {{context}}"}},
			SummaryTemplate: "\n\n<previous_review_summary>\n{{summary}}\n</previous_review_summary>",
		},
	})
}

func compressionRetryProbeCounts(t *testing.T, client *compressionRetryProbeClient) (int, int) {
	t.Helper()
	client.mu.Lock()
	defer client.mu.Unlock()
	unique := make(map[[sha256.Size]byte]struct{})
	for _, hash := range client.inputs {
		unique[hash] = struct{}{}
	}
	t.Logf("compression_requests=%d unique_compression_inputs=%d duplicate_inputs=%d input_bytes=%v",
		len(client.inputs), len(unique), len(client.inputs)-len(unique), client.inputBytes)
	return len(client.inputs), len(unique)
}

func TestCompressionRetryRejectedInput(t *testing.T) {
	t.Run("fixed_snapshot_direct_helper", func(t *testing.T) {
		client := &compressionRetryProbeClient{}
		runner := compressionRetryProbeRunner(t, client)
		messages := []llm.Message{
			msg("system", "system"), msg("user", "review"),
			msg("assistant", "read file"), msg("tool", strings.Repeat("word ", 650)),
		}
		st := &compressionState{}
		for i := 0; i < 4; i++ {
			runner.triggerAsyncCompression(context.Background(), st, messages, "sample.go")
			runner.WaitBackground()
		}
		calls, unique := compressionRetryProbeCounts(t, client)
		if calls != 1 || unique != 1 {
			t.Fatalf("rejected input was submitted again: calls=%d unique=%d", calls, unique)
		}
	})

}

func TestCompressionRetryChangedInputAndConversation(t *testing.T) {
	client := &compressionRetryProbeClient{}
	runner := compressionRetryProbeRunner(t, client)
	st := &compressionState{}
	messages := []llm.Message{msg("system", "system"), msg("user", "review"), msg("assistant", "old content")}
	before := copyMessages(messages)
	var firstErr error
	for i := 0; i < 2; i++ {
		got, err := runner.runCompression(context.Background(), messages, "sample.go", st)
		if err == nil || !reflect.DeepEqual(got, before) {
			t.Fatalf("failed compression changed messages: err=%v", err)
		}
		if i == 0 {
			firstErr = err
		} else if err != firstErr {
			t.Fatal("cached failure must preserve the original error")
		}
	}
	messages[2] = msg("assistant", "new content")
	if _, err := runner.runCompression(context.Background(), messages, "sample.go", st); err == nil {
		t.Fatal("changed input should make a new failing attempt")
	}
	if _, err := runner.runCompression(context.Background(), messages, "another.go", &compressionState{}); err == nil {
		t.Fatal("another conversation should make its own failing attempt")
	}
	calls, unique := compressionRetryProbeCounts(t, client)
	if calls != 3 || unique != 2 {
		t.Fatalf("calls=%d unique=%d, want 3 and 2", calls, unique)
	}
}

func TestCompressionRetryTransientFailureCanRecover(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, errors.New("400 context_length_exceeded")} {
		t.Run(failure.Error(), func(t *testing.T) {
			client := &compressionRetryProbeClient{}
			var attempts int
			client.compression = func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				attempts++
				if attempts == 1 {
					return nil, failure
				}
				summary := "summary"
				return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
			}
			runner := compressionRetryProbeRunner(t, client)
			st := &compressionState{}
			messages := []llm.Message{msg("system", "system"), msg("user", "review"), msg("assistant", "content")}
			if _, err := runner.runCompression(context.Background(), messages, "sample.go", st); !errors.Is(err, failure) {
				t.Fatalf("first error=%v, want %v", err, failure)
			}
			got, err := runner.runCompression(context.Background(), messages, "sample.go", st)
			if err != nil || len(got) != 2 || !strings.Contains(got[1].ExtractText(), "previous_review_summary") {
				t.Fatalf("retry failed to recover: err=%v messages=%v", err, got)
			}
			if attempts != 2 {
				t.Fatalf("attempts=%d, want 2", attempts)
			}
		})
	}
}

func TestCompressionRetryCancelledJobCannotReplaceRejection(t *testing.T) {
	client := &compressionRetryProbeClient{}
	started, release := make(chan struct{}), make(chan struct{})
	var attempts atomic.Int32
	client.compression = func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		if attempts.Add(1) == 1 {
			close(started)
			<-release
		}
		return nil, compressionRetryInputError()
	}
	runner := compressionRetryProbeRunner(t, client)
	st := &compressionState{}
	messages := []llm.Message{msg("system", "system"), msg("user", "review"), msg("assistant", "old content")}
	runner.triggerAsyncCompression(context.Background(), st, messages, "sample.go")
	<-started
	runner.cancelPendingCompression(st)
	messages[2] = msg("assistant", "new content")
	_, firstErr := runner.runCompression(context.Background(), messages, "sample.go", st)
	close(release)
	runner.WaitBackground()
	_, cachedErr := runner.runCompression(context.Background(), messages, "sample.go", st)
	if firstErr == nil || cachedErr != firstErr || attempts.Load() != 2 {
		t.Fatalf("cancelled job replaced newer rejection: attempts=%d first=%v cached=%v", attempts.Load(), firstErr, cachedErr)
	}
}
