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

func TestPartitionMessages_AccountsForToolCalls(t *testing.T) {
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

	if part := partitionMessages(msgs, 100, 0); part.activeCount != 0 || part.compressEnd != len(msgs) {
		t.Errorf("large tool arguments exceed the budget: %+v", part)
	}
	if part := partitionMessages(msgs, 10000, 0); part.activeCount != 1 || part.compressEnd != 2 {
		t.Errorf("round should fit under the larger budget: %+v", part)
	}
}

func TestPartitionMessages_CompleteRounds(t *testing.T) {
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

	for offset, start := range []int{7, 5, 2} {
		reserve := PromptTokenLimit(1000) - CountMessagesTokens(messages[:2]) - CountMessagesTokens(messages[start:]) - 1
		part := partitionMessages(messages, 1000, reserve)
		if part.compressEnd != start || part.activeCount != offset+1 {
			t.Errorf("want %d complete rounds starting at %d: %+v", offset+1, start, part)
		}
	}
}

func TestPartitionMessages_NoAssistant(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("user", "another"),
	}
	if part := partitionMessages(messages, 1000, 0); part.activeCount != 0 || part.compressEnd != 2 {
		t.Errorf("a fitting user-only tail needs no compression: %+v", part)
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
		t.Errorf("activeCount = %d, want 1 (everything fits)", result.activeCount)
	}
	if result.compressEnd != result.frozenEnd {
		t.Errorf("compressEnd = %d, want %d", result.compressEnd, result.frozenEnd)
	}
}

