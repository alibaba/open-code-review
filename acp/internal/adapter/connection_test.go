// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

type greetingParser struct{}

func (greetingParser) CallTool(context.Context, intent.LLMRequest) (intent.ToolCall, error) {
	return intent.ToolCall{Name: "submit_intent", Arguments: []byte(`{"action":"reject","reason":"Hello! I can help review code.","hint":"Send /review or /scan."}`)}, nil
}

func TestWireRejectionPreservesVisibleGuidance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		text   string
		parser intent.LLMClient
	}{
		{name: "greeting", text: "hello", parser: greetingParser{}},
		{name: "unconfigured parser", text: "review my changes"},
		{name: "invalid command", text: "/unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &captureRunner{}
			a := NewAgent("ocr", runner, intent.NewParser(tc.parser, nil))
			serverRead, clientWrite := io.Pipe()
			clientRead, serverWrite := io.Pipe()
			defer serverRead.Close()
			defer serverWrite.Close()
			defer clientRead.Close()
			defer clientWrite.Close()
			s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			server := NewConnection(a, serverWrite, serverRead)
			defer func() { clientWrite.Close(); <-server.Done() }()
			if err := json.NewEncoder(clientWrite).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "session/prompt",
				"params": acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock(tc.text)}},
			}); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(clientRead)
			var update struct {
				Method string                  `json:"method"`
				Params acp.SessionNotification `json:"params"`
			}
			if err := decoder.Decode(&update); err != nil {
				t.Fatal(err)
			}
			if update.Method != "session/update" || update.Params.SessionId != s.SessionId || update.Params.Update.AgentMessageChunk == nil || update.Params.Update.AgentMessageChunk.Content.Text.Text == "" {
				t.Fatalf("missing visible guidance before completion: %+v", update)
			}
			var response struct {
				Result acp.PromptResponse `json:"result"`
			}
			if err := decoder.Decode(&response); err != nil {
				t.Fatal(err)
			}
			// Zed removes this turn's messages on refusal. Application guidance
			// must complete normally while preserving the non-execution cause.
			if response.Result.StopReason != acp.StopReasonEndTurn {
				t.Fatalf("guidance would be discarded: %+v", response.Result)
			}
			ocr, _ := response.Result.Meta["ocr"].(map[string]any)
			if ocr["kind"] != "rejected" || len(runner.requests) != 0 {
				t.Fatalf("rejection metadata or no-execution guarantee lost: %+v", response.Result)
			}
		})
	}
}

