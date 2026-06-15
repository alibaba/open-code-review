package reviewbackend

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-code-review/open-code-review/internal/tool"
)

func TestReplayCursorTextToolCalls_CollectsComments(t *testing.T) {
	collector := tool.NewCommentCollector()
	provider := &tool.CodeCommentProvider{Collector: collector}
	tracker := &cursorReviewTracker{}
	baseExec := func(_ context.Context, call ToolCallInput) ToolCallOutput {
		var args map[string]any
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			t.Fatalf("unmarshal args: %v", err)
		}
		result, err := provider.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if call.Name == "task_done" {
			return ToolCallOutput{Completed: true, Result: result}
		}
		return ToolCallOutput{Result: result}
	}
	exec := func(ctx context.Context, call ToolCallInput) ToolCallOutput {
		tracker.markTool(call.Name)
		out := baseExec(ctx, call)
		if out.Completed {
			tracker.taskDone = true
		}
		return out
	}

	text := `{"tool":"code_comment","arguments":{"path":"internal/reviewbackend/review_probe.go","start_line":12,"end_line":18,"content":"Hardcoded API key fallback"}}
{"name":"task_done","arguments":{"state":"DONE"}}`

	replayCursorTextToolCalls(context.Background(), text, "internal/reviewbackend/review_probe.go", exec, func(string, ...any) {}, false)

	if len(collector.Comments()) == 0 {
		t.Fatal("expected comments in collector")
	}
	if collector.Comments()[0].Content != "Hardcoded API key fallback" {
		t.Fatalf("unexpected content: %q", collector.Comments()[0].Content)
	}
	if !tracker.taskDone {
		t.Fatal("expected task_done via replay")
	}
}
