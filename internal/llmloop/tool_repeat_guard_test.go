// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors
package llmloop

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
)

// countingProvider succeeds every time and counts its executions, so a test
// can tell a refused repeat (not executed) from an executed one.
type countingProvider struct {
	tool  tool.Tool
	calls int
}

func (p *countingProvider) Tool() tool.Tool { return p.tool }
func (p *countingProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	p.calls++
	return fmt.Sprintf("result %d", p.calls), nil
}

func newRepeatRunner(t *testing.T, names ...string) (*Runner, map[string]*countingProvider) {
	t.Helper()
	reg := tool.NewRegistry()
	providers := map[string]*countingProvider{}
	for _, n := range names {
		p := &countingProvider{tool: tool.Dynamic(n)}
		providers[n] = p
		reg.Register(p)
	}
	reg.Freeze()
	return NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()}), providers
}

func callTool(r *Runner, taskKey, name, args string) tool.TaskCheckpoint {
	return r.executeToolCall(context.Background(), taskKey, llm.ToolCall{
		Function: llm.FunctionCall{Name: name, Arguments: args},
	}, nil, "")
}

// TestExecuteToolCall_RepeatedIdenticalCallIsRefusedThenFails is the loop seen
// in production: the same call, over and over. The first two run; from the
// third the model is told it is repeating itself and the tool does not run;
// at the sixth the task fails.
func TestExecuteToolCall_RepeatedIdenticalCallIsRefusedThenFails(t *testing.T) {
	r, p := newRepeatRunner(t, "dyn_search")
	args := `{"search_text":"needle","file_patterns":["src/*.ts"]}`

	for i := 1; i < repeatWarnAt; i++ {
		cp := callTool(r, "a.go", "dyn_search", args)
		if cp.Failed || !strings.HasPrefix(cp.Data, "result ") {
			t.Fatalf("call %d = %+v, want the tool's own result", i, cp)
		}
	}
	if p["dyn_search"].calls != repeatWarnAt-1 {
		t.Fatalf("executions = %d, want %d", p["dyn_search"].calls, repeatWarnAt-1)
	}

	warn := callTool(r, "a.go", "dyn_search", args)
	if warn.Failed || warn.Completed {
		t.Fatalf("call %d = %+v, want a non-terminal refusal", repeatWarnAt, warn)
	}
	if !strings.Contains(warn.Data, "already called dyn_search") || !strings.Contains(warn.Data, "task_done") {
		t.Errorf("refusal = %q, want it to name the tool and the way out", warn.Data)
	}
	if p["dyn_search"].calls != repeatWarnAt-1 {
		t.Errorf("the refused call was executed (%d executions)", p["dyn_search"].calls)
	}

	var last tool.TaskCheckpoint
	for i := repeatWarnAt + 1; i <= repeatFailAt; i++ {
		last = callTool(r, "a.go", "dyn_search", args)
	}
	if !last.Failed {
		t.Fatalf("call %d = %+v, want the task to fail", repeatFailAt, last)
	}
	if !strings.Contains(last.Data, "tool call loop") {
		t.Errorf("failure = %q, want it to say why", last.Data)
	}
	if p["dyn_search"].calls != repeatWarnAt-1 {
		t.Errorf("executions = %d after the loop, want still %d", p["dyn_search"].calls, repeatWarnAt-1)
	}

	var codes []string
	for _, w := range r.Warnings() {
		codes = append(codes, w.Type)
	}
	if strings.Join(codes, ",") != "tool_call_repeated,tool_call_loop" {
		t.Errorf("warnings = %v, want one tool_call_repeated then one tool_call_loop", codes)
	}
}

