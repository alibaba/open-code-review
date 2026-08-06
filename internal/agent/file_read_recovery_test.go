package agent

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
	"github.com/alibaba/open-code-review/internal/tool"
)

type fileReadRecoveryClient struct {
	calls            int
	sawRecoveryError bool
}

func (c *fileReadRecoveryClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	if c.calls == 2 {
		for _, message := range req.Messages {
			if text, ok := message.Content.(string); ok && strings.Contains(text, "file not found:") {
				c.sawRecoveryError = true
			}
		}
	}

	switch c.calls {
	case 1:
		return recoveryToolResponse("call_read_wrong", "file_read", `{"file_path":"docs/adr/0103-always-show-upload-upgrade-prompt"}`), nil
	case 2:
		return recoveryToolResponse("call_find", "file_find", `{"query_name":"0103-always-show-upload-upgrade-prompt"}`), nil
	case 3:
		return recoveryToolResponse("call_read_correct", "file_read", `{"file_path":"docs/adr/0103-always-show-upload-upgrade-prompt.md"}`), nil
	default:
		return recoveryToolResponse("call_done", "task_done", `{"state":"DONE"}`), nil
	}
}

func recoveryToolResponse(id, name, args string) *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &content,
			ToolCalls: []llm.ToolCall{{
				ID:       id,
				Type:     "function",
				Function: llm.FunctionCall{Name: name, Arguments: args},
			}},
		}}},
		Model: "fake",
	}
}

func TestDispatchSubtasks_FileReadErrorCanRecover(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	filePath := "docs/adr/0103-always-show-upload-upgrade-prompt.md"
	absPath := filepath.Join(repoDir, filepath.FromSlash(filePath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte("# upload prompt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	client := &fileReadRecoveryClient{}
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeWorkspace,
		Operation:  session.OperationReview,
	})
	fr := &tool.FileReader{RepoDir: repoDir, Mode: tool.ModeWorkspace}
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileRead(fr))
	reg.Register(tool.NewFileFind(fr))

	a := New(Args{
		RepoDir:    repoDir,
		ReviewMode: session.ReviewModeWorkspace,
		LLMClient:  client,
		Model:      "fake",
		Session:    sh,
		Tools:      reg,
		Template: template.Template{
			MaxTokens:           100000,
			MaxToolRequestTimes: 5,
			MainTask: template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "Review {{current_file_path}} {{diff}}"}},
			},
		},
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "file_read", Description: "read a file"}},
			{Type: "function", Function: llm.FunctionDef{Name: "file_find", Description: "find a file"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "finish the review"}},
		},
	})
	a.diffs = []model.Diff{{OldPath: filePath, NewPath: filePath, Diff: "+upload prompt", Insertions: 1}}

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	manifest := finishManifestFlow(t, a)
	if manifest.TerminalState != session.StateComplete || len(manifest.Coverage.Completed) != 1 || len(manifest.Coverage.Failed) != 0 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !client.sawRecoveryError {
		t.Fatal("LLM did not receive the file_read recovery error")
	}

	var sawFailedRead bool
	for _, record := range sh.GetOrCreateFileSession(filePath).TaskRecords[session.MainTask] {
		for _, result := range record.ToolResults {
			if result.ToolName == "file_read" && strings.Contains(result.Result, "file not found:") {
				sawFailedRead = true
			}
		}
	}
	if !sawFailedRead {
		t.Fatal("initial failed file_read call was not retained in session history")
	}

	var sessionFile string
	sessionRoot := filepath.Join(os.Getenv("HOME"), ".opencodereview", "test-sessions")
	if err := filepath.WalkDir(sessionRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == sh.SessionID+".jsonl" {
			sessionFile = path
		}
		return nil
	}); err != nil {
		t.Fatalf("find persisted session: %v", err)
	}
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("read persisted session: %v", err)
	}
	if !strings.Contains(string(data), `"tool_name":"file_read"`) || !strings.Contains(string(data), `"ok":false`) {
		t.Fatal("initial failed file_read call was not persisted as a failed tool call")
	}
}
