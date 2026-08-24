// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
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
			{Role: "user", Content: "diff:\n{diff}\ncode:\n{existing_code}\nsuggestion:\n{suggestion_content}"},
		},
	}
}

func makeCandidateTask() *template.LlmConversation {
	return &template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "select candidate"},
			{Role: "user", Content: "comment:\n{suggestion_content}\ncode:\n{existing_code}\nsuggestion:\n{suggestion_code}\nthinking:\n{thinking}\ncandidates:\n{candidates}"},
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

	msgs := BuildReLocationMessages(&cm, d, makeTask())
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages")
	}

	ok, resp := ReLocateComment(context.Background(), &cm, d, client, msgs, "test-model", 1000)
	if !ok {
		t.Fatal("expected re-location to succeed")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if cm.StartLine == 0 || cm.EndLine == 0 {
		t.Fatalf("expected non-zero lines after re-location, got %d-%d", cm.StartLine, cm.EndLine)
	}
}

func TestReLocateCommentCandidate_SelectsPrecomputedLocation(t *testing.T) {
	cm := model.LlmComment{
		Path:           "main.go",
		Content:        "The second branch should not fail after success.",
		ExistingCode:   "status = failed",
		SuggestionCode: "status = succeeded",
	}
	candidates := []CommentLocationCandidate{
		{ID: "1", Path: "main.go", StartLine: 10, EndLine: 10, Snippet: "status = failed", Context: "if err != nil {\nstatus = failed\n}"},
		{ID: "2", Path: "main.go", StartLine: 42, EndLine: 44, Snippet: "status = failed", Context: "if remoteStatus == \"\" {\nstatus = failed\n}"},
	}
	client := &mockLLMClient{response: newMockResponse(`{"candidate_id":2}`)}
	msgs := BuildCandidateReLocationMessages(&cm, candidates, makeCandidateTask())
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	if !strings.Contains(msgs[1].ExtractText(), "remoteStatus") {
		t.Fatalf("candidate prompt did not include context: %s", msgs[1].ExtractText())
	}

	ok, resp := ReLocateCommentCandidate(context.Background(), &cm, candidates, client, msgs, "test-model", 1000)
	if !ok {
		t.Fatal("expected candidate re-location to succeed")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if cm.StartLine != 42 || cm.EndLine != 44 {
		t.Fatalf("lines = %d-%d, want 42-44", cm.StartLine, cm.EndLine)
	}
}

func TestBuildCandidateReLocationMessages_RendersCandidatesAsJSON(t *testing.T) {
	cm := model.LlmComment{
		Path:         "main.go",
		Content:      "The second branch is wrong.",
		ExistingCode: "target()",
	}
	candidates := []CommentLocationCandidate{
		{
			ID:        "1",
			Path:      "main.go",
			StartLine: 10,
			EndLine:   12,
			Snippet:   "fmt.Println(```)",
			Context:   "payload := `{\"candidate_id\":1}`\nfmt.Println(```)",
		},
	}

	msgs := BuildCandidateReLocationMessages(&cm, candidates, makeCandidateTask())
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	text := msgs[1].ExtractText()
	marker := "candidates:\n"
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatalf("candidate prompt missing candidate section: %s", text)
	}
	candidateJSON := text[start+len(marker):]
	if strings.Contains(candidateJSON, "matched code:\n```") || strings.Contains(candidateJSON, "candidate context:\n```") {
		t.Fatalf("candidate list should not use Markdown fences as structure: %s", candidateJSON)
	}
	var rendered []struct {
		CandidateID string `json:"candidate_id"`
		MatchedCode string `json:"matched_code"`
		Context     string `json:"context"`
	}
	if err := json.Unmarshal([]byte(candidateJSON), &rendered); err != nil {
		t.Fatalf("candidate list is not valid JSON: %v\n%s", err, candidateJSON)
	}
	if len(rendered) != 1 || rendered[0].CandidateID != "1" {
		t.Fatalf("rendered candidates = %+v, want candidate 1", rendered)
	}
	if rendered[0].MatchedCode != "fmt.Println(```)" || !strings.Contains(rendered[0].Context, `{"candidate_id":1}`) {
		t.Fatalf("rendered candidate lost code content: %+v", rendered[0])
	}
}

