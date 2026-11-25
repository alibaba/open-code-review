// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/toolsconfig"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/reviewtask"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestPrepareReviewCodeTools(test *testing.T) {
	repo := initResumeRepo(test)
	commitFile(test, repo, "version.go", "package example\nconst Value = 1\n", "old")
	previous := revParse(test, repo, "HEAD")
	commitFile(test, repo, "version.go", "package example\nconst Value = 2\n", "new")
	head := revParse(test, repo, "HEAD")
	runner := gitcmd.New(2)
	for _, opts := range []reviewOptions{{from: previous, to: "HEAD"}, {commit: "HEAD"}} {
		setup, err := prepareReviewCodeTools(context.Background(), repo, opts, runner, nil)
		if err != nil {
			test.Fatal(err)
		}
		if setup.access == nil || setup.resolution.ResolvedHead != head || setup.resolution.ResolvedBase != previous {
			test.Fatalf("incorrect setup: %+v", setup)
		}
		registry := buildToolRegistryWithCodeAccess(nil, setup.reader, setup.access)
		provider, _ := registry.Get("file_read")
		for _, side := range []string{"old", "new"} {
			got, err := provider.Execute(context.Background(), map[string]any{"file_path": "version.go", "side": side})
			want := "Value = 2"
			if side == "old" {
				want = "Value = 1"
			}
			if err != nil || !strings.Contains(got, want) {
				test.Fatalf("%s: %q %v", side, got, err)
			}
		}
	}
	opts := reviewOptions{commit: "HEAD"}
	setup, err := prepareReviewCodeTools(context.Background(), repo, opts, runner, nil)
	if err != nil {
		test.Fatal(err)
	}
	commitFile(test, repo, "version.go", "package example\nconst Value = 3\n", "advance")
	got, err := setup.reader.Read(context.Background(), "version.go")
	if err != nil || !strings.Contains(got, "Value = 2") {
		test.Fatalf("new side moved with HEAD: %q %v", got, err)
	}
	for _, resumedOpts := range []reviewOptions{opts, {from: previous, to: "HEAD"}} {
		resumed, err := prepareReviewCodeTools(context.Background(), repo, resumedOpts, runner, setup.resolution)
		if err != nil {
			test.Fatal(err)
		}
		if resumed.access == nil || resumed.reader.Ref != head {
			test.Fatalf("resume lost comparison access or snapshot: %+v", resumed)
		}
	}
	workspace, err := prepareReviewCodeTools(context.Background(), repo, reviewOptions{}, runner, nil)
	if err != nil || workspace.access == nil || workspace.reader.Mode != tool.ModeWorkspace || workspace.reader.Ref != "" {
		test.Fatalf("workspace changed: %+v %v", workspace, err)
	}
	root := revParse(test, repo, "HEAD~4")
	empty, err := prepareReviewCodeTools(context.Background(), repo, reviewOptions{commit: root}, runner, nil)
	if err != nil || !empty.resolution.BaseIsEmpty || empty.access == nil {
		test.Fatalf("root comparison: %+v %v", empty, err)
	}
}

func TestPrepareReviewCodeToolsErrors(test *testing.T) {
	repo := initResumeRepo(test)
	for _, scenario := range []struct {
		opts       reviewOptions
		resolution *diff.InputResolution
	}{
		{opts: reviewOptions{commit: "missing"}},
		{opts: reviewOptions{commit: "HEAD"}, resolution: &diff.InputResolution{}},
		{opts: reviewOptions{commit: "HEAD"}, resolution: &diff.InputResolution{ResolvedHead: "HEAD"}},
		{resolution: &diff.InputResolution{}},
		{resolution: &diff.InputResolution{ResolvedBase: "HEAD", BaseIsEmpty: true}},
		{resolution: &diff.InputResolution{ResolvedBase: "HEAD", ResolvedHead: "HEAD"}},
		{resolution: &diff.InputResolution{ResolvedBase: "HEAD", ExactRange: "HEAD..HEAD"}},
	} {
		if _, err := prepareReviewCodeTools(context.Background(), repo, scenario.opts, nil, scenario.resolution); err == nil {
			test.Fatalf("accepted invalid setup: %+v", scenario)
		}
	}
}

type reviewToolsClient func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)

func (c reviewToolsClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	return c(ctx, request)
}

func TestReviewLoopPreservesDiffAndComparisonReads(t *testing.T) {
	repo := initResumeRepo(t)
	commitFile(t, repo, "version.go", "package example\nconst Value = 1\n", "old")
	previous := revParse(t, repo, "HEAD")
	commitFile(t, repo, "version.go", "package example\nconst Value = 2\n", "new")
	setup, err := prepareReviewCodeTools(context.Background(), repo, reviewOptions{from: previous, to: "HEAD"}, gitcmd.New(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	collector := tool.NewCommentCollector()
	registry := buildToolRegistryWithCodeAccess(collector, setup.reader, setup.access)
	patch, err := runGitCmd(repo, "diff", previous, "HEAD", "--", "version.go")
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := registry.Get("file_read_diff")
	provider.(*tool.FileReadDiffProvider).SetDiffMap(tool.NewDiffMap(map[string]string{"version.go": string(patch)}))
	defs, err := loadToolDefs([]string{"file_read", "file_read_diff", "task_done"})
	if err != nil {
		t.Fatal(err)
	}
	defs, err = toolsconfig.WithComparisonSides(defs)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := reviewToolsClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests++
		var calls []llm.ToolCall
		if requests == 1 {
			calls = []llm.ToolCall{
				{ID: "old", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{"file_path":"version.go","side":"old"}`}},
				{ID: "new", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{"file_path":"version.go","side":"new"}`}},
				{ID: "diff", Type: "function", Function: llm.FunctionCall{Name: "file_read_diff", Arguments: `{"path_array":["version.go"]}`}},
			}
		} else {
			results := map[string]string{}
			for _, message := range request.Messages {
				if message.Role == "tool" {
					results[message.ToolCallID] = message.ExtractText()
				}
			}
			for id, want := range map[string]string{"old": "const Value = 1", "new": "const Value = 2", "diff": string(patch)} {
				if !strings.Contains(results[id], want) {
					t.Errorf("%s result = %q, want %q", id, results[id], want)
				}
			}
			calls = []llm.ToolCall{{ID: "done", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: `{}`}}}
		}
		return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: calls}}}}, nil
	})
	runner := reviewtask.NewRunner(reviewtask.Deps{
		LLMClient: client, Model: "test", Template: *loadTestTemplate(t), Tools: registry, MainToolDefs: defs,
		CommentCollector: collector, Session: session.New(t.TempDir(), "main", "test", session.SessionOptions{}),
	})
	completed, _, err := runner.RunMainTask(context.Background(), []llm.Message{llm.NewTextMessage("user", "review version.go")}, "version.go")
	if err != nil || !completed || requests != 2 {
		t.Fatalf("completed=%v, requests=%d, err=%v", completed, requests, err)
	}
}
