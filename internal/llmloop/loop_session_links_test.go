// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
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
