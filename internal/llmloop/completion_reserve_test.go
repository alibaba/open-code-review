// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func completionTestTools() []llm.ToolDef {
	return []llm.ToolDef{{Function: llm.FunctionDef{Name: "file_read"}}, {Function: llm.FunctionDef{Name: "task_done"}}}
}

func TestScopedCompletionReserveForRepair(t *testing.T) {
	for _, limit := range []string{"requests", "tokens"} {
		t.Run(limit, func(t *testing.T) {
			read := fileReadToolCallResponse("read", `{"path":"a.go"}`)
			read.Usage = &llm.UsageInfo{PromptTokens: 150, CompletionTokens: 10}
			client := &fakeClient{responses: []*llm.ChatResponse{read, taskDoneResponse(), taskDoneResponseWithArguments(`{"repaired":true}`)}}
			deps := newTestDeps(client)
			deps.Template.MaxToolRequestTimes = 3
			deps.Template.MaxCompletionTokens = 50
			if limit == "tokens" {
				deps.Template.MaxToolRequestTimes = 5
				deps.MaxTokensBudget = 1200
			}
			r := NewRunner(deps)
			validations := 0
			completed, _, err := r.RunMainTaskWithOptions(context.Background(), nil, "a.go", TaskOptions{
				Tools: completionTestTools(), Complete: func(args map[string]any) ([]model.LlmComment, error) {
					validations++
					if args["repaired"] != true {
						return nil, fmt.Errorf("missing source_ids [test-1]")
					}
					return nil, nil
				},
			})
			if err != nil || !completed || validations != 2 || len(client.requests) != 3 {
				t.Fatalf("completion=%v error=%v validations=%d requests=%d", completed, err, validations, len(client.requests))
			}
			if len(client.requests[0].Tools) != 2 {
				t.Fatal("investigation ended before approaching a limit")
			}
			for _, request := range client.requests[1:] {
				if len(request.Tools) != 1 || request.Tools[0].Function.Name != "task_done" {
					t.Fatal("closing request still allows investigation")
				}
			}
			found := false
			for _, message := range client.requests[2].Messages {
				found = found || strings.Contains(message.ExtractText(), "missing source_ids [test-1]")
			}
			if !found {
				t.Fatal("repair request omitted validation feedback")
			}
		})
	}
}

func TestScopedClosingRejectsInvestigation(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{fileReadToolCallResponse("read", `{"path":"a.go"}`), taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 1
	r := NewRunner(deps)
	completed, stop, err := r.RunMainTaskWithOptions(context.Background(), nil, "a.go", TaskOptions{
		Tools: completionTestTools(), Complete: func(map[string]any) ([]model.LlmComment, error) { return nil, nil },
	})
	if !completed || err != nil || stop != StopMaxRounds || r.ToolCalls()["file_read"] != 0 {
		t.Fatalf("completion=%v stop=%v error=%v tools=%v", completed, stop, err, r.ToolCalls())
	}
	records := deps.Session.GetOrCreateFileSession("a.go").TaskRecords[session.MainTask]
	if len(records[0].ToolResults) != 1 || !strings.Contains(records[0].ToolResults[0].Result, "not available") {
		t.Fatal("unavailable closing tool was not recorded")
	}
	for _, message := range client.requests[1].Messages {
		if message.Role == "user" && strings.Contains(message.ExtractText(), "code_comment") {
			t.Fatal("closing prompt advertises a forbidden publication tool")
		}
	}
}

func TestClassicClosingIsUnchanged(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{fileReadToolCallResponse("read", `{"path":"a.go"}`), taskDoneResponse()}}
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 1
	deps.MainToolDefs = completionTestTools()
	r := NewRunner(deps)
	completed, stop, err := r.RunMainTask(context.Background(), nil, "a.go")
	if !completed || err != nil || stop != StopMaxRounds || r.ToolCalls()["file_read"] != 1 || len(client.requests[0].Tools) != 2 {
		t.Fatalf("classic behavior changed: completed=%v stop=%v error=%v tools=%v", completed, stop, err, r.ToolCalls())
	}
}

type scopedGraceErrorClient struct{}

func (scopedGraceErrorClient) CompletionsWithCtx(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, errors.New("provider unavailable")
}

func TestScopedGraceFailureIsReturned(t *testing.T) {
	deps := newTestDeps(scopedGraceErrorClient{})
	deps.Template.MaxToolRequestTimes = 0
	r := NewRunner(deps)
	completed, _, err := r.RunMainTaskWithOptions(context.Background(), nil, "a.go", TaskOptions{Tools: completionTestTools()})
	if completed || err == nil || !strings.Contains(err.Error(), "provider unavailable") || deps.Session.LLMFailures() != 1 {
		t.Fatalf("completion=%v error=%v failures=%d", completed, err, deps.Session.LLMFailures())
	}
	if warnings := graceLimitWarnings(r); len(warnings) != 1 ||
		!strings.Contains(warnings[0].Message, "the grace round did not complete the task") {
		t.Fatalf("scoped grace failure lost its budget warning: %+v", warnings)
	}
}

