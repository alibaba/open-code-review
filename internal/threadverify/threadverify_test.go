// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package threadverify

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func init() { session.UseTestSessions() }

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "ocr-test-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", tmp)
	_ = os.Setenv("USERPROFILE", tmp)
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

const head = "0123456789abcdef0123456789abcdef01234567"

const headFile = "package a\n\nfunc Cap(n int) error {\n\tif n > max {\n\t\treturn errCap\n\t}\n\treturn nil\n}\n"

// scriptedClient returns one tool call per request, in order, and records
// every request so a test can inspect what the model was offered.
type scriptedClient struct {
	mu       sync.Mutex
	calls    []llm.ToolCall
	err      error
	requests []llm.ChatRequest
}

func (c *scriptedClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	if len(c.calls) == 0 {
		return nil, errors.New("script exhausted")
	}
	call := c.calls[0]
	c.calls = c.calls[1:]
	content := ""
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content, ToolCalls: []llm.ToolCall{call}}}}}, nil
}

func toolCall(name, args string) llm.ToolCall {
	return llm.ToolCall{ID: "c-" + name, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: args}}
}

func done() llm.ToolCall { return toolCall("task_done", `{"state":"DONE"}`) }

var baseDefs = []llm.ToolDef{
	{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
	{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
	{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
	{Type: "function", Function: llm.FunctionDef{Name: "file_read_diff"}},
	{Type: "function", Function: llm.FunctionDef{Name: "code_search"}},
}

func runWith(t *testing.T, client *scriptedClient, in Input) Output {
	t.Helper()
	tpl, err := template.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	out, err := Run(context.Background(), Args{
		LLMClient: client,
		Model:     "test-model",
		Template:  *tpl,
		Tools:     tool.NewRegistry(),
		ToolDefs:  baseDefs,
		Session:   session.New(t.TempDir(), "main", "test-model", session.SessionOptions{ReviewMode: "verify-threads"}),
		ReadHead: func(_ context.Context, path string) (string, error) {
			if path == "a.go" {
				return headFile, nil
			}
			return "", errors.New("not found")
		},
	}, in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out
}

func oneThread() Input {
	return Input{HeadSHA: head, Threads: []Thread{{ID: "T1", Path: "a.go", Line: 4, Finding: "n is not capped", Digest: "d1"}}}
}

const fixedArgs = `{"thread_id":"T1","decision":"fixed","reason":"guard added","evidence":{"path":"a.go","start_line":4,"end_line":6,"quote":"\tif n > max {\n\t\treturn errCap\n\t}"}}`

func TestRunDecisions(t *testing.T) {
	tests := []struct {
		name  string
		calls []llm.ToolCall
		want  string
	}{
		{name: "fixed with verbatim evidence on an unchanged line", calls: []llm.ToolCall{toolCall(verdictToolName, fixedArgs), done()}, want: Fixed},
		{name: "unfixed", calls: []llm.ToolCall{toolCall(verdictToolName, `{"thread_id":"T1","decision":"unfixed","reason":"still uncapped"}`), done()}, want: Unfixed},
		{name: "disputed by a human reply", calls: []llm.ToolCall{toolCall(verdictToolName, `{"thread_id":"T1","decision":"disputed","reason":"author defers it"}`), done()}, want: Disputed},
		{
			name: "fixed whose quote is not the head text is rejected and never recorded",
			calls: []llm.ToolCall{
				toolCall(verdictToolName, `{"thread_id":"T1","decision":"fixed","reason":"x","evidence":{"path":"a.go","start_line":4,"end_line":4,"quote":"if n > limit {"}}`),
				done(),
			},
			want: Uncertain,
		},
		{
			name:  "fixed without evidence is rejected",
			calls: []llm.ToolCall{toolCall(verdictToolName, `{"thread_id":"T1","decision":"fixed","reason":"x"}`), done()},
			want:  Uncertain,
		},
		{
			name:  "verdict for another thread is rejected",
			calls: []llm.ToolCall{toolCall(verdictToolName, strings.Replace(fixedArgs, `"T1"`, `"T9"`, 1)), done()},
			want:  Uncertain,
		},
		{name: "task_done without a verdict", calls: []llm.ToolCall{done()}, want: Uncertain},
		{
			name:  "a digest the model supplies is overwritten by the input digest",
			calls: []llm.ToolCall{toolCall(verdictToolName, `{"thread_id":"T1","decision":"unfixed","reason":"r","digest":"forged"}`), done()},
			want:  Unfixed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := runWith(t, &scriptedClient{calls: tt.calls}, oneThread())
			if len(out.Verdicts) != 1 || out.Verdicts[0].ThreadID != "T1" {
				t.Fatalf("verdicts = %+v, want one for T1", out.Verdicts)
			}
			if got := out.Verdicts[0].Decision; got != tt.want {
				t.Fatalf("decision = %q, want %q (reason %q)", got, tt.want, out.Verdicts[0].Reason)
			}
			if out.Verdicts[0].Digest != "d1" {
				t.Fatalf("digest = %q, want the input digest", out.Verdicts[0].Digest)
			}
			if out.HeadSHA != head {
				t.Fatalf("head_sha = %q, want %q", out.HeadSHA, head)
			}
		})
	}
}

func TestRunStopsCallingTheModelOnceTheBudgetIsSpent(t *testing.T) {
	tpl, err := template.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	used := int64(500)
	call := toolCall(verdictToolName, `{"thread_id":"T1","decision":"unfixed","reason":"r"}`)
	content := ""
	client := &budgetClient{resp: &llm.ChatResponse{
		Usage:   &llm.UsageInfo{PromptTokens: used, TotalTokens: used},
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content, ToolCalls: []llm.ToolCall{call, done()}}}},
	}}
	in := Input{HeadSHA: head, Threads: []Thread{{ID: "T1", Path: "a.go"}, {ID: "T2", Path: "a.go"}, {ID: "T3", Path: "a.go"}}}
	out, err := Run(context.Background(), Args{
		LLMClient:       client,
		Model:           "m",
		Template:        *tpl,
		Tools:           tool.NewRegistry(),
		ToolDefs:        baseDefs,
		Session:         session.New(t.TempDir(), "main", "m", session.SessionOptions{}),
		MaxTokensBudget: used,
		ReadHead:        func(context.Context, string) (string, error) { return headFile, nil },
	}, in)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("model calls = %d, want 1: no call once the budget is spent", client.calls)
	}
	if got := out.Verdicts[0].Decision; got != Unfixed {
		t.Fatalf("first verdict = %q, want %q", got, Unfixed)
	}
	for _, v := range out.Verdicts[1:] {
		if v.Decision != Uncertain {
			t.Fatalf("%s decision = %q, want %q", v.ThreadID, v.Decision, Uncertain)
		}
	}
}

