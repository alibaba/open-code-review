package reviewbackend

import (
	"context"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
	"github.com/remdev/cursor-go-sdk/cursor"
)

func TestMessagesToPrompt(t *testing.T) {
	prompt := messagesToPrompt([]llm.Message{
		llm.NewTextMessage("system", "You are a reviewer."),
		llm.NewTextMessage("user", "Review this file."),
		llm.NewTextMessage("assistant", "Checking..."),
	})

	if !strings.Contains(prompt, "System:\nYou are a reviewer.") {
		t.Errorf("missing system block: %q", prompt)
	}
	if !strings.Contains(prompt, "User:\nReview this file.") {
		t.Errorf("missing user block: %q", prompt)
	}
	if !strings.Contains(prompt, "Assistant:\nChecking...") {
		t.Errorf("missing assistant block: %q", prompt)
	}
}

func TestToolDefsToCustomTools_Executor(t *testing.T) {
	var gotName, gotArgs string
	tools := toolDefsToCustomTools(context.Background(), []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "code_comment",
			Description: "Leave a comment",
			Parameters:  map[string]any{"type": "object"},
		},
	}}, func(_ context.Context, call ToolCallInput) ToolCallOutput {
		gotName = call.Name
		gotArgs = call.Arguments
		return ToolCallOutput{Result: `{"ok":true}`}
	})

	tool, ok := tools["code_comment"]
	if !ok {
		t.Fatal("code_comment tool not mapped")
	}
	if tool.Description != "Leave a comment" {
		t.Errorf("Description = %q", tool.Description)
	}

	out, err := tool.Execute(map[string]any{"line": float64(10)}, cursor.CustomToolContext{ToolCallID: "tc-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotName != "code_comment" {
		t.Errorf("executor name = %q", gotName)
	}
	if !strings.Contains(gotArgs, `"line"`) {
		t.Errorf("executor args = %q", gotArgs)
	}
	if out != `{"ok":true}` {
		t.Errorf("result = %v, want json payload", out)
	}
}

func TestToolDefsToCustomTools_TaskDone(t *testing.T) {
	tools := toolDefsToCustomTools(context.Background(), []llm.ToolDef{{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "task_done",
			Description: "Finish review",
		},
	}}, func(_ context.Context, _ ToolCallInput) ToolCallOutput {
		return ToolCallOutput{Completed: true}
	})

	out, err := tools["task_done"].Execute(nil, cursor.CustomToolContext{ToolCallID: "done-1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "Task completed successfully." {
		t.Errorf("result = %v", out)
	}
}

func TestWrapCursorError_AgentError(t *testing.T) {
	err := wrapCursorError(&cursor.AgentError{Code: "auth", Message: "invalid key"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "auth") || !strings.Contains(msg, "invalid key") {
		t.Errorf("unexpected message: %s", msg)
	}
	if !strings.Contains(msg, bridgeSetupHint) {
		t.Errorf("missing bridge hint: %s", msg)
	}
}

func TestNewCursorAgentBackend_MissingAPIKey(t *testing.T) {
	_, err := NewCursorAgentBackend(context.Background(), CursorConfig{Model: "auto"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing api key")
	}
}
