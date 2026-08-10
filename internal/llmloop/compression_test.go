// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
)

type countingLLMClient struct {
	calls int
}

func (c *countingLLMClient) CompletionsWithCtx(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.calls++
	summary := "compressed summary"
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
}

func msg(role, text string) llm.Message {
	return llm.NewTextMessage(role, text)
}

func replayableAssistantToolConversation(userPrompt string) []llm.Message {
	toolCalls := []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{}`}}}
	return []llm.Message{
		msg("system", "system"),
		msg("user", userPrompt),
		llm.NewReplayStateMessageForTesting("inspect", toolCalls),
		llm.NewToolResultMessage("call-1", "result"),
	}
}

func newCompressionTestRunner(t *testing.T, client llm.LLMClient, maxTokens int) *Runner {
	t.Helper()
	t_tempDir = t.TempDir()
	return newTestRunner(client, template.Template{
		MaxTokens:             maxTokens,
		MemoryCompressionTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Summarize {{context}}"}}},
	})
}

func TestCompressionRetainsOversizedLatestRoundWithReplayState(t *testing.T) {
	t_tempDir = t.TempDir()
	visible := strings.Repeat("visible ", 100)
	toolCalls := []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{}`}}}
	messages := []llm.Message{
		msg("system", "system"),
		msg("user", "review"),
		llm.NewReplayStateMessageForTesting(visible, toolCalls),
		llm.NewToolResultMessage("call-1", "result"),
	}
	runner := newTestRunner(&fakeLLMClient{response: &llm.ChatResponse{}}, template.Template{
		MaxTokens:             10,
		MemoryCompressionTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Summarize {{context}}"}}},
	})
	got, err := runner.runCompression(context.Background(), messages, "test.go")
	if err == nil {
		t.Fatal("expected oversized replay-carrying round to stop compression")
	}
	if len(got) != len(messages) || got[2].Role != "assistant" || got[3].ToolCallID != "call-1" {
		t.Fatalf("compression dropped or split latest assistant/tool round: %+v", got)
	}
}

// TestCompressionDegradesOversizedLatestRoundWithoutReplayState pins the
// pre-envelope behavior: an oversized latest round that carries no opaque
// replay state is summarized like any other round instead of aborting the
// review.
func TestCompressionDegradesOversizedLatestRoundWithoutReplayState(t *testing.T) {
	t_tempDir = t.TempDir()
	visible := strings.Repeat("visible ", 100)
	turn := (&llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{
		Content:   &visible,
		ToolCalls: []llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "file_read", Arguments: `{}`}}},
	}}}}).AssistantTurn().Message()
	messages := []llm.Message{
		msg("system", "system"),
		msg("user", "review"),
		turn,
		llm.NewToolResultMessage("call-1", "result"),
	}
	summary := "condensed history"
	runner := newTestRunner(&fakeLLMClient{response: &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}},
	}}, template.Template{
		MaxTokens:             10,
		MemoryCompressionTask: template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Summarize {{context}}"}}},
	})
	got, err := runner.runCompression(context.Background(), messages, "test.go")
	if err != nil {
		t.Fatalf("runCompression: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected degradation to frozen zone + summary, got %d messages", len(got))
	}
	if !strings.Contains(got[1].ExtractText(), "<previous_review_summary>") {
		t.Fatalf("summary missing from rebuilt prompt: %s", got[1].ExtractText())
	}
}

func TestCompressionRetainsLatestAssistantToolRoundWhenFrozenZoneConsumesBudget(t *testing.T) {
	client := &countingLLMClient{}
	messages := replayableAssistantToolConversation(strings.Repeat("frozen prompt ", 100))
	runner := newCompressionTestRunner(t, client, 100)

	got, err := runner.runCompression(context.Background(), messages, "test.go")

	if err == nil {
		t.Fatal("expected frozen zone to leave too little budget for the latest replayable round")
	}
	if client.calls != 0 {
		t.Fatalf("compression client calls = %d, want 0", client.calls)
	}
	if len(got) != len(messages) {
		t.Fatalf("compression dropped or split latest assistant/tool round: %+v", got)
	}
	if !got[2].IsReplayable() || len(got[2].ToolCalls) != 1 || got[2].ToolCalls[0].ID != "call-1" {
		t.Fatalf("compression changed latest assistant replay: %+v", got[2])
	}
	if got[3].Role != "tool" || got[3].ToolCallID != "call-1" || got[3].ExtractText() != "result" {
		t.Fatalf("compression detached latest tool result: %+v", got[3])
	}
}