type budgetClient struct {
	calls int
	resp  *llm.ChatResponse
}

func (c *budgetClient) CompletionsWithCtx(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	return c.resp, nil
}

func TestRunLLMErrorIsUncertain(t *testing.T) {
	out := runWith(t, &scriptedClient{err: errors.New("provider down")}, oneThread())
	if got := out.Verdicts[0].Decision; got != Uncertain {
		t.Fatalf("decision = %q, want %q", got, Uncertain)
	}
}

func TestRunRejectsMalformedHead(t *testing.T) {
	_, err := Run(context.Background(), Args{Tools: tool.NewRegistry()}, Input{HeadSHA: "HEAD"})
	if err == nil {
		t.Fatal("Run accepted a non-sha head")
	}
}

func TestRunCapsThreads(t *testing.T) {
	in := Input{HeadSHA: head}
	for i := 0; i < MaxThreads+5; i++ {
		in.Threads = append(in.Threads, Thread{ID: "T" + strings.Repeat("x", i+1), Path: "a.go"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := Run(ctx, Args{Tools: tool.NewRegistry(), Session: session.New(t.TempDir(), "main", "m", session.SessionOptions{})}, in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out.Verdicts) != MaxThreads {
		t.Fatalf("verdicts = %d, want %d", len(out.Verdicts), MaxThreads)
	}
	for _, v := range out.Verdicts {
		if v.Decision != Uncertain {
			t.Fatalf("expired run produced %q", v.Decision)
		}
	}
}

func TestOfferedToolsAreReadOnlyPlusVerdict(t *testing.T) {
	client := &scriptedClient{calls: []llm.ToolCall{done()}}
	runWith(t, client, oneThread())
	var names []string
	for _, d := range client.requests[0].Tools {
		names = append(names, d.Function.Name)
	}
	if got, want := strings.Join(names, ","), "task_done,file_read,code_search,thread_verdict"; got != want {
		t.Fatalf("offered tools = %s, want %s", got, want)
	}
	if _, ok := verdictToolDef.Function.Parameters["required"].([]any); !ok {
		t.Fatalf("verdict schema required decodes as %T, want []any", verdictToolDef.Function.Parameters["required"])
	}
}

func TestQuoteMatches(t *testing.T) {
	tests := []struct {
		name       string
		start, end int
		quote      string
		want       bool
	}{
		{"exact", 4, 4, "\tif n > max {", true},
		{"trailing whitespace ignored", 4, 4, "\tif n > max {  \n", true},
		{"different text", 4, 4, "\tif n > limit {", false},
		{"past end of file", 8, 20, "}", false},
		{"inverted range", 5, 4, "x", false},
	}
	for _, tt := range tests {
		if got := QuoteMatches(headFile, tt.start, tt.end, tt.quote); got != tt.want {
			t.Errorf("%s: QuoteMatches = %v, want %v", tt.name, got, tt.want)
		}
	}
}
