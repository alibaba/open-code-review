// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestTaskCompletionContract(t *testing.T) {
	for _, args := range []string{`{}`, `{"state":"DONE"}`} {
		t.Run(args, func(t *testing.T) {
			r := NewRunner(newTestDeps(&fakeClient{}))
			calls := 0
			opts := &TaskOptions{
				Tools: []llm.ToolDef{{Function: llm.FunctionDef{Name: "task_done"}}},
				Complete: func(map[string]any) ([]model.LlmComment, error) {
					calls++
					return nil, fmt.Errorf("missing evidence")
				},
			}
			ctx := context.WithValue(context.Background(), taskOptionsKey{}, opts)
			cp := r.executeToolCall(ctx, "a.go", llm.ToolCall{Function: llm.FunctionCall{Name: "task_done", Arguments: args}}, nil, "")
			if cp.Completed || cp.Failed || calls != 1 {
				t.Fatalf("completion bypassed validation: %+v, calls=%d", cp, calls)
			}
			opts.Complete = func(map[string]any) ([]model.LlmComment, error) { return nil, nil }
			cp = r.executeToolCall(ctx, "a.go", llm.ToolCall{Function: llm.FunctionCall{Name: "code_comment", Arguments: `{}`}}, nil, "")
			if cp.Completed || r.ToolCalls()["code_comment"] != 0 {
				t.Fatal("unadvertised tool bypassed stage restriction")
			}
			cp = r.executeToolCall(ctx, "a.go", llm.ToolCall{Function: llm.FunctionCall{Name: "task_done", Arguments: args}}, nil, "")
			if !cp.Completed {
				t.Fatalf("valid completion rejected: %+v", cp)
			}
			cp = r.executeToolCall(context.Background(), "a.go", llm.ToolCall{Function: llm.FunctionCall{Name: "task_done", Arguments: `{}`}}, nil, "")
			if !cp.Completed {
				t.Fatal("stage options leaked into classic review")
			}
		})
	}
}

func TestScopedCompletionOwnsStateField(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state any
	}{
		{"custom string", "verified"},
		{"boolean", false},
		{"native failure string", "FAILED"},
		{"object", map[string]any{"phase": "verify"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRunner(newTestDeps(&fakeClient{}))
			args := map[string]any{"state": tc.state, "report": "retained"}
			raw, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			opts := &TaskOptions{
				Tools: []llm.ToolDef{{Function: llm.FunctionDef{Name: "task_done"}}},
				Complete: func(submitted map[string]any) ([]model.LlmComment, error) {
					calls++
					if !reflect.DeepEqual(submitted, args) {
						t.Fatalf("completion arguments changed: got %#v, want %#v", submitted, args)
					}
					return nil, nil
				},
			}
			ctx := context.WithValue(context.Background(), taskOptionsKey{}, opts)
			cp := r.executeToolCall(ctx, "a.go", llm.ToolCall{Function: llm.FunctionCall{Name: "task_done", Arguments: string(raw)}}, nil, "")
			if !cp.Completed || cp.Failed || calls != 1 {
				t.Fatalf("plugin completion bypassed validation: %+v, calls=%d", cp, calls)
			}
		})
	}
}

func TestTaskCompletionWithoutHandlerKeepsNativeState(t *testing.T) {
	for _, tc := range []struct {
		args      string
		completed bool
		failed    bool
	}{
		{`{}`, true, false},
		{`{"state":"DONE"}`, true, false},
		{`{"state":"FAILED"}`, false, true},
		{`{"state":"verified"}`, false, false},
		{`{"state":false}`, false, false},
		{`{"state":{"phase":"verify"}}`, false, false},
	} {
		t.Run(tc.args, func(t *testing.T) {
			r := NewRunner(newTestDeps(&fakeClient{}))
			opts := &TaskOptions{Tools: []llm.ToolDef{{Function: llm.FunctionDef{Name: "task_done"}}}}
			for _, ctx := range []context.Context{context.Background(), context.WithValue(context.Background(), taskOptionsKey{}, opts)} {
				cp := r.executeToolCall(ctx, "a.go", llm.ToolCall{Function: llm.FunctionCall{Name: "task_done", Arguments: tc.args}}, nil, "")
				if cp.Completed != tc.completed || cp.Failed != tc.failed {
					t.Fatalf("native completion changed: %+v", cp)
				}
			}
		})
	}
}

func TestScopedCompletionPublishesOnce(t *testing.T) {
	for _, grace := range []bool{false, true} {
		t.Run(fmt.Sprintf("grace=%v", grace), func(t *testing.T) {
			response := taskDoneResponse()
			duplicate := response.Choices[0].Message.ToolCalls[0]
			duplicate.ID = "duplicate"
			response.Choices[0].Message.ToolCalls = append(response.Choices[0].Message.ToolCalls, duplicate)
			client := &fakeClient{responses: []*llm.ChatResponse{response}}
			r, collector := newThinkingTestRunner(t, client)
			if grace {
				r.deps.Template.MaxToolRequestTimes = 0
			}
			calls := 0
			completed, stop, err := r.RunMainTaskWithOptions(context.Background(), nil, "file.go", TaskOptions{
				Tools: []llm.ToolDef{{Function: llm.FunctionDef{Name: "task_done"}}},
				Complete: func(map[string]any) ([]model.LlmComment, error) {
					calls++
					return []model.LlmComment{{Path: "file.go", Content: "value is lost", ExistingCode: "value"}}, nil
				},
			})
			if err != nil || !completed || calls != 1 || len(collector.Comments()) != 1 {
				t.Fatalf("completion=%v stop=%v error=%v publications=%d comments=%d", completed, stop, err, calls, len(collector.Comments()))
			}
			if grace && stop != StopMaxRounds {
				t.Fatalf("grace report hid the exhausted request budget: %v", stop)
			}
			warnings := graceLimitWarnings(r)
			if grace {
				if len(warnings) != 1 || warnings[0].File != "file.go" ||
					warnings[0].Message != stop.Reason()+"; the grace round completed the task" {
					t.Fatalf("scoped completion lost its budget warning: %+v", warnings)
				}
			} else if len(warnings) != 0 {
				t.Fatalf("normal completion recorded a budget warning: %+v", warnings)
			}
		})
	}
}

func TestScopedCompletionRepairAndPublicationFailure(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse(), taskDoneResponseWithArguments(`{"report":"repaired"}`)}}
	deps := newTestDeps(client)
	r := NewRunner(deps)
	calls := 0
	completed, _, err := r.RunMainTaskWithOptions(context.Background(), nil, "file.go", TaskOptions{
		Tools: []llm.ToolDef{{Function: llm.FunctionDef{Name: "task_done"}}},
		Complete: func(args map[string]any) ([]model.LlmComment, error) {
			calls++
			if args["report"] != "repaired" {
				return nil, fmt.Errorf("missing evidence")
			}
			return []model.LlmComment{{Content: "issue", ExistingCode: "x"}}, nil
		},
	})
	// The registry deliberately has no code_comment provider. A repaired report
	// cannot complete after its publication fails.
	if completed || err == nil || calls != 2 {
		t.Fatalf("completion=%v error=%v calls=%d", completed, err, calls)
	}
	records := deps.Session.GetOrCreateFileSession("file.go").TaskRecords[session.MainTask]
	if len(records) != 2 || len(records[0].ToolResults) != 1 || !strings.Contains(records[0].ToolResults[0].Result, "missing evidence") || records[1].Response.ToolCalls[0].Function.Arguments != `{"report":"repaired"}` {
		t.Fatal("rejected and repaired completion evidence was not preserved")
	}
}
