package testconnection

import (
	"testing"
)

func TestLoadDefault(t *testing.T) {
	conv, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if conv == nil {
		t.Fatal("expected non-nil conversation")
	}
	if conv.Timeout <= 0 {
		t.Errorf("expected positive timeout, got %d", conv.Timeout)
	}
	if len(conv.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	hasSystem := false
	hasUser := false
	for _, m := range conv.Messages {
		switch m.Role {
		case "system":
			hasSystem = true
		case "user":
			hasUser = true
		}
	}
	if !hasSystem {
		t.Error("expected a system message")
	}
	if !hasUser {
		t.Error("expected a user message")
	}
}

func TestResolveLang(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "English"},
		{"Chinese", "Chinese"},
		{"Japanese", "Japanese"},
	}
	for _, tc := range tests {
		got := resolveLang(tc.input)
		if got != tc.want {
			t.Errorf("resolveLang(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestApplyLanguage(t *testing.T) {
	conv := &LlmConversation{
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a bot."},
			{Role: "user", Content: "Hello"},
		},
	}

	conv.ApplyLanguage("Chinese")

	if conv.Messages[0].Content == "You are a bot." {
		t.Error("expected system message to be modified")
	}
	expected := "You are a bot.\n\nAlways respond in Chinese."
	if conv.Messages[0].Content != expected {
		t.Errorf("system content = %q, want %q", conv.Messages[0].Content, expected)
	}
	// User message should not be modified
	if conv.Messages[1].Content != "Hello" {
		t.Errorf("user content should not change, got %q", conv.Messages[1].Content)
	}
}

func TestApplyLanguage_EmptyLang(t *testing.T) {
	conv := &LlmConversation{
		Messages: []ChatMessage{
			{Role: "system", Content: "Base."},
		},
	}

	conv.ApplyLanguage("")

	expected := "Base.\n\nAlways respond in English."
	if conv.Messages[0].Content != expected {
		t.Errorf("content = %q, want %q", conv.Messages[0].Content, expected)
	}
}
