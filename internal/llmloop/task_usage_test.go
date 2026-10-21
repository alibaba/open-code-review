// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/progress"
)

// The dashboard's per-row token column answers "what did this row cost". The
// run-wide counters cannot answer it — while several groups run at once they
// cannot say which group spent what — so each task publishes its own total.

// captureProgress collects the events fn publishes.
func captureProgress(t *testing.T, fn func()) []progress.Event {
	t.Helper()
	var got []progress.Event
	restore := progress.SetSinkForTest(func(ev progress.Event) {
		got = append(got, ev)
	})
	defer restore()
	fn()
	return got
}

func TestRecordTaskUsagePublishesTheTasksRunningTotal(t *testing.T) {
	r := &Runner{}

	got := captureProgress(t, func() {
		r.recordTaskUsage("a.go", &llm.UsageInfo{PromptTokens: 100, CompletionTokens: 10})
		r.recordTaskUsage("a.go", &llm.UsageInfo{PromptTokens: 50, CompletionTokens: 5})
	})

	if len(got) != 2 {
		t.Fatalf("expected an event per response, got %d", len(got))
	}
	last := got[1]
	if last.Kind != progress.KindUsage {
		t.Errorf("kind = %v, want KindUsage", last.Kind)
	}
	if last.Group != "a.go" {
		t.Errorf("group = %q, want the task's key", last.Group)
	}
	// The total, not the last response's delta: a task makes several calls
	// and the row must show what it spent altogether.
	if last.InputTokens != 150 || last.OutputTokens != 15 {
		t.Errorf("usage = %d in / %d out, want 150 in / 15 out",
			last.InputTokens, last.OutputTokens)
	}
}

func TestRecordTaskUsageKeepsTasksApart(t *testing.T) {
	r := &Runner{}

	got := captureProgress(t, func() {
		r.recordTaskUsage("a.go", &llm.UsageInfo{PromptTokens: 100, CompletionTokens: 10})
		r.recordTaskUsage("b.go", &llm.UsageInfo{PromptTokens: 7, CompletionTokens: 3})
	})

	if len(got) != 2 {
		t.Fatalf("expected an event per response, got %d", len(got))
	}
	if got[1].Group != "b.go" {
		t.Errorf("group = %q, want b.go", got[1].Group)
	}
	if got[1].InputTokens != 7 || got[1].OutputTokens != 3 {
		t.Errorf("usage = %d in / %d out, want 7 in / 3 out",
			got[1].InputTokens, got[1].OutputTokens)
	}
}

// A response with no usage reports nothing rather than a zero total, which
// would read as a task that spent nothing.
func TestRecordTaskUsageIgnoresAResponseWithoutUsage(t *testing.T) {
	r := &Runner{}

	got := captureProgress(t, func() { r.recordTaskUsage("a.go", nil) })

	if len(got) != 0 {
		t.Fatalf("expected no event, got %d: %+v", len(got), got)
	}
}
