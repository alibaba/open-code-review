// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

type recoveryAgentClient struct {
	responses   []*llm.ChatResponse
	requests    []llm.ChatRequest
	beforeFirst func() error
}

func (c *recoveryAgentClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(c.requests) == 0 && c.beforeFirst != nil {
		if err := c.beforeFirst(); err != nil {
			return nil, err
		}
	}
	c.requests = append(c.requests, req)
	if len(c.requests) > len(c.responses) {
		return nil, fmt.Errorf("unexpected request %d", len(c.requests))
	}
	return c.responses[len(c.requests)-1], nil
}

func recoveryReadResponse(id, path string) *llm.ChatResponse {
	response := agentTaskDoneResponse()
	response.Choices[0].Message.ToolCalls[0] = llm.ToolCall{
		ID: id, Type: "function",
		Function: llm.FunctionCall{Name: tool.FileRead.Name(), Arguments: fmt.Sprintf(`{"file_path":%q}`, path)},
	}
	return response
}

func recoveryHead(t *testing.T, dir string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}

func TestRunFileReadRecoveryUsesResolvedTarget(t *testing.T) {
	for _, mode := range []tool.ReviewMode{tool.ModeRange, tool.ModeCommit} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			dir := initPreviewRepo(t)
			commitIn(t, dir, "widget.tsx", "export const Widget = () => <div>reviewed</div>;\n", "add widget")
			gitIn(t, dir, "branch", "-M", "main")
			gitIn(t, dir, "checkout", "-b", "feature")
			commitIn(t, dir, "app.tsx", "import { Widget } from './widget';\nexport const App = Widget;\n", "add importer")
			head := recoveryHead(t, dir)
			gitIn(t, dir, "mv", "widget.tsx", "widget.ts")
			gitIn(t, dir, "commit", "-m", "move widget")
			laterHead := recoveryHead(t, dir)
			gitIn(t, dir, "update-ref", "refs/heads/feature", head)

			reader := &tool.FileReader{RepoDir: dir, Mode: mode, Ref: "feature"}
			registry := tool.NewRegistry()
			registry.Register(tool.NewFileRead(reader))
			registry.Register(tool.NewFileFind(reader))
			registry.Register(tool.NewCodeSearch(reader))
			client := &recoveryAgentClient{
				responses: []*llm.ChatResponse{
					recoveryReadResponse("missing", "widget.ts"),
					recoveryReadResponse("candidate", "widget.tsx"),
					agentTaskDoneResponse(),
				},
				beforeFirst: func() error {
					output, err := exec.Command("git", "-C", dir, "update-ref", "refs/heads/feature", laterHead).CombinedOutput()
					if err != nil {
						return fmt.Errorf("move review ref: %w: %s", err, output)
					}
					return nil
				},
			}
			args := Args{
				RepoDir: dir, LLMClient: client, Model: "fake", Tools: registry,
				Template: budgetAgentTestTemplate(),
				MainToolDefs: []llm.ToolDef{
					{Type: "function", Function: llm.FunctionDef{Name: "file_read"}},
					{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
				},
			}
			if mode == tool.ModeRange {
				args.From, args.To = "main", "feature"
			} else {
				args.Commit = "feature"
			}
			a := New(args)
			if _, err := a.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if reader.Ref != head {
				t.Fatalf("file reader ref = %q, want resolved target %q", reader.Ref, head)
			}
			manifest := a.RunManifest()
			if manifest == nil || manifest.TerminalState != session.StateComplete || len(manifest.Coverage.Completed) != 1 {
				t.Fatalf("manifest = %+v, want complete review of importer", manifest)
			}
			failures := a.ToolFailures()
			if len(failures) != 1 || failures[0].Arguments != `{"file_path":"widget.ts"}` {
				t.Fatalf("historical failures = %+v", failures)
			}
			encoded, err := json.Marshal(failures)
			if err != nil {
				t.Fatal(err)
			}
			var output []struct {
				Recovery *struct {
					Status string `json:"status"`
				} `json:"recovery"`
			}
			if err := json.Unmarshal(encoded, &output); err != nil {
				t.Fatal(err)
			}
			if output[0].Recovery == nil || output[0].Recovery.Status != "candidate_read" {
				t.Fatalf("failure JSON = %s, want verified candidate read", encoded)
			}
			var candidateResult string
			for _, message := range client.requests[len(client.requests)-1].Messages {
				if message.ToolCallID == "candidate" {
					candidateResult, _ = message.Content.(string)
				}
			}
			if !strings.Contains(candidateResult, "<div>reviewed</div>") {
				t.Fatalf("candidate result = %q, want content from the reviewed commit", candidateResult)
			}
			find, _ := registry.Get(tool.FileFind.Name())
			found, err := find.Execute(context.Background(), map[string]any{"query_name": "widget"})
			if err != nil || found != "widget.tsx" {
				t.Fatalf("file_find = %q, %v, want widget.tsx from shared target", found, err)
			}
			search, _ := registry.Get(tool.CodeSearch.Name())
			found, err = search.Execute(context.Background(), map[string]any{"search_text": "reviewed"})
			if err != nil || !strings.Contains(found, "widget.tsx") || strings.Contains(found, "widget.ts:") {
				t.Fatalf("code_search = %q, %v, want widget.tsx from shared target", found, err)
			}
		})
	}
}

func TestBindFileReaderTargetLeavesUnresolvedAndWorkspaceReadsAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode tool.ReviewMode
		head string
	}{
		{name: "unresolved range", mode: tool.ModeRange},
		{name: "unresolved commit", mode: tool.ModeCommit},
		{name: "workspace", mode: tool.ModeWorkspace, head: strings.Repeat("a", 40)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &tool.FileReader{Mode: tc.mode, Ref: "unchanged"}
			registry := tool.NewRegistry()
			registry.Register(tool.NewFileRead(reader))
			a := &Agent{args: Args{Tools: registry}, inputResolution: diff.InputResolution{ResolvedHead: tc.head}}
			a.bindFileReaderTarget()
			if reader.Ref != "unchanged" {
				t.Fatalf("reader ref = %q, want unchanged", reader.Ref)
			}
		})
	}
}

func TestBindFileReaderTargetOptionalProviders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		registry *tool.Registry
	}{
		{name: "no registry"},
		{name: "no file read", registry: tool.NewRegistry()},
		{name: "no reader", registry: func() *tool.Registry {
			registry := tool.NewRegistry()
			registry.Register(tool.NewFileRead(nil))
			return registry
		}()},
		{name: "custom provider", registry: func() *tool.Registry {
			registry := tool.NewRegistry()
			registry.Register(struct{ tool.Provider }{tool.NewFileRead(nil)})
			return registry
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Agent{args: Args{Tools: tc.registry}, inputResolution: diff.InputResolution{ResolvedHead: strings.Repeat("a", 40)}}
			a.bindFileReaderTarget()
		})
	}
}
