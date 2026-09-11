// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmresolve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
)

const (
	anthropicVersion   = "2023-06-01"
	anthropicMaxTokens = 2048
	defaultHTTPTimeout = 5 * time.Minute
	maxErrorBody       = 500
)

// Client is the phase 4 parsing client for the openai and anthropic protocols.
// Either protocol failing to accept a forced tool call is a hard error; the
// client never falls back to parsing free text.
type Client struct {
	ep   Endpoint
	http *http.Client
}

var _ intent.LLMClient = (*Client)(nil)

// NewClient validates that the resolved endpoint can be served and returns the
// client. Unsupported protocols are an explicit, actionable error.
func NewClient(ep Endpoint) (*Client, error) {
	switch ep.Protocol {
	case ProtocolOpenAI, ProtocolAnthropic:
	case ProtocolOpenAIResponses, ProtocolAnthropicBedrock:
		return nil, fmt.Errorf("the parsing client does not support protocol %q yet; configure a provider that speaks %q or %q instead", ep.Protocol, ProtocolOpenAI, ProtocolAnthropic)
	default:
		return nil, fmt.Errorf("the parsing client does not know protocol %q", ep.Protocol)
	}
	if strings.TrimSpace(ep.Model) == "" {
		return nil, fmt.Errorf("the parsing endpoint has no model configured")
	}
	if strings.TrimSpace(ep.URL) == "" || strings.TrimSpace(ep.Token) == "" {
		return nil, fmt.Errorf("the parsing endpoint is missing a url or a token")
	}
	hc := &http.Client{Timeout: defaultHTTPTimeout}
	if ep.Timeout > 0 {
		hc.Timeout = ep.Timeout
	}
	return &Client{ep: ep, http: hc}, nil
}

// Endpoint returns the resolved endpoint the client is using.
func (c *Client) Endpoint() Endpoint { return c.ep }

// CallTool implements intent.LLMClient.
func (c *Client) CallTool(ctx context.Context, req intent.LLMRequest) (intent.ToolCall, error) {
	switch c.ep.Protocol {
	case ProtocolOpenAI:
		return c.callOpenAI(ctx, req)
	case ProtocolAnthropic:
		return c.callAnthropic(ctx, req)
	default:
		return intent.ToolCall{}, fmt.Errorf("unsupported protocol %q", c.ep.Protocol)
	}
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			ToolCalls []struct {
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) callOpenAI(ctx context.Context, req intent.LLMRequest) (intent.ToolCall, error) {
	body := map[string]any{
		"model": c.ep.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        req.Tool.Name,
				"description": req.Tool.Description,
				"parameters":  json.RawMessage(req.Tool.Parameters),
			},
		}},
		"tool_choice": map[string]any{
			"type":     "function",
			"function": map[string]string{"name": req.Tool.Name},
		},
	}
	for k, v := range c.ep.ExtraBody {
		body[k] = v
	}

	data, err := c.post(ctx, openAIChatURL(c.ep.URL), body, openAIHeaders(c.ep))
	if err != nil {
		return intent.ToolCall{}, err
	}

	var resp openAIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return intent.ToolCall{}, fmt.Errorf("openai response was not valid JSON: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return intent.ToolCall{}, fmt.Errorf("openai endpoint error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return intent.ToolCall{}, fmt.Errorf("openai response contained no choices")
	}
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		return intent.ToolCall{}, fmt.Errorf("openai response contained %d tool calls, want exactly 1; the endpoint may not support forced tool calls", len(calls))
	}
	return intent.ToolCall{Name: calls[0].Function.Name, Arguments: []byte(calls[0].Function.Arguments)}, nil
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) callAnthropic(ctx context.Context, req intent.LLMRequest) (intent.ToolCall, error) {
	body := map[string]any{
		"model":      c.ep.Model,
		"max_tokens": anthropicMaxTokens,
		"system":     req.System,
		"messages": []map[string]string{
			{"role": "user", "content": req.User},
		},
		"tools": []map[string]any{{
			"name":         req.Tool.Name,
			"description":  req.Tool.Description,
			"input_schema": json.RawMessage(req.Tool.Parameters),
		}},
		"tool_choice": map[string]string{"type": "tool", "name": req.Tool.Name},
	}
	for k, v := range c.ep.ExtraBody {
		body[k] = v
	}

	data, err := c.post(ctx, anthropicMessagesURL(c.ep.URL), body, anthropicHeaders(c.ep))
	if err != nil {
		return intent.ToolCall{}, err
	}

	var resp anthropicResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return intent.ToolCall{}, fmt.Errorf("anthropic response was not valid JSON: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return intent.ToolCall{}, fmt.Errorf("anthropic endpoint error: %s", resp.Error.Message)
	}
	var found []intent.ToolCall
	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}
		found = append(found, intent.ToolCall{Name: block.Name, Arguments: block.Input})
	}
	if len(found) != 1 {
		return intent.ToolCall{}, fmt.Errorf("anthropic response contained %d tool calls, want exactly 1", len(found))
	}
	return found[0], nil
}

func (c *Client) post(ctx context.Context, rawURL string, body any, headers map[string]string) ([]byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call LLM endpoint: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read LLM response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(data))
		if len(snippet) > maxErrorBody {
			snippet = snippet[:maxErrorBody]
		}
		return nil, fmt.Errorf("LLM endpoint returned HTTP %d: %s", resp.StatusCode, snippet)
	}
	return data, nil
}

func openAIChatURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(u, "/chat/completions") {
		return u
	}
	return u + "/chat/completions"
}

func anthropicMessagesURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(u, "/v1/messages") {
		return u
	}
	return u + "/v1/messages"
}

func openAIHeaders(ep Endpoint) map[string]string {
	h := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + ep.Token,
	}
	applyExtraHeaders(h, ep.ExtraHeaders)
	return h
}

func anthropicHeaders(ep Endpoint) map[string]string {
	h := map[string]string{
		"Content-Type":      "application/json",
		"anthropic-version": anthropicVersion,
	}
	if ep.AuthHeader == "x-api-key" {
		h["x-api-key"] = ep.Token
	} else {
		h["Authorization"] = "Bearer " + ep.Token
	}
	applyExtraHeaders(h, ep.ExtraHeaders)
	return h
}

// applyExtraHeaders copies configured headers but never lets them clobber the
// auth or content headers the client manages.
func applyExtraHeaders(dst map[string]string, extra map[string]string) {
	for k, v := range extra {
		switch strings.ToLower(k) {
		case "authorization", "x-api-key", "content-type", "user-agent":
			continue
		}
		dst[k] = v
	}
}
