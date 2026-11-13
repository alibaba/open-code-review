// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package reviewtask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/config/toolsconfig"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type equivalenceClient struct {
	fakeClient
	metadata []llm.RequestMeta
}

func (client *equivalenceClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	metadata, _ := llm.RequestMetaFromContext(ctx)
	client.metadata = append(client.metadata, metadata)
	return client.fakeClient.CompletionsWithCtx(ctx, request)
}

func TestReviewExecutionEquivalence(t *testing.T) {
	entries, err := toolsconfig.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := template.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := toolsconfig.Resolve(entries, defaults.MainTask.Tools)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"review", "scan"} {
		for _, scenario := range []string{"normal", "failed", "invalid", "invalid_comment_shape", "repaired", "async", "round_budget", "token_budget", "compression", "relocation"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				comment := codeCommentResponse("reasoning", "visible")
				comment.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":[{"path":"sample.go","content":"issue","existing_code":"package main"},{"path":"sample.go","content":"second issue","existing_code":"func main() {}"}]}`
				client := &equivalenceClient{fakeClient: fakeClient{responses: []*llm.ChatResponse{
					fileReadToolCallResponse("read", `{"path":"sample.go"}`), comment, taskDoneResponse(),
				}}}
				deps := newTestDeps(client)
				deps.Session = session.New(t.TempDir(), "main", "fake", session.SessionOptions{ReviewMode: mode})
				deps.Template.MaxCompletionTokens = 321
				deps.Tools.Register(&tool.CodeCommentProvider{Collector: deps.CommentCollector})
				deps.MainToolDefs = definitions
				deps.DiffLookup = func(path string) *model.Diff {
					return &model.Diff{NewPath: path, NewFileContent: "package main\nfunc main() {}\n"}
				}
				if mode == "review" {
					deps.NewRequestMeta = metaFactory("openai", "fake")
				}
				switch scenario {
				case "failed":
					client.responses[2] = taskDoneResponseWithArguments(`{"state":"FAILED"}`)
				case "invalid":
					client.responses = append([]*llm.ChatResponse{taskDoneResponseWithArguments(`{"state":1}`)}, client.responses...)
				case "invalid_comment_shape":
					comment.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":{"path":"sample.go","content":"issue","existing_code":"package main"}} trailing text`
				case "repaired":
					serialized := `[{"path":"sample.go","content":"the name suggests "a trusted proxy exists" here","existing_code":"package main"},{"path":"sample.go","content":"this uses "untrusted data" directly","existing_code":"func main() {}"}]`
					arguments, err := json.Marshal(map[string]any{"comments": serialized})
					if err != nil {
						t.Fatal(err)
					}
					comment.Choices[0].Message.ToolCalls[0].Function.Arguments = string(arguments)
				case "async":
					deps.CommentWorkerPool = NewCommentWorkerPool(1)
				case "round_budget":
					deps.Template.MaxToolRequestTimes = 2
				case "token_budget":
					deps.MaxTokensBudget = 1
					client.responses = []*llm.ChatResponse{client.responses[0], taskDoneResponse()}
				case "compression", "relocation":
					defaults, err := template.LoadDefault()
					if err != nil {
						t.Fatal(err)
					}
					text := "The pending issue concerns sample.go."
					auxiliary := &llm.ChatResponse{
						Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &text}}},
						Usage:   &llm.UsageInfo{PromptTokens: 11, CompletionTokens: 7, CacheReadTokens: 3, CacheWriteTokens: 2},
					}
					if scenario == "compression" {
						deps.Template.MaxTokens = 100
						deps.Template.MemoryCompressionTask = defaults.MemoryCompressionTask
						deps.Tools.Register(&fakeFileReadProvider{result: strings.Repeat("source data ", 500)})
						client.responses = []*llm.ChatResponse{client.responses[0], auxiliary, taskDoneResponse()}
					} else {
						deps.Template.ReLocationTask = defaults.ReLocationTask
						comment.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":[{"path":"sample.go","content":"issue","existing_code":"missing code"}]}`
						client.responses = []*llm.ChatResponse{client.responses[0], comment, auxiliary, taskDoneResponse()}
					}
				}
				runner := NewRunner(deps)
				completed, stop, runErr := runner.RunMainTask(context.Background(), []llm.Message{
					llm.NewTextMessage("system", "Review the supplied code."),
					llm.NewTextMessage("user", "Review sample.go."),
				}, "sample.go")
				comments := runner.CollectPendingComments()
				runner.WaitBackground()
				if scenario == "repaired" {
					if runErr != nil || !completed || stop != StopNone {
						t.Fatalf("repaired execution = %v, %v, %v", completed, stop, runErr)
					}
					if len(comments) != 2 || comments[0].Content != `the name suggests "a trusted proxy exists" here` || comments[1].Content != `this uses "untrusted data" directly` {
						t.Fatalf("repaired comments lost entries or quoted content: %+v", comments)
					}
					warnings := runner.Warnings()
					if len(warnings) != 1 || warnings[0].Type != "comment_args_repaired" || warnings[0].File != "sample.go" {
						t.Fatalf("repaired execution warnings = %+v", warnings)
					}
					if failures := runner.ToolFailures(); len(failures) != 0 {
						t.Fatalf("repaired execution recorded tool failures: %+v", failures)
					}
				}
				conversationID := client.requests[0].SessionID
				for index := range client.requests {
					taskType := session.MainTask
					if len(client.requests[index].Tools) == 0 {
						if client.requests[index].SessionID != "" {
							t.Fatal("auxiliary request acquired a main conversation ID")
						}
						if scenario == "compression" {
							taskType = session.MemoryCompressionTask
						} else {
							taskType = session.ReLocationTask
						}
					} else {
						if client.requests[index].SessionID != conversationID {
							t.Fatal("conversation identity changed")
						}
						client.requests[index].SessionID = "conversation"
					}
					if client.sessionKeys[index] != llm.SessionTaskKey(deps.Session.SessionID, string(taskType), "sample.go") {
						t.Fatal("request affinity does not match its session task")
					}
					client.sessionKeys[index] = llm.SessionTaskKey("session", string(taskType), "sample.go")
				}
				records := deps.Session.GetOrCreateFileSession("sample.go").TaskRecords
				for _, tasks := range records {
					for _, record := range tasks {
						record.Duration = 0
						for index := range record.ToolResults {
							record.ToolResults[index].Duration = 0
						}
					}
				}
				errText := ""
				if runErr != nil {
					errText = runErr.Error()
				}
				snapshot := map[string]any{
					"requests": client.requests, "affinity": client.sessionKeys, "metadata": client.metadata,
					"completed": completed, "stop": stop, "error": errText,
					"comments": comments, "warnings": runner.Warnings(), "records": records,
					"tool_calls": runner.ToolCalls(), "failures": runner.ToolFailures(),
					"usage": []int64{runner.TotalInputTokens(), runner.TotalOutputTokens(), runner.TotalCacheReadTokens(), runner.TotalCacheWriteTokens()},
				}
				actual, err := json.MarshalIndent(snapshot, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				actual = append(actual, '\n')
				path := filepath.Join("testdata", "equivalence", mode+"_"+scenario+".json")
				if os.Getenv("UPDATE_EQUIVALENCE") == "1" {
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, actual, 0644); err != nil {
						t.Fatal(err)
					}
				}
				expected, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(actual) != string(expected) {
					t.Fatalf("execution differs from the pre-refactor fixture %s", path)
				}
			})
		}
	}
}
