// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

type mockLLMClient struct {
	response *llm.ChatResponse
	err      error
}

func (m *mockLLMClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return m.response, m.err
}

func newMockResponse(content string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Choices: []llm.Choice{
			{Message: llm.ResponseMessage{Role: "assistant", Content: &content}},
		},
	}
}

func makeTask() *template.LlmConversation {
	return &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "you are a helper"},
			{Role: "user", Content: "diff:\n{diff}\n\ncomment:\n{suggestion_content}"},
		},
	}
}

func makeDiff() *model.Diff {
	return &model.Diff{
		NewPath: "main.go",
		Diff: `@@ -10,6 +10,8 @@
 import "fmt"

 func main() {
+    x := 1
+    y := 2
     fmt.Println("hello")
 }
`,
	}
}

func TestResolveComment_TextMatchSuccess(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "unused variable",
		ExistingCode: "x := 1\ny := 2",
	}
	d := makeDiff()

	ok := ResolveComment(&cm, d)
	if !ok {
		t.Fatal("expected ResolveComment to succeed")
	}
	if cm.StartLine == 0 || cm.EndLine == 0 {
		t.Fatalf("expected non-zero lines, got %d-%d", cm.StartLine, cm.EndLine)
	}
}

func TestResolveComment_AlreadyResolved(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "test",
		ExistingCode: "whatever",
		StartLine:    5,
		EndLine:      10,
	}
	d := makeDiff()
	ok := ResolveComment(&cm, d)
	if !ok {
		t.Fatal("expected true for already-resolved comment")
	}
	if cm.StartLine != 5 || cm.EndLine != 10 {
		t.Fatal("should not change already-resolved lines")
	}
}

func TestResolveComment_EmptyExistingCode(t *testing.T) {
	cm := model.LlmComment{Path: "main.go", Content: "test"}
	d := makeDiff()
	ok := ResolveComment(&cm, d)
	if ok {
		t.Fatal("expected false for empty ExistingCode")
	}
}

func TestReLocateComment_LLMReturnsValidCode(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "unused variable",
		ExistingCode: "totally wrong code that won't match",
	}
	d := makeDiff()

	client := &mockLLMClient{
		response: newMockResponse("Here is the code:\n```go\nx := 1\ny := 2\n```\n"),
	}

	ok, resp, msgs := ReLocateComment(context.Background(), &cm, d, client, makeTask(), "test-model", 1000)
	if !ok {
		t.Fatal("expected re-location to succeed")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages")
	}
	if cm.StartLine == 0 || cm.EndLine == 0 {
		t.Fatalf("expected non-zero lines after re-location, got %d-%d", cm.StartLine, cm.EndLine)
	}
}

func TestReLocateComment_LLMReturnsInvalidContent(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "unused variable",
		ExistingCode: "totally wrong code",
	}
	d := makeDiff()

	client := &mockLLMClient{
		response: newMockResponse("I cannot find the code."),
	}

	ok, resp, msgs := ReLocateComment(context.Background(), &cm, d, client, makeTask(), "test-model", 1000)
	if ok {
		t.Fatal("expected re-location to fail for invalid LLM response")
	}
	if resp == nil {
		t.Fatal("expected non-nil response even on failure")
	}
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages")
	}
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Fatal("lines should remain 0-0")
	}
}

func TestReLocateComment_LLMError(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "test",
		ExistingCode: "bad code",
	}
	d := makeDiff()

	client := &mockLLMClient{err: errors.New("network error")}

	ok, resp, msgs := ReLocateComment(context.Background(), &cm, d, client, makeTask(), "test-model", 1000)
	if ok {
		t.Fatal("expected false on LLM error")
	}
	if resp != nil {
		t.Fatal("expected nil response on error")
	}
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages even on error")
	}
}

func TestReLocateComment_NilTask(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "test",
		ExistingCode: "bad code",
	}
	d := makeDiff()
	client := &mockLLMClient{}

	ok, resp, msgs := ReLocateComment(context.Background(), &cm, d, client, nil, "test-model", 1000)
	if ok {
		t.Fatal("expected false when task is nil")
	}
	if resp != nil {
		t.Fatal("expected nil response when task is nil")
	}
	if msgs != nil {
		t.Fatal("expected nil messages when task is nil")
	}
}

func TestExtractCodeBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with language tag", "```go\nfoo\nbar\n```", "foo\nbar"},
		{"without language tag", "```\nfoo\n```", "foo"},
		{"with surrounding text", "Here:\n```\ncode\n```\ndone", "code"},
		{"no code block", "just text", ""},
		{"empty block", "```\n```", ""},
		{"opening fence without newline", "```go", ""},
		{"no closing fence", "```\nfoo\nbar", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCodeBlock(tt.input)
			if got != tt.want {
				t.Errorf("extractCodeBlock() = %q, want %q", got, tt.want)
			}
		})
	}
}
