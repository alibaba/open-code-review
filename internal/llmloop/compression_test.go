package llmloop

import (
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/llm"
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
