// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func proseAssistant(content string) llm.Message {
	return llm.NewToolCallMessage(content, nil, llm.NativeTurn{}, "")
}

func toolCallAssistant() llm.Message {
	return llm.NewToolCallMessage("", []llm.ToolCall{{
		ID:       "call_1",
		Type:     "function",
		Function: llm.FunctionCall{Name: "task_done", Arguments: `{}`},
	}}, llm.NativeTurn{}, "")
}

func toolResult() llm.Message {
	return llm.NewToolResultMessage("call_1", "Task completed successfully.")
}

// TestStripFailedToolRounds_FirstRoundPassthrough covers requests with no
// failed retry rounds: they must pass through untouched.
func TestStripFailedToolRounds_FirstRoundPassthrough(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage("system", "you are a reviewer"),
		llm.NewTextMessage("user", "review this file"),
	}
	got := stripFailedToolRounds(messages)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (untouched)", len(got))
	}
}

// TestStripFailedToolRounds_KeepsSuccessfulToolRounds covers a history where
// tool calls were made: those rounds must survive the strip.
func TestStripFailedToolRounds_KeepsSuccessfulToolRounds(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
		toolCallAssistant(),
		toolResult(),
	}
	got := stripFailedToolRounds(messages)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (successful round kept)", len(got))
	}
}

// TestStripFailedToolRounds_StripsTrailingFailedPairs covers one and several
// trailing [prose assistant, retry hint] pairs.
func TestStripFailedToolRounds_StripsTrailingFailedPairs(t *testing.T) {
	base := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
	}
	hint := llm.NewTextMessage("user", toolRetryNudgePrefix+". Please try again or use task_done if finished.")
	history := append(append([]llm.Message{}, base...),
		proseAssistant("first prose"), hint,
		proseAssistant("second prose"), hint,
	)
	got := stripFailedToolRounds(history)
	if len(got) != len(base) {
		t.Fatalf("len = %d, want %d; got %+v", len(got), len(base), got)
	}
}

// TestStripFailedToolRounds_StripsReanchorHints covers histories containing
// the re-anchor message: it carries the same prefix, so it strips too.
func TestStripFailedToolRounds_StripsReanchorHints(t *testing.T) {
	base := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
	}
	history := append(append([]llm.Message{}, base...),
		proseAssistant("prose"), llm.NewTextMessage("user", toolRetryReanchor),
	)
	got := stripFailedToolRounds(history)
	if len(got) != len(base) {
		t.Fatalf("len = %d, want %d (re-anchor hint stripped)", len(got), len(base))
	}
}

// TestStripFailedToolRounds_KeepsSuccessfulBetweenFailed covers failed rounds
// before and after a successful one: only the trailing failed pair is removed.
func TestStripFailedToolRounds_KeepsSuccessfulBetweenFailed(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
		proseAssistant("early prose"),
		llm.NewTextMessage("user", toolRetryNudgePrefix+". Please try again."),
		toolCallAssistant(),
		toolResult(),
		proseAssistant("late prose"),
		llm.NewTextMessage("user", toolRetryNudgePrefix+". Please try again."),
	}
	got := stripFailedToolRounds(messages)
	if len(got) != 6 {
		t.Fatalf("len = %d, want 6 (early pair kept, trailing pair stripped)", len(got))
	}
	if text := got[2].ExtractText(); text != "early prose" {
		t.Errorf("got[2] = %q, want early prose", text)
	}
}

// TestStripFailedToolRounds_StopsAtRealUserMessage covers a trailing user
// message that is not a retry hint: nothing may be stripped.
func TestStripFailedToolRounds_StopsAtRealUserMessage(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
		proseAssistant("prose"),
		llm.NewTextMessage("user", "a genuine follow-up question"),
	}
	got := stripFailedToolRounds(messages)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (non-hint user message stops the strip)", len(got))
	}
}

