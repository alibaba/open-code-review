// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// calcGo is a one-file scan target. The defect is on line 4.
const calcGo = "package calc\n\nfunc Add(a, b int) int {\n\treturn a - b\n}\n"

func TestScanCodeComment_AnchorsLineWhenSnippetIsInFile(t *testing.T) {
	comments := runScanComments(t, `{"comments":[{"path":"calc.go","content":"Add subtracts","existing_code":"return a - b","category":"bug","severity":"high"}]}`)
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want 1", comments)
	}
	if comments[0].StartLine == 0 || comments[0].EndLine == 0 {
		t.Fatalf("finding line = %d-%d, want a non-zero anchor for existing_code in the file", comments[0].StartLine, comments[0].EndLine)
	}
	if comments[0].StartLine != 4 || comments[0].EndLine != 4 {
		t.Fatalf("finding line = %d-%d, want 4-4", comments[0].StartLine, comments[0].EndLine)
	}
}

func TestScanCodeComment_KeepsExplicitLineWhenModelSuppliesIt(t *testing.T) {
	// The snippet is not a whole file line, so text matching cannot place it.
	// The tool call still names an in-file line. That anchor must survive.
	comments := runScanComments(t, `{"comments":[{"path":"calc.go","content":"Add subtracts","existing_code":"a - b","start_line":4,"end_line":4,"category":"bug","severity":"high"}]}`)
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want 1", comments)
	}
	if comments[0].StartLine != 4 || comments[0].EndLine != 4 {
		t.Fatalf("finding line = %d-%d, want 4-4 from the tool call", comments[0].StartLine, comments[0].EndLine)
	}
}

func TestScanCodeComment_KeepsExplicitLineWithoutSnippet(t *testing.T) {
	comments := runScanComments(t, `{"comments":[{"path":"calc.go","content":"Add subtracts","start_line":4,"end_line":4,"category":"bug","severity":"high"}]}`)
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want 1", comments)
	}
	if comments[0].StartLine != 4 || comments[0].EndLine != 4 {
		t.Fatalf("finding line = %d-%d, want 4-4 from the tool call", comments[0].StartLine, comments[0].EndLine)
	}
}

func TestScanCodeComment_FileMatchWinsOverConflictingModelLine(t *testing.T) {
	comments := runScanComments(t, `{"comments":[{"path":"calc.go","content":"package clause","existing_code":"package calc","start_line":4,"end_line":4}]}`)
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want 1", comments)
	}
	if comments[0].StartLine != 1 || comments[0].EndLine != 1 {
		t.Fatalf("finding line = %d-%d, want 1-1 from the file match", comments[0].StartLine, comments[0].EndLine)
	}
}

func TestScanCodeComment_DropsModelLineOutsideFile(t *testing.T) {
	comments := runScanComments(t, `{"comments":[{"path":"calc.go","content":"nope","existing_code":"not in the file","start_line":99,"end_line":99}]}`)
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want 1", comments)
	}
	if comments[0].StartLine != 0 || comments[0].EndLine != 0 {
		t.Fatalf("finding line = %d-%d, want 0-0 when the model line is outside the file and the snippet does not match", comments[0].StartLine, comments[0].EndLine)
	}
}

func TestScanCodeComment_OmitsLineWhenNothingIsAnchorable(t *testing.T) {
	comments := runScanComments(t, `{"comments":[{"path":"calc.go","content":"something is wrong","existing_code":"this text is not in the file","category":"bug","severity":"low"}]}`)
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want 1", comments)
	}
	if comments[0].StartLine != 0 || comments[0].EndLine != 0 {
		t.Fatalf("finding line = %d-%d, want 0-0 when the model omitted a line and the snippet does not match", comments[0].StartLine, comments[0].EndLine)
	}
}

func runScanComments(t *testing.T, arguments string) []model.LlmComment {
	t.Helper()
	repo := t.TempDir()
	item := model.ScanItem{Path: "calc.go", Content: calcGo, LineCount: 5}
	tpl := makeTemplateWithFullScan()
	// Default scan templates include re-location. An empty synthetic diff
	// must not be what decides whether an anchorable comment keeps a line.
	defaults, err := template.LoadScanDefault()
	if err != nil {
		t.Fatalf("LoadScanDefault: %v", err)
	}
	tpl.ReLocationTask = defaults.ReLocationTask

	collector := tool.NewCommentCollector()
	registry := tool.NewRegistry()
	registry.Register(&tool.CodeCommentProvider{Collector: collector})
	registry.Freeze()

	client := &scriptedScanClient{
		toolResponses: []*llm.ChatResponse{
			scanCommentResponse(arguments),
			scanTaskDoneResponse(),
		},
	}
	agent := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        client,
		Model:            "test-model",
		Tools:            registry,
		MainToolDefs:     []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}}},
		CommentCollector: collector,
		MaxConcurrency:   1,
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		Session: session.New(repo, "main", "test-model", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	agent.items = []model.ScanItem{item}
	agent.currentDate = "2026-11-10"

	comments, err := agent.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}
	return comments
}

// scriptedScanClient returns tool-loop responses in order. Auxiliary calls
// (re-location has no tools) get a reply with no code block, so a failed
// text match cannot invent a line number.
type scriptedScanClient struct {
	toolResponses []*llm.ChatResponse
	toolIdx       int
}

func (c *scriptedScanClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Tools) == 0 {
		text := "unable to locate"
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}},
			Usage:   &llm.UsageInfo{PromptTokens: 1, CompletionTokens: 1},
		}, nil
	}
	if c.toolIdx >= len(c.toolResponses) {
		empty := ""
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &empty}}},
			Usage:   &llm.UsageInfo{},
		}, nil
	}
	resp := c.toolResponses[c.toolIdx]
	c.toolIdx++
	return resp, nil
}

func scanCommentResponse(arguments string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				ToolCalls: []llm.ToolCall{{
					ID:   "comment",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "code_comment",
						Arguments: arguments,
					},
				}},
			},
		}},
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}
}
