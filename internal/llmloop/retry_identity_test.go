// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// capturedRequest is one observed CompletionsWithCtx call and the request
// identity its context carried, if any.
type capturedRequest struct {
	meta llm.RequestMeta
	ok   bool
}

// metaCaptureClient records the RequestMeta of every request it receives. It is
// the only way to check identity from outside package llm: the meta travels in
// the context, so the client is where it becomes observable.
type metaCaptureClient struct {
	mu       sync.Mutex
	captured []capturedRequest
	// respond is called with the zero-based call index so a test can drive a
	// multi-round tool loop.
	respond func(n int) *llm.ChatResponse
	// respondErr drives request failures and takes precedence over respond.
	respondErr func(n int) (*llm.ChatResponse, error)
}

func (c *metaCaptureClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	meta, ok := llm.RequestMetaFromContext(ctx)
	c.mu.Lock()
	n := len(c.captured)
	c.captured = append(c.captured, capturedRequest{meta: meta, ok: ok})
	c.mu.Unlock()
	if c.respondErr != nil {
		return c.respondErr(n)
	}
	if c.respond == nil {
		return emptyResponse(), nil
	}
	return c.respond(n), nil
}

func (c *metaCaptureClient) requests() []capturedRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]capturedRequest(nil), c.captured...)
}

func emptyResponse() *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
	}
}

// metaFactory builds the same closure internal/agent injects, so these tests
// exercise the review wiring rather than a test-only shape.
func metaFactory(provider, modelName string) func(string, session.TaskType, int) llm.RequestMeta {
	return func(filePath string, taskType session.TaskType, requestNo int) llm.RequestMeta {
		return llm.RequestMeta{
			Provider:  provider,
			Model:     modelName,
			FilePath:  filePath,
			TaskType:  string(taskType),
			RequestNo: requestNo,
		}
	}
}

func wantMeta(t *testing.T, got capturedRequest, want llm.RequestMeta) {
	t.Helper()
	if !got.ok {
		t.Fatalf("request carried no identity, want %+v", want)
	}
	if got.meta != want {
		t.Errorf("meta = %+v, want %+v", got.meta, want)
	}
}

// TestRunMainTask_MainTaskIdentity checks that every main_task round carries the
// identity of the TaskRecord created for it, including the per-round RequestNo.
// An empty provider is covered as its own case: it is the real value for an
// unnamed endpoint, so it must still produce identity rather than be read as
// "no meta".
func TestRunMainTask_MainTaskIdentity(t *testing.T) {
	for _, provider := range []string{"openai", ""} {
		name := provider
		if name == "" {
			name = "empty-provider"
		}
		t.Run(name, func(t *testing.T) {
			client := &metaCaptureClient{}
			client.respond = func(n int) *llm.ChatResponse {
				if n == 0 {
					return fileReadToolCallResponse("call_1", `{"path":"main.go"}`)
				}
				return taskDoneResponse()
			}

			deps := newTestDeps(client)
			deps.NewRequestMeta = metaFactory(provider, deps.Model)
			runner := NewRunner(deps)

			if _, _, err := runner.RunMainTask(
				context.Background(),
				[]llm.Message{llm.NewTextMessage("user", "review this file")},
				"main.go",
			); err != nil {
				t.Fatalf("RunMainTask: %v", err)
			}

			reqs := client.requests()
			if len(reqs) != 2 {
				t.Fatalf("got %d requests, want 2", len(reqs))
			}
			for i, got := range reqs {
				wantMeta(t, got, llm.RequestMeta{
					Provider:  provider,
					Model:     "fake",
					FilePath:  "main.go",
					TaskType:  string(session.MainTask),
					RequestNo: i + 1,
				})
			}

			// The report joins on these fields, so they must match the records
			// the session actually wrote, not just each other.
			fs := deps.Session.GetOrCreateFileSession("main.go")
			if n := len(fs.TaskRecords[session.MainTask]); n != 2 {
				t.Fatalf("session holds %d main_task records, want 2", n)
			}
			for i, rec := range fs.TaskRecords[session.MainTask] {
				if reqs[i].meta.RequestNo != rec.RequestNo {
					t.Errorf("request %d: meta RequestNo = %d, record = %d", i, reqs[i].meta.RequestNo, rec.RequestNo)
				}
			}
		})
	}
}

