// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/llm"
)

func msg(role, text string) llm.Message {
	return llm.NewTextMessage(role, text)
}

func TestCountMessagesTokens(t *testing.T) {
	msgs := []llm.Message{
		msg("user", "hello world"),
		msg("assistant", "hi there"),
	}
	got := CountMessagesTokens(msgs)
	if got <= 0 {
		t.Errorf("expected positive token count, got %d", got)
	}
}

func TestCountMessagesTokens_Empty(t *testing.T) {
	got := CountMessagesTokens(nil)
	if got != 0 {
		t.Errorf("expected 0 for nil, got %d", got)
	}
}

// TestCountMessagesTokens_IncludesNativePayload guards a gap found in review
// of the #805 fix: once an assistant turn's Native payload (thinking blocks,
// reasoning items, encrypted_content, reasoning_content) actually replays on
// the wire, a token budget computed only from ExtractText() would
// systematically under-count reasoning-heavy conversations and could let
// compression's threshold checks fire too late.
func TestCountMessagesTokens_IncludesNativePayload(t *testing.T) {
	withoutNative := []llm.Message{msg("user", "hello world")}
	withNative := []llm.Message{
		msg("user", "hello world"),
		llm.NewToolCallMessage("", nil, llm.NativeTurn{
			Family:  "openai-chat-completions",
			Payload: llm.ReasoningPayload(strings.Repeat("reasoning ", 200)),
		}, ""),
	}

	base := CountMessagesTokens(withoutNative)
	got := CountMessagesTokens(withNative)
	if got <= base {
		t.Errorf("CountMessagesTokens with a Native payload = %d, want more than the base count %d", got, base)
	}
}

// TestCountMessagesTokens_IncludesToolCalls guards against under-counting
// assistant turns that invoke tools under protocols where tool calls live on
// ToolCalls rather than a Native payload (e.g. OpenAI chat completions).
func TestCountMessagesTokens_IncludesToolCalls(t *testing.T) {
	withoutTools := []llm.Message{msg("user", "hello world")}
	withTools := []llm.Message{
		msg("user", "hello world"),
		llm.NewToolCallMessage("", []llm.ToolCall{
			{
				ID: "call_1",
				Function: llm.FunctionCall{
					Name:      "file_write",
					Arguments: strings.Repeat("content line\n", 100),
				},
			},
		}, llm.NativeTurn{}, ""),
	}

	base := CountMessagesTokens(withoutTools)
	got := CountMessagesTokens(withTools)
	if got <= base {
		t.Errorf("CountMessagesTokens with tool calls = %d, want more than base count %d", got, base)
	}

	expectedToolTokens := (len("call_1") + len("file_write") + len(strings.Repeat("content line\n", 100))) / 4
	if diff := got - base; diff < expectedToolTokens-10 || diff > expectedToolTokens+10 {
		t.Errorf("token diff for tool calls = %d, expected roughly %d", diff, expectedToolTokens)
	}
}

