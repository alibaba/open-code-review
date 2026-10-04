// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// recoveryTestAgent builds just enough Agent for the resilience helper: real
// session (records), real runner (warnings), nil-safe coverage transitions
// (no manifest builder), and a stubbed subtask executor.
func recoveryTestAgent(t *testing.T) *Agent {
	t.Helper()
	sess := session.New(t.TempDir(), "main", "m", session.SessionOptions{})
	return &Agent{
		args:    Args{CommentCollector: tool.NewCommentCollector()},
		session: sess,
		runner:  llmloop.NewRunner(llmloop.Deps{Session: sess, CommentCollector: tool.NewCommentCollector()}),
	}
}

func recoveryGroup(paths ...string) FileGroup {
	g := FileGroup{Label: "grp"}
	for _, p := range paths {
		g.Diffs = append(g.Diffs, model.Diff{NewPath: p, OldPath: p, Diff: "--- a/" + p + "\n+++ b/" + p + "\n"})
	}
	return g
}

func deadlineErr() error {
	return fmt.Errorf("LLM completion error: %w", context.DeadlineExceeded)
}

func TestExecuteGroupResilientRetryThenSuccess(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go", "b.go")
	calls := 0
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		calls++
		// Each attempt must carry a fresh per-attempt deadline, not the
		// already-expired one from the failed attempt.
		if _, ok := ctx.Deadline(); !ok {
			t.Errorf("attempt %d: ctx has no deadline; want a fresh per-attempt timeout", calls)
		}
		if calls == 1 {
			return false, nil, deadlineErr()
		}
		return true, nil, nil
	}
	a.executeGroupResilient(context.Background(), g, 30*time.Minute, exec)
	if calls != 2 {
		t.Fatalf("exec calls = %d, want exactly one retry", calls)
	}
	if atomic.LoadInt64(&a.subtaskFailed) != 0 {
		t.Fatalf("subtaskFailed = %d, want 0 after recovery", atomic.LoadInt64(&a.subtaskFailed))
	}
	assertRecoveryWarnings(t, a, true, false)
}

func TestExecuteGroupResilientSplitAfterFailedRetry(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go", "b.go")
	var callSizes []int
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		callSizes = append(callSizes, len(g.Diffs))
		if len(g.Diffs) == 2 {
			return false, nil, deadlineErr() // whole-group attempts keep stalling
		}
		if g.Diffs[0].NewPath == "b.go" {
			return false, nil, deadlineErr() // one file is genuinely stuck
		}
		return true, nil, nil
	}
	a.executeGroupResilient(context.Background(), g, 0, exec)
	want := []int{2, 2, 1, 1}
	if fmt.Sprint(callSizes) != fmt.Sprint(want) {
		t.Fatalf("call sizes = %v, want %v (retry the group once, then one attempt per file)", callSizes, want)
	}
	if atomic.LoadInt64(&a.subtaskFailed) != 1 {
		t.Fatalf("subtaskFailed = %d, want 1 (only the stuck file)", atomic.LoadInt64(&a.subtaskFailed))
	}
	assertRecoveryWarnings(t, a, true, true)
}

func TestExecuteGroupResilientNonDeadlineErrorNoRetry(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go", "b.go")
	calls := 0
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		calls++
		return false, nil, errors.New("provider exploded")
	}
	a.executeGroupResilient(context.Background(), g, 0, exec)
	if calls != 1 {
		t.Fatalf("exec calls = %d, want 1 (only deadline failures retry)", calls)
	}
	if atomic.LoadInt64(&a.subtaskFailed) != 2 {
		t.Fatalf("subtaskFailed = %d, want 2 (whole group failed)", atomic.LoadInt64(&a.subtaskFailed))
	}
	assertRecoveryWarnings(t, a, false, false)
}

func TestExecuteGroupResilientParentCancelNoRetry(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go", "b.go")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		calls++
		return false, nil, deadlineErr()
	}
	a.executeGroupResilient(ctx, g, 0, exec)
	if calls != 1 {
		t.Fatalf("exec calls = %d, want 1 (a cancelled run is shutting down, not stalling)", calls)
	}
	assertRecoveryWarnings(t, a, false, false)
}

func TestExecuteGroupResilientGateOnExistingComments(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go", "b.go")
	a.args.CommentCollector.Add(model.LlmComment{Path: "a.go", Content: "found something"})
	calls := 0
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		calls++
		return false, nil, deadlineErr()
	}
	a.executeGroupResilient(context.Background(), g, 0, exec)
	if calls != 1 {
		t.Fatalf("exec calls = %d, want 1 (a fresh attempt would re-file existing comments)", calls)
	}
	if atomic.LoadInt64(&a.subtaskFailed) != 2 {
		t.Fatalf("subtaskFailed = %d, want 2", atomic.LoadInt64(&a.subtaskFailed))
	}
	assertRecoveryWarnings(t, a, false, false)
}

// TestExecuteGroupResilientGateReevaluatedAfterRetry pins the duplicate
// guard: if the first retry attempt files a comment before dying on deadline
// again, the group must NOT split — the split's fresh conversations would
// re-file that comment. The group keeps its whole-group failed outcome.
func TestExecuteGroupResilientGateReevaluatedAfterRetry(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go", "b.go")
	calls := 0
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		calls++
		if calls == 2 {
			a.args.CommentCollector.Add(model.LlmComment{Path: "a.go", Content: "filed by the retry"})
		}
		return false, nil, deadlineErr()
	}
	a.executeGroupResilient(context.Background(), g, 0, exec)
	if calls != 2 {
		t.Fatalf("exec calls = %d, want 2 (retry, then stop)", calls)
	}
	if atomic.LoadInt64(&a.subtaskFailed) != 2 {
		t.Fatalf("subtaskFailed = %d, want 2 (whole group failed, no split)", atomic.LoadInt64(&a.subtaskFailed))
	}
	assertRecoveryWarnings(t, a, true, false)
}

func TestExecuteGroupResilientSingleFileRetriesButNeverSplits(t *testing.T) {
	a := recoveryTestAgent(t)
	g := recoveryGroup("a.go")
	calls := 0
	exec := func(ctx context.Context, g FileGroup) (bool, *subtaskStop, error) {
		calls++
		return false, nil, deadlineErr()
	}
	a.executeGroupResilient(context.Background(), g, 0, exec)
	if calls != 2 {
		t.Fatalf("exec calls = %d, want 2 (retry once, no split below the group level)", calls)
	}
	if atomic.LoadInt64(&a.subtaskFailed) != 1 {
		t.Fatalf("subtaskFailed = %d, want 1", atomic.LoadInt64(&a.subtaskFailed))
	}
	assertRecoveryWarnings(t, a, true, false)
}

// assertRecoveryWarnings checks the recovery signals on the runner's warning
// list: these must be recorded even when the recovered run then succeeds,
// because they are the only durable trace that coverage was restored by a
// retry/split rather than a single clean pass.
func assertRecoveryWarnings(t *testing.T, a *Agent, wantRetry, wantSplit bool) {
	t.Helper()
	var sawRetry, sawSplit bool
	for _, w := range a.runner.Warnings() {
		switch w.Type {
		case "group_retry":
			sawRetry = true
		case "group_split":
			sawSplit = true
		}
	}
	if sawRetry != wantRetry || sawSplit != wantSplit {
		t.Fatalf("warnings retry=%v split=%v, want retry=%v split=%v", sawRetry, sawSplit, wantRetry, wantSplit)
	}
}
