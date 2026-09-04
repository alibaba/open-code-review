// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/tool"
)

// erroringProvider is a dynamic tool provider whose Execute always fails.
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
	}, nil, "")

	if cp.Data != tool.NotAvailableMsg {
		t.Errorf("cp.Data = %q, want NotAvailableMsg", cp.Data)
	}
}

// TestExecuteToolCall_DynamicExecuteError verifies newly registered tools use
// the common failure-observation path without tool-specific instrumentation.
func TestExecuteToolCall_DynamicExecuteError(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&erroringProvider{tool: tool.Dynamic("dyn_fail")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "dyn_fail", Arguments: `{"query":"needle"}`},
	}, rec, "")

	if !strings.Contains(cp.Data, "Error executing tool dyn_fail") {
		t.Errorf("cp.Data = %q, want execute-error message", cp.Data)
	}
	failures := r.ToolFailures()
	if len(failures) != 1 {
		t.Fatalf("ToolFailures() = %+v, want one failure", failures)
	}
	failure := failures[0]
	if failure.ToolCallNumber != 1 || failure.ToolName != "dyn_fail" || failure.FilePath != "file.go" {
		t.Errorf("failure identity = %+v", failure)
	}
	if failure.Arguments != `{"query":"needle"}` {
		t.Errorf("failure arguments = %q, want raw tool arguments", failure.Arguments)
	}
	if failure.Error != "boom" {
		t.Errorf("failure details = %+v", failure)
	}
	if len(rec.ToolResults) != 1 || rec.ToolResults[0].OK || rec.ToolResults[0].Result != "boom" {
		t.Errorf("session tool failure = %+v", rec.ToolResults)
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
	}, rec, "")

	if cp.Data != "ok" {
		t.Errorf("cp.Data = %q, want ok", cp.Data)
	}
	if len(rec.ToolResults) != 1 {
		t.Fatalf("expected 1 recorded tool result, got %d", len(rec.ToolResults))
	}
	if rec.ToolResults[0].ToolName != "dyn_ok" || rec.ToolResults[0].Result != "ok" {
		t.Errorf("recorded result = %+v, want dyn_ok/ok", rec.ToolResults[0])
	}
	if !rec.ToolResults[0].OK {
		t.Errorf("successful result marked failed: %+v", rec.ToolResults[0])
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
	}, nil, "")

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

// TestExecuteToolCall_DynamicParseError verifies malformed calls to registered
// tools are captured by the same failure-observation path.
func TestExecuteToolCall_DynamicParseError(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&argsCapturingProvider{tool: tool.Dynamic("dyn_ok")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	var cp tool.TaskCheckpoint
	progress, errOut := captureToolTerminal(t, func() {
		cp = r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: "dyn_ok", Arguments: `{bad`},
		}, nil, "")
	})

	if !strings.Contains(cp.Data, "Error parsing tool arguments for dyn_ok") {
		t.Errorf("cp.Data = %q, want parse-error message", cp.Data)
	}
	if !strings.Contains(progress, "[ocr]   ▶ dyn_ok") {
		t.Errorf("progress = %q, want tool start", progress)
	}
	if !strings.Contains(errOut, "[ocr]   ✘ dyn_ok failed: Error parsing tool arguments for dyn_ok") {
		t.Errorf("stderr = %q, want terminal parse failure", errOut)
	}
	failures := r.ToolFailures()
	if len(failures) != 1 || failures[0].ToolName != "dyn_ok" || failures[0].ToolCallNumber != 1 {
		t.Errorf("ToolFailures() = %+v, want dyn_ok tool call 1", failures)
	}
	if !strings.Contains(failures[0].Error, "Error parsing tool arguments") {
		t.Errorf("parse failure details = %+v", failures[0])
	}
	if failures[0].Arguments != `{bad` {
		t.Errorf("parse failure arguments = %q, want malformed raw arguments", failures[0].Arguments)
	}
}

func TestExecuteToolCall_CodeCommentValidationErrorPrintsTerminalFailure(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: tool.NewCommentCollector()})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	progress, errOut := captureToolTerminal(t, func() {
		r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: tool.CodeComment.Name(), Arguments: `{}`},
		}, nil, "")
	})

	if !strings.Contains(progress, "[ocr]   ▶ code_comment") {
		t.Errorf("progress = %q, want tool start", progress)
	}
	if !strings.Contains(errOut, "[ocr]   ✘ code_comment failed: Error: 'comments' array is required") {
		t.Errorf("stderr = %q, want terminal validation failure", errOut)
	}
}

func captureToolTerminal(t *testing.T, fn func()) (string, string) {
	t.Helper()

	var progress bytes.Buffer
	restoreProgress := stdout.Swap(&progress)
	defer restoreProgress()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = oldStderr

	errBytes, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return progress.String(), string(errBytes)
}