// TestComputeActiveZoneSize_AccountsForToolCalls verifies that rounds with
// heavy tool-call arguments consume active-zone budget properly rather than
// evaluating as 0 tokens.
func TestComputeActiveZoneSize_AccountsForToolCalls(t *testing.T) {
	largeArgs := strings.Repeat("argument data ", 200) // ~2800 bytes => ~700 tokens
	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "task"),
		llm.NewToolCallMessage("", []llm.ToolCall{
			{
				ID: "call_large",
				Function: llm.FunctionCall{
					Name:      "code_search",
					Arguments: largeArgs,
				},
			},
		}, llm.NativeTurn{}, ""),
		msg("tool", "search result"),
	}

	rounds := groupIntoRounds(msgs, 2)
	if len(rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(rounds))
	}

	smallBudgetMaxTokens := 100
	count := computeActiveZoneSize(rounds, msgs, smallBudgetMaxTokens, 0)
	if count != 0 {
		t.Errorf("computeActiveZoneSize() = %d, want 0 because large tool arguments exceed budget", count)
	}

	generousMaxTokens := 10000
	count = computeActiveZoneSize(rounds, msgs, generousMaxTokens, 0)
	if count != 1 {
		t.Errorf("computeActiveZoneSize() = %d, want 1 under generous budget", count)
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
	if result.activeCount != 0 {
		t.Errorf("activeCount = %d, want 0 (everything fits)", result.activeCount)
	}
	if result.compressEnd != len(messages) {
		t.Errorf("compressEnd = %d, want %d", result.compressEnd, len(messages))
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

func TestCopyMessages(t *testing.T) {
	orig := []llm.Message{msg("user", "a"), msg("assistant", "b")}
	cp := copyMessages(orig)
	if len(cp) != 2 {
		t.Fatalf("len = %d, want 2", len(cp))
	}
	cp[0] = msg("system", "mutated")
	if orig[0].Role == "system" {
		t.Error("copyMessages should return independent slice")
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

func TestTruncateToByteBudget(t *testing.T) {
	t.Run("under budget unchanged", func(t *testing.T) {
		text := "short text"
		if got := truncateToByteBudget(text, 1000); got != text {
			t.Errorf("truncateToByteBudget = %q, want unchanged %q", got, text)
		}
	})
	t.Run("over budget truncates with suffix", func(t *testing.T) {
		got := truncateToByteBudget(strings.Repeat("word ", 500), 100)
		if len(got) > 100 {
			t.Errorf("truncated text = %d bytes, want <= 100", len(got))
		}
		if !strings.HasSuffix(got, truncationSuffix) {
			t.Errorf("truncated text should end with suffix, got %q", got)
		}
	})
	t.Run("budget below suffix returns suffix only", func(t *testing.T) {
		if got := truncateToByteBudget("anything longer", 0); got != truncationSuffix {
			t.Errorf("truncateToByteBudget with 0 budget = %q, want suffix only", got)
		}
	})
	t.Run("does not split a multi-byte rune", func(t *testing.T) {
		text := strings.Repeat("\u00e9", 100) // 2 bytes per rune
		for budget := len(truncationSuffix) + 1; budget < 60; budget++ {
			got := truncateToByteBudget(text, budget)
			if !utf8.ValidString(got) {
				t.Fatalf("budget %d produced invalid UTF-8: %q", budget, got)
			}
			if len(got) > budget {
				t.Fatalf("budget %d produced %d bytes", budget, len(got))
			}
		}
	})
}

var plainTemplate = []MessageTemplate{{Role: "user", Content: "Summarize the history below.\n{{context}}"}}

func renderedFor(msgs []llm.Message) int {
	return renderedSize(plainTemplate, buildMessageXML(msgs))
}

func TestFitCompressionInput_UnderBudgetUnchanged(t *testing.T) {
	msgs := []llm.Message{msg("assistant", "short reply"), msg("tool", "ok")}
	got, err := fitCompressionInput(msgs, plainTemplate, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ExtractText() != "short reply" || got[1].ExtractText() != "ok" {
		t.Errorf("messages altered under budget: %+v", got)
	}
}

// A single unbounded one-line tool result must not push the compression
// request past its budget, and only that result should be elided.
func TestFitCompressionInput_OversizedToolResult(t *testing.T) {
	msgs := []llm.Message{
		msg("assistant", "read the file"),
		llm.NewToolResultMessage("call_1", strings.Repeat("x", 170*1024)),
		msg("assistant", "done"),
	}
	const budget = 4096
	got, err := fitCompressionInput(msgs, plainTemplate, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size := renderedFor(got); size > budget {
		t.Fatalf("rendered request = %d bytes, want <= %d", size, budget)
	}
	if got[0].ExtractText() != "read the file" || got[2].ExtractText() != "done" {
		t.Errorf("small messages were altered: %q, %q", got[0].ExtractText(), got[2].ExtractText())
	}
	if !strings.HasSuffix(got[1].ExtractText(), truncationSuffix) {
		t.Errorf("oversized result should carry the truncation marker")
	}
	if got[1].ToolCallID != "call_1" {
		t.Errorf("ToolCallID = %q, want call_1 preserved", got[1].ToolCallID)
	}
	if msgs[1].ExtractText() == got[1].ExtractText() {
		t.Error("input slice was mutated instead of copied")
	}
}

// Reasoning is rendered into the request but invisible to ExtractText, so it
// must count toward the budget and be trimmed like content.
func TestFitCompressionInput_OversizedReasoning(t *testing.T) {
	calls := []llm.ToolCall{{ID: "call_1", Function: llm.FunctionCall{Name: "file_read"}}}
	msgs := []llm.Message{
		llm.NewToolCallMessage("ok", calls, llm.NativeTurn{}, strings.Repeat("r", 170*1024)),
	}
	const budget = 4096
	got, err := fitCompressionInput(msgs, plainTemplate, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size := renderedFor(got); size > budget {
		t.Fatalf("rendered request = %d bytes, want <= %d", size, budget)
	}
	if got[0].ExtractText() != "ok" {
		t.Errorf("content = %q, want ok", got[0].ExtractText())
	}
	if !strings.HasSuffix(got[0].ReasoningContent, truncationSuffix) {
		t.Error("reasoning should carry the truncation marker")
	}
	if len(got[0].ToolCalls) != 1 {
		t.Errorf("ToolCalls dropped: %+v", got[0].ToolCalls)
	}
}

// Visible text exactly at the budget still overflows once markup and the
// template text are added.
func TestFitCompressionInput_CountsMarkupAndTemplate(t *testing.T) {
	const budget = 4096
	msgs := []llm.Message{llm.NewToolResultMessage("call_1", strings.Repeat("y", budget))}
	got, err := fitCompressionInput(msgs, plainTemplate, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size := renderedFor(got); size > budget {
		t.Fatalf("rendered request = %d bytes, want <= %d", size, budget)
	}
}

func TestFitCompressionInput_MarkupAloneTooLarge(t *testing.T) {
	msgs := []llm.Message{msg("assistant", "a"), msg("tool", "b"), msg("assistant", "c")}
	_, err := fitCompressionInput(msgs, plainTemplate, 64)
	if !errors.Is(err, errCompressionInputTooLarge) {
		t.Fatalf("err = %v, want errCompressionInputTooLarge", err)
	}
}
