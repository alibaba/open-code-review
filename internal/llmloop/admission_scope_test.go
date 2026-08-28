// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// scopeCaptureClient records whether each request's context still carries the
// review admission scope. It is the admission twin of metaCaptureClient
// (retry_identity_test.go): the scope travels in the context, so the client
// boundary is the only place it is observable from this package.
type scopeCaptureClient struct {
	mu      sync.Mutex
	scoped  []bool
	respond func(n int) *llm.ChatResponse
}

func (c *scopeCaptureClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	n := len(c.scoped)
	c.scoped = append(c.scoped, llm.HasAdmissionScope(ctx))
	c.mu.Unlock()
	if c.respond != nil {
		return c.respond(n), nil
	}
	return emptyResponse(), nil
}

func (c *scopeCaptureClient) snapshots() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.scoped...)
}

// wantAllScoped fails unless every captured request carried the scope. It also
// demands at least one request, so a path that silently stops calling the LLM
// cannot pass by vacuity.
func wantAllScoped(t *testing.T, got []bool) {
	t.Helper()
	if len(got) == 0 {
		t.Fatal("no requests captured — path never called the LLM")
	}
	for i, ok := range got {
		if !ok {
			t.Fatalf("request %d lost the admission scope mid-path", i)
		}
	}
}

// TestAdmissionScope_SurvivesCompression asserts both compression paths keep
// the scope: the synchronous runCompression call and the async job, whose
// context is rebuilt via context.WithTimeout(context.WithoutCancel(ctx), 5m) —
// the exact spot where a maintainer fixing "detached deadline" semantics might
// swap in context.Background() and silently drop every context value.
func TestAdmissionScope_SurvivesCompression(t *testing.T) {
	t.Run("synchronous", func(t *testing.T) {
		summary := "compressed"
		client := &scopeCaptureClient{respond: func(int) *llm.ChatResponse {
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
		}}
		r, msgs := newCompressionRunner(t, client, nil)
		if _, err := r.runCompression(llm.ContextWithAdmissionScope(context.Background()), msgs, "test.go"); err != nil {
			t.Fatalf("runCompression: %v", err)
		}
		wantAllScoped(t, client.snapshots())
	})

	t.Run("async WithoutCancel child", func(t *testing.T) {
		summary := "compressed"
		client := &scopeCaptureClient{respond: func(int) *llm.ChatResponse {
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
		}}
		r, msgs := newCompressionRunner(t, client, nil)

		// The parent is cancelled before the job runs, as a finishing file
		// conversation is; the child must still carry the scope.
		parent, cancel := context.WithCancel(llm.ContextWithAdmissionScope(context.Background()))
		st := &compressionState{}
		r.triggerAsyncCompression(parent, st, msgs, "test.go")
		cancel()
		r.WaitBackground()

		if st.pendingJob == nil {
			t.Fatal("no compression job was triggered")
		}
		wantAllScoped(t, client.snapshots())
	})
}
