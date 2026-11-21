// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

// failureCompressionClient lets the test hold a compression request open so
// the async job's state transitions are observable without polling.
type failureCompressionClient struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan error
}

func newFailureCompressionClient() *failureCompressionClient {
	return &failureCompressionClient{
		started: make(chan struct{}, 8),
		release: make(chan error, 8),
	}
}

func (c *failureCompressionClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()

	select {
	case c.started <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case err := <-c.release:
		if err != nil {
			return nil, err
		}
		summary := "summary"
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}},
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *failureCompressionClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func waitCompressionStarted(t *testing.T, c *failureCompressionClient) {
	t.Helper()
	select {
	case <-c.started:
	case <-time.After(2 * time.Second):
		t.Fatal("compression request did not start")
	}
}

func TestAsyncCompression_DeterministicFailureBlocksSnapshot(t *testing.T) {
	client := newFailureCompressionClient()
	r, msgs := newCompressionRunner(t, client, nil)
	st := &compressionState{}

	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")
	waitCompressionStarted(t, client)
	client.release <- fmt.Errorf("context window exceeded: %w", llm.ErrTokenBudget)
	r.WaitBackground()

	if !st.compressionBlocked(msgs) {
		t.Fatal("a deterministic rejection must block the failed snapshot")
	}
	if got := client.callCount(); got != 1 {
		t.Fatalf("compression requests = %d, want 1", got)
	}

	// The unchanged snapshot must not be resubmitted.
	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")
	select {
	case <-client.started:
		t.Fatal("unchanged failed snapshot was resubmitted")
	case <-time.After(100 * time.Millisecond):
	}

	// A genuinely changed history is a new request and is allowed.
	changed := append(append([]llm.Message(nil), msgs...), llm.NewTextMessage("user", "next turn"))
	if st.compressionBlocked(changed) {
		t.Fatal("a changed snapshot must not inherit the old failure")
	}
	r.triggerAsyncCompression(context.Background(), st, changed, "test.go")
	waitCompressionStarted(t, client)
	client.release <- nil
	r.WaitBackground()

	if st.compressionBlocked(changed) {
		t.Fatal("a successful compression must clear the block")
	}
	if got := client.callCount(); got != 2 {
		t.Fatalf("compression requests = %d, want 2", got)
	}
}

func TestAsyncCompression_TransientFailureRetriesOnce(t *testing.T) {
	client := newFailureCompressionClient()
	r, msgs := newCompressionRunner(t, client, nil)
	st := &compressionState{}

	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")
	waitCompressionStarted(t, client)
	client.release <- errors.New("temporary transport error")
	r.WaitBackground()

	if st.compressionBlocked(msgs) {
		t.Fatal("the first transient failure must allow one retry")
	}

	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")
	waitCompressionStarted(t, client)
	client.release <- errors.New("temporary transport error")
	r.WaitBackground()

	if !st.compressionBlocked(msgs) {
		t.Fatal("a second transient failure must block the snapshot")
	}
	if got := client.callCount(); got != 2 {
		t.Fatalf("compression requests = %d, want 2", got)
	}

	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")
	select {
	case <-client.started:
		t.Fatal("exhausted snapshot was resubmitted")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAddNextMessage_StopsOnBlockedCompression(t *testing.T) {
	client := &metaCaptureClient{}
	r, msgs := newCompressionRunner(t, client, nil)
	st := &compressionState{
		failedSnapshotID: compressionSnapshotID(msgs),
		failedRetries:    maxCompressionAttempts,
		blocked:          true,
	}

	stop := r.addNextMessage(context.Background(), "", nil, llm.NativeTurn{}, "", nil, &msgs, "test.go", st)
	if stop != StopCompression {
		t.Fatalf("stop = %v, want StopCompression", stop)
	}
	if got := len(client.requests()); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

type statusError struct{ code int }

func (e statusError) Error() string       { return "provider error" }
func (e statusError) HTTPStatusCode() int { return e.code }

func TestPermanentCompressionFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"token budget", fmt.Errorf("wrapped: %w", llm.ErrTokenBudget), true},
		{"bad request", statusError{http.StatusBadRequest}, true},
		{"unauthorized", statusError{http.StatusUnauthorized}, true},
		{"payload too large", statusError{http.StatusRequestEntityTooLarge}, true},
		{"rate limited", statusError{http.StatusTooManyRequests}, false},
		{"server error", statusError{http.StatusInternalServerError}, false},
		{"plain transient", errors.New("temporary"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := permanentCompressionFailure(tc.err); got != tc.want {
				t.Fatalf("permanentCompressionFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestCompressionSnapshotID(t *testing.T) {
	base := []llm.Message{
		llm.NewTextMessage("system", "sys"),
		llm.NewTextMessage("user", "one"),
	}
	same := append([]llm.Message(nil), base...)
	if compressionSnapshotID(base) != compressionSnapshotID(same) {
		t.Fatal("identical snapshots must have the same ID")
	}

	changed := append(append([]llm.Message(nil), base...), llm.NewTextMessage("assistant", "two"))
	if compressionSnapshotID(base) == compressionSnapshotID(changed) {
		t.Fatal("a changed snapshot must have a different ID")
	}

	call := func(args string) []llm.Message {
		return append(append([]llm.Message(nil), base...), llm.NewToolCallMessage("", []llm.ToolCall{{
			ID:       "call-1",
			Type:     "function",
			Function: llm.FunctionCall{Name: "file_read", Arguments: args},
		}}, llm.NativeTurn{}, ""))
	}
	if compressionSnapshotID(call(`{"path":"a.go"}`)) == compressionSnapshotID(call(`{"path":"b.go"}`)) {
		t.Fatal("a changed tool-call argument must change the snapshot ID")
	}
}