type wireRunner struct {
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (r *wireRunner) Run(ctx context.Context, _ orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	events := make(chan orchestrator.Event)
	outcomes := make(chan orchestrator.Outcome, 1)
	close(r.started)
	go func() {
		kind := orchestrator.OutcomeCompleted
		select {
		case <-ctx.Done():
			close(r.cancelled)
			kind = orchestrator.OutcomeCancelled
			<-r.release
		case <-r.release:
		}
		close(events)
		outcomes <- orchestrator.Outcome{Kind: kind}
		close(outcomes)
	}()
	return events, outcomes
}

func wirePair(t *testing.T, a *Agent) (*Connection, *acp.Connection, func()) {
	t.Helper()
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	server := NewConnection(a, serverWrite, serverRead)
	client := acp.NewConnection(func(context.Context, string, json.RawMessage) (any, *acp.RequestError) { return nil, nil }, clientWrite, clientRead)
	disconnect := func() { _ = clientWrite.Close() }
	t.Cleanup(func() {
		_ = clientWrite.Close()
		_ = serverWrite.Close()
		_ = serverRead.Close()
		_ = clientRead.Close()
	})
	return server, client, disconnect
}

func TestWireBusyDoesNotCancelRunningPrompt(t *testing.T) {
	r := &wireRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	a := NewAgent("ocr", r)
	_, client, _ := wirePair(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := acp.SendRequest[acp.NewSessionResponse](client, ctx, acp.AgentMethodSessionNew, acp.NewSessionRequest{Cwd: testGitDir(t), McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatal(err)
	}
	p := acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}}
	done := make(chan error, 1)
	go func() {
		resp, err := acp.SendRequest[acp.PromptResponse](client, ctx, acp.AgentMethodSessionPrompt, p)
		if err == nil && resp.StopReason != acp.StopReasonEndTurn {
			err = acp.NewInternalError(string(resp.StopReason))
		}
		done <- err
	}()
	select {
	case <-r.started:
	case <-ctx.Done():
		t.Fatal("prompt did not start")
	}
	_, err = acp.SendRequest[acp.PromptResponse](client, ctx, acp.AgentMethodSessionPrompt, p)
	if err == nil || !strings.Contains(err.Error(), "session_busy") {
		t.Fatalf("busy error: %v", err)
	}
	select {
	case <-r.cancelled:
		t.Fatal("busy request cancelled existing prompt")
	default:
	}
	close(r.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWireDisconnectShutdownWaitsForTaskCleanup(t *testing.T) {
	r := &wireRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	a := NewAgent("ocr", r)
	server, client, disconnect := wirePair(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := acp.SendRequest[acp.NewSessionResponse](client, ctx, acp.AgentMethodSessionNew, acp.NewSessionRequest{Cwd: testGitDir(t), McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = acp.SendRequest[acp.PromptResponse](client, ctx, acp.AgentMethodSessionPrompt, acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	}()
	select {
	case <-r.started:
	case <-ctx.Done():
		t.Fatal("prompt did not start")
	}
	disconnect()
	select {
	case <-server.Done():
	case <-ctx.Done():
		t.Fatal("connection did not close")
	}
	done := make(chan error, 1)
	go func() { done <- a.Shutdown(ctx) }()
	select {
	case <-r.cancelled:
	case <-ctx.Done():
		t.Fatal("task was not cancelled")
	}
	select {
	case err := <-done:
		t.Fatalf("shutdown returned before cleanup: %v", err)
	default:
	}
	close(r.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWireRejectsInvalidPromptSchema(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	_, client, _ := wirePair(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, params := range []any{map[string]any{"sessionId": "s-1", "prompt": "wrong"}, map[string]any{"sessionId": "s-1"}} {
		_, err := acp.SendRequest[acp.PromptResponse](client, ctx, acp.AgentMethodSessionPrompt, params)
		if err == nil || !strings.Contains(err.Error(), "-32602") {
			t.Fatalf("schema error = %v", err)
		}
	}
}

func TestWireInitializeAndUnsupportedMethods(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	_, client, _ := wirePair(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	init, err := acp.SendRequest[acp.InitializeResponse](client, ctx, acp.AgentMethodInitialize, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber})
	if err != nil || init.ProtocolVersion != acp.ProtocolVersionNumber {
		t.Fatalf("initialize: %+v %v", init, err)
	}
	for _, test := range []struct {
		method string
		params any
	}{
		{acp.AgentMethodSessionResume, map[string]any{"sessionId": "missing", "cwd": testGitDir(t), "mcpServers": []any{}}},
		{acp.AgentMethodSessionList, map[string]any{}},
		{"unsupported", map[string]any{}},
	} {
		_, err := acp.SendRequest[json.RawMessage](client, ctx, test.method, test.params)
		if err == nil || !strings.Contains(err.Error(), "-32601") {
			t.Fatalf("%s: %v", test.method, err)
		}
	}
}

func TestWireCancelNotificationEndsPrompt(t *testing.T) {
	r := &wireRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	a := NewAgent("ocr", r)
	_, client, _ := wirePair(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := acp.SendRequest[acp.NewSessionResponse](client, ctx, acp.AgentMethodSessionNew, acp.NewSessionRequest{Cwd: testGitDir(t), McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan acp.PromptResponse, 1)
	go func() {
		resp, _ := acp.SendRequest[acp.PromptResponse](client, ctx, acp.AgentMethodSessionPrompt, acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
		done <- resp
	}()
	select {
	case <-r.started:
	case <-ctx.Done():
		t.Fatal("prompt did not start")
	}
	for range 2 {
		if err := client.SendNotification(ctx, acp.AgentMethodSessionCancel, acp.CancelNotification{SessionId: s.SessionId}); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-r.cancelled:
	case <-ctx.Done():
		t.Fatal("cancel not delivered")
	}
	close(r.release)
	if resp := <-done; resp.StopReason != acp.StopReasonCancelled {
		t.Fatalf("stop reason: %s", resp.StopReason)
	}
	if _, err := acp.SendRequest[acp.CloseSessionResponse](client, ctx, acp.AgentMethodSessionClose, acp.CloseSessionRequest{SessionId: s.SessionId}); err != nil {
		t.Fatal(err)
	}
}
