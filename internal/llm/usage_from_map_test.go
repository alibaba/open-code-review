package llm

import "testing"

func TestUsageFromMap_CursorCamelCase(t *testing.T) {
	ui := UsageFromMap(map[string]any{
		"inputTokens":  100,
		"outputTokens": 42,
	})
	if ui == nil {
		t.Fatal("expected usage")
	}
	if ui.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d", ui.PromptTokens)
	}
	if ui.CompletionTokens != 42 {
		t.Errorf("CompletionTokens = %d", ui.CompletionTokens)
	}
	if ui.TotalTokens != 142 {
		t.Errorf("TotalTokens = %d, want 142", ui.TotalTokens)
	}
}

func TestUsageFromMap_OpenAISnakeCase(t *testing.T) {
	ui := UsageFromMap(map[string]any{
		"prompt_tokens":     50,
		"completion_tokens": 10,
		"total_tokens":      60,
	})
	if ui == nil {
		t.Fatal("expected usage")
	}
	if ui.TotalTokens != 60 {
		t.Errorf("TotalTokens = %d", ui.TotalTokens)
	}
}