func TestCompressionSummary(t *testing.T) {
	for _, fixture := range []struct{ text, want string }{
		{"a<think>private</think>b", "ab"},
		{"private</think>", ""},
		{"private notes\n</think>\nfinal answer", "final answer"},
		{"`malformed <think>private</think>", "`malformed"},
		{"The `<think>` tag is literal.", "The `<think>` tag is literal."},
		{"```json\n \t\n```", ""},
		{"~~~~go\n\n~~~~~", ""},
		{"```go\n\n", ""},
		{"~~~go\n\n", ""},
		{"~~~go\n~~~\n\n~~~json\n~~~", ""},
		{"```token``` is a keyword\n```", "```token``` is a keyword\n```"},
		{"~~~md\n    ~~~", "~~~md\n    ~~~"},
		{"```md\nKeep finding A.", "```md\nKeep finding A."},
		{"```json\n{}\n```", "```json\n{}\n```"},
		{"~~~text\nKeep finding A.\n~~~", "~~~text\nKeep finding A.\n~~~"},
		{"```json\n```\nKeep finding A.", "```json\n```\nKeep finding A."},
		{"```text\n<think>notes</think>\n```", ""},
		{"<think>notes</think>\nFinal answer with `</think>`.", "Final answer with `</think>`."},
		{"<think>Quote `</think>` literally.</think>\nFinal answer", "Final answer"},
		{"<think>Quote `</think>` literally.\nNo final answer", ""},
		{"```go\nf()\n```\nFixed the caller.<think>notes</think>", "```go\nf()\n```\nFixed the caller."},
		{"Use \\` here. <think>private</think> Escape \\` there.", "Use \\` here.  Escape \\` there."},
		{`Keep \<think>literal\</think>.`, `Keep \<think>literal\</think>.`},
		{"```go\nf()\n```\nFixed the caller.", "```go\nf()\n```\nFixed the caller."},
		{"```xml\n<config><think>off</think></config>\n```", "```xml\n<config><think>off</think></config>\n```"},
		{"```go\n// <think>\n````\nAnswer<think>notes</think>", "```go\n// <think>\n````\nAnswer"},
		{"<think>\n~~~xml\n</think>\n~~~\nstill reasoning", ""},
		{"Delimiter `.\n\n<think>private</think>\n\nAnother `.", "Delimiter `.\n\n\n\nAnother `."},
	} {
		if got := compressionSummary(fixture.text); got != fixture.want {
			t.Errorf("compressionSummary(%q) = %q, want %q", fixture.text, got, fixture.want)
		}
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
			name:  "single-line json fence",
			input: "```json{}```",
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

// A single compression round can collect many oversized tool results before
// the synchronous pass runs. Each must be elided to fit the budget without
// re-rendering the whole request on every cut (the O(n^2 * L) path).
func TestFitCompressionInput_ManyLargeResults(t *testing.T) {
	const (
		numResults = 32
		resultSize = 170 * 1024 // ~170 KiB each
		budget     = 800000     // 800 KB, roughly the default context ceiling
	)
	msgs := make([]llm.Message, 0, numResults+1)
	msgs = append(msgs, msg("assistant", "here are the tool results"))
	for i := 0; i < numResults; i++ {
		msgs = append(msgs, llm.NewToolResultMessage("call", strings.Repeat("x", resultSize)))
	}

	got, err := fitCompressionInput(msgs, plainTemplate, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size := renderedFor(got); size > budget {
		t.Fatalf("rendered request = %d bytes, want <= %d", size, budget)
	}
	// The leading assistant message must stay intact. The largest results are
	// elided first, so at least one result must be truncated while the tail of
	// smaller results may survive untouched; every result is either untouched
	// or carries the truncation marker.
	if got[0].ExtractText() != "here are the tool results" {
		t.Errorf("leading message altered: %q", got[0].ExtractText())
	}
	truncated := 0
	for i := 1; i < len(got); i++ {
		text := got[i].ExtractText()
		if text == strings.Repeat("x", resultSize) {
			continue // untouched
		}
		if !strings.HasSuffix(text, truncationSuffix) {
			t.Errorf("result %d was altered without the truncation marker", i-1)
		}
		truncated++
	}
	if truncated == 0 {
		t.Error("expected at least one result to be truncated")
	}
	// The input slice must not be mutated.
	if msgs[1].ExtractText() == got[1].ExtractText() {
		t.Error("input slice was mutated instead of copied")
	}
}

// A template that references {{context}} more than once must not over-truncate:
// each byte removed from a field shrinks the rendered request by ctxCount bytes,
// so the field-level cut is sized against excess/ctxCount, not the raw excess.
func TestFitCompressionInput_MultipleContextPlaceholders(t *testing.T) {
	// Three {{context}} references → ctxCount = 3, matching the over-truncation
	// scenario: a 2000-byte field under a 4096-byte budget. The naive
	// `target = largest - excess` crushes the field to ~15 bytes (a bare
	// suffix); the correct `largest - ceil(excess/ctxCount)` keeps ~1290 bytes.
	tmpl := []MessageTemplate{
		{Role: "user", Content: "{{context}}\n{{context}}\n{{context}}"},
	}
	const budget = 4096
	msgs := []llm.Message{
		llm.NewToolResultMessage("call_1", strings.Repeat("z", 2000)),
	}
	got, err := fitCompressionInput(msgs, tmpl, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	size := renderedSize(tmpl, buildMessageXML(got))
	if size > budget {
		t.Fatalf("rendered request = %d bytes, want <= %d", size, budget)
	}
	// Over-truncation would leave only the suffix (~15 bytes). The corrected
	// arithmetic keeps the bulk of the field: assert it retained the majority
	// of its content rather than collapsing to a bare marker.
	if got[0].ExtractText() == truncationSuffix || len(got[0].ExtractText()) < 1000 {
		t.Fatalf("field over-truncated to %d bytes (want ~1290): %q", len(got[0].ExtractText()), got[0].ExtractText())
	}
}

// BenchmarkFitCompressionInput_ManyLargeResults guards the O(n^2 * L)
// re-rendering regression: with many oversized fields, the incremental byte
// accounting must dominate over repeated full-request rendering.
func BenchmarkFitCompressionInput_ManyLargeResults(b *testing.B) {
	msgs := make([]llm.Message, 0, 33)
	msgs = append(msgs, msg("assistant", "here are the tool results"))
	for i := 0; i < 32; i++ {
		msgs = append(msgs, llm.NewToolResultMessage("call", strings.Repeat("x", 170*1024)))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fitCompressionInput(msgs, plainTemplate, 800000)
	}
}
