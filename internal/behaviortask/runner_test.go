// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

type fakeClient func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)

func (client fakeClient) CompletionsWithCtx(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	return client(ctx, request)
}

func response(name, arguments string) *llm.ChatResponse {
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{ToolCalls: []llm.ToolCall{{ID: name, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: arguments}}}}}}, Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}
}

func git(test *testing.T, directory string, args ...string) string {
	test.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com", "LC_ALL=C")
	data, err := command.CombinedOutput()
	if err != nil {
		test.Fatalf("git %v: %v: %s", args, err, data)
	}
	return strings.TrimSpace(string(data))
}

func fixture(test *testing.T) Input {
	test.Helper()
	home := test.TempDir()
	test.Setenv("HOME", home)
	test.Setenv("USERPROFILE", home)
	repo := test.TempDir()
	git(test, repo, "init", "-q")
	write := func(name, content string) {
		test.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
			test.Fatal(err)
		}
	}
	write("main.go", "old value\nunchanged\n")
	write("deleted.go", "deleted workflow\n")
	write("consumer.go", "old value\nconsumer\n")
	write(".env", "SECRET old value\n")
	write("excluded.go", "EXCLUDED old value\n")
	git(test, repo, "add", ".")
	git(test, repo, "commit", "-qm", "Before")
	base := git(test, repo, "rev-parse", "HEAD")
	write("main.go", "new value\nunchanged\n")
	if err := os.Remove(filepath.Join(repo, "deleted.go")); err != nil {
		test.Fatal(err)
	}
	git(test, repo, "mv", "consumer.go", "renamed.go")
	git(test, repo, "add", ".")
	git(test, repo, "commit", "-qm", "After")
	head := git(test, repo, "rev-parse", "HEAD")
	diffText := git(test, repo, "diff", "--no-ext-diff", "--no-textconv", base, head)
	changes, err := diff.ParseDiffText(context.Background(), diffText, repo, head, nil)
	if err != nil {
		test.Fatal(err)
	}
	return Input{RepoDir: repo, Mode: "range", Resolution: diff.InputResolution{ResolvedBase: base, ResolvedHead: head}, Diffs: changes, ExcludePatterns: []string{"excluded.go"}, Contexts: []Context{{ID: "goal", Kind: "background", Source: "user", Version: "1", Content: "Explain the change; {{not_a_template}} is data."}}}
}

func newTestRunner(test *testing.T, input Input, client fakeClient, configure func(*Deps)) *Runner {
	test.Helper()
	tpl, err := template.LoadBehaviorDefault()
	if err != nil {
		test.Fatal(err)
	}
	tpl.MaxToolRequestTimes = 10
	deps := Deps{LLMClient: client, Model: "fixed-test-model", Template: tpl, ArtifactDir: filepath.Join(test.TempDir(), "artifacts")}
	if configure != nil {
		configure(&deps)
	}
	runner, err := NewRunner(context.Background(), deps, input)
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = runner.history.Finalize() })
	return runner
}

func marshal(test *testing.T, value any) string {
	test.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		test.Fatal(err)
	}
	return string(data)
}

func validAnalysis(input Input) Analysis {
	return Analysis{Markdown: fmt.Sprintf("## Behavior\nOld value at %s [[obs-000001]]; new value at %s [[obs-000002]].\n\n## Impact\nThe consumer reads a different value.\n\n## Maintainer Decisions\nNo policy choice identified.\n\n## Verification\nNo tests executed.\n", input.Resolution.ResolvedBase, input.Resolution.ResolvedHead), DeclaredCompletion: "complete"}
}

func observePair(test *testing.T, runner *Runner) {
	test.Helper()
	for _, side := range []string{"old", "new"} {
		outcome, err := runner.dispatch(context.Background(), "file_read", fmt.Sprintf(`{"side":%q,"file_path":"main.go","start_line":1,"end_line":2}`, side))
		if err != nil || !strings.Contains(outcome.Content, "obs-") {
			test.Fatalf("read: %s, %v", outcome.Content, err)
		}
	}
}

func TestRunnerSubmissionRepairAndArtifacts(test *testing.T) {
	input := fixture(test)
	requests := 0
	var requestKey string
	client := fakeClient(func(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests++
		if len(request.Tools) != 6 {
			test.Fatal("wrong tool set")
		}
		if requests == 1 {
			if len(request.Messages) != 2 || !strings.Contains(request.Messages[1].ExtractText(), input.Resolution.ResolvedHead) || !strings.Contains(request.Messages[1].ExtractText(), "{{not_a_template}}") {
				test.Fatal("unresolved or missing input")
			}
			return response("file_read", `{"file_path":"main.go","side":"old"}`), nil
		}
		if requests == 2 {
			return response("file_read", `{"file_path":"main.go"}`), nil
		}
		if requests == 3 {
			return response("submit_analysis", `{}`), nil
		}
		if !strings.Contains(request.Messages[len(request.Messages)-1].ExtractText(), "Error:") {
			test.Fatal("validation failure was not repairable")
		}
		return response("submit_analysis", marshal(test, validAnalysis(input))), nil
	})
	runner := newTestRunner(test, input, client, func(deps *Deps) {
		deps.NewRequestMeta = func(key string, _ session.TaskType, _ int) llm.RequestMeta {
			requestKey = key
			return llm.RequestMeta{}
		}
	})
	result, err := runner.Run(context.Background())
	if err != nil || !result.Completed || result.StopReason != "none" || result.SubmissionStatus != "accepted" || result.Analysis == nil {
		test.Fatalf("Run: %+v, %v", result, err)
	}
	if result.Usage.InputTokens != 40 || result.Usage.OutputTokens != 20 || len(result.ObservationIDs) != 2 || requestKey != result.Comparison.ID {
		test.Fatalf("run facts: %+v", result)
	}
	if len(result.ToolFailures) != 1 {
		test.Fatalf("failures: %v", result.ToolFailures)
	}
	for _, name := range []string{"result.json", "report.md", "observations.jsonl"} {
		if data, err := os.ReadFile(filepath.Join(runner.deps.ArtifactDir, name)); err != nil || len(data) == 0 {
			test.Fatalf("artifact %s: %v", name, err)
		}
	}
	if _, err := runner.Run(context.Background()); err == nil {
		test.Fatal("runner reused")
	}
	if _, err := runner.dispatch(context.Background(), "submit_analysis", marshal(test, validAnalysis(input))); err == nil {
		test.Fatal("submission overwritten")
	}
	if _, err := runner.dispatch(context.Background(), "file_read", `{"file_path":"main.go"}`); err == nil {
		test.Fatal("read after freeze")
	}
}

