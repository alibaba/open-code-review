// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestExecuteToolCall_RepeatedFailureBackoff drives the same (taskKey,
// toolName) pair to failure three times in a row and checks the response
// escalates instead of repeating the same retryable error forever: the 2nd
// failure names the repeat, and the 3rd stops reading as an error at all so
// the model has no reason to send a 4th attempt.
func TestExecuteToolCall_RepeatedFailureBackoff(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&erroringProvider{tool: tool.Dynamic("dyn_fail")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	call := func() tool.TaskCheckpoint {
		return r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: "dyn_fail", Arguments: `{"query":"needle"}`},
		}, nil, "")
	}

	first := call()
	if !strings.Contains(first.Data, "Error executing tool dyn_fail") {
		t.Fatalf("1st failure cp.Data = %q, want plain execute-error message", first.Data)
	}

	second := call()
	if !strings.Contains(second.Data, "Error executing tool dyn_fail") {
		t.Errorf("2nd failure cp.Data = %q, want it to still contain the underlying error", second.Data)
	}
	if !strings.Contains(second.Data, "second consecutive failure") {
		t.Errorf("2nd failure cp.Data = %q, want it to name the repeat", second.Data)
	}
	if !strings.Contains(second.Data, "dyn_fail") {
		t.Errorf("2nd failure cp.Data = %q, want it to name the tool", second.Data)
	}

	third := call()
	if strings.Contains(third.Data, "Error") {
		t.Errorf("3rd failure cp.Data = %q, want no retryable error wording", third.Data)
	}
	if third.Failed || third.Completed {
		t.Errorf("3rd failure checkpoint = %+v, want a plain (non-terminal) result", third)
	}
	if third.Data == "" {
		t.Error("3rd failure cp.Data is empty, want a skipped/accepted message")
	}

	fourth := call()
	if fourth.Data != third.Data {
		t.Errorf("4th failure cp.Data = %q, want it to keep reading as skipped like the 3rd (%q)", fourth.Data, third.Data)
	}
}

// TestExecuteToolCall_FailureStreakScopedPerTaskAndTool confirms the counter
// is keyed by (taskKey, toolName): failures against a different taskKey or a
// different toolName must not contribute to, or be affected by, another
// pair's streak.
func TestExecuteToolCall_FailureStreakScopedPerTaskAndTool(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&erroringProvider{tool: tool.Dynamic("dyn_fail")})
	reg.Register(&erroringProvider{tool: tool.Dynamic("dyn_fail_2")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	callFor := func(taskKey, toolName string) tool.TaskCheckpoint {
		return r.executeToolCall(context.Background(), taskKey, llm.ToolCall{
			Function: llm.FunctionCall{Name: toolName, Arguments: `{}`},
		}, nil, "")
	}

	// Two failures for (file.go, dyn_fail) — one short of the skip threshold.
	callFor("file.go", "dyn_fail")
	streakTwo := callFor("file.go", "dyn_fail")
	if !strings.Contains(streakTwo.Data, "second consecutive failure") {
		t.Fatalf("setup: (file.go, dyn_fail) cp.Data = %q, want the strengthened 2nd-failure message", streakTwo.Data)
	}

	// A different taskKey with the same tool name starts its own streak at 1.
	otherTask := callFor("other.go", "dyn_fail")
	if !strings.Contains(otherTask.Data, "Error executing tool dyn_fail") || strings.Contains(otherTask.Data, "second consecutive failure") {
		t.Errorf("(other.go, dyn_fail) 1st call cp.Data = %q, want a fresh 1st-failure message", otherTask.Data)
	}

	// A different toolName under the same taskKey also starts its own streak at 1.
	otherTool := callFor("file.go", "dyn_fail_2")
	if !strings.Contains(otherTool.Data, "Error executing tool dyn_fail_2") || strings.Contains(otherTool.Data, "second consecutive failure") {
		t.Errorf("(file.go, dyn_fail_2) 1st call cp.Data = %q, want a fresh 1st-failure message", otherTool.Data)
	}

	// The original pair's streak must be untouched by the two calls above:
	// its next failure is still the 3rd, which reads as skipped.
	streakThree := callFor("file.go", "dyn_fail")
	if strings.Contains(streakThree.Data, "Error") {
		t.Errorf("(file.go, dyn_fail) 3rd call cp.Data = %q, want it to have advanced to the skip message", streakThree.Data)
	}
}

// TestExecuteToolCall_SuccessResetsFailureStreak confirms a success for a
// (taskKey, toolName) pair resets its streak, so a later failure starts over
// at the 1st-failure message rather than continuing to escalate or skip.
func TestExecuteToolCall_SuccessResetsFailureStreak(t *testing.T) {
	reg := tool.NewRegistry()
	dyn := &flakyProvider{tool: tool.Dynamic("dyn_flaky")}
	reg.Register(dyn)
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	call := func() tool.TaskCheckpoint {
		return r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: "dyn_flaky", Arguments: `{}`},
		}, nil, "")
	}

	dyn.fail = true
	first := call()
	if !strings.Contains(first.Data, "Error executing tool dyn_flaky") {
		t.Fatalf("1st call cp.Data = %q, want a plain execute-error message", first.Data)
	}

	dyn.fail = false
	success := call()
	if success.Data != "ok" {
		t.Fatalf("success call cp.Data = %q, want ok", success.Data)
	}

	dyn.fail = true
	afterReset := call()
	if !strings.Contains(afterReset.Data, "Error executing tool dyn_flaky") || strings.Contains(afterReset.Data, "second consecutive failure") {
		t.Errorf("post-success failure cp.Data = %q, want a fresh 1st-failure message", afterReset.Data)
	}
}

// flakyProvider fails or succeeds depending on the fail field, letting tests
// toggle the outcome of consecutive calls without swapping the registry.
type flakyProvider struct {
	tool tool.Tool
	fail bool
}

func (p *flakyProvider) Tool() tool.Tool { return p.tool }
func (p *flakyProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	if p.fail {
		return "", errFlaky
	}
	return "ok", nil
}

var errFlaky = errors.New("flaky failure")
