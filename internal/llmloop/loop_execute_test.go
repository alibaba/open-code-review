// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// erroringProvider is a dynamic tool provider whose Execute always fails, used
// to drive executeToolCall's dynamic-tool error branch.
type erroringProvider struct {
	tool tool.Tool
}

func (p *erroringProvider) Tool() tool.Tool { return p.tool }
func (p *erroringProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "", errors.New("boom")
}

// TestExecuteToolCall_DynamicNotRegistered covers the path where the LLM calls
// a name that is neither a built-in tool nor present in the registry.
func TestExecuteToolCall_DynamicNotRegistered(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "totally_unknown", Arguments: `{}`},
	}, nil)

	if cp.Data != tool.NotAvailableMsg {
		t.Errorf("cp.Data = %q, want NotAvailableMsg", cp.Data)
	}
}

// TestExecuteToolCall_DynamicExecuteError covers the dynamic-tool branch where
// the provider's Execute returns an error.
func TestExecuteToolCall_DynamicExecuteError(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&erroringProvider{tool: tool.Dynamic("dyn_fail")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "dyn_fail", Arguments: `{}`},
	}, nil)

	if !strings.Contains(cp.Data, "Error executing tool dyn_fail") {
		t.Errorf("cp.Data = %q, want execute-error message", cp.Data)
	}
}

// TestExecuteToolCall_DynamicSuccessRecordsResult covers the dynamic-tool
// success path with a non-nil TaskRecord so AddToolResult runs.
func TestExecuteToolCall_DynamicSuccessRecordsResult(t *testing.T) {
	reg := tool.NewRegistry()
	dyn := &argsCapturingProvider{tool: tool.Dynamic("dyn_ok")}
	reg.Register(dyn)
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "dyn_ok", Arguments: `{"k":"v"}`},
	}, rec)

	if cp.Data != "ok" {
		t.Errorf("cp.Data = %q, want ok", cp.Data)
	}
	if len(rec.ToolResults) != 1 {
		t.Fatalf("expected 1 recorded tool result, got %d", len(rec.ToolResults))
	}
	if rec.ToolResults[0].ToolName != "dyn_ok" || rec.ToolResults[0].Result != "ok" {
		t.Errorf("recorded result = %+v, want dyn_ok/ok", rec.ToolResults[0])
	}
}

// TestExecuteToolCall_KnownToolNotRegistered covers the lookupTool-nil branch:
// a built-in tool the model may call but which is absent from the registry.
func TestExecuteToolCall_KnownToolNotRegistered(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: tool.FileRead.Name(), Arguments: `{"path":"x"}`},
	}, nil)

	if cp.Data != tool.NotAvailableMsg {
		t.Errorf("cp.Data = %q, want NotAvailableMsg", cp.Data)
	}
}

// TestCollectPendingComments_AwaitsPool covers the worker-pool drain branch of
// CollectPendingComments.
func TestCollectPendingComments_AwaitsPool(t *testing.T) {
	collector := tool.NewCommentCollector()
	pool := NewCommentWorkerPool(2)
	r := NewRunner(Deps{
		Tools:             tool.NewRegistry(),
		CommentCollector:  collector,
		CommentWorkerPool: pool,
	})

	done := make(chan struct{})
	pool.Submit(func() ([]model.LlmComment, error) {
		close(done)
		return nil, nil
	})

	got := r.CollectPendingComments()
	select {
	case <-done:
	default:
		t.Fatal("CollectPendingComments returned before pool work drained")
	}
	if len(got) != 0 {
		t.Errorf("comments = %d, want 0", len(got))
	}
}

// TestExecuteToolCall_DynamicParseError covers the dynamic-tool branch where
// the arguments string fails to parse.
func TestExecuteToolCall_DynamicParseError(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&argsCapturingProvider{tool: tool.Dynamic("dyn_ok")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "dyn_ok", Arguments: `{bad`},
	}, nil)

	if !strings.Contains(cp.Data, "Error parsing tool arguments for dyn_ok") {
		t.Errorf("cp.Data = %q, want parse-error message", cp.Data)
	}
}
