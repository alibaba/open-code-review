// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// scopeCaptureClient records whether each request's context still carries the
// review admission scope. It is the admission twin of metaCaptureClient
// (retry_identity_test.go): the scope travels in the context, so the client
// boundary is the only place it is observable from this package.
type scopeCaptureClient struct {
	mu      sync.Mutex
	scoped  []bool
	respond func(n int) *llm.ChatResponse
}

func (c *scopeCaptureClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.mu.Lock()
	n := len(c.scoped)
	c.scoped = append(c.scoped, llm.HasAdmissionScope(ctx))
	c.mu.Unlock()
	if c.respond != nil {
		return c.respond(n), nil
	}
	return emptyResponse(), nil
}

func (c *scopeCaptureClient) snapshots() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.scoped...)
}

// wantAllScoped fails unless every captured request carried the scope. It also
// demands at least one request, so a path that silently stops calling the LLM
// cannot pass by vacuity.
func wantAllScoped(t *testing.T, got []bool) {
	t.Helper()
	if len(got) == 0 {
		t.Fatal("no requests captured — path never called the LLM")
	}
	for i, ok := range got {
		if !ok {
			t.Fatalf("request %d lost the admission scope mid-path", i)
		}
	}
}

// TestAdmissionScope_SurvivesMainAndGraceRounds pins the wiring contract for
// the review main loop: a ctx scoped by review_cmd flows to every main-task
// request and to the grace round that the same loop fires on round exhaustion.
// The regression this guards is any future refactor that rebuilds the request
// context from scratch instead of deriving it from the run ctx.
func TestAdmissionScope_SurvivesMainAndGraceRounds(t *testing.T) {
	client := &scopeCaptureClient{respond: func(n int) *llm.ChatResponse {
		if n == 0 {
			return fileReadToolCallResponse("call_1", `{"path":"main.go"}`)
		}
		return emptyResponse()
	}}
	deps := newTestDeps(client)
	deps.Template.MaxToolRequestTimes = 1
	deps.MainToolDefs = []llm.ToolDef{
		{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
		{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
	}

	completed, stop, err := NewRunner(deps).RunPerFile(
		llm.ContextWithAdmissionScope(context.Background()),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	if completed || stop != StopMaxRounds {
		t.Fatalf("completed = %v, stop = %v; want false, StopMaxRounds (grace round must fire)", completed, stop)
	}
	// Two requests: the exhausted main round and the grace round.
	wantAllScoped(t, client.snapshots())
}

// TestAdmissionScope_SurvivesCompression asserts both compression paths keep
// the scope: the synchronous runCompression call and the async job, whose
// context is rebuilt via context.WithTimeout(context.WithoutCancel(ctx), 5m) —
// the exact spot where a maintainer fixing "detached deadline" semantics might
// swap in context.Background() and silently drop every context value.
func TestAdmissionScope_SurvivesCompression(t *testing.T) {
	t.Run("synchronous", func(t *testing.T) {
		summary := "compressed"
		client := &scopeCaptureClient{respond: func(int) *llm.ChatResponse {
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
		}}
		r, msgs := newCompressionRunner(t, client, nil)
		if _, err := r.runCompression(llm.ContextWithAdmissionScope(context.Background()), msgs, "test.go"); err != nil {
			t.Fatalf("runCompression: %v", err)
		}
		wantAllScoped(t, client.snapshots())
	})

	t.Run("async WithoutCancel child", func(t *testing.T) {
		summary := "compressed"
		client := &scopeCaptureClient{respond: func(int) *llm.ChatResponse {
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
		}}
		r, msgs := newCompressionRunner(t, client, nil)

		// The parent is cancelled before the job runs, as a finishing file
		// conversation is; the child must still carry the scope.
		parent, cancel := context.WithCancel(llm.ContextWithAdmissionScope(context.Background()))
		st := &compressionState{}
		r.triggerAsyncCompression(parent, st, msgs, "test.go")
		cancel()
		r.WaitBackground()

		if st.pendingJob == nil {
			t.Fatal("no compression job was triggered")
		}
		wantAllScoped(t, client.snapshots())
	})
}

// TestAdmissionScope_SurvivesReLocation pins the pooled re-location path:
// like compression it runs under a WithoutCancel child (loop.go builds it with
// context.WithoutCancel(ctx) for the comment worker pool), so the scope must
// survive the detach.
func TestAdmissionScope_SurvivesReLocation(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
	reply := "cannot find it"
	client := &scopeCaptureClient{respond: func(int) *llm.ChatResponse {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}
	}}

	r := NewRunner(Deps{
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens: 10000,
			ReLocationTask: &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "relocate {suggestion_content} in {diff} near {existing_code}"}},
			},
		},
		Tools:            reg,
		CommentCollector: collector,
		Session:          sess,
		DiffLookup: func(path string) *model.Diff {
			return &model.Diff{NewPath: path, NewFileContent: "line one\nline two\n"}
		},
	})

	// newPath empty: the comment keeps the path it named, other.go.
	cp := r.executeToolCall(llm.ContextWithAdmissionScope(context.Background()), "", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"path":"other.go","content":"issue","existing_code":"no such code"}]}`,
		},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}
	wantAllScoped(t, client.snapshots())
}

// TestAdmissionScope_AbsentWithoutScope is the scan guarantee at the loop
// level: the same Runner, driven with a ctx that review_cmd never wrapped,
// must send requests the gate ignores.
func TestAdmissionScope_AbsentWithoutScope(t *testing.T) {
	client := &scopeCaptureClient{respond: func(int) *llm.ChatResponse { return taskDoneResponse() }}
	if _, _, err := NewRunner(newTestDeps(client)).RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	); err != nil {
		t.Fatalf("RunPerFile: %v", err)
	}
	got := client.snapshots()
	if len(got) != 1 {
		t.Fatalf("got %d requests, want 1", len(got))
	}
	if got[0] {
		t.Fatal("scan-style request carried the review admission scope")
	}
}
