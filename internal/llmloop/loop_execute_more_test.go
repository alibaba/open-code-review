// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestExecuteToolCall_TaskDone covers every branch of the task_done handling:
// argument parse error, missing state (implicit completion), non-string state,
// explicit DONE / FAILED, and an unrecognized state value.
func TestExecuteToolCall_TaskDone(t *testing.T) {
	newRunner := func() *Runner {
		reg := tool.NewRegistry()
		reg.Freeze()
		return NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})
	}

	call := func(args string) tool.TaskCheckpoint {
		return newRunner().executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: tool.TaskDone.Name(), Arguments: args},
		}, nil)
	}

	t.Run("parse error", func(t *testing.T) {
		cp := call(`{bad`)
		if !strings.Contains(cp.Data, "Error parsing tool arguments") {
			t.Errorf("cp.Data = %q, want parse-error message", cp.Data)
		}
	})

	t.Run("missing state completes", func(t *testing.T) {
		cp := call(`{}`)
		if !cp.Completed || cp.Failed {
			t.Errorf("cp = %+v, want Completed", cp)
		}
	})

	t.Run("non-string state", func(t *testing.T) {
		cp := call(`{"state":123}`)
		if !strings.Contains(cp.Data, "must be DONE or FAILED") {
			t.Errorf("cp.Data = %q, want non-string state message", cp.Data)
		}
	})

	t.Run("DONE completes", func(t *testing.T) {
		cp := call(`{"state":"DONE"}`)
		if !cp.Completed || cp.Failed {
			t.Errorf("cp = %+v, want Completed", cp)
		}
	})

	t.Run("FAILED fails", func(t *testing.T) {
		cp := call(`{"state":"FAILED"}`)
		if !cp.Failed {
			t.Errorf("cp = %+v, want Failed", cp)
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		cp := call(`{"state":"MAYBE"}`)
		if !strings.Contains(cp.Data, "invalid task_done state") {
			t.Errorf("cp.Data = %q, want invalid-state message", cp.Data)
		}
	})
}

// TestExecuteToolCall_CodeCommentAsyncPool covers the async dispatch path where a
// CommentWorkerPool is present: the call returns immediately with a success
// checkpoint, records "(async)" on the task record, and the comment lands in the
// collector once the pool drains.
func TestExecuteToolCall_CodeCommentAsyncPool(t *testing.T) {
	collector := tool.NewCommentCollector()
	pool := NewCommentWorkerPool(2)
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	r := NewRunner(Deps{
		Tools:             reg,
		CommentCollector:  collector,
		CommentWorkerPool: pool,
	})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "async.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"content":"issue","existing_code":"foo"}]}`,
		},
	}, rec)

	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	if len(rec.ToolResults) != 1 || rec.ToolResults[0].Result != "(async)" {
		t.Errorf("recorded results = %+v, want one (async) entry", rec.ToolResults)
	}

	// Drain the pool and confirm the comment was collected with the injected path.
	comments := r.CollectPendingComments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	if comments[0].Path != "async.go" {
		t.Errorf("comment path = %q, want async.go", comments[0].Path)
	}
}

// TestExecuteToolCall_CodeCommentDiffResolved covers the synchronous code_comment
// path where DiffLookup returns a diff and ResolveComment resolves the line
// numbers from file content (so the re-location LLM branch is skipped), with a
// non-nil task record so AddToolResult runs.
func TestExecuteToolCall_CodeCommentDiffResolved(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	diffLookup := func(path string) *model.Diff {
		return &model.Diff{
			NewPath:        path,
			NewFileContent: "line one\nfoo bar\nline three\n",
		}
	}

	r := NewRunner(Deps{
		Tools:            reg,
		CommentCollector: collector,
		DiffLookup:       diffLookup,
	})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "resolved.go", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"content":"issue","existing_code":"foo bar"}]}`,
		},
	}, rec)

	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	if len(rec.ToolResults) != 1 || rec.ToolResults[0].Result != tool.CommentSucceed {
		t.Errorf("recorded results = %+v, want one success entry", rec.ToolResults)
	}

	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatalf("collected %d comments, want 1", len(comments))
	}
	// ResolveComment should have located "foo bar" on line 2 of NewFileContent.
	if comments[0].StartLine != 2 {
		t.Errorf("comment StartLine = %d, want 2 (resolved from file content)", comments[0].StartLine)
	}
}