func TestCompressionAllFittingConversationIsNoOp(t *testing.T) {
	client := &countingLLMClient{}
	messages := replayableAssistantToolConversation("review")
	runner := newCompressionTestRunner(t, client, 1000)

	got, err := runner.runCompression(context.Background(), messages, "test.go")

	if err != nil {
		t.Fatalf("runCompression: %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("compression client calls = %d, want 0", client.calls)
	}
	if len(got) != len(messages) {
		t.Fatalf("all-fitting conversation changed: %+v", got)
	}
	for i := range messages {
		if got[i].Role != messages[i].Role || got[i].ExtractText() != messages[i].ExtractText() || got[i].ToolCallID != messages[i].ToolCallID {
			t.Errorf("all-fitting message %d changed: got %+v, want %+v", i, got[i], messages[i])
		}
	}
	if len(got[2].ToolCalls) != 1 || got[2].ToolCalls[0].ID != "call-1" {
		t.Fatalf("all-fitting assistant replay changed: %+v", got[2])
	}
}

func TestGroupIntoRounds(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("assistant", "resp1"),
		msg("tool", "result1"),
		msg("tool", "result2"),
		msg("assistant", "resp2"),
		msg("tool", "result3"),
		msg("assistant", "resp3"),
	}

	rounds := groupIntoRounds(messages, 2)
	if len(rounds) != 3 {
		t.Fatalf("expected 3 rounds, got %d", len(rounds))
	}

	if rounds[0].assistantIdx != 2 {
		t.Errorf("round[0].assistantIdx = %d, want 2", rounds[0].assistantIdx)
	}
	if len(rounds[0].toolIdxs) != 2 {
		t.Errorf("round[0] should have 2 tool messages, got %d", len(rounds[0].toolIdxs))
	}
	if rounds[1].assistantIdx != 5 {
		t.Errorf("round[1].assistantIdx = %d, want 5", rounds[1].assistantIdx)
	}
	if rounds[2].assistantIdx != 7 {
		t.Errorf("round[2].assistantIdx = %d, want 7", rounds[2].assistantIdx)
	}
	if len(rounds[2].toolIdxs) != 0 {
		t.Errorf("round[2] should have 0 tool messages")
	}
}

func TestGroupIntoRounds_NoAssistant(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("user", "another"),
	}
	rounds := groupIntoRounds(messages, 2)
	if len(rounds) != 0 {
		t.Errorf("expected 0 rounds, got %d", len(rounds))
	}
}

func TestPartitionMessages_ShortConversation(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
	}
	result := partitionMessages(messages, 100000, 0)
	if result.frozenEnd != 2 {
		t.Errorf("frozenEnd = %d, want 2", result.frozenEnd)
	}
	if result.compressEnd != 2 {
		t.Errorf("compressEnd = %d, want 2", result.compressEnd)
	}
}

func TestPartitionMessages_EverythingFits(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("assistant", "short reply"),
		msg("tool", "ok"),
	}
	result := partitionMessages(messages, 100000, 0)
	if result.activeCount != 1 {
		t.Errorf("activeCount = %d, want 1 (the only round fits)", result.activeCount)
	}
	// Everything fits: the compress zone must be empty so live rounds are
	// kept verbatim instead of being summarized away.
	if result.compressEnd != result.frozenEnd {
		t.Errorf("compressEnd = %d, want frozenEnd %d (empty compress zone)", result.compressEnd, result.frozenEnd)
	}
}

// TestPartitionMessages_FrozenZoneReservesBudget verifies that the frozen
// zone's own tokens count against the prompt budget when sizing the active
// zone: with a fat frozen prompt only the newest round fits, so the older
// round must land in the compress zone.
func TestPartitionMessages_FrozenZoneReservesBudget(t *testing.T) {
	roundText := strings.Repeat("round content ", 100)
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", strings.Repeat("frozen prompt content ", 200)),
		msg("assistant", roundText),
		msg("tool", roundText),
		msg("assistant", roundText),
		msg("tool", roundText),
	}

	frozenTokens := llm.ApproxMessagesTokenCount(messages[:2])
	oneRound := llm.ApproxMessagesTokenCount(messages[2:4])
	// Budget admits the frozen zone plus one round with headroom, but not
	// two rounds on top of the frozen zone. Without the frozen reservation
	// both rounds would appear to fit.
	budget := frozenTokens + oneRound + oneRound/2
	maxTokens := budget * 5 / 4 // PromptTokenLimit is 80% of MaxTokens

	actualBudget := PromptTokenLimit(maxTokens)
	if actualBudget-frozenTokens < oneRound {
		t.Fatalf("test setup: one round (%d tokens) must fit budget %d after reserving %d", oneRound, actualBudget, frozenTokens)
	}
	if actualBudget-frozenTokens >= 2*oneRound {
		t.Fatalf("test setup: two rounds (%d tokens) must not fit budget %d after reserving %d", 2*oneRound, actualBudget, frozenTokens)
	}
	if actualBudget < 2*oneRound {
		t.Fatalf("test setup: two rounds (%d tokens) must fit budget %d when the frozen zone is ignored", 2*oneRound, actualBudget)
	}

	result := partitionMessages(messages, maxTokens, 0)
	if result.activeCount != 1 {
		t.Errorf("activeCount = %d, want 1 (frozen zone must reserve budget)", result.activeCount)
	}
	if result.compressEnd != 4 {
		t.Errorf("compressEnd = %d, want 4 (older round compressed)", result.compressEnd)
	}
}

