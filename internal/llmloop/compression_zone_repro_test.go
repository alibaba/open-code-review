package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/config/template"
)

func msg(role, content string) llm.Message {
	return llm.NewTextMessage(role, content)
}

func TestRepro_EverythingFitsSummarizesLiveTail(t *testing.T) {
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("assistant", "resp1"),
		msg("tool", "result1"),
		msg("assistant", "resp2"),
		msg("tool", "result2"),
	}
	result := partitionMessages(messages, 1_000_000, 0)
	if result.compressEnd != result.frozenEnd {
		t.Errorf("compressEnd = %d, want %d", result.compressEnd, result.frozenEnd)
	}
}

func TestRepro_FrozenZoneNotReservedFromBudget(t *testing.T) {
	roundText := strings.Repeat("round content ", 100)
	messages := []llm.Message{
		msg("system", "sys"),
		msg("user", strings.Repeat("frozen prompt content ", 200)),
		msg("assistant", roundText),
		msg("tool", roundText),
		msg("assistant", roundText),
		msg("tool", roundText),
	}

	frozenTokens := CountMessagesTokens(messages[:2])
	oneRound := CountMessagesTokens(messages[2:4])
	budget := frozenTokens + oneRound + oneRound/2
	maxTokens := budget * 5 / 4 

	actualBudget := PromptTokenLimit(maxTokens)
	if actualBudget-frozenTokens < oneRound || actualBudget-frozenTokens >= 2*oneRound || actualBudget < 2*oneRound {
		t.Fatalf("test setup out of range: budget=%d frozen=%d round=%d", actualBudget, frozenTokens, oneRound)
	}

	result := partitionMessages(messages, maxTokens, 0)
	if result.activeCount != 1 {
		t.Errorf("activeCount = %d, want 1", result.activeCount)
	}
}

func TestRepro_MissingTemplateTruncatesConversation(t *testing.T) {
	r := newTestRunner(&fakeLLMClient{}, template.Template{MaxTokens: 1000})

	msgs := []llm.Message{
		msg("system", "sys"),
		msg("user", "prompt"),
		msg("assistant", "resp"),
	}
	got, err := r.runCompression(context.Background(), msgs, "test.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(msgs) {
		t.Errorf("runCompression returned %d messages, want %d", len(got), len(msgs))
	}
}
