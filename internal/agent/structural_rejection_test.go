// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestDispatchSubtasks_StructuralRejectionStillCompletes covers the end of
// the path-validity change as a review sees it: the model asks for a path
// that is not at the read target, gets a rejection it can act on, and the
// run still ends with the reviewed file covered.
func TestDispatchSubtasks_StructuralRejectionStillCompletes(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
		Operation:  session.OperationReview,
	})
	t.Cleanup(func() { _ = sh.Finalize() })

	collector := tool.NewCommentCollector()
	registry := tool.NewRegistry()
	registry.Register(&tool.CodeCommentProvider{Collector: collector})
	registry.Register(tool.NewFileRead(&tool.FileReader{RepoDir: repoDir, Mode: tool.ModeWorkspace}))

	client := &fakeAgentClient{responses: []*llm.ChatResponse{
		{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{
					ID: "read", Type: "function",
					Function: llm.FunctionCall{Name: "file_read", Arguments: `{"file_path":"missing.go"}`},
				}}},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 10},
		},
		agentTaskDoneResponse(),
	}}

	a := New(Args{
		RepoDir:    repoDir,
		From:       "main",
		To:         "feature",
		ReviewMode: session.ReviewModeRange,
		LLMClient:  client,
		Model:      "fake",
		Session:    sh,
		Tools:      registry,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{diffs}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		CommentCollector: collector,
	})
	a.diffs = []model.Diff{{OldPath: "a.go", NewPath: "a.go", Diff: "+ok", Insertions: 1}}
	a.currentDate = "2026-07-26 12:00"

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StateComplete {
		t.Fatalf("terminal state = %q, want %q", manifest.TerminalState, session.StateComplete)
	}
	if len(manifest.Coverage.Completed) != 1 {
		t.Fatalf("completed coverage = %+v, want the reviewed file to be covered", manifest.Coverage.Completed)
	}
	if len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("failed coverage = %+v, want none", manifest.Coverage.Failed)
	}

	// The rejection is still visible as a failed tool call, carrying the
	// message that identifies it — not the ladder's escalated wording, and
	// not the old "not found" label.
	failures := a.ToolFailures()
	if len(failures) != 1 {
		t.Fatalf("recorded tool failures = %d, want 1", len(failures))
	}
	if !strings.Contains(failures[0].Error, `read file "missing.go"`) {
		t.Errorf("recorded failure = %q, want it to identify the rejection", failures[0].Error)
	}
	if !strings.Contains(failures[0].Error, "reading it again will not help") {
		t.Errorf("recorded failure = %q, want the rejection's own wording", failures[0].Error)
	}
	if strings.Contains(failures[0].Error, "not found") {
		t.Errorf("recorded failure = %q, want the rejection not to be labelled as a not-found failure", failures[0].Error)
	}
}
