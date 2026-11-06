// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestRunMainTaskCommentEvidenceDuplicateAnchor(t *testing.T) {
	setTestHome(t, t.TempDir())
	const snapshot = "if timeout {\n  ev.ErrorClass = ArgosRuntimeStatusTimeout\n}\nif analyze {\n  ev.ErrorClass = ArgosRuntimeStatusTimeout\n}\n"
	response := codeCommentResponse("", "")
	response.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":[{"path":"a.go","content":"Analyze marks a non-timeout error as timeout","existing_code":"ev.ErrorClass = ArgosRuntimeStatusTimeout"}]}`
	client := &scriptedLLMClient{responses: []*llm.ChatResponse{response, taskDoneResponse()}}
	runner, collector := newThinkingTestRunner(t, client)
	d := model.Diff{NewPath: "a.go", NewFileContent: snapshot}
	runner.deps.DiffLookup = func(string) *model.Diff { return &d }
	ok, _, err := runner.RunMainTask(context.Background(), []llm.Message{msg("user", snapshot)}, "a.go")
	if err != nil || !ok {
		t.Fatalf("run = %v, %v", ok, err)
	}
	comments := collector.Comments()
	if len(comments) != 1 {
		t.Fatal("missing review comment")
	}
	sh := runner.deps.Session
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp", comments)
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	report, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{CommentID: comments[0].EvidenceID})
	if err != nil {
		t.Fatal(err)
	}
	cm := report.Comment
	// This fixture preserves the resolver's existing first-match behavior.
	// Text matching is not a verdict that the model's intended branch is correct.
	if cm.OriginalComment.StartLine != 0 || cm.Comment.StartLine != 2 || !strings.Contains(cm.OriginalComment.Content, "Analyze") || cm.Code.Status != "matched" || cm.RequestID == "" || cm.ToolCallID != "call_comment" {
		t.Fatalf("cannot distinguish original claim from actual location: %+v", cm)
	}
	request, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{RequestID: cm.RequestID})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range request.Tasks[0].Phases[0].Events {
		if event.Type == "llm_request" {
			raw, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{EventLine: event.Line})
			if err != nil || !strings.Contains(string(raw.Event.Record), "if analyze") {
				t.Fatal("original reviewed context cannot be reached from the comment")
			}
			return
		}
	}
	t.Fatal("comment has no inspectable source request")
}

func TestRunMainTaskCommentEvidence(t *testing.T) {
	for _, mode := range []string{"sync", "async", "cross-file", "llm-relocation"} {
		t.Run(mode, func(t *testing.T) {
			setTestHome(t, t.TempDir())
			response := codeCommentResponse("", "")
			response.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":[{"path":"a.go","content":"check x","existing_code":"x","start_line":99,"end_line":99}]}`
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
			if mode == "llm-relocation" {
				response.Choices[0].Message.ToolCalls[0].Function.Arguments = `{"comments":[{"path":"a.go","content":"check x","existing_code":"invented()"}]}`
				reply := "```go\nx\n```"
				client.responses = []*llm.ChatResponse{response, {Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &reply}}}}, taskDoneResponse()}
				runner.deps.Template.ReLocationTask = &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "relocate {existing_code} in {diff}"}}}
			}
			ok, _, err := runner.RunMainTask(context.Background(), []llm.Message{msg("user", "review")}, "group-a")
			if err != nil || !ok {
				t.Fatalf("RunMainTask = %v, %v", ok, err)
			}
			if runner.deps.CommentWorkerPool != nil {
				runner.deps.CommentWorkerPool.AwaitKey("group-a")
			}
			comments := collector.Comments()
			if len(comments) != 1 || comments[0].EvidenceID == "" || comments[0].Path != d.NewPath || comments[0].StartLine != 2 {
				t.Fatalf("collected comment lost evidence or resolution: %+v", comments)
			}
			sh := runner.deps.Session
			sh.RecordReviewItemDone(d.NewPath, d.NewPath, d.NewPath, "fingerprint", comments)
			if err := sh.Finalize(); err != nil {
				t.Fatal(err)
			}
			// Later source changes must not change the persisted reviewed snapshot.
			if err := os.WriteFile(filepath.Join(sh.RepoDir, d.NewPath), []byte("changed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			d.NewFileContent = "changed in memory\n"
			report, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{CommentID: comments[0].EvidenceID})
			if err != nil {
				t.Fatal(err)
			}
			cm := report.Comment
			if len(report.Observations) != 0 || cm.TaskKey != "group-a" || cm.RequestID == "" || cm.ResponseID == "" || cm.ToolCallID != "call_comment" || cm.CommentIndex != 0 || cm.EvidenceStatus != "matched" || len(cm.Checkpoints) != 1 {
				t.Fatalf("bad runtime provenance: %+v, %+v", cm, report.Observations)
			}
			if cm.OriginalComment.Path != "a.go" || cm.Code.Spans[0].Excerpt != "\tx" || cm.Code.SnapshotSHA256 == "" {
				t.Fatalf("original or reviewed snapshot changed: %+v", cm)
			}
			if mode == "llm-relocation" {
				if cm.OriginalComment.ExistingCode != "invented()" || cm.Comment.ExistingCode != "x" || cm.RelocationRequestID == "" {
					t.Fatalf("re-location erased original evidence: %+v", cm)
				}
				relocation, err := session.Diagnose(sh.RepoDir, sh.SessionID, session.DiagnoseOptions{RequestID: cm.RelocationRequestID})
				if err != nil || relocation.Tasks[0].Phases[0].Type != string(session.ReLocationTask) {
					t.Fatalf("re-location target invalid: %+v, %v", relocation, err)
				}
			}
			loaded, err := session.LoadComments(sh.RepoDir, sh.SessionID)
			if err != nil || len(loaded) != 1 || loaded[0].EvidenceID != cm.ID {
				t.Fatalf("checkpoint replay lost evidence ID: %+v, %v", loaded, err)
			}
		})
	}
}
