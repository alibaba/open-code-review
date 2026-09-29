// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/location"
	"github.com/alibaba/open-code-review/internal/model"
)

type mockLLMClient struct {
	response  *llm.ChatResponse
	err       error
	callCount int
}

func (m *mockLLMClient) CompletionsWithCtx(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.callCount++
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

func TestResolveCommentSidePreservesNewAndOldProvenance(t *testing.T) {
	tests := []struct {
		name         string
		existingCode string
		want         location.Side
	}{
		{name: "added line", existingCode: "x := 1", want: location.SideNew},
		{name: "deleted line", existingCode: "removed := true", want: location.SideOld},
	}
	d := &model.Diff{
		NewPath: "main.go",
		OldPath: "main.go",
		Diff:    "@@ -10,2 +10,2 @@\n-removed := true\n+x := 1\n fmt.Println()",
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comment := model.LlmComment{Path: "main.go", ExistingCode: tc.existingCode}
			got, ok := ResolveCommentSide(&comment, d)
			if !ok || got != tc.want {
				t.Fatalf("ResolveCommentSide = %q, %v; want %q, true", got, ok, tc.want)
			}
		})
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

	msgs := BuildReLocationMessages(&cm, d, makeTask())
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages")
	}

	side, resp := ReLocateCommentSide(context.Background(), &cm, d, client, msgs, "test-model", 1000)
	if side != location.SideNew {
		t.Fatalf("re-location side = %q, want NEW", side)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
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

	ok, resp := ReLocateComment(context.Background(), &cm, d, client, BuildReLocationMessages(&cm, d, makeTask()), "test-model", 1000)
	if ok {
		t.Fatal("expected re-location to fail for invalid LLM response")
	}
	if resp == nil {
		t.Fatal("expected non-nil response even on failure")
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

	ok, resp := ReLocateComment(context.Background(), &cm, d, client, BuildReLocationMessages(&cm, d, makeTask()), "test-model", 1000)
	if ok {
		t.Fatal("expected false on LLM error")
	}
	if resp != nil {
		t.Fatal("expected nil response on error")
	}
}

// TestBuildReLocationMessages_Rendering pins what the split moved out of
// ReLocateComment: the prompt the model receives, byte for byte. Splitting the
// function was for request ordering, so the rendering must be unchanged.
func TestBuildReLocationMessages_Rendering(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "unused variable",
		ExistingCode: "x := 1",
	}
	d := makeDiff()
	task := &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "you are a helper"},
			{Role: "user", Content: "diff:\n{diff}\ncode:\n{existing_code}\nsuggestion:\n{suggestion_content}"},
		},
	}

	msgs := BuildReLocationMessages(&cm, d, task)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].ExtractText() != "you are a helper" {
		t.Errorf("message 0 = %q/%q", msgs[0].Role, msgs[0].ExtractText())
	}
	want := "diff:\n" + d.Diff + "\ncode:\nx := 1\nsuggestion:\nunused variable"
	if msgs[1].Role != "user" || msgs[1].ExtractText() != want {
		t.Errorf("message 1 = %q/%q, want user/%q", msgs[1].Role, msgs[1].ExtractText(), want)
	}
}

func TestBuildReLocationMessages_NilOrEmptyTask(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "test",
		ExistingCode: "bad code",
	}
	d := makeDiff()

	if msgs := BuildReLocationMessages(&cm, d, nil); msgs != nil {
		t.Fatalf("expected nil messages for nil task, got %d", len(msgs))
	}
	if msgs := BuildReLocationMessages(&cm, d, &template.LlmConversation{}); msgs != nil {
		t.Fatalf("expected nil messages for task without messages, got %d", len(msgs))
	}
}

// TestReLocateComment_CodeBlockStillUnresolvable covers the rollback branch: the
// model returned a well-formed snippet that still does not appear in the diff, so
// the original ExistingCode must be restored rather than left overwritten.
func TestReLocateComment_CodeBlockStillUnresolvable(t *testing.T) {
	const original = "totally wrong code"
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "unused variable",
		ExistingCode: original,
	}
	d := makeDiff()

	client := &mockLLMClient{
		response: newMockResponse("```go\nnot in the diff either\n```"),
	}

	ok, resp := ReLocateComment(context.Background(), &cm, d, client, BuildReLocationMessages(&cm, d, makeTask()), "test-model", 1000)
	if ok {
		t.Fatal("expected false when the new snippet still does not match")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if cm.ExistingCode != original {
		t.Errorf("ExistingCode = %q, want the original %q restored", cm.ExistingCode, original)
	}
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Errorf("lines = %d-%d, want 0-0", cm.StartLine, cm.EndLine)
	}
}

// The caller skips the session record and the request entirely when there are no
// messages, so ReLocateComment must not reach the client on that path.
func TestReLocateComment_NoMessages(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "test",
		ExistingCode: "bad code",
	}
	d := makeDiff()
	client := &mockLLMClient{response: newMockResponse("```go\nx := 1\n```")}

	ok, resp := ReLocateComment(context.Background(), &cm, d, client, nil, "test-model", 1000)
	if ok {
		t.Fatal("expected false when there are no messages")
	}
	if resp != nil {
		t.Fatal("expected nil response when there are no messages")
	}
	if client.callCount != 0 {
		t.Fatalf("expected no LLM call, got %d", client.callCount)
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
