// Package testconnection loads the LLM test connection task configuration.
package testconnection

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// TestTask holds the conversation template for the LLM connectivity test.
type TestTask struct {
	TestTask LlmConversation `json:"TEST_TASK"`
}

// LlmConversation represents a single conversation preset for testing.
type LlmConversation struct {
	Timeout  int           `json:"timeout"`
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

//go:embed task.json
var defaultTask []byte

// LoadDefault parses the embedded task.json and returns the TEST_TASK conversation.
func LoadDefault() (*LlmConversation, error) {
	var tasks TestTask
	if err := json.Unmarshal(defaultTask, &tasks); err != nil {
		return nil, fmt.Errorf("unmarshal test task config: %w", err)
	}
	return &tasks.TestTask, nil
}

func resolveLang(lang string) string {
	if lang == "" {
		return "English"
	}
	return lang
}

// ApplyLanguage injects a language directive into all system-role messages of this conversation.
func (c *LlmConversation) ApplyLanguage(lang string) {
	instruction := "\n\nAlways respond in " + resolveLang(lang) + "."
	for i := range c.Messages {
		if c.Messages[i].Role == "system" {
			c.Messages[i].Content += instruction
		}
	}
}
