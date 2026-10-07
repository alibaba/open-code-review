// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CodexOAuthClient struct{ cfg ClientConfig }

func NewCodexOAuthClient(cfg ClientConfig) *CodexOAuthClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	return &CodexOAuthClient{cfg: cfg}
}

type codexPacket struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type codexRead struct {
	packet codexPacket
	err    error
}

type codexRPC struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	cancel  context.CancelFunc
	reads   chan codexRead
	pending []codexPacket
	nextID  int
}

func startCodexRPC(ctx context.Context) (*codexRPC, error) {
	path, err := oauthExecutable(ProtocolCodexOAuth)
	if err != nil {
		return nil, err
	}
	childCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(childCtx, path, "-c", `forced_login_method="chatgpt"`, "-c", `model_provider="openai"`, "app-server", "--listen", "stdio://")
	cmd.Env = oauthEnvironment(ProtocolCodexOAuth)
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		in.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		in.Close()
		out.Close()
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	rpc := &codexRPC{cmd: cmd, in: in, cancel: cancel, reads: make(chan codexRead, 32)}
	go func() {
		defer close(rpc.reads)
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
		for scanner.Scan() {
			var p codexPacket
			err := json.Unmarshal(scanner.Bytes(), &p)
			select {
			case rpc.reads <- codexRead{p, err}:
			case <-childCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case rpc.reads <- codexRead{err: err}:
			case <-childCtx.Done():
			}
		}
	}()
	return rpc, nil
}

func (r *codexRPC) close() {
	defer r.cancel()
	_ = r.in.Close()
	done := make(chan struct{})
	go func() { _ = r.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		r.cancel()
		<-done
	}
}

func (r *codexRPC) send(value any) error { return json.NewEncoder(r.in).Encode(value) }

func (r *codexRPC) read(ctx context.Context) (codexPacket, error) {
	select {
	case <-ctx.Done():
		return codexPacket{}, ctx.Err()
	case got, ok := <-r.reads:
		if !ok {
			return codexPacket{}, fmt.Errorf("Codex app-server closed the connection")
		}
		if got.err != nil {
			return codexPacket{}, fmt.Errorf("decode Codex app-server event: %w", got.err)
		}
		return got.packet, nil
	}
}