func TestBudgetsCancellationAndProviderFailure(test *testing.T) {
	input := fixture(test)
	for _, mode := range []string{"rounds", "tokens", "invalid_grace", "canceled", "provider"} {
		test.Run(mode, func(test *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			client := fakeClient(func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
				requests++
				if mode == "provider" {
					return nil, errors.New("provider failed")
				}
				if mode == "canceled" {
					cancel()
					return nil, context.Canceled
				}
				if requests == 1 {
					return response("context_read", `{"kind":"issue","id":"missing"}`), nil
				}
				if len(request.Tools) != 1 || request.Tools[0].Function.Name != "submit_analysis" {
					test.Fatal("grace advertised investigation tools")
				}
				if mode == "invalid_grace" {
					return response("submit_analysis", `{}`), nil
				}
				analysis := Analysis{Markdown: "Budget exhausted before evidence was read; no tests executed.", DeclaredCompletion: "partial"}
				return response("submit_analysis", marshal(test, analysis)), nil
			})
			runner := newTestRunner(test, input, client, func(deps *Deps) {
				deps.Template.MaxToolRequestTimes = 1
				if mode == "tokens" {
					deps.Template.MaxToolRequestTimes = 10
					deps.MaxTokensBudget = 1
				}
			})
			result, err := runner.Run(ctx)
			if mode == "provider" || mode == "canceled" {
				if err == nil || result.Completed || len(result.Errors) == 0 || requests != 1 {
					test.Fatalf("failure lost: %+v, %v", result, err)
				}
				return
			}
			wantStop := "max_rounds"
			if mode == "tokens" {
				wantStop = "token_budget"
			}
			if err != nil || result.StopReason != wantStop || result.Completed != (mode != "invalid_grace") || requests != 2 || len(result.Warnings) == 0 {
				test.Fatalf("budget result: %+v, %v", result, err)
			}
		})
	}
}

func TestLegacyToolsCannotCompleteBehavior(test *testing.T) {
	input := fixture(test)
	requests := 0
	runner := newTestRunner(test, input, func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		requests++
		if requests == 1 {
			return response("task_done", `{"state":"DONE"}`), nil
		}
		rejected := false
		for _, message := range request.Messages {
			if message.Role == "tool" && strings.Contains(message.ExtractText(), "not available") {
				rejected = true
			}
		}
		if !rejected {
			test.Fatal("missing request-level rejection")
		}
		return response("file_read", `{"file_path":".env"}`), nil
	}, func(deps *Deps) { deps.Template.MaxToolRequestTimes = 1 })
	result, err := runner.Run(context.Background())
	if err != nil || result.Completed || len(result.ObservationIDs) != 0 {
		test.Fatalf("unauthorized tool executed: %+v, %v", result, err)
	}
}

func TestRootAndMissingComparison(test *testing.T) {
	input := fixture(test)
	root := input.Resolution.ResolvedBase
	input.Mode = "commit"
	input.Resolution = diff.InputResolution{ResolvedHead: root, BaseIsEmpty: true}
	input.Diffs = []model.Diff{{OldPath: "main.go", NewPath: "main.go", IsNew: true, Diff: "@@ -0,0 +1,1 @@\n+old value\n"}}
	code, err := prepareCodeTools(context.Background(), input)
	if err != nil {
		test.Fatal(err)
	}
	observation, err := code.execute(context.Background(), "code_search", map[string]any{"search_text": "value", "side": "old"})
	if err != nil || !strings.Contains(observation.ReturnedContent, "No matches") {
		test.Fatalf("empty tree: %+v, %v", observation, err)
	}
	rootRunner := newTestRunner(test, input, nil, nil)
	if _, err := rootRunner.dispatch(context.Background(), "file_read_diff", `{"path_array":["main.go"]}`); err != nil {
		test.Fatal(err)
	}
	analysis := validAnalysis(input)
	analysis.Markdown = "The old side is the empty tree; the new source is in [[obs-000001]]. No execution evidence.\n"
	if _, err := rootRunner.dispatch(context.Background(), "submit_analysis", marshal(test, analysis)); err != nil {
		test.Fatal(err)
	}
	input.Resolution.BaseIsEmpty = false
	if _, err := prepareCodeTools(context.Background(), input); err == nil {
		test.Fatal("missing parent became empty tree")
	}
	input.Resolution = diff.InputResolution{ResolvedHead: strings.Repeat("f", 40), BaseIsEmpty: true}
	if _, err := prepareCodeTools(context.Background(), input); err == nil {
		test.Fatal("missing revision accepted")
	}
	input.Resolution = diff.InputResolution{ResolvedHead: git(test, input.RepoDir, "rev-parse", "HEAD"), BaseIsEmpty: true}
	if _, err := prepareCodeTools(context.Background(), input); err == nil {
		test.Fatal("nonroot accepted as empty tree")
	}
}
