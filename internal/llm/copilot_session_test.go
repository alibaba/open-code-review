// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

func TestCopilotConversationWaitCollectsChunksAndParallelTools(t *testing.T) {
	chunkCount, firstChunk, lastChunk := int64(2), int64(0), int64(1)
	input, output := int64(23), int64(7)
	apiCall := "api-call-1"
	usage := &copilot.AssistantUsageData{InputTokens: &input, OutputTokens: &output}
	events := []copilot.SessionEvent{
		{Data: &copilot.ExternalToolRequestedData{ToolCallID: "read-1", RequestID: "request-1"}},
		{Data: &copilot.AssistantMessageData{
			MessageID: "message-1", APICallID: &apiCall, Content: "Inspect ", ChunkCount: &chunkCount, ChunkIndex: &firstChunk,
			ToolRequests: []copilot.AssistantMessageToolRequest{{ToolCallID: "read-1", Name: "file_read"}},
		}},
		{Data: usage},
		{Data: &copilot.AssistantMessageData{
			MessageID: "message-2", APICallID: &apiCall, Content: "files.", ChunkCount: &chunkCount, ChunkIndex: &lastChunk, OutputTokens: &output,
			ToolRequests: []copilot.AssistantMessageToolRequest{{ToolCallID: "read-2", Name: "file_read"}},
		}},
		{Data: &copilot.ExternalToolRequestedData{ToolCallID: "read-2", RequestID: "request-2"}},
	}
	conversation := &copilotConversation{events: make(chan copilot.SessionEvent, len(events))}
	for _, event := range events {
		conversation.events <- event
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	message, usages, pending, err := conversation.wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "Inspect files." || len(message.ToolRequests) != 2 || message.OutputTokens == nil || *message.OutputTokens != output {
		t.Fatalf("assembled message = %#v", message)
	}
	if len(usages) != 1 || usages[0] != usage {
		t.Fatalf("usage events = %#v", usages)
	}
	if !reflect.DeepEqual(pending, map[string]string{"read-1": "request-1", "read-2": "request-2"}) {
		t.Fatalf("pending OCR tools = %#v", pending)
	}
	if len(conversation.events) != 0 {
		t.Fatal("returned before all assistant chunks and external tools arrived")
	}
}

func TestCopilotConversationWaitForIdleWithoutTools(t *testing.T) {
	conversation := &copilotConversation{events: make(chan copilot.SessionEvent, 2)}
	conversation.events <- copilot.SessionEvent{Data: &copilot.AssistantMessageData{Content: "Review complete."}}
	conversation.events <- copilot.SessionEvent{Data: &copilot.SessionIdleData{}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	message, _, pending, err := conversation.wait(ctx)
	if err != nil || message == nil || message.Content != "Review complete." || len(pending) != 0 {
		t.Fatalf("plain response = (%#v, %#v, %v)", message, pending, err)
	}
}

func TestCopilotConversationWaitRejectsInvalidEvents(t *testing.T) {
	chunkCount, chunkIndex := int64(2), int64(0)
	request := copilot.SessionEvent{Data: &copilot.AssistantMessageData{
		MessageID: "message-1", ToolRequests: []copilot.AssistantMessageToolRequest{{ToolCallID: "read-1", Name: "file_read"}},
	}}
	cases := []struct {
		name   string
		events []copilot.SessionEvent
		want   string
	}{
		{name: "idle without message", events: []copilot.SessionEvent{{Data: &copilot.SessionIdleData{}}}, want: "without an assistant message"},
		{name: "missing pending request", events: []copilot.SessionEvent{request, {Data: &copilot.SessionIdleData{}}}, want: "without all pending"},
		{name: "mismatched pending ID", events: []copilot.SessionEvent{
			{Data: &copilot.ExternalToolRequestedData{ToolCallID: "unknown", RequestID: "request-1"}}, request,
			{Data: &copilot.SessionIdleData{}},
		}, want: "without all pending"},
		{name: "incomplete chunks", events: []copilot.SessionEvent{
			{Data: &copilot.AssistantMessageData{MessageID: "message-1", Content: "Partial.", ChunkCount: &chunkCount, ChunkIndex: &chunkIndex}},
			{Data: &copilot.SessionIdleData{}},
		}, want: "without all pending"},
		{name: "missing tool ID", events: []copilot.SessionEvent{{Data: &copilot.ExternalToolRequestedData{RequestID: "request-1"}}}, want: "invalid pending"},
		{name: "missing request ID", events: []copilot.SessionEvent{{Data: &copilot.ExternalToolRequestedData{ToolCallID: "read-1"}}}, want: "invalid pending"},
		{name: "unannounced tool", events: []copilot.SessionEvent{request,
			{Data: &copilot.ExternalToolRequestedData{ToolCallID: "shell-1", RequestID: "request-1"}},
		}, want: "unannounced tool call"},
		{name: "advanced past OCR tool", events: []copilot.SessionEvent{request,
			{Data: &copilot.AssistantMessageData{MessageID: "message-2", Content: "Continued without OCR."}},
		}, want: "advanced past an OCR tool request"},
		{name: "upstream error", events: []copilot.SessionEvent{{Data: &copilot.SessionErrorData{Message: "authentication failed"}}}, want: "authentication failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conversation := &copilotConversation{events: make(chan copilot.SessionEvent, len(tc.events))}
			for _, event := range tc.events {
				conversation.events <- event
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			message, usages, pending, err := conversation.wait(ctx)
			if err == nil || !strings.Contains(err.Error(), tc.want) || message != nil || usages != nil || pending != nil {
				t.Fatalf("invalid event result = (%#v, %#v, %#v, %v), want %q", message, usages, pending, err, tc.want)
			}
		})
	}
}

func TestCopilotConversationWaitCancellationAndOverflow(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		conversation := &copilotConversation{}
		if _, _, _, err := conversation.wait(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled wait = %v", err)
		}
	})
	t.Run("overflow", func(t *testing.T) {
		conversation := &copilotConversation{overflow: make(chan struct{}, 1)}
		conversation.overflow <- struct{}{}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, _, _, err := conversation.wait(ctx); err == nil || !strings.Contains(err.Error(), "buffer overflowed") {
			t.Fatalf("overflowed wait = %v", err)
		}
	})
}