func (r *codexRPC) call(ctx context.Context, method string, params any, result any) error {
	r.nextID++
	id := r.nextID
	if err := r.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		p, err := r.read(ctx)
		if err != nil {
			return err
		}
		if len(p.ID) == 0 || p.Method != "" {
			r.pending = append(r.pending, p)
			continue
		}
		if string(p.ID) != fmt.Sprint(id) {
			return fmt.Errorf("unexpected Codex response id during %s", method)
		}
		if p.Error != nil {
			return fmt.Errorf("Codex %s (%d): %s", method, p.Error.Code, p.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(p.Result, result)
	}
}

func (r *codexRPC) event(ctx context.Context) (codexPacket, error) {
	if len(r.pending) > 0 {
		p := r.pending[0]
		r.pending = r.pending[1:]
		return p, nil
	}
	return r.read(ctx)
}

func (c *CodexOAuthClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Temperature != nil {
		return nil, fmt.Errorf("Codex app-server does not support temperature overrides")
	}
	if req.ToolChoice != "" && req.ToolChoice != "auto" && req.ToolChoice != "none" && req.ToolChoice != "required" {
		return nil, fmt.Errorf("unsupported Codex tool_choice %q", req.ToolChoice)
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	rpc, err := startCodexRPC(ctx)
	if err != nil {
		return nil, err
	}
	defer rpc.close()
	if err := rpc.call(ctx, "initialize", map[string]any{"clientInfo": map[string]any{"name": "open_code_review", "version": AppVersion}, "capabilities": map[string]any{"experimentalApi": true}}, nil); err != nil {
		return nil, err
	}
	if err := rpc.send(map[string]any{"method": "initialized"}); err != nil {
		return nil, err
	}
	var account struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if err := rpc.call(ctx, "account/read", map[string]any{"refreshToken": false}, &account); err != nil {
		return nil, err
	}
	if account.Account == nil || account.Account.Type != "chatgpt" {
		return nil, fmt.Errorf("Codex OAuth requires a ChatGPT account; run 'ocr auth login codex-oauth'")
	}
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}
	replay := req
	replay.Messages = append([]Message(nil), req.Messages...)
	for i := range replay.Messages {
		replay.Messages[i].Native = NativeTurn{}
	}
	params := (&OpenAIResponsesClient{}).buildResponsesParams(model, replay)
	instructions := params.Instructions.Value + "\nUse only the supplied OCR tools. Do not use built-in tools, commands, filesystem operations, web search, plugins or other agents. Return the next assistant message or request an OCR tool."
	if req.ToolChoice == "required" {
		instructions += "\nYou must invoke an OCR tool instead of answering with text."
	}
	tools := []map[string]any{}
	if req.ToolChoice != "none" {
		for _, tool := range req.Tools {
			tools = append(tools, map[string]any{"type": "function", "name": tool.Function.Name, "description": tool.Function.Description, "inputSchema": tool.Function.Parameters})
		}
	}
	if req.ToolChoice == "required" && len(tools) == 0 {
		return nil, fmt.Errorf("Codex tool_choice required needs at least one tool")
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := rpc.call(ctx, "thread/start", map[string]any{"model": model, "modelProvider": "openai", "approvalPolicy": "never", "sandbox": "read-only", "ephemeral": true, "environments": []any{}, "baseInstructions": instructions, "dynamicTools": tools}, &thread); err != nil {
		return nil, err
	}
	if thread.Thread.ID == "" {
		return nil, fmt.Errorf("Codex returned an empty thread id")
	}
	// OCR owns history and tool execution. Fresh ephemeral threads let concurrent
	// file reviews and resumed sessions replay the same complete input snapshot.
	if len(params.Input.OfInputItemList) > 0 {
		items, err := codexHistoryItems(params.Input.OfInputItemList)
		if err != nil {
			return nil, err
		}
		if err := rpc.call(ctx, "thread/inject_items", map[string]any{"threadId": thread.Thread.ID, "items": items}, nil); err != nil {
			return nil, err
		}
	}
	if err := rpc.call(ctx, "turn/start", map[string]any{"threadId": thread.Thread.ID, "input": []map[string]any{{"type": "text", "text": "Continue the supplied conversation with the next assistant response.", "text_elements": []any{}}}}, nil); err != nil {
		return nil, err
	}
	return readCodexTurn(ctx, rpc, req, model)
}

func codexHistoryItems(input any) ([]map[string]any, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	for _, item := range items {
		if role, ok := item["role"].(string); ok {
			item["type"] = "message"
			if content, ok := item["content"].(string); ok {
				kind := "input_text"
				if role == "assistant" {
					kind = "output_text"
				}
				item["content"] = []map[string]any{{"type": kind, "text": content}}
			}
		}
	}
	return items, nil
}

func readCodexTurn(ctx context.Context, rpc *codexRPC, req ChatRequest, model string) (*ChatResponse, error) {
	var textParts []string
	var finalParts []string
	var usage *UsageInfo
	for {
		p, err := rpc.event(ctx)
		if err != nil {
			return nil, err
		}
		switch p.Method {
		case "item/started":
			var event struct {
				Item struct {
					Type string `json:"type"`
				} `json:"item"`
			}
			if err := json.Unmarshal(p.Params, &event); err != nil {
				return nil, err
			}
			switch event.Item.Type {
			case "commandExecution", "fileChange", "mcpToolCall", "webSearch":
				return nil, fmt.Errorf("Codex attempted a built-in tool outside OCR's tool loop")
			}
		case "item/tool/call":
			var call struct {
				CallID    string          `json:"callId"`
				Tool      string          `json:"tool"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(p.Params, &call); err != nil {
				return nil, err
			}
			known := false
			for _, tool := range req.Tools {
				if tool.Function.Name == call.Tool {
					known = true
					break
				}
			}
			if !known || req.ToolChoice == "none" || call.CallID == "" || !json.Valid(call.Arguments) {
				return nil, fmt.Errorf("Codex requested an invalid or unavailable OCR tool")
			}
			content := strings.Join(textParts, "\n")
			return &ChatResponse{Model: model, Usage: usage, Choices: []Choice{{FinishReason: "tool_calls", Message: ResponseMessage{Role: "assistant", Content: &content, ToolCalls: []ToolCall{{ID: call.CallID, Type: "function", Function: FunctionCall{Name: call.Tool, Arguments: string(call.Arguments)}}}}}}}, nil
		case "item/completed":
			var event struct {
				Item struct {
					Type  string `json:"type"`
					Text  string `json:"text"`
					Phase string `json:"phase"`
				} `json:"item"`
			}
			if err := json.Unmarshal(p.Params, &event); err != nil {
				return nil, err
			}
			if event.Item.Type == "agentMessage" {
				textParts = append(textParts, event.Item.Text)
				if event.Item.Phase == "final_answer" {
					finalParts = append(finalParts, event.Item.Text)
				}
			}
		case "thread/tokenUsage/updated":
			var event struct {
				TokenUsage struct {
					Last struct {
						Input  int64 `json:"inputTokens"`
						Output int64 `json:"outputTokens"`
						Cached int64 `json:"cachedInputTokens"`
						Total  int64 `json:"totalTokens"`
					} `json:"last"`
				} `json:"tokenUsage"`
			}
			if err := json.Unmarshal(p.Params, &event); err != nil {
				return nil, err
			}
			u := event.TokenUsage.Last
			usage = &UsageInfo{PromptTokens: u.Input, CompletionTokens: u.Output, CacheReadTokens: u.Cached, TotalTokens: u.Total}
		case "turn/completed":
			var event struct {
				Turn struct {
					Status string `json:"status"`
					Error  *struct {
						Message string `json:"message"`
					} `json:"error"`
				} `json:"turn"`
			}
			if err := json.Unmarshal(p.Params, &event); err != nil {
				return nil, err
			}
			if event.Turn.Status != "completed" {
				if event.Turn.Error != nil {
					return nil, fmt.Errorf("Codex turn %s: %s", event.Turn.Status, event.Turn.Error.Message)
				}
				return nil, fmt.Errorf("Codex turn %s", event.Turn.Status)
			}
			if req.ToolChoice == "required" {
				return nil, fmt.Errorf("Codex did not invoke the required OCR tool")
			}
			if len(finalParts) > 0 {
				textParts = finalParts
			}
			content := strings.Join(textParts, "\n")
			if strings.TrimSpace(content) == "" {
				return nil, fmt.Errorf("Codex completed without an assistant response")
			}
			return &ChatResponse{Model: model, Usage: usage, Choices: []Choice{{FinishReason: "stop", Message: ResponseMessage{Role: "assistant", Content: &content}}}}, nil
		default:
			if len(p.ID) > 0 {
				return nil, fmt.Errorf("unexpected Codex server request %q", p.Method)
			}
		}
	}
}
