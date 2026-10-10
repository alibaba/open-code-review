// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	if mode := os.Getenv("OCR_TEST_OAUTH_CHILD"); mode != "" {
		if mode == "auth" {
			_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
			os.Exit(0)
		}
		if strings.HasPrefix(mode, "claude-") {
			fakeClaudeClient(mode)
			os.Exit(0)
		}
		fakeCodexServer(mode)
		os.Exit(0)
	}
}

func fakeCodexServer(mode string) {
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	toolResults := 0
	respond := func(p codexPacket, result any) { _ = encoder.Encode(map[string]any{"id": p.ID, "result": result}) }
	notify := func(method string, params any) {
		_ = encoder.Encode(map[string]any{"method": method, "params": params})
	}
	for {
		var p codexPacket
		if decoder.Decode(&p) != nil {
			return
		}
		if p.Method == "" && (mode == "tool" || mode == "multi-tool") {
			toolResults++
			wantResults := 1
			if mode == "multi-tool" {
				wantResults = 2
			}
			if toolResults == wantResults {
				notify("thread/tokenUsage/updated", map[string]any{"tokenUsage": map[string]any{"last": map[string]any{"inputTokens": 10, "outputTokens": 5, "cachedInputTokens": 3, "totalTokens": 15}}})
				notify("item/completed", map[string]any{"item": map[string]any{"type": "agentMessage", "text": "review complete", "phase": "final_answer"}})
				notify("turn/completed", map[string]any{"turn": map[string]any{"status": "completed"}})
			}
			continue
		}
		switch p.Method {
		case "initialize":
			if os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("CODEX_API_KEY") != "" {
				os.Exit(20)
			}
			if mode == "rpc-error" {
				_ = encoder.Encode(map[string]any{"id": p.ID, "error": map[string]any{"code": -1, "message": "test protocol error"}})
				continue
			}
			respond(p, map[string]any{})
		case "account/read":
			typeName := "chatgpt"
			if mode == "api-key" {
				typeName = "apiKey"
			}
			respond(p, map[string]any{"account": map[string]any{"type": typeName}})
		case "thread/start":
			var args struct {
				Ephemeral    bool   `json:"ephemeral"`
				Sandbox      string `json:"sandbox"`
				Environments []any  `json:"environments"`
			}
			_ = json.Unmarshal(p.Params, &args)
			if !args.Ephemeral || args.Sandbox != "read-only" || len(args.Environments) != 0 {
				os.Exit(21)
			}
			respond(p, map[string]any{"thread": map[string]any{"id": "thread-test"}})
		case "thread/inject_items":
			var history struct {
				Items []map[string]any `json:"items"`
			}
			_ = json.Unmarshal(p.Params, &history)
			for _, item := range history.Items {
				if item["type"] == "message" {
					if _, ok := item["content"].([]any); !ok {
						os.Exit(23)
					}
				}
			}
			if mode == "replay" && (!bytes.Contains(p.Params, []byte("function_call_output")) || !bytes.Contains(p.Params, []byte("result-of-test-call"))) {
				os.Exit(22)
			}
			respond(p, map[string]any{})
		case "turn/start":
			if mode == "timeout" {
				time.Sleep(time.Minute)
				return
			}
			if mode == "malformed" {
				fmt.Fprintln(os.Stdout, "invalid-json")
				return
			}
			if mode == "tool" || mode == "multi-tool" || mode == "unknown-tool" {
				name := "search_code"
				if mode == "unknown-tool" {
					name = "unavailable"
				}
				_ = encoder.Encode(map[string]any{"id": "server-call", "method": "item/tool/call", "params": map[string]any{"callId": "call-123", "tool": name, "arguments": map[string]any{"query": "auth"}}})
				if mode == "multi-tool" {
					_ = encoder.Encode(map[string]any{"id": "server-call-2", "method": "item/tool/call", "params": map[string]any{"callId": "call-456", "tool": name, "arguments": map[string]any{"query": "providers"}}})
				}
				respond(p, map[string]any{"turn": map[string]any{"id": "turn-test"}})
				continue
			}
			if mode == "builtin" {
				notify("item/started", map[string]any{"item": map[string]any{"type": "commandExecution"}})
			}
			notify("item/completed", map[string]any{"item": map[string]any{"type": "agentMessage", "text": "review progress", "phase": "commentary"}})
			notify("item/completed", map[string]any{"item": map[string]any{"type": "agentMessage", "text": "review complete", "phase": "final_answer"}})
			notify("thread/tokenUsage/updated", map[string]any{"tokenUsage": map[string]any{"last": map[string]any{"inputTokens": 10, "outputTokens": 5, "cachedInputTokens": 3, "totalTokens": 15}}})
			respond(p, map[string]any{"turn": map[string]any{"id": "turn-test"}})
			status := "completed"
			if mode == "failed" {
				status = "failed"
			}
			notify("turn/completed", map[string]any{"turn": map[string]any{"status": status}})
		}
	}
}

