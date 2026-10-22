// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// TestScanFileRead_MissingPathKeepsItemStatus covers the workspace branch of
// the path-validity change as scan sees it: a path that is not in the working
// tree is rejected as a structural rejection, and the rejection must not
// change how the item's status is derived — the file still completes, and no
// scan_subtask_error warning is recorded for it.
func TestScanFileRead_MissingPathKeepsItemStatus(t *testing.T) {
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)
	t.Setenv("USERPROFILE", testHome)
	repoDir := t.TempDir()
	items := []model.ScanItem{{Path: "a.go", Content: "package a\nfunc f() {}\n", LineCount: 2}}

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100000
	tpl.MaxToolRequestTimes = 5

	collector := tool.NewCommentCollector()
	registry := tool.NewRegistry()
	registry.Register(&tool.CodeCommentProvider{Collector: collector})
	registry.Register(tool.NewFileRead(&tool.FileReader{RepoDir: repoDir, Mode: tool.ModeWorkspace}))
	registry.Freeze()

	client := &fakeScanClient{responses: []*llm.ChatResponse{
		{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{
					ID: "read", Type: "function",
					Function: llm.FunctionCall{Name: "file_read", Arguments: `{"file_path":"missing.go"}`},
				}}},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 10},
		},
		scanTaskDoneResponse(),
	}}

	a := NewAgent(Args{
		Template:  tpl,
		LLMClient: client,
		Model:     "test",
		Tools:     registry,
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		CommentCollector: collector,
		MaxConcurrency:   1,
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
		RepoDir:          repoDir,
		Session: session.New(repoDir, "main", "test", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
	a.items = items
	a.currentDate = "2026-06-26"

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	if a.subtaskFailed != 0 {
		t.Errorf("subtaskFailed = %d, want 0: a rejected path must not fail the item", a.subtaskFailed)
	}
	for _, w := range a.Warnings() {
		if w.Type == "scan_subtask_error" {
			t.Errorf("warning %+v, want none: a rejected path must not fail the item", w)
		}
	}

	// The rejection is still visible as a failed tool call, carrying the
	// message that identifies it.
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