func TestCopilotConversationContinuationRequiresAllToolResults(t *testing.T) {
	start := []Message{NewTextMessage("system", "Review the patch."), NewTextMessage("user", "Inspect files.")}
	calls := []ToolCall{
		{ID: "read-1", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{"path":"a.go"}`}},
		{ID: "read-2", Type: "function", Function: FunctionCall{Name: "file_read", Arguments: `{"path":"b.go"}`}},
	}
	continued := append(append([]Message(nil), start...), NewToolCallMessage("", calls, NativeTurn{}, ""),
		NewToolResultMessage("read-2", "file b"), NewToolResultMessage("read-1", "file a"))
	newConversation := func() *copilotConversation {
		return &copilotConversation{
			session: &copilot.Session{}, model: "auto", system: "Review the patch.", toolsKey: "tools", maxTokens: 128,
			history: copilotHistorySnapshot(start), pending: map[string]string{"read-1": "request-1", "read-2": "request-2"},
		}
	}
	req := ChatRequest{Messages: continued, MaxTokens: 128}
	results, ok := newConversation().continuation(req, "auto", "Review the patch.", "tools")
	if !ok || !reflect.DeepEqual(results, map[string]string{"read-1": "file a", "read-2": "file b"}) {
		t.Fatalf("parallel continuation = (%#v, %t)", results, ok)
	}
	cases := []struct {
		name   string
		change func(*copilotConversation, *ChatRequest)
	}{
		{name: "missing result", change: func(_ *copilotConversation, req *ChatRequest) { req.Messages = continued[:len(continued)-1] }},
		{name: "unknown result", change: func(_ *copilotConversation, req *ChatRequest) {
			req.Messages = append(append([]Message(nil), continued...), NewToolResultMessage("unknown", "unexpected"))
		}},
		{name: "no pending tools", change: func(c *copilotConversation, _ *ChatRequest) { c.pending = nil }},
		{name: "no new turn", change: func(_ *copilotConversation, req *ChatRequest) { req.Messages = start }},
		{name: "model changed", change: func(c *copilotConversation, _ *ChatRequest) { c.model = "another-model" }},
		{name: "system changed", change: func(c *copilotConversation, _ *ChatRequest) { c.system = "Another review." }},
		{name: "limit changed", change: func(_ *copilotConversation, req *ChatRequest) { req.MaxTokens = 64 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conversation := newConversation()
			changed := req
			tc.change(conversation, &changed)
			if _, ok := conversation.continuation(changed, "auto", "Review the patch.", "tools"); ok {
				t.Fatal("continued a stale or incomplete SDK conversation")
			}
		})
	}
}

func TestCopilotCloseSessionReleasesState(t *testing.T) {
	stateDir := t.TempDir()
	unsubscribed := 0
	conversation := &copilotConversation{
		stateDir: stateDir, unsubscribe: func() { unsubscribed++ },
		events: make(chan copilot.SessionEvent, 1), overflow: make(chan struct{}, 1),
		history: copilotHistorySnapshot([]Message{NewTextMessage("user", "Inspect files.")}), pending: map[string]string{"read-1": "request-1"},
	}
	client := NewCopilotClient(ClientConfig{Model: "auto"})
	client.sessions.Store("review", conversation)
	client.CloseSession("")
	client.CloseSession("missing")
	if _, kept := client.sessions.Load("review"); !kept {
		t.Fatal("closing an unrelated session removed the active review")
	}
	client.CloseSession("review")
	client.CloseSession("review")
	if _, kept := client.sessions.Load("review"); kept || unsubscribed != 1 {
		t.Fatalf("session retained = %t, unsubscribe calls = %d", kept, unsubscribed)
	}
	if _, err := os.Stat(stateDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session directory remains: %v", err)
	}
	if conversation.events != nil || conversation.overflow != nil || conversation.history != nil || conversation.pending != nil {
		t.Fatal("closed session retained conversation state")
	}
}

func TestCopilotDiscardConversationKeepsReplacement(t *testing.T) {
	client := NewCopilotClient(ClientConfig{Model: "auto"})
	old, replacement := &copilotConversation{}, &copilotConversation{}
	client.sessions.Store("review", replacement)
	client.discardConversation("review", old)
	if current, ok := client.sessions.Load("review"); !ok || current != replacement {
		t.Fatal("discarding stale state removed the replacement session")
	}
	client.discardConversation("review", replacement)
	if _, ok := client.sessions.Load("review"); ok {
		t.Fatal("discarded conversation remains registered")
	}
}

func TestCopilotCloseSessionInterruptsActiveRequest(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conversation := &copilotConversation{}
	conversation.mu.Lock()
	ctx, finishRequest := conversation.requestContext(parent)
	client := NewCopilotClient(ClientConfig{Model: "auto"})
	client.sessions.Store("active", conversation)
	finished := make(chan error, 1)
	go func() {
		_, _, _, err := conversation.wait(ctx)
		finishRequest()
		conversation.mu.Unlock()
		finished <- err
	}()
	closed := make(chan struct{})
	go func() {
		client.CloseSession("active")
		close(closed)
	}()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("interrupted request = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("CloseSession did not interrupt the request holding the state lock")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("CloseSession did not release the interrupted conversation")
	}
	if parent.Err() != nil {
		t.Fatal("session close relied on cancelling the caller's context")
	}
}

func TestCopilotClosedConversationCannotStartCLI(t *testing.T) {
	client := NewCopilotClient(ClientConfig{Model: "auto"})
	client.cliPath = filepath.Join(t.TempDir(), "missing-copilot")
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	unsubscribed := 0
	conversation := &copilotConversation{stateDir: stateDir, unsubscribe: func() { unsubscribed++ }}
	conversation.cancelRequest()
	client.sessions.Store("closed", conversation)
	_, err := client.CompletionsWithCtx(context.Background(), ChatRequest{
		SessionID: "closed", Messages: []Message{NewTextMessage("user", "Inspect file.go")},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("closed conversation = %v, want cancellation before CLI startup", err)
	}
	if _, kept := client.sessions.Load("closed"); kept {
		t.Fatal("closed conversation remains registered")
	}
	if _, err := os.Stat(stateDir); !errors.Is(err, os.ErrNotExist) || unsubscribed != 1 {
		t.Fatalf("closed conversation cleanup = (%v, %d)", err, unsubscribed)
	}
}

func TestCopilotConversationWaitUsesAPITokenTotal(t *testing.T) {
	total, smaller := int64(12), int64(9)
	cases := []struct {
		name   string
		tokens []*int64
	}{
		{name: "repeated total", tokens: []*int64{&total, &total}},
		{name: "lower final report", tokens: []*int64{&total, &smaller}},
		{name: "higher final report", tokens: []*int64{&smaller, &total}},
		{name: "missing final report", tokens: []*int64{&total, nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conversation := &copilotConversation{events: make(chan copilot.SessionEvent, len(tc.tokens)+1)}
			count := int64(len(tc.tokens))
			for i, tokens := range tc.tokens {
				index := int64(i)
				conversation.events <- copilot.SessionEvent{Data: &copilot.AssistantMessageData{
					MessageID: "message-1", Content: "Review ", ChunkIndex: &index, ChunkCount: &count, OutputTokens: tokens,
				}}
			}
			conversation.events <- copilot.SessionEvent{Data: &copilot.SessionIdleData{}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			message, usage, _, err := conversation.wait(ctx)
			if err != nil {
				t.Fatal(err)
			}
			response, err := copilotResponse(message, usage, "Review the patch.", "Inspect file.go", nil, nil, "gpt-4.1")
			if err != nil || response.Usage.CompletionTokens != total {
				t.Fatalf("chunk token total = (%#v, %v), want %d", response, err, total)
			}
		})
	}
}

func TestCopilotHistorySnapshotDetectsMutation(t *testing.T) {
	cases := []struct {
		name   string
		prefix Message
		mutate func(*Message)
	}{
		{name: "content blocks", prefix: Message{Role: "user", Content: []ContentBlock{{Type: "text", Text: "Original text."}}}, mutate: func(m *Message) {
			m.Content.([]ContentBlock)[0].Text = "Compressed text."
		}},
		{name: "tool arguments", prefix: NewToolCallMessage("", []ToolCall{{ID: "prior-1", Function: FunctionCall{Name: "file_read", Arguments: `{"path":"a.go"}`}}}, NativeTurn{}, ""), mutate: func(m *Message) {
			m.ToolCalls[0].Function.Arguments = `{"path":"b.go"}`
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix := []Message{tc.prefix}
			conversation := &copilotConversation{
				session: &copilot.Session{}, model: "auto", system: "system", toolsKey: "tools",
				history: copilotHistorySnapshot(prefix), pending: map[string]string{"read-1": "request-1"},
			}
			messages := append(prefix, NewToolCallMessage("", []ToolCall{{ID: "read-1", Function: FunctionCall{Name: "file_read"}}}, NativeTurn{}, ""),
				NewToolResultMessage("read-1", "file contents"))
			tc.mutate(&messages[0])
			if _, ok := conversation.continuation(ChatRequest{Messages: messages}, "auto", "system", "tools"); ok {
				t.Fatal("mutable input changed the stored history snapshot")
			}
		})
	}
}

func TestCopilotHistoryIgnoresUnusedProviderState(t *testing.T) {
	prefix := []Message{NewTextMessage("user", "Inspect file.go"),
		NewToolCallMessage("", []ToolCall{{ID: "prior-1", Function: FunctionCall{Name: "file_read"}}}, NativeTurn{}, ""),
	}
	conversation := &copilotConversation{
		session: &copilot.Session{}, model: "auto", system: "system", toolsKey: "tools",
		history: copilotHistorySnapshot(prefix), pending: map[string]string{"read-1": "request-1"},
	}
	prefix[0].Native = NativeTurn{Family: "other-provider", Payload: make(chan struct{})}
	prefix[0].ReasoningContent = "Display-only state."
	prefix[1].ToolCalls[0].ExtraContent = []byte(`{"signature":"unused"}`)
	messages := append(prefix, NewToolCallMessage("", []ToolCall{{ID: "read-1", Function: FunctionCall{Name: "file_read"}}}, NativeTurn{}, ""),
		NewToolResultMessage("read-1", "file contents"))
	results, ok := conversation.continuation(ChatRequest{Messages: messages}, "auto", "system", "tools")
	if !ok || results["read-1"] != "file contents" {
		t.Fatalf("unused provider state interrupted continuation: (%#v, %t)", results, ok)
	}
}
