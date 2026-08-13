// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

// blockingCompressionClient stands in for the real client boundary of a
// background memory-compression request: it records the request's attempts on a
// shared collector, blocks until the test releases it, and only then finalizes
// the logical request — the same order internal/llm's CompletionsWithCtx uses
// (observer per attempt, Finalize in the boundary defer). Blocking is what makes
// the un-finalized window observable; a real background job's window is however
// long the request takes.
type blockingCompressionClient struct {
	collector *llm.RetryCollector
	started   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func newBlockingCompressionClient(c *llm.RetryCollector) *blockingCompressionClient {
	return &blockingCompressionClient{
		collector: c,
		started:   make(chan struct{}),
		release:   make(chan struct{}),
	}
}

func (c *blockingCompressionClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, ok := llm.RequestMetaFromContext(ctx)
	if !ok {
		return emptyResponse(), nil
	}
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	// One retried request, so the frozen report has something to list rather
	// than being the (nil, nil) "nothing worth reporting" shape.
	c.collector.RecordAttempt(meta, llm.AttemptRecord{
		ErrorClass: llm.ErrorClassRateLimited, FailurePhase: llm.FailurePhaseHTTP, StatusCode: 429,
	}, base, base.Add(10*time.Millisecond))
	c.collector.RecordAttempt(meta, llm.AttemptRecord{}, base.Add(time.Second), base.Add(time.Second+10*time.Millisecond))

	c.once.Do(func() { close(c.started) })
	<-c.release

	c.collector.Finalize(meta, nil, false)
	summary := "compressed summary"
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
}

// The run boundary joins background memory compression before
// the report is frozen. Without that join, Freeze can see a request that
// recorded attempts but was never finalized, which is an invariant violation and
// drops the whole run's report — so the barrier is what makes the report
// publishable at all, not just more complete.
func TestWaitBackground_JoinsCompressionBeforeFreeze(t *testing.T) {
	collector := llm.NewRetryCollector()
	client := newBlockingCompressionClient(collector)
	r, msgs := newCompressionRunner(t, client, metaFactory("openai", "fake"))

	st := &compressionState{}
	r.triggerAsyncCompression(context.Background(), st, msgs, "test.go")

	// The job is mid-request: its attempts are recorded, its outcome is not.
	<-client.started
	if rep, err := collector.Freeze("run-uuid"); err == nil {
		t.Fatalf("an un-finalized background request must fail Freeze, got report %+v", rep)
	} else if !strings.Contains(err.Error(), "not finalized") {
		t.Errorf("Freeze error = %v, want it to name the un-finalized request", err)
	}

	close(client.release)
	r.WaitBackground()

	rep, err := collector.Freeze("run-uuid")
	if err != nil {
		t.Fatalf("Freeze after WaitBackground: %v", err)
	}
	if rep == nil {
		t.Fatal("expected a report for the retried compression request")
	}
	if len(rep.Requests) != 1 {
		t.Fatalf("report lists %d requests, want the compression one: %+v", len(rep.Requests), rep.Requests)
	}
	got := rep.Requests[0]
	if got.TaskType != string(session.MemoryCompressionTask) {
		t.Errorf("task_type = %q, want %q", got.TaskType, session.MemoryCompressionTask)
	}
	if got.Outcome != llm.OutcomeRecovered {
		t.Errorf("outcome = %q, want recovered", got.Outcome)
	}
	if rep.RetriedRequests != 1 || rep.TotalRetries != 1 || rep.RecoveredRequests != 1 {
		t.Errorf("aggregates = %+v, want one retried/recovered request", rep)
	}
}

// WaitBackground must also be safe when no job ever started and when it is
// called twice, because the run boundary calls it unconditionally on every exit.
func TestWaitBackground_NoJobIsANoOp(t *testing.T) {
	collector := llm.NewRetryCollector()
	r, _ := newCompressionRunner(t, newBlockingCompressionClient(collector), metaFactory("openai", "fake"))

	r.WaitBackground()
	r.WaitBackground()

	rep, err := collector.Freeze("run-uuid")
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if rep != nil {
		t.Errorf("a run that made no request must report nothing, got %+v", rep)
	}
}
