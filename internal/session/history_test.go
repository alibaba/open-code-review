package session

import (
	"errors"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/llm"
)

func TestNew(t *testing.T) {
	sh := New("/tmp/repo", "main", "gpt-4", SessionOptions{
		ReviewMode: ReviewModeWorkspace,
		DiffFrom:   "a",
		DiffTo:     "b",
		DiffCommit: "c",
	})
	if sh == nil {
		t.Fatal("New returned nil")
	}
	if sh.SessionID == "" {
		t.Error("SessionID should not be empty")
	}
	if sh.RepoDir != "/tmp/repo" {
		t.Errorf("RepoDir = %q", sh.RepoDir)
	}
	if sh.GitBranch != "main" {
		t.Errorf("GitBranch = %q", sh.GitBranch)
	}
	if sh.Model != "gpt-4" {
		t.Errorf("Model = %q", sh.Model)
	}
	if sh.ReviewMode != ReviewModeWorkspace {
		t.Errorf("ReviewMode = %q", sh.ReviewMode)
	}
	if sh.DiffFrom != "a" || sh.DiffTo != "b" || sh.DiffCommit != "c" {
		t.Errorf("Diff fields mismatch")
	}
	if sh.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
	if sh.FileSessions == nil {
		t.Error("FileSessions map should be initialized")
	}
}

func TestGetOrCreateFileSession(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})

	fs1 := sh.GetOrCreateFileSession("main.go")
	if fs1 == nil {
		t.Fatal("nil FileSession")
	}
	if fs1.FilePath != "main.go" {
		t.Errorf("FilePath = %q", fs1.FilePath)
	}

	fs2 := sh.GetOrCreateFileSession("main.go")
	if fs1 != fs2 {
		t.Error("expected same FileSession instance on second call")
	}

	fs3 := sh.GetOrCreateFileSession("other.go")
	if fs3 == fs1 {
		t.Error("different paths should yield different sessions")
	}
}

func TestAppendTaskRecord(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")

	msgs := []llm.Message{llm.NewTextMessage("user", "hello")}
	rec := fs.AppendTaskRecord(MainTask, msgs)
	if rec == nil {
		t.Fatal("nil TaskRecord")
	}
	if rec.Type != MainTask {
		t.Errorf("Type = %v", rec.Type)
	}
	if rec.RequestNo != 1 {
		t.Errorf("RequestNo = %d, want 1", rec.RequestNo)
	}

	rec2 := fs.AppendTaskRecord(MainTask, msgs)
	if rec2.RequestNo != 2 {
		t.Errorf("second RequestNo = %d, want 2", rec2.RequestNo)
	}

	rec3 := fs.AppendTaskRecord(PlanTask, msgs)
	if rec3.RequestNo != 1 {
		t.Errorf("PlanTask RequestNo = %d, want 1 (separate counter)", rec3.RequestNo)
	}
}

func TestAppendTaskRecord_DefensiveCopy(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")

	msgs := []llm.Message{llm.NewTextMessage("user", "original")}
	rec := fs.AppendTaskRecord(MainTask, msgs)
	msgs[0] = llm.NewTextMessage("user", "mutated")

	if rec.RequestMessages[0].ExtractText() == "mutated" {
		t.Error("AppendTaskRecord should store a copy of messages")
	}
}

func TestSetResponse(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	content := "response text"
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
			},
		}},
		Model: "gpt-4",
		Usage: &llm.UsageInfo{
			PromptTokens:     100,
			CompletionTokens: 50,
		},
	}

	rec.SetResponse(resp, 2*time.Second)

	if rec.Response == nil {
		t.Fatal("Response should be set")
	}
	if rec.Response.Content != "response text" {
		t.Errorf("Content = %q", rec.Response.Content)
	}
	if rec.Response.Model != "gpt-4" {
		t.Errorf("Model = %q", rec.Response.Model)
	}
	if rec.Response.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d", rec.Response.Usage.PromptTokens)
	}
	if rec.Response.Usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d", rec.Response.Usage.CompletionTokens)
	}
	if rec.Duration != 2*time.Second {
		t.Errorf("Duration = %v", rec.Duration)
	}
}

func TestSetResponse_EmptyResponse(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	rec.SetResponse(nil, time.Second)
	if rec.Error == "" {
		t.Error("expected error for nil response")
	}
}

func TestSetError(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	rec.SetError(errors.New("timeout"), 5*time.Second)

	if rec.Error != "timeout" {
		t.Errorf("Error = %q, want %q", rec.Error, "timeout")
	}
	if rec.Duration != 5*time.Second {
		t.Errorf("Duration = %v", rec.Duration)
	}
}

func TestLLMFailures(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	if sh.LLMFailures() != 0 {
		t.Errorf("initial failures = %d", sh.LLMFailures())
	}

	fs := sh.GetOrCreateFileSession("a.go")
	rec := fs.AppendTaskRecord(MainTask, nil)
	rec.SetError(errors.New("fail1"), time.Second)

	rec2 := fs.AppendTaskRecord(MainTask, nil)
	rec2.SetError(errors.New("fail2"), time.Second)

	if sh.LLMFailures() != 2 {
		t.Errorf("failures = %d, want 2", sh.LLMFailures())
	}
}

func TestAddToolResult(t *testing.T) {
	sh := New("/tmp/repo", "main", "model", SessionOptions{})
	fs := sh.GetOrCreateFileSession("file.go")
	rec := fs.AppendTaskRecord(MainTask, nil)

	rec.AddToolResult("file_read", `{"path":"main.go"}`, "package main")

	if len(rec.ToolResults) != 1 {
		t.Fatalf("len = %d", len(rec.ToolResults))
	}
	tr := rec.ToolResults[0]
	if tr.ToolName != "file_read" {
		t.Errorf("ToolName = %q", tr.ToolName)
	}
	if tr.Arguments != `{"path":"main.go"}` {
		t.Errorf("Arguments = %q", tr.Arguments)
	}
	if tr.Result != "package main" {
		t.Errorf("Result = %q", tr.Result)
	}
}