func TestScopedClosingSurvivesEmptyResponseAndCorrection(t *testing.T) {
	for _, repaired := range []bool{false, true} {
		t.Run(fmt.Sprintf("repaired=%v", repaired), func(t *testing.T) {
			empty := &llm.ChatResponse{Choices: []llm.Choice{{FinishReason: "length"}}}
			last := taskDoneResponse()
			if repaired {
				last = taskDoneResponseWithArguments(`{"repaired":true}`)
			}
			client := &fakeClient{responses: []*llm.ChatResponse{empty, taskDoneResponse(), last}}
			deps := newTestDeps(client)
			deps.Template.MaxToolRequestTimes = 100
			deps.MaxTokensBudget = 1000000
			r := NewRunner(deps)
			validations := 0
			completed, stop, err := r.RunMainTaskWithOptions(context.Background(), nil, "a.go", TaskOptions{
				Tools: completionTestTools(), TokenLimit: 1,
				Complete: func(args map[string]any) ([]model.LlmComment, error) {
					validations++
					if args["repaired"] != true {
						return nil, fmt.Errorf("checks[C1]: missing refutation target and reason")
					}
					return nil, nil
				},
			})
			if err != nil || completed != repaired || stop != StopCompletionBudget || validations != 2 || len(client.requests) != 3 {
				t.Fatalf("completed=%v stop=%v error=%v validations=%d requests=%d", completed, stop, err, validations, len(client.requests))
			}
			outcome := "the grace round did not complete the task"
			if repaired {
				outcome = "the grace round completed the task"
			}
			if warnings := graceLimitWarnings(r); len(warnings) != 1 || warnings[0].File != "a.go" ||
				warnings[0].Message != stop.Reason()+"; "+outcome {
				t.Fatalf("scoped closing verdict lost its budget warning: %+v", warnings)
			}
			for _, req := range client.requests {
				if len(req.Tools) != 1 || req.Tools[0].Function.Name != "task_done" {
					t.Fatal("investigation reopened during bounded closure")
				}
			}
			records := deps.Session.GetOrCreateFileSession("a.go").TaskRecords[session.MainTask]
			if len(records) != 3 || r.TotalTokensUsed() != 30 {
				t.Fatalf("closing requests lost accounting: records=%d tokens=%d", len(records), r.TotalTokensUsed())
			}
			found := false
			for _, msg := range client.requests[2].Messages {
				found = found || strings.Contains(msg.ExtractText(), "checks[C1]: missing refutation")
			}
			if !found {
				t.Fatal("correction opportunity omitted the rejection receipt")
			}
		})
	}
}

func TestScopedTokenLimitDoesNotRelaxGlobalBudget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		global, task int64
		wantTools    int
	}{
		{"global tighter", 1, 1000000, 1},
		{"task tighter", 1000000, 1, 1},
		{"task without global", 0, 1, 1},
		{"unlimited", 0, 0, 2},
		{"ample allowance", 10000000, 5000000, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeClient{responses: []*llm.ChatResponse{taskDoneResponse()}}
			deps := newTestDeps(client)
			deps.MaxTokensBudget = tc.global
			r := NewRunner(deps)
			completed, _, err := r.RunMainTaskWithOptions(context.Background(), nil, "a.go", TaskOptions{
				Tools: completionTestTools(), TokenLimit: tc.task,
			})
			if !completed || err != nil || len(client.requests) != 1 || len(client.requests[0].Tools) != tc.wantTools {
				t.Fatalf("completed=%v error=%v requests=%+v", completed, err, client.requests)
			}
		})
	}
}

func TestCompletionReserveUsesClientOutputOverride(t *testing.T) {
	for _, tc := range []struct {
		name      string
		override  bool
		scoped    bool
		budget    int64
		wantTools int
	}{
		{"scoped provider override", true, true, 250000, 1},
		{"scoped template control", false, true, 250000, 2},
		{"classic provider override", true, false, 250000, 2},
		{"scoped without token budget", true, true, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]any, 128)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"test","model":"test","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"done","type":"function","function":{"name":"task_done","arguments":"{}"}}]}}],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}`)
			}))
			defer server.Close()
			cfg := llm.ClientConfig{URL: server.URL, APIKey: "test"}
			if tc.override {
				cfg.ExtraBody = map[string]any{"max_tokens": 32768}
			}
			deps := newTestDeps(llm.NewOpenAIClient(cfg))
			deps.Template.MaxCompletionTokens = 16384
			deps.Template.MaxToolRequestTimes = 100
			deps.MaxTokensBudget = tc.budget
			deps.MainToolDefs = completionTestTools()
			runner := NewRunner(deps)
			var complete bool
			var err error
			if tc.scoped {
				complete, _, err = runner.RunMainTaskWithOptions(context.Background(), nil, "a.go", TaskOptions{Tools: completionTestTools()})
			} else {
				complete, _, err = runner.RunMainTask(context.Background(), nil, "a.go")
			}
			if err != nil || !complete {
				t.Fatalf("complete=%v error=%v", complete, err)
			}
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			body := <-requests
			if got := len(body["tools"].([]any)); got != tc.wantTools {
				t.Fatalf("advertised tools = %d, want %d", got, tc.wantTools)
			}
			if body["max_completion_tokens"] != float64(16384) {
				t.Fatalf("request cap changed: %v", body["max_completion_tokens"])
			}
			if tc.override && body["max_tokens"] != float64(32768) {
				t.Fatalf("provider override changed: %v", body["max_tokens"])
			}
		})
	}
}

func TestCompletionReserveLargeClientOutputCap(t *testing.T) {
	client := llm.NewOpenAIClient(llm.ClientConfig{
		URL:       "http://unused.invalid",
		ExtraBody: map[string]any{"max_tokens": int(^uint(0) >> 1)},
	})
	deps := newTestDeps(client)
	deps.MaxTokensBudget = 1000000
	runner := NewRunner(deps)
	ctx := context.WithValue(context.Background(), taskOptionsKey{}, &TaskOptions{Tools: completionTestTools()})
	if !runner.completionDue(ctx, 100, 1, nil) {
		t.Fatal("an output cap larger than the allowance must trigger closure without overflowing")
	}
}
