package agent

import (
	"context"
	"testing"

	"github.com/open-code-review/open-code-review/internal/config/template"
	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/open-code-review/open-code-review/internal/reviewbackend"
	"github.com/open-code-review/open-code-review/internal/session"
	"github.com/open-code-review/open-code-review/internal/tool"
)

type recordingBackend struct {
	reviewCalled bool
	lastReq      reviewbackend.ReviewFileRequest
	executor     reviewbackend.ToolExecutor
}

func (b *recordingBackend) Kind() reviewbackend.Kind { return reviewbackend.KindCursorAgent }
func (b *recordingBackend) Model() string            { return "test-model" }
func (b *recordingBackend) Source() string           { return "test" }
func (b *recordingBackend) Complete(context.Context, reviewbackend.CompleteRequest) (*reviewbackend.CompleteResponse, error) {
	return &reviewbackend.CompleteResponse{}, nil
}
func (b *recordingBackend) ReviewFile(ctx context.Context, req reviewbackend.ReviewFileRequest, exec reviewbackend.ToolExecutor, _ *reviewbackend.ReviewHooks) error {
	b.reviewCalled = true
	b.lastReq = req
	b.executor = exec
	if exec != nil {
		exec(ctx, reviewbackend.ToolCallInput{
			ID:        "tc-1",
			Name:      "code_comment",
			Arguments: `{"body":"note"}`,
		})
	}
	return nil
}

func TestPerformLlmCodeReview_UsesBackend(t *testing.T) {
	backend := &recordingBackend{}
	reg := tool.NewRegistry()
	a := &Agent{
		args: Args{
			Model:   "test-model",
			Backend: backend,
			Tools:   reg,
			MainToolDefs: []llm.ToolDef{{
				Type:     "function",
				Function: llm.FunctionDef{Name: "code_comment"},
			}},
			Template: templateWithMaxRounds(2),
		},
		session: session.New(t.TempDir(), "main", "test-model", session.SessionOptions{}),
	}

	err := a.performLlmCodeReview(context.Background(), []llm.Message{
		llm.NewTextMessage("user", "review file"),
	}, "src/main.go")
	if err != nil {
		t.Fatalf("performLlmCodeReview: %v", err)
	}
	if !backend.reviewCalled {
		t.Fatal("backend.ReviewFile was not called")
	}
	if backend.lastReq.FilePath != "src/main.go" {
		t.Errorf("FilePath = %q", backend.lastReq.FilePath)
	}
	if backend.lastReq.Model != "test-model" {
		t.Errorf("Model = %q", backend.lastReq.Model)
	}
}

func TestPerformLlmCodeReview_RequiresBackend(t *testing.T) {
	a := &Agent{args: Args{}}
	err := a.performLlmCodeReview(context.Background(), nil, "x.go")
	if err == nil {
		t.Fatal("expected error when backend is nil")
	}
}

func templateWithMaxRounds(n int) template.Template {
	return template.Template{MaxToolRequestTimes: n, MaxTokens: 1024}
}