// TestStripFailedToolRounds_KeepsAssistantWithToolCalls covers an assistant
// turn that did call tools: it is not a failed round even when followed by a
// retry hint.
func TestStripFailedToolRounds_KeepsAssistantWithToolCalls(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
		toolCallAssistant(),
		llm.NewTextMessage("user", toolRetryNudgePrefix+". Please try again."),
	}
	got := stripFailedToolRounds(messages)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (assistant with tool calls is not failed)", len(got))
	}
}

// TestStripFailedToolRounds_KeepsOpeningMessages covers the floor: the failed
// pair is stripped but the opening user message always survives.
func TestStripFailedToolRounds_KeepsOpeningMessages(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage("user", "review"),
		proseAssistant("prose"),
		llm.NewTextMessage("user", toolRetryNudgePrefix+". Please try again."),
	}
	got := stripFailedToolRounds(messages)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (failed pair stripped, opening message kept)", len(got))
	}
	if text := got[0].ExtractText(); text != "review" {
		t.Errorf("got[0] = %q, want the opening user message", text)
	}
}

// historyRecordingClient drives a scripted inner client and snapshots the
// message history of every request: the loop reuses one backing array across
// retry rounds, so later appends would otherwise rewrite an earlier request's
// retained slice.
type historyRecordingClient struct {
	inner    *fakeClient
	snapshot [][]llm.Message
}

func (c *historyRecordingClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.snapshot = append(c.snapshot, append([]llm.Message(nil), req.Messages...))
	return c.inner.CompletionsWithCtx(ctx, req)
}

// TestRunMainTask_FailedRoundsDoNotAccumulate runs the real retry branch: two
// prose rounds followed by task_done. Every request must carry at most one
// failed pair and end with the re-anchor instruction.
func TestRunMainTask_FailedRoundsDoNotAccumulate(t *testing.T) {
	prose1 := "I reviewed the file and found no issues. task_done(state='DONE')"
	prose2 := "Looking at the diff now."
	inner := &fakeClient{responses: []*llm.ChatResponse{
		proseChatResponse(prose1),
		proseChatResponse(prose2),
		taskDoneResponse(),
	}}
	client := &historyRecordingClient{inner: inner}
	runner := NewRunner(newTestDeps(client))

	completed, _, err := runner.RunMainTask(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review this file")},
		"main.go",
	)
	if err != nil {
		t.Fatalf("RunMainTask: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done to complete RunMainTask")
	}
	if inner.calls != 3 {
		t.Fatalf("LLM calls = %d, want 3", inner.calls)
	}
	if len(client.snapshot) != 3 {
		t.Fatalf("recorded requests = %d, want 3", len(client.snapshot))
	}

	// Round 2 request: the first prose round replayed once, re-anchored.
	second := client.snapshot[1]
	if len(second) != 3 {
		t.Fatalf("round-2 history len = %d, want 3: %+v", len(second), second)
	}
	if text := second[1].ExtractText(); text != prose1 {
		t.Errorf("round-2 assistant = %q, want the first prose", text)
	}
	if text := second[2].ExtractText(); !strings.HasPrefix(text, toolRetryNudgePrefix) || !strings.Contains(text, "Plain text answers are discarded") {
		t.Errorf("round-2 hint = %q, want the re-anchor instruction", text)
	}

	// Round 3 request: the previous failed pair is stripped, not accumulated;
	// only the newest prose remains, so no repeating pattern is visible.
	third := client.snapshot[2]
	if len(third) != 3 {
		t.Fatalf("round-3 history len = %d, want 3 (earlier failed pair stripped): %+v", len(third), third)
	}
	if text := third[1].ExtractText(); text != prose2 {
		t.Errorf("round-3 assistant = %q, want only the newest prose", text)
	}
	for _, m := range third {
		if strings.Contains(m.ExtractText(), prose1) {
			t.Error("round-3 history still contains the first failed prose")
		}
	}
}

func proseChatResponse(content string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &content}}},
		Model:   "fake",
	}
}