func TestBuildCandidateReLocationMessages_DoesNotExpandInsertedPlaceholders(t *testing.T) {
	cm := model.LlmComment{
		Path:           "main.go",
		Content:        "Do not replace this literal token: {candidates}",
		ExistingCode:   "target()",
		SuggestionCode: "Do not replace this literal token either: {thinking}",
		Thinking:       "Keep {existing_code} as plain text.",
	}
	candidates := []CommentLocationCandidate{{
		ID:        "1",
		Path:      "main.go",
		StartLine: 10,
		EndLine:   10,
		Snippet:   "target()",
	}}

	msgs := BuildCandidateReLocationMessages(&cm, candidates, makeCandidateTask())
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	text := msgs[1].ExtractText()
	if !strings.Contains(text, "Do not replace this literal token: {candidates}") {
		t.Fatalf("suggestion content placeholder literal was re-expanded: %s", text)
	}
	if !strings.Contains(text, "Do not replace this literal token either: {thinking}") {
		t.Fatalf("suggestion code placeholder literal was re-expanded: %s", text)
	}
	if !strings.Contains(text, "Keep {existing_code} as plain text.") {
		t.Fatalf("thinking placeholder literal was re-expanded: %s", text)
	}
}

func TestReLocateCommentCandidate_NullCandidateDeclines(t *testing.T) {
	cm := model.LlmComment{Path: "main.go", Content: "issue", ExistingCode: "x"}
	candidates := []CommentLocationCandidate{{ID: "1", Path: "main.go", StartLine: 1, EndLine: 1, Snippet: "x"}}
	client := &mockLLMClient{response: newMockResponse(`{"candidate_id":null}`)}

	ok, resp := ReLocateCommentCandidate(context.Background(), &cm, candidates, client, BuildCandidateReLocationMessages(&cm, candidates, makeCandidateTask()), "test-model", 1000)
	if ok {
		t.Fatal("expected null candidate to decline")
	}
	if resp == nil {
		t.Fatal("expected response to be recorded")
	}
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Fatalf("lines = %d-%d, want 0-0", cm.StartLine, cm.EndLine)
	}
}

func TestReLocateCommentCandidate_StrictParserRejectsProse(t *testing.T) {
	cm := model.LlmComment{Path: "main.go", Content: "issue", ExistingCode: "x"}
	candidates := []CommentLocationCandidate{{ID: "2", Path: "main.go", StartLine: 2, EndLine: 2, Snippet: "x"}}
	client := &mockLLMClient{response: newMockResponse("The candidate is 2.")}

	ok, resp := ReLocateCommentCandidate(context.Background(), &cm, candidates, client, BuildCandidateReLocationMessages(&cm, candidates, makeCandidateTask()), "test-model", 1000)
	if ok {
		t.Fatal("expected strict parser to reject prose")
	}
	if resp == nil {
		t.Fatal("expected response to be returned")
	}
	if cm.StartLine != 0 || cm.EndLine != 0 {
		t.Fatalf("lines = %d-%d, want 0-0", cm.StartLine, cm.EndLine)
	}
}

func TestParseCandidateID_StrictJSONObjectOnly(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantID     string
		wantParsed bool
	}{
		{name: "number id", content: `{"candidate_id":2}`, wantID: "2", wantParsed: true},
		{name: "string id", content: `{"candidate_id":"2"}`, wantID: "2", wantParsed: true},
		{name: "null id", content: `{"candidate_id":null}`, wantParsed: true},
		{name: "string null", content: `{"candidate_id":"null"}`, wantParsed: false},
		{name: "prose", content: "Candidate 2 is correct.", wantParsed: false},
		{name: "bare number", content: "2", wantParsed: false},
		{name: "fenced json", content: "```json\n{\"candidate_id\":2}\n```", wantID: "2", wantParsed: true},
		{name: "fenced json with prose", content: "Here is the answer:\n```json\n{\"candidate_id\":2}\n```", wantParsed: false},
		{name: "missing field", content: `{}`, wantParsed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotParsed := ParseCandidateID(tt.content)
			if gotID != tt.wantID || gotParsed != tt.wantParsed {
				t.Fatalf("ParseCandidateID() = %q/%v, want %q/%v", gotID, gotParsed, tt.wantID, tt.wantParsed)
			}
		})
	}
}

func TestBuildCandidateReLocationRetryMessages(t *testing.T) {
	base := []llm.Message{
		llm.NewTextMessage("system", "select"),
		llm.NewTextMessage("user", "candidates"),
	}
	got := BuildCandidateReLocationRetryMessages(base, "candidate 2")
	if len(got) != 4 {
		t.Fatalf("messages = %d, want 4", len(got))
	}
	if got[2].Role != "assistant" || got[2].ExtractText() != "candidate 2" {
		t.Fatalf("assistant retry context = %q/%q", got[2].Role, got[2].ExtractText())
	}
	if got[3].Role != "user" ||
		!strings.Contains(got[3].ExtractText(), "Return exactly one JSON object") ||
		!strings.Contains(got[3].ExtractText(), `{"candidate_id":null}`) {
		t.Fatalf("retry instruction = %q/%q", got[3].Role, got[3].ExtractText())
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
