// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ClaudeOAuthClient struct{ cfg ClientConfig }

func NewClaudeOAuthClient(cfg ClientConfig) *ClaudeOAuthClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	return &ClaudeOAuthClient{cfg: cfg}
}

type claudeDecision struct {
	Content   string `json:"content"`
	ToolCalls []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"tool_calls"`
}

func claudeDecisionSchema(req ChatRequest) map[string]any {
	properties := map[string]any{
		"name":      map[string]any{"type": "string"},
		"arguments": map[string]any{"type": "string"},
	}
	names := []string{}
	if req.ToolChoice != "none" {
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
	}
	if len(names) > 0 {
		properties["name"] = map[string]any{"type": "string", "enum": names}
	}
	calls := map[string]any{"type": "array", "items": map[string]any{
		"type": "object", "properties": properties,
		"required": []string{"name", "arguments"}, "additionalProperties": false,
	}}
	if len(names) == 0 {
		calls["maxItems"] = 0
	}
	if req.ToolChoice == "required" {
		calls["minItems"] = 1
	}
	return map[string]any{"type": "object", "properties": map[string]any{
		"content": map[string]any{"type": "string"}, "tool_calls": calls,
	}, "required": []string{"content", "tool_calls"}, "additionalProperties": false}
}

const claudeOCRInstructions = `You are the assistant in the supplied OCR conversation. The input JSON contains the complete conversation history and the available OCR tool definitions. Follow its system instructions and answer the last user request.
OCR owns tool execution. Return only the next assistant decision using the output schema. To call an OCR tool, add its name and arguments to tool_calls. Each arguments value must be a JSON object serialized as a string. Do not execute tools or invent their results. Previous assistant tool calls and tool messages in the input are conversation history. When answering normally, put the complete answer in content and return an empty tool_calls array. Do not put the decision envelope itself inside content.`

func (c *ClaudeOAuthClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Temperature != nil {
		return nil, fmt.Errorf("Claude Code does not support temperature overrides")
	}
	if req.ToolChoice != "" && req.ToolChoice != "auto" && req.ToolChoice != "none" && req.ToolChoice != "required" {
		return nil, fmt.Errorf("unsupported Claude Code tool_choice %q", req.ToolChoice)
	}
	if req.ToolChoice == "required" && len(req.Tools) == 0 {
		return nil, fmt.Errorf("Claude Code tool_choice required needs at least one tool")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	path, err := oauthExecutable(ProtocolAnthropicOAuth)
	if err != nil {
		return nil, err
	}
	env := oauthEnvironment(ProtocolAnthropicOAuth)
	if err := claudeSubscriptionStatus(ctx, path, env); err != nil {
		return nil, err
	}
	if req.MaxTokens > 0 {
		for i := len(env) - 1; i >= 0; i-- {
			key, _, _ := strings.Cut(env[i], "=")
			if strings.EqualFold(key, "CLAUDE_CODE_MAX_OUTPUT_TOKENS") {
				env = append(env[:i], env[i+1:]...)
			}
		}
		env = append(env, "CLAUDE_CODE_MAX_OUTPUT_TOKENS="+strconv.Itoa(req.MaxTokens))
	}
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}
	replay := req
	replay.Messages = nil
	instructions := []string{}
	for _, message := range req.Messages {
		if message.Role == "system" || message.Role == "developer" {
			instructions = append(instructions, message.ExtractText())
		} else {
			replay.Messages = append(replay.Messages, message)
		}
	}
	instructions = append(instructions, claudeOCRInstructions)
	if req.ToolChoice == "none" {
		replay.Tools = nil
	}
	prompt, err := json.Marshal(replay)
	if err != nil {
		return nil, fmt.Errorf("encode Claude Code conversation: %w", err)
	}
	if len(prompt) > 10*1024*1024 {
		return nil, fmt.Errorf("Claude Code conversation exceeds the CLI's 10 MiB stdin limit")
	}
	schema, err := json.Marshal(claudeDecisionSchema(req))
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "ocr-claude-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	systemPath := filepath.Join(dir, "system.txt")
	if err := os.WriteFile(systemPath, []byte(strings.Join(instructions, "\n\n")), 0o600); err != nil {
		return nil, err
	}
	// Structured decisions keep OCR's existing tool loop and replay format;
	// Claude Code owns subscription authentication, refresh and inference.
	args := []string{"-p", "--safe-mode", "--restricted", "--tools", "",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--no-chrome",
		"--no-session-persistence", "--setting-sources", "", "--permission-mode", "dontAsk",
		"--permission-prompts", "none", "--output-format", "json", "--system-prompt-file", systemPath,
		"--json-schema", string(schema)}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Dir, cmd.Stdin, cmd.Stderr = env, dir, bytes.NewReader(prompt), io.Discard
	output := &claudeOutputBuffer{}
	cmd.Stdout = output
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var result struct {
		Subtype          string          `json:"subtype"`
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		Errors           []string        `json:"errors"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		Usage            struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("Claude Code invocation failed (use a current CLI): %w", runErr)
		}
		return nil, fmt.Errorf("decode Claude Code result: %w", err)
	}
	if result.IsError || runErr != nil || result.Subtype != "success" {
		return nil, fmt.Errorf("Claude Code %s: %s %s", result.Subtype, result.Result, strings.Join(result.Errors, "; "))
	}
	var decision claudeDecision
	if len(result.StructuredOutput) == 0 || string(result.StructuredOutput) == "null" {
		return nil, fmt.Errorf("Claude Code returned no structured assistant decision")
	}
	if err := json.Unmarshal(result.StructuredOutput, &decision); err != nil {
		return nil, fmt.Errorf("decode Claude Code assistant decision: %w", err)
	}
	calls := []ToolCall{}
	for _, call := range decision.ToolCalls {
		known := false
		for _, tool := range req.Tools {
			if tool.Function.Name == call.Name {
				known = true
				break
			}
		}
		var arguments map[string]any
		if !known || req.ToolChoice == "none" || json.Unmarshal([]byte(call.Arguments), &arguments) != nil || arguments == nil {
			return nil, fmt.Errorf("Claude Code requested an invalid or unavailable OCR tool")
		}
		calls = append(calls, ToolCall{ID: "ocr_claude_" + rand.Text(), Type: "function", Function: FunctionCall{Name: call.Name, Arguments: call.Arguments}})
	}
	finishReason := "stop"
	if len(calls) > 0 {
		finishReason = "tool_calls"
	} else if req.ToolChoice == "required" {
		return nil, fmt.Errorf("Claude Code did not invoke the required OCR tool")
	} else if strings.TrimSpace(decision.Content) == "" {
		return nil, fmt.Errorf("Claude Code completed without an assistant response")
	}
	u := result.Usage
	usage := &UsageInfo{PromptTokens: u.Input + u.CacheRead + u.CacheWrite, CompletionTokens: u.Output, CacheReadTokens: u.CacheRead, CacheWriteTokens: u.CacheWrite}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return &ChatResponse{Model: model, Usage: usage, Choices: []Choice{{FinishReason: finishReason, Message: ResponseMessage{Role: "assistant", Content: &decision.Content, ToolCalls: calls}}}}, nil
}

func claudeSubscriptionStatus(ctx context.Context, path string, env []string) error {
	cmd := exec.CommandContext(ctx, path, "auth", "status", "--json")
	cmd.Env, cmd.Dir, cmd.Stderr = env, os.TempDir(), io.Discard
	output := &claudeOutputBuffer{}
	cmd.Stdout = output
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var status struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
	}
	if err != nil || json.Unmarshal(output.Bytes(), &status) != nil || !status.LoggedIn || status.AuthMethod != "claude.ai" {
		return fmt.Errorf("Claude subscription login required; run 'ocr auth login anthropic-oauth' (uses 'claude auth login --claudeai')")
	}
	return nil
}

type claudeOutputBuffer struct{ bytes.Buffer }

func (b *claudeOutputBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32*1024*1024 {
		return 0, fmt.Errorf("Claude Code output exceeds 32 MiB")
	}
	return b.Buffer.Write(p)
}
