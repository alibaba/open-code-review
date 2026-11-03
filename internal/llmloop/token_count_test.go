// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestConversationTokens(t *testing.T) {
	var counter conversationTokens
	messages := []llm.Message{msg("system", "Review this code"), msg("user", "Initial request")}
	check := func() {
		t.Helper()
		if got, want := counter.count(messages), CountMessagesTokens(messages); got != want {
			t.Fatalf("incremental count = %d, full count = %d", got, want)
		}
	}
	check()
	check()
	messages = append(messages, llm.NewToolCallMessage("Reading code", []llm.ToolCall{{ID: "read", Function: llm.FunctionCall{Name: "file_read", Arguments: `{"path":"main.go"}`}}}, llm.NativeTurn{Family: "openai-chat-completions", Payload: llm.ReasoningPayload("Inspect the entry point")}, ""))
	messages = append(messages, llm.NewToolResultMessage("read", "package main"))
	check()
	// Retry turns can append multiple messages before the next threshold check.
	messages = append(messages, msg("assistant", "Partial response"), msg("user", "Continue"))
	check()
	messages[1] = msg("user", "Compressed context with a longer summary")
	counter.reset()
	check()
	messages = messages[:2]
	counter.reset()
	check()
	messages = nil
	counter.reset()
	check()
}

func BenchmarkContextTokenCount(b *testing.B) {
	messages := []llm.Message{msg("system", strings.Repeat("Review code carefully. ", 500)), msg("user", strings.Repeat("Initial code context. ", 1000))}
	for i := 0; i < 20; i++ {
		messages = append(messages, msg("assistant", strings.Repeat("Inspect this function. ", 100)), msg("tool", strings.Repeat("File content line. ", 200)))
	}
	CountMessagesTokens(messages)
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			CountMessagesTokens(messages)
		}
	})
	b.Run("incremental", func(b *testing.B) {
		var counter conversationTokens
		counter.count(messages[:len(messages)-2])
		counted, total := counter.counted, counter.total
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			counter.counted, counter.total = counted, total
			counter.count(messages)
		}
	})
}
