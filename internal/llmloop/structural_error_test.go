// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// structuralProvider returns the structural rejection on demand and an
// ordinary failure otherwise, so one registry can drive both.
type structuralProvider struct {
	tool tool.Tool
}

func (p *structuralProvider) Tool() tool.Tool { return p.tool }
func (p *structuralProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "", &tool.PathNotFoundError{
		Path:   "docs/missing.go",
		Target: `at the reviewed ref "abc1234"`,
	}
}

// TestExecuteToolCall_StructuralErrorIsNotEscalated covers the contract that
// a path that is not present at the read target is a structural rejection,
// not a flaky failure: the ladder must not invite a retry, must not replace
// the reason with its generic wording, and must not spend the (task, tool)
// retry budget on it.
func TestExecuteToolCall_StructuralErrorIsNotEscalated(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&structuralProvider{tool: tool.Dynamic("dyn_structural")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	call := func() tool.TaskCheckpoint {
		return r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: "dyn_structural", Arguments: `{}`},
		}, nil, "")
	}

	first := call()
	if !strings.Contains(first.Data, `read file "docs/missing.go"`) {
		t.Fatalf("1st call cp.Data = %q, want the rejection to name the path", first.Data)
	}
	if !strings.Contains(first.Data, "does not exist") {
		t.Errorf("1st call cp.Data = %q, want it to state that the path does not exist", first.Data)
	}
	if !strings.Contains(first.Data, "reading it again will not help") {
		t.Errorf("1st call cp.Data = %q, want it to say a retry will not help", first.Data)
	}

	// The 2nd and 3rd attempts must read exactly the same: the ladder's
	// repeat wording must never appear, and the reason must survive.
	for attempt := 2; attempt <= 3; attempt++ {
		next := call()
		if next.Data != first.Data {
			t.Errorf("%d call cp.Data = %q, want it unchanged from %q", attempt, next.Data, first.Data)
		}
		if strings.Contains(next.Data, "second consecutive failure") {
			t.Errorf("%d call cp.Data = %q, want no ladder repeat wording", attempt, next.Data)
		}
		if strings.Contains(next.Data, "has failed repeatedly") {
			t.Errorf("%d call cp.Data = %q, want no ladder skip wording", attempt, next.Data)
		}
	}
}

// TestExecuteToolCall_StructuralErrorLeavesStreakUntouched confirms the
// structural rejection does not consume the consecutive-failure budget: an
// ordinary failure for the same (taskKey, toolName) afterwards still reads as
// a first failure.
func TestExecuteToolCall_StructuralErrorLeavesStreakUntouched(t *testing.T) {
	reg := tool.NewRegistry()
	provider := &switchingProvider{tool: tool.Dynamic("dyn_switch")}
	reg.Register(provider)
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	call := func() tool.TaskCheckpoint {
		return r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
			Function: llm.FunctionCall{Name: "dyn_switch", Arguments: `{}`},
		}, nil, "")
	}

	provider.structural = true
	for attempt := 1; attempt <= 3; attempt++ {
		if cp := call(); strings.Contains(cp.Data, "consecutive failure") {
			t.Fatalf("%d structural call cp.Data = %q, want no ladder wording", attempt, cp.Data)
		}
	}

	provider.structural = false
	ordinary := call()
	if !strings.Contains(ordinary.Data, "Error executing tool dyn_switch") {
		t.Errorf("ordinary failure cp.Data = %q, want a plain execute-error message", ordinary.Data)
	}
	if strings.Contains(ordinary.Data, "second consecutive failure") {
		t.Errorf("ordinary failure cp.Data = %q, want a first-failure message: the structural rejections must not have consumed the streak", ordinary.Data)
	}
}

// TestExecuteToolCall_StructuralErrorIsRecordedAsFailure confirms the
// rejection stays visible as a failed tool call in the recorded failure.
func TestExecuteToolCall_StructuralErrorIsRecordedAsFailure(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&structuralProvider{tool: tool.Dynamic("dyn_structural")})
	reg.Freeze()
	r := NewRunner(Deps{Tools: reg, CommentCollector: tool.NewCommentCollector()})

	rec := &session.TaskRecord{}
	cp := r.executeToolCall(context.Background(), "file.go", llm.ToolCall{
		Function: llm.FunctionCall{Name: "dyn_structural", Arguments: `{}`},
	}, rec, "")

	if cp.Data == "" {
		t.Fatal("cp.Data is empty, want the rejection")
	}
	if len(r.ToolFailures()) != 1 {
		t.Fatalf("recorded failures = %d, want 1", len(r.ToolFailures()))
	}
	failure := r.ToolFailures()[0]
	if !strings.Contains(failure.Error, `read file "docs/missing.go"`) {
		t.Errorf("recorded failure = %q, want it to identify the rejection on its own", failure.Error)
	}
}

// switchingProvider returns a structural rejection or an ordinary failure
// depending on the structural field.
type switchingProvider struct {
	tool       tool.Tool
	structural bool
}

func (p *switchingProvider) Tool() tool.Tool { return p.tool }
func (p *switchingProvider) Execute(_ context.Context, _ map[string]any) (string, error) {
	if p.structural {
		return "", &tool.PathNotFoundError{
			Path:   "docs/missing.go",
			Target: `at the reviewed ref "abc1234"`,
		}
	}
	return "", errors.New("boom")
}
