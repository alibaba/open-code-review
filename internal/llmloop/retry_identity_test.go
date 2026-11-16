// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

// capturedRequest is one observed CompletionsWithCtx call and the request
// identity its context carried, if any.
type capturedRequest struct {
	meta llm.RequestMeta
	ok   bool
}

// metaCaptureClient records the RequestMeta of every request it receives. It is
// the only way to check identity from outside package llm: the meta travels in
// the context, so the client is where it becomes observable.
type metaCaptureClient struct {
	mu       sync.Mutex
	captured []capturedRequest
	// respond is called with the zero-based call index so a test can drive a
	// multi-round tool loop.
	respond func(n int) *llm.ChatResponse
	// respondErr drives request failures and takes precedence over respond.
	respondErr func(n int) (*llm.ChatResponse, error)
}

func (c *metaCaptureClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, ok := llm.RequestMetaFromContext(ctx)
	c.mu.Lock()
	n := len(c.captured)
	c.captured = append(c.captured, capturedRequest{meta: meta, ok: ok})
	c.mu.Unlock()
	if c.respondErr != nil {
		return c.respondErr(n)
	}
	if c.respond == nil {
		return emptyResponse(), nil
	}
	return c.respond(n), nil
}

func (c *metaCaptureClient) requests() []capturedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedRequest(nil), c.captured...)
}

func emptyResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
	}
}

// metaFactory builds the same closure internal/agent injects, so these tests
// exercise the review wiring rather than a test-only shape.
func metaFactory(provider, modelName string) func(string, session.TaskType, int) llm.RequestMeta {
	return func(filePath string, taskType session.TaskType, requestNo int) llm.RequestMeta {
		return llm.RequestMeta{
			Provider:  provider,
			Model:     modelName,
			FilePath:  filePath,
			TaskType:  string(taskType),
			RequestNo: requestNo,
		}
	}
}

func wantMeta(t *testing.T, got capturedRequest, want llm.RequestMeta) {
	t.Helper()
	if !got.ok {
		t.Fatalf("request carried no identity, want %+v", want)
	}
	if got.meta != want {
		t.Errorf("meta = %+v, want %+v", got.meta, want)
	}
}

// newCompressionRunner returns a Runner whose template forces runCompression to
// issue a request, plus the message slice to feed it.
func newCompressionRunner(t *testing.T, client llm.LLMClient, factory func(string, session.TaskType, int) llm.RequestMeta) (*Runner, []llm.Message) {
	t.Helper()
	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
	r := NewRunner(Deps{
		LLMClient: client, Model: "fake", Session: sess, NewRequestMeta: factory,
	}, Task{
		Compression: CompressionPolicy{
			Messages:        []MessageTemplate{{Role: "user", Content: "Summarize: {{context}}"}},
			SummaryTemplate: "\n\n<summary>\n{{summary}}\n</summary>",
		},
		MaxContextTokens: 50,
	})

	msgs := []llm.Message{
		llm.NewTextMessage("system", "sys"),
		llm.NewTextMessage("user", "prompt"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.NewTextMessage("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, llm.NewTextMessage("tool", strings.Repeat("data ", 50)))
	}
	return r, msgs
}

// TestRunCompression_Identity covers both compression paths at once: the
// synchronous one is this call, and the async one reaches the same function
// through triggerAsyncCompression, so identity is stamped for both.
func TestRunCompression_Identity(t *testing.T) {
	summary := "compressed summary"
	client := &metaCaptureClient{respond: func(int) *llm.ChatResponse {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
	}}
	r, msgs := newCompressionRunner(t, client, metaFactory("openai", "fake"))

	if _, err := r.runCompression(context.Background(), msgs, "test.go"); err != nil {
		t.Fatalf("runCompression: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	wantMeta(t, reqs[0], llm.RequestMeta{
		Provider:  "openai",
		Model:     "fake",
		FilePath:  "test.go",
		TaskType:  string(session.MemoryCompressionTask),
		RequestNo: 1,
	})
}

func TestRunCompression_NoIdentityWhenFactoryNil(t *testing.T) {
	summary := "compressed summary"
	client := &metaCaptureClient{respond: func(int) *llm.ChatResponse {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
	}}
	r, msgs := newCompressionRunner(t, client, nil)

	if _, err := r.runCompression(context.Background(), msgs, "test.go"); err != nil {
		t.Fatalf("runCompression: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	if reqs[0].ok {
		t.Errorf("scan compression carried identity %+v, want none", reqs[0].meta)
	}
}
