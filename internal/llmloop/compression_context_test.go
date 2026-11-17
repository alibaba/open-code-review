// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

type compressionContextMessage struct {
	Role      string `xml:"role,attr"`
	Content   string `xml:"content"`
	Activity  string `xml:"tool_activity"`
	Reasoning string `xml:"reasoning"`
}

func compressionContextMessages(t *testing.T, text string) []compressionContextMessage {
	t.Helper()
	var document struct {
		Messages []compressionContextMessage `xml:"message"`
	}
	if err := xml.Unmarshal([]byte("<messages>"+text+"</messages>"), &document); err != nil {
		t.Fatal(err)
	}
	return document.Messages
}

func TestBuildMessageXML_ToolActivity(t *testing.T) {
	calls := []llm.ToolCall{{
		ID: "call\"\n<&>", Type: "function",
		Function:     llm.FunctionCall{Name: "code_search", Arguments: "{\"pattern\":\"</tool_activity>\\n\",\"path\":\"src/\u8def\u5f84.go\"}"},
		ExtraContent: json.RawMessage(`{"signature":"opaque-call-signature"}`),
	}}
	assistant := llm.NewToolCallMessage("", calls, llm.NativeTurn{
		Family: "test", Payload: "opaque-native-payload",
	}, "readable reasoning")
	result := llm.NewToolResultMessage(calls[0].ID, "No matches found.")
	text := buildMessageXML([]llm.Message{assistant, result})
	for _, opaque := range []string{"opaque-call-signature", "opaque-native-payload", "extra_content"} {
		if strings.Contains(text, opaque) {
			t.Errorf("compression context contains provider replay metadata %q", opaque)
		}
	}
	messages := compressionContextMessages(t, text)
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	var activity struct {
		ToolCalls []llm.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(messages[0].Activity), &activity); err != nil {
		t.Fatal(err)
	}
	want := calls[0]
	want.ExtraContent = nil
	if !reflect.DeepEqual(activity.ToolCalls, []llm.ToolCall{want}) {
		t.Errorf("tool calls = %+v, want %+v", activity.ToolCalls, want)
	}
	var reply struct {
		ToolCallID string `json:"tool_call_id"`
	}
	if err := json.Unmarshal([]byte(messages[1].Activity), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.ToolCallID != calls[0].ID || strings.TrimSpace(messages[1].Content) != "No matches found." {
		t.Errorf("tool result lost its call identity or content: %+v, %+v", messages[1], reply)
	}
	if strings.TrimSpace(messages[0].Reasoning) != assistant.ReasoningContent {
		t.Error("readable reasoning was not preserved")
	}
}

func TestBuildMessageXML_TextOnlyUnchanged(t *testing.T) {
	want := "<message id=\"0\" role=\"assistant\">\n    <content>\n      visible text\n    </content>\n    <reasoning>\n      readable reasoning\n    </reasoning>\n</message>"
	message := llm.NewToolCallMessage("visible text", nil, llm.NativeTurn{}, "readable reasoning")
	if got := buildMessageXML([]llm.Message{message}); got != want {
		t.Errorf("text-only context changed: %q", got)
	}
}

type compressionContextClient struct {
	request llm.ChatRequest
}

func (c *compressionContextClient) CompletionsWithCtx(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	c.request = request
	summary := "compressed history"
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}}}, nil
}

func TestRunCompression_PreservesToolEvidence(t *testing.T) {
	client := &compressionContextClient{}
	runner := NewRunner(Deps{
		LLMClient: client, Model: "test-model",
		Session: session.New(t.TempDir(), "main", "test-model", session.SessionOptions{}),
	}, Task{
		Compression: CompressionPolicy{
			Messages:        []MessageTemplate{{Role: "user", Content: "{{context}}"}},
			SummaryTemplate: "\n<summary>{{summary}}</summary>",
		},
		MaxContextTokens: 1,
	})
	calls := []llm.ToolCall{
		{ID: "search", Type: "function", Function: llm.FunctionCall{Name: "code_search", Arguments: `{"pattern":"validateUser","path":"src/user.go"}`}},
		{ID: "comment", Type: "function", Function: llm.FunctionCall{Name: "code_comment", Arguments: `{"comments":[{"content":"Missing validation","existing_code":"save(user)"}]}`}},
	}
	messages := []llm.Message{
		llm.NewTextMessage("system", "system"),
		llm.NewTextMessage("user", "review"),
		llm.NewToolCallMessage("", calls, llm.NativeTurn{}, ""),
		llm.NewToolResultMessage("comment", "Recorded 1 comment."),
		llm.NewToolResultMessage("search", "No matches found."),
	}
	rebuilt, err := runner.runCompression(context.Background(), messages, "src/user.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.request.Messages) != 1 {
		t.Fatalf("compression request messages = %d, want 1", len(client.request.Messages))
	}
	contextMessages := compressionContextMessages(t, client.request.Messages[0].ExtractText())
	if len(contextMessages) != 3 {
		t.Fatalf("compression context messages = %d, want 3", len(contextMessages))
	}
	for i, original := range messages[2:] {
		var activity struct {
			ToolCalls  []llm.ToolCall `json:"tool_calls"`
			ToolCallID string         `json:"tool_call_id"`
		}
		if err := json.Unmarshal([]byte(contextMessages[i].Activity), &activity); err != nil {
			t.Fatalf("message %d lost its tool evidence: %v", i, err)
		}
		if contextMessages[i].Role != original.Role || !reflect.DeepEqual(activity.ToolCalls, original.ToolCalls) || activity.ToolCallID != original.ToolCallID {
			t.Errorf("message %d tool evidence = %+v, want calls %+v and result ID %q", i, activity, original.ToolCalls, original.ToolCallID)
		}
	}
	if len(rebuilt) != 2 || !strings.Contains(rebuilt[1].ExtractText(), "compressed history") {
		t.Errorf("unexpected rebuilt conversation: %+v", rebuilt)
	}
}
