// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func TestExecuteToolCall_BuiltInRejections(t *testing.T) {
	reader := &tool.FileReader{Mode: tool.ModeWorkspace}
	for _, test := range []struct {
		provider tool.Provider
		args     string
		want     string
	}{
		{tool.NewFileRead(reader), `{}`, "Error: file_path is required"},
		{tool.NewCodeSearch(reader), `{"search_text":"  "}`, "Error: search_text is blank"},
		{tool.NewFileReadDiff(tool.NewDiffMap(map[string]string{"reviewed.go": "+new"})), `{"path_array":["missing.go"]}`,
			"Error: diff not found for: missing.go\nDiffs exist for: reviewed.go\nUse one of those paths, or file_read for a file outside the reviewed set.\n"},
	} {
		name := test.provider.Tool().Name()
		t.Run(name, func(t *testing.T) {
			reg := tool.NewRegistry()
			reg.Register(test.provider)
			reg.Freeze()
			r := NewRunner(Deps{Tools: reg})
			rec := &session.TaskRecord{}
			const task = "reviewed.go"
			call := llm.ToolCall{Function: llm.FunctionCall{Name: name, Arguments: test.args}}

			var results [4]tool.TaskCheckpoint
			progress, errOut := captureToolTerminal(t, func() {
				for i := range results {
					results[i] = r.executeToolCall(t.Context(), task, call, rec, "")
				}
			})
			if got := strings.Count(errOut, "[ocr]   ✘ "+name+" failed: "+test.want); got != 4 || strings.Contains(progress, "[ocr]   ✔ "+name) {
				t.Errorf("terminal output: stdout = %q, stderr = %q", progress, errOut)
			}
			if results[0].Data != test.want || !strings.HasPrefix(results[1].Data, test.want) ||
				!strings.Contains(results[1].Data, "second consecutive failure calling "+name) {
				t.Errorf("retry feedback = %q, %q", results[0].Data, results[1].Data)
			}
			if !strings.Contains(results[2].Data, "has been skipped") || results[3].Data != results[2].Data {
				t.Errorf("skip feedback = %q, %q", results[2].Data, results[3].Data)
			}
			failures := r.ToolFailures()
			if r.ToolCalls()[name] != 4 || len(failures) != 4 || len(rec.ToolResults) != 4 {
				t.Fatalf("calls = %v, failures = %+v, session = %+v", r.ToolCalls(), failures, rec.ToolResults)
			}
			for i, result := range results {
				if result.Failed || result.Completed {
					t.Errorf("call %d ended the task: %+v", i+1, result)
				}
				failure := failures[i]
				if failure.ToolCallNumber != int64(i+1) || failure.ToolName != name || failure.FilePath != task ||
					failure.Arguments != test.args || failure.Error != test.want {
					t.Errorf("failure %d = %+v", i+1, failure)
				}
				saved := rec.ToolResults[i]
				if saved.OK || saved.Result != test.want {
					t.Errorf("session result %d = %+v", i+1, saved)
				}
			}
		})
	}
}

func TestExecuteToolCall_BuiltInSuccessResetsFailureStreak(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "reviewed.go"), []byte("package reviewed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := tool.FileReader{RepoDir: dir, Mode: tool.ModeWorkspace}
	access, err := tool.NewWorkspaceCodeAccess(reader, tool.FileReader{Mode: tool.ModeCommit, Ref: "base-revision"})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		provider     tool.Provider
		rejectedArgs string
		diagnostic   string
		validArgs    string
		validResult  string
	}{
		{&tool.FileReadProvider{Access: access}, `{"side":"old"}`, "SIDE: old\nREVISION: base-revision\nError: file_path is required",
			`{"file_path":"reviewed.go"}`, "1|package reviewed"},
		{tool.NewCodeSearch(&reader), `{"search_text":" "}`, "Error: search_text is blank",
			`{"search_text":"absent-needle"}`, "No matches found"},
		{tool.NewFileReadDiff(tool.NewDiffMap(map[string]string{"reviewed.go": "+new"})), `{"path_array":[]}`, "Error: no files found",
			`{"path_array":["reviewed.go","missing.go"]}`, "+new\n\nError: diff not found for: missing.go"},
	} {
		name := test.provider.Tool().Name()
		t.Run(name, func(t *testing.T) {
			reg := tool.NewRegistry()
			reg.Register(test.provider)
			reg.Freeze()
			r := NewRunner(Deps{Tools: reg})
			rec := &session.TaskRecord{}
			call := func(args string) string {
				return r.executeToolCall(t.Context(), "reviewed.go", llm.ToolCall{
					Function: llm.FunctionCall{Name: name, Arguments: args},
				}, rec, "").Data
			}
			if got := call(test.rejectedArgs); got != test.diagnostic {
				t.Errorf("first failure = %q, want %q", got, test.diagnostic)
			}
			if got := call(test.rejectedArgs); !strings.Contains(got, "second consecutive failure") {
				t.Errorf("second failure = %q", got)
			}
			if got := call(test.validArgs); !strings.Contains(got, test.validResult) {
				t.Errorf("corrected result = %q, want %q", got, test.validResult)
			}
			if got := call(test.rejectedArgs); got != test.diagnostic {
				t.Errorf("first failure after success = %q, want %q", got, test.diagnostic)
			}
			if len(r.ToolFailures()) != 3 || len(rec.ToolResults) != 4 || !rec.ToolResults[2].OK {
				t.Errorf("failures = %+v, session = %+v", r.ToolFailures(), rec.ToolResults)
			}
		})
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
