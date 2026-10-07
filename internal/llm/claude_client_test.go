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

func fakeClaudeClient(mode string) {
	for _, key := range claudeBlockedTestEnv {
		if os.Getenv(key) != "" {
			os.Exit(30)
		}
	}
	if os.Args[1] == "auth" {
		if mode == "claude-auth-error" {
			fmt.Fprintln(os.Stdout, "bad-json")
			return
		}
		method := "claude.ai"
		if mode == "claude-api-key" {
			method = "api_key"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"loggedIn": mode != "claude-signed-out", "authMethod": method})
		return
	}
	flag := func(name string) (string, bool) {
		for i, arg := range os.Args[1:] {
			if arg == name && i+2 < len(os.Args) {
				return os.Args[i+2], true
			}
		}
		return "", false
	}
	for _, name := range []string{"--safe-mode", "--restricted", "--strict-mcp-config", "--no-session-persistence", "--no-chrome"} {
		if !strings.Contains(strings.Join(os.Args, " "), name) {
			os.Exit(31)
		}
	}
	if tools, ok := flag("--tools"); !ok || tools != "" {
		os.Exit(32)
	}
	if os.Getenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS") != "2048" {
		os.Exit(37)
	}
	systemPath, _ := flag("--system-prompt-file")
	system, err := os.ReadFile(systemPath)
	if err != nil || !bytes.Contains(system, []byte("Review code")) || !bytes.Contains(system, []byte(claudeOCRInstructions)) {
		os.Exit(33)
	}
	prompt, _ := io.ReadAll(os.Stdin)
	var request ChatRequest
	if json.Unmarshal(prompt, &request) != nil || len(request.Messages) == 0 || request.Messages[0].Role != "user" {
		os.Exit(34)
	}
	if mode == "claude-replay" && (!bytes.Contains(prompt, []byte("result-of-test-call")) || !bytes.Contains(prompt, []byte("tool_call_id"))) {
		os.Exit(35)
	}
	if mode == "claude-none" && len(request.Tools) > 0 {
		os.Exit(36)
	}
	if mode == "claude-timeout" {
		time.Sleep(time.Minute)
		return
	}
	if mode == "claude-malformed" {
		fmt.Fprintln(os.Stdout, "bad-json")
		return
	}
	decision := map[string]any{"content": "review complete", "tool_calls": []any{}}
	if mode == "claude-tool" || mode == "claude-unknown-tool" || mode == "claude-invalid-arguments" {
		name, arguments := "search_code", `{"query":"auth"}`
		if mode == "claude-unknown-tool" {
			name = "unavailable"
		}
		if mode == "claude-invalid-arguments" {
			arguments = "null"
		}
		decision["tool_calls"] = []any{map[string]any{"name": name, "arguments": arguments}}
	}
	if mode == "claude-empty" {
		decision["content"] = ""
	}
	result := map[string]any{"subtype": "success", "is_error": false, "structured_output": decision, "usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "cache_read_input_tokens": 3, "cache_creation_input_tokens": 2}}
	if mode == "claude-missing-decision" {
		delete(result, "structured_output")
	}
	if mode == "claude-cli-error" {
		result["is_error"], result["subtype"], result["result"] = true, "error_during_execution", "test service error"
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if mode == "claude-cli-error" {
		os.Exit(1)
	}
}

var claudeBlockedTestEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_EXTRA_BODY", "CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR", "CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CODE_SIMPLE"}

func claudeTestRequest() ChatRequest {
	return ChatRequest{MaxTokens: 2048, Messages: []Message{{Role: "system", Content: "Review code"}, {Role: "user", Content: "Find bugs"}}, Tools: []ToolDef{{Function: FunctionDef{Name: "search_code", Parameters: map[string]any{"type": "object"}}}}}
}

func TestClaudeOAuthRoundTrip(t *testing.T) {
	for _, mode := range []string{"claude-text", "claude-tool", "claude-replay", "claude-none"} {
		t.Run(mode, func(t *testing.T) {
			fakeOAuthExecutable(t, mode)
			for _, key := range claudeBlockedTestEnv {
				t.Setenv(key, "must-not-reach-Claude")
			}
			t.Setenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS", "9999")
			req := claudeTestRequest()
			if mode == "claude-tool" {
				req.ToolChoice = "required"
			}
			if mode == "claude-none" {
				req.ToolChoice = "none"
			}
			if mode == "claude-replay" {
				req.Messages = append(req.Messages, Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "previous-call", Type: "function", Function: FunctionCall{Name: "search_code", Arguments: `{}`}}}}, Message{Role: "tool", ToolCallID: "previous-call", Content: "result-of-test-call"})
			}
			client := NewLLMClient(ResolvedEndpoint{Protocol: ProtocolAnthropicOAuth, Model: "test-model"}, nil, nil)
			if _, ok := client.(*ClaudeOAuthClient); !ok {
				t.Fatalf("subscription provider selected %T", client)
			}
			resp, err := client.CompletionsWithCtx(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "claude-tool" {
				calls := resp.ToolCalls()
				if len(calls) != 1 || !strings.HasPrefix(calls[0].ID, "ocr_claude_") || calls[0].Function.Arguments != `{"query":"auth"}` {
					t.Fatalf("unexpected tool calls: %+v", calls)
				}
			} else if resp.Content() != "review complete" || resp.Usage == nil || resp.Usage.TotalTokens != 20 || resp.Usage.CacheReadTokens != 3 || resp.Model != "test-model" {
				t.Fatalf("unexpected response: %+v", resp)
			}
		})
	}
}

