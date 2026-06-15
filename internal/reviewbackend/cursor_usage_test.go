package reviewbackend

import (
	"testing"

	"github.com/remdev/cursor-go-sdk/cursor"
)

func TestCursorUsageAccumulator_TurnEnded(t *testing.T) {
	acc := &cursorUsageAccumulator{}
	acc.observe(cursor.InteractionUpdate{
		Type: "turn-ended",
		Usage: map[string]any{
			"inputTokens":  1200,
			"outputTokens": 340,
		},
	})
	acc.observe(cursor.InteractionUpdate{
		Type: "turn-ended",
		Usage: map[string]any{
			"prompt_tokens":     800,
			"completion_tokens": 120,
		},
	})

	usage := acc.usage()
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.PromptTokens != 2000 {
		t.Errorf("PromptTokens = %d, want 2000", usage.PromptTokens)
	}
	if usage.CompletionTokens != 460 {
		t.Errorf("CompletionTokens = %d, want 460", usage.CompletionTokens)
	}
	if usage.TotalTokens != 2460 {
		t.Errorf("TotalTokens = %d, want 2460", usage.TotalTokens)
	}
}

func TestCursorUsageAccumulator_TokenDeltaFallback(t *testing.T) {
	acc := &cursorUsageAccumulator{}
	acc.observe(cursor.InteractionUpdate{Type: "token-delta", Tokens: 50})
	acc.observe(cursor.InteractionUpdate{Type: "token-delta", Tokens: 25})

	usage := acc.usage()
	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.CompletionTokens != 75 {
		t.Errorf("CompletionTokens = %d, want 75", usage.CompletionTokens)
	}
}

func TestCursorUsageAccumulator_TurnEndedDisablesTokenDelta(t *testing.T) {
	acc := &cursorUsageAccumulator{}
	acc.observe(cursor.InteractionUpdate{Type: "token-delta", Tokens: 100})
	acc.observe(cursor.InteractionUpdate{
		Type: "turn-ended",
		Usage: map[string]any{
			"inputTokens":  10,
			"outputTokens": 5,
		},
	})
	acc.observe(cursor.InteractionUpdate{Type: "token-delta", Tokens: 999})

	usage := acc.usage()
	if usage.PromptTokens != 10 || usage.CompletionTokens != 5 {
		t.Errorf("usage = %+v, want prompt=10 completion=5", usage)
	}
}