// TestExecuteToolCall_AlternatingCallsAreCaught covers the shape that a
// consecutive-streak counter would miss: two calls taking turns.
func TestExecuteToolCall_AlternatingCallsAreCaught(t *testing.T) {
	r, p := newRepeatRunner(t, "dyn_search")
	a := `{"search_text":"a"}`
	b := `{"search_text":"b"}`

	var cp tool.TaskCheckpoint
	for i := 0; i < repeatFailAt; i++ {
		cp = callTool(r, "a.go", "dyn_search", a)
		if cp.Failed {
			break
		}
		cp = callTool(r, "a.go", "dyn_search", b)
		if cp.Failed {
			break
		}
	}
	if !cp.Failed {
		t.Fatalf("alternating a/b never failed the task; last = %+v", cp)
	}
	// Each of a and b ran repeatWarnAt-1 times before its repeats were refused.
	if want := 2 * (repeatWarnAt - 1); p["dyn_search"].calls != want {
		t.Errorf("executions = %d, want %d", p["dyn_search"].calls, want)
	}
}

// TestExecuteToolCall_RepeatGuardIsPerTaskAndPerArguments: the same call from
// another task, or the same tool with other arguments, is not a repeat.
func TestExecuteToolCall_RepeatGuardIsPerTaskAndPerArguments(t *testing.T) {
	r, p := newRepeatRunner(t, "dyn_read")

	for i := 0; i < repeatFailAt; i++ {
		cp := callTool(r, fmt.Sprintf("file-%d.go", i), "dyn_read", `{"path":"x"}`)
		if cp.Failed || !strings.HasPrefix(cp.Data, "result ") {
			t.Fatalf("task %d = %+v, want a normal result", i, cp)
		}
	}
	for i := 0; i < repeatFailAt; i++ {
		cp := callTool(r, "one.go", "dyn_read", fmt.Sprintf(`{"path":"x","offset":%d}`, i))
		if cp.Failed || !strings.HasPrefix(cp.Data, "result ") {
			t.Fatalf("distinct args %d = %+v, want a normal result", i, cp)
		}
	}
	if p["dyn_read"].calls != 2*repeatFailAt {
		t.Errorf("executions = %d, want %d", p["dyn_read"].calls, 2*repeatFailAt)
	}
}

// TestExecuteToolCall_RepeatGuardCanonicalisesArguments: key order and
// whitespace in the raw JSON do not make two identical calls different.
func TestExecuteToolCall_RepeatGuardCanonicalisesArguments(t *testing.T) {
	r, _ := newRepeatRunner(t, "dyn_search")
	forms := []string{
		`{"search_text":"n","file_patterns":["a"]}`,
		`{ "file_patterns": ["a"], "search_text": "n" }`,
		`{"search_text":"n",   "file_patterns":["a"]}`,
	}
	var cp tool.TaskCheckpoint
	for i := 0; i < repeatWarnAt; i++ {
		cp = callTool(r, "a.go", "dyn_search", forms[i%len(forms)])
	}
	if !strings.Contains(cp.Data, "already called dyn_search") {
		t.Errorf("call %d = %+v, want the repeat to be recognised across argument spellings", repeatWarnAt, cp)
	}
}

// TestExecuteToolCall_RepeatGuardWindowForgets: a call repeated a few times
// far apart, with enough other calls between, is not a loop.
func TestExecuteToolCall_RepeatGuardWindowForgets(t *testing.T) {
	r, p := newRepeatRunner(t, "dyn_read")
	same := `{"path":"same"}`
	for round := 0; round < repeatFailAt; round++ {
		cp := callTool(r, "a.go", "dyn_read", same)
		if cp.Failed || !strings.HasPrefix(cp.Data, "result ") {
			t.Fatalf("round %d = %+v, want a normal result", round, cp)
		}
		for i := 0; i < repeatWindow; i++ {
			callTool(r, "a.go", "dyn_read", fmt.Sprintf(`{"path":"other-%d-%d"}`, round, i))
		}
	}
	if want := repeatFailAt * (repeatWindow + 1); p["dyn_read"].calls != want {
		t.Errorf("executions = %d, want %d (nothing refused)", p["dyn_read"].calls, want)
	}
}