func TestClaudeOAuthFailures(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"claude-api-key", "subscription login required"}, {"claude-signed-out", "subscription login required"}, {"claude-auth-error", "subscription login required"},
		{"claude-malformed", "decode Claude Code result"}, {"claude-cli-error", "test service error"}, {"claude-unknown-tool", "unavailable OCR tool"},
		{"claude-invalid-arguments", "unavailable OCR tool"}, {"claude-empty", "without an assistant"}, {"claude-missing-decision", "no structured assistant"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			fakeOAuthExecutable(t, tc.mode)
			_, err := NewClaudeOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), claudeTestRequest())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
	for _, req := range []ChatRequest{{ToolChoice: "invalid"}, {ToolChoice: "required"}, {Temperature: new(float64)}} {
		if _, err := NewClaudeOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), req); err == nil {
			t.Fatal("unsupported request accepted")
		}
	}
	fakeOAuthExecutable(t, "claude-text")
	req := claudeTestRequest()
	req.ToolChoice = "required"
	if _, err := NewClaudeOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), req); err == nil || !strings.Contains(err.Error(), "required OCR tool") {
		t.Fatalf("required tool error = %v", err)
	}
	req = claudeTestRequest()
	req.Messages[1].Content = func() {}
	if _, err := NewClaudeOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), req); err == nil || !strings.Contains(err.Error(), "encode Claude Code") {
		t.Fatalf("invalid conversation error = %v", err)
	}
	t.Setenv("OCR_CLAUDE_PATH", filepath.Join(t.TempDir(), "missing"))
	if _, err := NewClaudeOAuthClient(ClientConfig{}).CompletionsWithCtx(context.Background(), claudeTestRequest()); err == nil {
		t.Fatal("missing executable accepted")
	}
}

func TestClaudeOAuthCancellation(t *testing.T) {
	fakeOAuthExecutable(t, "claude-timeout")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := NewClaudeOAuthClient(ClientConfig{}).CompletionsWithCtx(ctx, claudeTestRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestClaudeOAuthLiveRoundTrip(t *testing.T) {
	if os.Getenv("OCR_TEST_LIVE_CLAUDE") != "1" {
		t.Skip("set OCR_TEST_LIVE_CLAUDE=1 with an existing Claude subscription login")
	}
	client := NewClaudeOAuthClient(ClientConfig{Model: "sonnet", Timeout: 90 * time.Second})
	req := ChatRequest{MaxTokens: 4096, ToolChoice: "required", Messages: []Message{
		{Role: "system", Content: "This is a synthetic connection test. Call self_test once with message ping. After its result, answer exactly pong without another tool call."},
		{Role: "user", Content: "Run the connection test."},
	}, Tools: []ToolDef{{Type: "function", Function: FunctionDef{Name: "self_test", Description: "Returns pong to verify OCR tool replay", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string", "enum": []string{"ping"}}}, "required": []string{"message"},
	}}}}}
	resp, err := client.CompletionsWithCtx(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	calls := resp.ToolCalls()
	if len(calls) != 1 || calls[0].Function.Name != "self_test" {
		t.Fatalf("expected one self_test call, received %d calls", len(calls))
	}
	var args struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(calls[0].Function.Arguments), &args) != nil || args.Message != "ping" {
		t.Fatal("self_test did not receive ping")
	}
	req.Messages = append(req.Messages, NewToolCallMessage(resp.VisibleContent(), calls, resp.Native(), ""), Message{Role: "tool", ToolCallID: calls[0].ID, Content: "pong"})
	req.ToolChoice = "none"
	resp, err = client.CompletionsWithCtx(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(resp.Content()) != "pong" || len(resp.ToolCalls()) != 0 || resp.Usage == nil || resp.Usage.TotalTokens == 0 {
		t.Fatal("subscription inference did not complete the OCR tool round trip")
	}
	t.Log("Claude subscription inference and OCR tool replay succeeded")
}