// TestRunMainTask_GraceRoundIsAMainTaskRound verifies that the grace round uses
// the next main_task identity and records both responses and errors.
func TestRunMainTask_GraceRoundIsAMainTaskRound(t *testing.T) {
	// Delay the grace response so duration assertions are stable across platforms.
	const graceDelay = 20 * time.Millisecond

	cases := []struct {
		name   string
		client *metaCaptureClient
		want   func(t *testing.T, grace *session.TaskRecord)
	}{
		{
			name: "response recorded",
			client: &metaCaptureClient{respond: func(n int) *llm.ChatResponse {
				if n == 0 {
					return fileReadToolCallResponse("call_1", `{"path":"main.go"}`)
				}
				time.Sleep(graceDelay)
				return emptyResponse()
			}},
			want: func(t *testing.T, grace *session.TaskRecord) {
				if grace.Response == nil {
					t.Error("grace round response was not recorded")
				}
			},
		},
		{
			name: "suppressed error recorded",
			client: &metaCaptureClient{respondErr: func(n int) (*llm.ChatResponse, error) {
				if n == 0 {
					return fileReadToolCallResponse("call_1", `{"path":"main.go"}`), nil
				}
				time.Sleep(graceDelay)
				return nil, errors.New("grace request failed")
			}},
			want: func(t *testing.T, grace *session.TaskRecord) {
				if !strings.Contains(grace.Error, "grace request failed") {
					t.Errorf("grace round error = %q, want the request error", grace.Error)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestDeps(tc.client)
			deps.Template.MaxToolRequestTimes = 1
			deps.MainToolDefs = []llm.ToolDef{
				{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
				{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
				{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
			}
			deps.NewRequestMeta = metaFactory("openai", deps.Model)

			completed, stop, err := NewRunner(deps).RunMainTask(
				context.Background(),
				[]llm.Message{llm.NewTextMessage("user", "review this file")},
				"main.go",
			)
			if err != nil {
				t.Fatalf("RunMainTask: %v", err)
			}
			if completed || stop != StopMaxRounds {
				t.Fatalf("completed = %v, stop = %v; want false, StopMaxRounds", completed, stop)
			}

			reqs := tc.client.requests()
			if len(reqs) != 2 {
				t.Fatalf("got %d requests, want 2", len(reqs))
			}
			wantMeta(t, reqs[1], llm.RequestMeta{
				Provider:  "openai",
				Model:     "fake",
				FilePath:  "main.go",
				TaskType:  string(session.MainTask),
				RequestNo: 2,
			})

			recs := deps.Session.GetOrCreateFileSession("main.go").TaskRecords[session.MainTask]
			if len(recs) != 2 {
				t.Fatalf("session holds %d main_task records, want 2", len(recs))
			}
			grace := recs[1]
			if grace.Duration < graceDelay/2 {
				t.Errorf("grace round duration = %v, want at least %v", grace.Duration, graceDelay/2)
			}
			tc.want(t, grace)
		})
	}
}

// TestRunMainTask_NoIdentityWhenFactoryNil is the scan guarantee: the Runner is
// shared, and with NewRequestMeta left nil no request may carry identity.
func TestRunMainTask_NoIdentityWhenFactoryNil(t *testing.T) {
	client := &metaCaptureClient{respond: func(int) *llm.ChatResponse { return taskDoneResponse() }}
	runner := NewRunner(newTestDeps(client)) // NewRequestMeta unset, as scan leaves it

	if _, _, err := runner.RunMainTask(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	); err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	if reqs[0].ok {
		t.Errorf("scan request carried identity %+v, want none", reqs[0].meta)
	}
}

// newCompressionRunner returns a Runner whose template forces runCompression to
// issue a request, plus the message slice to feed it.
func newCompressionRunner(t *testing.T, client llm.LLMClient, factory func(string, session.TaskType, int) llm.RequestMeta) (*Runner, []llm.Message) {
	t.Helper()
	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
	r := NewRunner(Deps{
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MemoryCompressionTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Summarize: {{context}}"}},
			},
			MaxTokens: 50,
		},
		CommentCollector: tool.NewCommentCollector(),
		Session:          sess,
		NewRequestMeta:   factory,
	})

	msgs := []llm.Message{
		llm.NewTextMessage("system", "sys"),
		llm.NewTextMessage("user", "prompt"),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.NewTextMessage("assistant", strings.Repeat("word ", 100)))
		msgs = append(msgs, llm.NewTextMessage("tool", strings.Repeat("data ", 50)))
	}
	return r, msgs
}

// TestRunCompression_Identity covers both compression paths at once: the
// synchronous one is this call, and the async one reaches the same function
// through triggerAsyncCompression, so identity is stamped for both.
func TestRunCompression_Identity(t *testing.T) {
	summary := "compressed summary"
	client := &metaCaptureClient{respond: func(int) *llm.ChatResponse {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
	}}
	r, msgs := newCompressionRunner(t, client, metaFactory("openai", "fake"))

	if _, err := r.runCompression(context.Background(), msgs, "test.go"); err != nil {
		t.Fatalf("runCompression: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	wantMeta(t, reqs[0], llm.RequestMeta{
		Provider:  "openai",
		Model:     "fake",
		FilePath:  "test.go",
		TaskType:  string(session.MemoryCompressionTask),
		RequestNo: 1,
	})
}

func TestRunCompression_NoIdentityWhenFactoryNil(t *testing.T) {
	summary := "compressed summary"
	client := &metaCaptureClient{respond: func(int) *llm.ChatResponse {
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}
	}}
	r, msgs := newCompressionRunner(t, client, nil)

	if _, err := r.runCompression(context.Background(), msgs, "test.go"); err != nil {
		t.Fatalf("runCompression: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	if reqs[0].ok {
		t.Errorf("scan compression carried identity %+v, want none", reqs[0].meta)
	}
}

// TestReLocation_Identity pins the field that is easy to get wrong: FilePath is
// the comment's path, which is the file session the re-location record was
// written to — not the tool loop's taskKey, which is only the fallback for a
// comment that named no path. The test passes an empty taskKey against a
// comment that does name one, so a regression that switched the identity to
// taskKey would show up as an empty FilePath rather than as other.go.
func TestReLocation_Identity(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
	// No fenced code block in the reply: re-location fails to improve the match,
	// which keeps the assertion on identity rather than on resolution.
	reply := "cannot find it"
	client := &metaCaptureClient{respond: func(int) *llm.ChatResponse {
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
		NewRequestMeta: metaFactory("openai", "fake"),
	})

	// taskKey is empty, so the fallback cannot supply a path and the comment
	// keeps the target the tool call named, other.go.
	cp := r.executeToolCall(context.Background(), "", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"path":"other.go","content":"issue","existing_code":"no such code"}]}`,
		},
	}, nil, "")
	if cp.Data != tool.CommentSucceed {
		t.Fatalf("cp.Data = %q, want CommentSucceed", cp.Data)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	wantMeta(t, reqs[0], llm.RequestMeta{
		Provider:  "openai",
		Model:     "fake",
		FilePath:  "other.go",
		TaskType:  string(session.ReLocationTask),
		RequestNo: 1,
	})
	if n := len(sess.GetOrCreateFileSession("other.go").TaskRecords[session.ReLocationTask]); n != 1 {
		t.Errorf("other.go holds %d re-location records, want 1", n)
	}
}

// TestReLocation_NoRequestWithoutTemplate covers the branch that skips both the
// session record and the request: with no prompt there is nothing to send, so
// no orphan re-location record may appear.
func TestReLocation_NoRequestWithoutTemplate(t *testing.T) {
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	reg.Freeze()

	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: "diff"})
	client := &metaCaptureClient{}

	r := NewRunner(Deps{
		LLMClient: client,
		Model:     "fake",
		Template: template.Template{
			MaxTokens: 10000,
			// Present but empty: BuildReLocationMessages returns nil, which the
			// caller must treat exactly like a nil task.
			ReLocationTask: &template.LlmConversation{},
		},
		Tools:            reg,
		CommentCollector: collector,
		Session:          sess,
		DiffLookup: func(path string) *model.Diff {
			return &model.Diff{NewPath: path, NewFileContent: "line one\nline two\n"}
		},
		NewRequestMeta: metaFactory("openai", "fake"),
	})

	r.executeToolCall(context.Background(), "", llm.ToolCall{
		Function: llm.FunctionCall{
			Name:      tool.CodeComment.Name(),
			Arguments: `{"comments":[{"path":"other.go","content":"issue","existing_code":"no such code"}]}`,
		},
	}, nil, "")

	if reqs := client.requests(); len(reqs) != 0 {
		t.Fatalf("got %d requests, want 0", len(reqs))
	}
	if n := len(sess.GetOrCreateFileSession("other.go").TaskRecords[session.ReLocationTask]); n != 0 {
		t.Errorf("other.go holds %d re-location records, want 0", n)
	}
}
