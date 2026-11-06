// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestRunMainTaskPersistsRequestLinks(t *testing.T) {
	setTestHome(t, t.TempDir())
	client := &scriptedLLMClient{responses: []*llm.ChatResponse{
		codeCommentResponse("", ""), taskDoneResponse(),
	}}
	runner, _ := newThinkingTestRunner(t, client)
	ok, _, err := runner.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "file.go")
	if err != nil || !ok {
		t.Fatalf("RunMainTask = %v, %v", ok, err)
	}
	sh := runner.deps.Session
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	report, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{TaskKey: "file.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Observations) != 0 {
		t.Fatalf("ordinary tool and task_done produced inconsistent evidence: %+v", report.Observations)
	}
	var requestID, responseID string
	found := false
	for _, event := range report.Tasks[0].Phases[0].Events {
		switch event.Type {
		case "llm_request":
			requestID = event.RequestID
		case "llm_response":
			responseID = event.ID
		case "tool_call":
			found = true
			if requestID == "" || responseID == "" || event.RequestID != requestID || event.ResponseID != responseID || event.ToolCallID != "call_comment" {
				t.Fatalf("runtime tool call lost its request/response identity: %+v", event)
			}
		}
	}
	if !found {
		t.Fatal("runtime tool result not persisted")
	}
}

func TestRunMainTaskCommentLocation(t *testing.T) {
	for _, mode := range []string{"sync", "async", "cross-file", "relocation", "unlocated"} {
		t.Run(mode, func(t *testing.T) {
			setTestHome(t, t.TempDir())
			response := codeCommentResponse("", "")
			response.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":[{"path":"a.go","content":"private finding","existing_code":"x"}]}`
			client := &scriptedLLMClient{responses: []*llm.ChatResponse{response, taskDoneResponse()}}
			runner, collector := newThinkingTestRunner(t, client)
			d := model.Diff{NewPath: "a.go", NewFileContent: "header\n\tx\n"}
			runner.deps.DiffLookup = func(path string) *model.Diff {
				if path == d.NewPath {
					return &d
				}
				return nil
			}
			if mode == "async" {
				runner.deps.CommentWorkerPool = NewCommentWorkerPool(1)
				defer runner.deps.CommentWorkerPool.Await()
			}
			if mode == "cross-file" {
				d.NewPath = "b.go"
				runner.deps.AllDiffs = func() []model.Diff { return []model.Diff{d} }
			}
			if mode == "unlocated" {
				d.NewFileContent = "unrelated()\n"
			}
			if mode == "relocation" {
				d.NewFileContent = "header\n\tactual()\n"
				reply := "```go\nactual()\n```"
				client.responses = []*llm.ChatResponse{response, {Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}, taskDoneResponse()}
				runner.deps.Template.ReLocationTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "relocate {existing_code} in {diff}"}}}
			}
			ok, _, err := runner.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "group-a")
			if err != nil || !ok {
				t.Fatalf("run = %v, %v", ok, err)
			}
			if runner.deps.CommentWorkerPool != nil {
				runner.deps.CommentWorkerPool.AwaitKey("group-a")
			}
			sh := runner.deps.Session
			if err := sh.Finalize(); err != nil {
				t.Fatal(err)
			}
			report, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{TaskKey: "group-a"})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Observations) != 0 {
				t.Fatalf("normal processing acquired failure semantics: %+v", report.Observations)
			}
			var locations []session.DiagnosticEvent
			for _, phase := range report.Tasks[0].Phases {
				for _, event := range phase.Events {
					if event.Type == "comment_location" {
						locations = append(locations, event)
					}
				}
			}
			comments := collector.Comments()
			if len(locations) != 1 || len(comments) != 1 {
				t.Fatalf("locations=%d comments=%d", len(locations), len(comments))
			}
			location := locations[0]
			cm := comments[0]
			if location.Location.Path != cm.Path || location.Location.StartLine != cm.StartLine || location.Location.EndLine != cm.EndLine || location.ToolCallID != "call_comment" || location.RequestID == "" || location.ResponseID == "" {
				t.Fatalf("stage did not describe the processing result: %+v", location)
			}
			if mode == "unlocated" {
				if cm.StartLine != 0 {
					t.Fatal("unlocated comment acquired coordinates")
				}
			} else if cm.StartLine != 2 {
				t.Fatal("record was written before location processing returned")
			}
			wantCalls := 2
			if mode == "relocation" {
				wantCalls = 3
			}
			if client.calls != wantCalls {
				t.Fatalf("recording added model calls: %d", client.calls)
			}
		})
	}
}