func TestPartitionMessages_ReservesFrozenZoneTokens(t *testing.T) {
	messages := replayableAssistantToolConversation(strings.Repeat("frozen prompt ", 100))

	result := partitionMessages(messages, 100, 0)

	if result.activeCount != 0 || !result.activeTooLarge {
		t.Fatalf("partition kept replayable round despite frozen-zone budget: %+v", result)
	}
}

func TestPartitionMessages_ReservesPreviousSummaryTokens(t *testing.T) {
	messages := replayableAssistantToolConversation(strings.Repeat("frozen ", 40))
	const (
		maxTokens             = 100
		previousSummaryTokens = 40
	)
	frozenAndRoundTokens := llm.ApproxMessagesTokenCount(messages)
	if frozenAndRoundTokens > PromptTokenLimit(maxTokens) {
		t.Fatalf("test setup without summary already exceeds budget: %d", frozenAndRoundTokens)
	}
	if frozenAndRoundTokens+previousSummaryTokens <= PromptTokenLimit(maxTokens) {
		t.Fatalf("test setup with summary still fits budget: %d", frozenAndRoundTokens+previousSummaryTokens)
	}

	result := partitionMessages(messages, maxTokens, previousSummaryTokens)

	if result.activeCount != 0 || !result.activeTooLarge {
		t.Fatalf("partition kept replayable round despite reserved summary budget: %+v", result)
	}
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no fences",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "json fence",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "plain fence",
			input: "```\ncontent\n```",
			want:  "content",
		},
		{
			name:  "fence with surrounding whitespace",
			input: "  ```json\n{}\n```  ",
			want:  "{}",
		},
		{
			name:  "empty after strip",
			input: "```json\n```",
			want:  "",
		},
		{
			name:  "single-line json fence without newline",
			input: "```json",
			want:  "",
		},
		{
			name:  "bare fence without newline",
			input: "```",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripMarkdownFences(tt.input)
			if got != tt.want {
				t.Errorf("StripMarkdownFences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildMessageXML(t *testing.T) {
	messages := []llm.Message{
		msg("user", "hello"),
		msg("assistant", "world"),
	}
	got := buildMessageXML(messages)
	if !strings.Contains(got, `<message id="0" role="user">`) {
		t.Errorf("missing user message tag: %s", got)
	}
	if !strings.Contains(got, `<message id="1" role="assistant">`) {
		t.Errorf("missing assistant message tag: %s", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("missing content: %s", got)
	}
}

func TestPromptTokenLimit(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		want      int
	}{
		{name: "zero", maxTokens: 0, want: 0},
		{name: "one truncates to zero", maxTokens: 1, want: 0},
		{name: "four truncates to three", maxTokens: 4, want: 3},
		{name: "five rounds to exact 4.0 via float half-ULP", maxTokens: 5, want: 4},
		{name: "typical 4k context", maxTokens: 4096, want: 3276},
		{name: "default max tokens", maxTokens: 58888, want: 47110},
		{name: "typical 128k context", maxTokens: 128000, want: 102400},
		{name: "typical 200k context", maxTokens: 200000, want: 160000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PromptTokenLimit(tt.maxTokens); got != tt.want {
				t.Errorf("PromptTokenLimit(%d) = %d, want %d", tt.maxTokens, got, tt.want)
			}
		})
	}
}

// TestPromptTokenLimitMatchesReplacedExpression pins the one-time migration:
// PromptTokenLimit replaced a literal `maxTokens*4/5` at four call sites, so the
// float form must agree with the integer form it replaced across the realistic
// max_tokens range. This is specific to tokenWarningThreshold being 0.80 — if the
// threshold ever changes, delete this test rather than "fixing" it.
func TestPromptTokenLimitMatchesReplacedExpression(t *testing.T) {
	for _, maxTokens := range []int{0, 1, 2, 3, 4, 5, 7, 40, 100, 1000, 4096, 8192, 32768, 58888, 128000, 200000, 1_000_000} {
		if got, want := PromptTokenLimit(maxTokens), maxTokens*4/5; got != want {
			t.Errorf("PromptTokenLimit(%d) = %d, want %d (maxTokens*4/5)", maxTokens, got, want)
		}
	}
}