func fakeOAuthExecutable(t *testing.T, mode string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCR_CODEX_PATH", executable)
	t.Setenv("OCR_CLAUDE_PATH", executable)
	t.Setenv("OCR_TEST_OAUTH_CHILD", mode)
}

func TestCodexOAuthRoundTrip(t *testing.T) {
	for _, mode := range []string{"text", "tool", "multi-tool", "replay"} {
		t.Run(mode, func(t *testing.T) {
			fakeOAuthExecutable(t, mode)
			t.Setenv("OPENAI_API_KEY", "must-not-reach-codex")
			t.Setenv("CODEX_API_KEY", "must-not-reach-codex")
			req := ChatRequest{Messages: []Message{{Role: "system", Content: "Review code"}, {Role: "user", Content: "Find bugs"}}, Tools: []ToolDef{{Function: FunctionDef{Name: "search_code", Parameters: map[string]any{"type": "object"}}}}}
			if mode == "replay" {
				req.Messages = append(req.Messages, Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-123", Type: "function", Function: FunctionCall{Name: "search_code", Arguments: `{}`}}}}, Message{Role: "tool", ToolCallID: "call-123", Content: "result-of-test-call"})
			}
			client := NewCodexOAuthClient(ClientConfig{Model: "test-model"})
			resp, err := client.CompletionsWithCtx(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "tool" || mode == "multi-tool" {
				calls := resp.ToolCalls()
				wantCalls := 1
				if mode == "multi-tool" {
					wantCalls = 2
				}
				if len(calls) != wantCalls || calls[0].ID != "call-123" || calls[0].Function.Arguments != `{"query":"auth"}` {
					t.Fatalf("unexpected tool calls: %+v", calls)
				}
				if mode == "multi-tool" {
					if calls[1].ID != "call-456" || calls[1].Function.Arguments != `{"query":"providers"}` {
						t.Fatalf("unexpected second tool call: %+v", resp)
					}
				}
				toolResults := make([]Message, 0, len(calls)*2)
				toolResults = append(toolResults, Message{Role: "assistant", Content: resp.Content(), ToolCalls: calls})
				for _, call := range calls {
					toolResults = append(toolResults, Message{Role: "tool", ToolCallID: call.ID, Content: "result-of-" + call.ID})
				}
				followup := ChatRequest{Messages: append(req.Messages, toolResults...), Tools: req.Tools}
				next, err := client.CompletionsWithCtx(context.Background(), followup)
				if err != nil {
					t.Fatal(err)
				}
				if next.Content() != "review complete" || next.Usage == nil || next.Usage.TotalTokens != 15 {
					t.Fatalf("unexpected follow-up response: %+v", next)
				}
			} else if resp.Content() != "review complete" || resp.Usage == nil || resp.Usage.TotalTokens != 15 || resp.Usage.CacheReadTokens != 3 {
				t.Fatalf("unexpected response: %+v", resp)
			}
		})
	}
}

func TestCodexOAuthFailures(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{{"api-key", "ChatGPT account"}, {"rpc-error", "test protocol error"}, {"unknown-tool", "unavailable OCR tool"}, {"malformed", "decode Codex"}, {"builtin", "built-in tool"}, {"failed", "turn failed"}} {
		t.Run(tc.mode, func(t *testing.T) {
			fakeOAuthExecutable(t, tc.mode)
			_, err := NewCodexOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "test"}}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
	for _, req := range []ChatRequest{{ToolChoice: "invalid"}, {ToolChoice: "required"}, {Temperature: new(float64)}} {
		fakeOAuthExecutable(t, "text")
		if _, err := NewCodexOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), req); err == nil {
			t.Fatal("unsupported request succeeded")
		}
	}
}

func TestCodexOAuthCancellation(t *testing.T) {
	fakeOAuthExecutable(t, "timeout")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := NewCodexOAuthClient(ClientConfig{}).CompletionsWithCtx(ctx, ChatRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestOfficialAuthCommands(t *testing.T) {
	fakeOAuthExecutable(t, "auth")
	for _, tc := range []struct {
		provider, action, flag string
		headless               bool
	}{
		{"codex-oauth", "login", "--device-auth", true}, {"codex-oauth", "status", "status", false}, {"codex-oauth", "logout", "logout", false},
		{"anthropic-oauth", "login", "--claudeai", false}, {"anthropic-oauth", "status", "status", false}, {"anthropic-oauth", "logout", "logout", false},
	} {
		var output bytes.Buffer
		if err := RunOAuthAuth(context.Background(), tc.provider, tc.action, tc.headless, nil, &output, io.Discard); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), tc.flag) {
			t.Fatalf("args = %s, want %s", output.String(), tc.flag)
		}
	}
	for _, tc := range []struct {
		provider, action string
		headless         bool
	}{{"openai", "login", false}, {"codex-oauth", "invalid", false}, {"anthropic-oauth", "status", true}, {"anthropic-oauth", "login", true}} {
		if RunOAuthAuth(context.Background(), tc.provider, tc.action, tc.headless, nil, io.Discard, io.Discard) == nil {
			t.Fatal("invalid auth command succeeded")
		}
	}
	t.Setenv("OCR_CODEX_PATH", filepath.Join(t.TempDir(), "missing"))
	if RunOAuthAuth(context.Background(), "codex-oauth", "login", false, nil, io.Discard, io.Discard) == nil {
		t.Fatal("missing executable accepted")
	}
}

func TestAnthropicOAuthUsesSubscriptionLogin(t *testing.T) {
	fakeOAuthExecutable(t, "auth")
	executable, _ := os.Executable()
	t.Setenv("OCR_CLAUDE_PATH", executable)
	t.Setenv("OCR_ANT_PATH", filepath.Join(t.TempDir(), "must-not-use-console"))
	var output bytes.Buffer
	if err := RunOAuthAuth(context.Background(), "anthropic-oauth", "login", false, nil, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(output.Bytes(), &args); err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "auth login --claudeai" {
		t.Fatalf("subscription login args = %v", args)
	}
}

func TestOAuthProviderResolution(t *testing.T) {
	for _, name := range []string{"codex-oauth", "anthropic-oauth"} {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, map[string]any{"provider": name, "model": "account-specific-model", "providers": map[string]any{name: map[string]any{}}})
			ep, err := ResolveEndpoint(path)
			if err != nil {
				t.Fatal(err)
			}
			if !ep.AmbientAuth || ep.Token != "" || ep.Protocol != name {
				t.Fatalf("endpoint = %+v", ep)
			}
			for _, field := range []string{"api_key", "api_key_cmd", "url", "auth_header"} {
				path := writeConfig(t, map[string]any{"provider": name, "model": "test", "providers": map[string]any{name: map[string]any{field: "must-not-be-used"}}})
				if _, err := ResolveEndpoint(path); err == nil || !strings.Contains(err.Error(), "official OAuth credentials") {
					t.Fatalf("%s: error = %v", field, err)
				}
			}
			t.Setenv("OCR_LLM_EXTRA_HEADERS", "Authorization=Bearer must-not-be-used")
			for _, opts := range []ResolveOptions{{}, {Provider: name}} {
				if _, err := ResolveEndpointWithOptions(path, opts); err == nil || !strings.Contains(err.Error(), "OCR_LLM_EXTRA_HEADERS") {
					t.Fatalf("global OAuth header override error = %v", err)
				}
			}
		})
	}
}

func TestOAuthRelativeExecutableRoundTrip(t *testing.T) {
	for _, tc := range []struct{ protocol, key, mode string }{
		{ProtocolCodexOAuth, "OCR_CODEX_PATH", "text"},
		{ProtocolAnthropicOAuth, "OCR_CLAUDE_PATH", "claude-text"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			fakeOAuthExecutable(t, tc.mode)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			t.Chdir(dir)
			relative, err := filepath.Rel(dir, executable)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(tc.key, relative)
			client := NewLLMClient(ResolvedEndpoint{Protocol: tc.protocol, Model: "test-model"}, nil, nil)
			response, err := client.CompletionsWithCtx(context.Background(), claudeTestRequest())
			if err != nil {
				t.Fatal(err)
			}
			if response.Content() != "review complete" {
				t.Fatalf("unexpected response: %q", response.Content())
			}
		})
	}
}
