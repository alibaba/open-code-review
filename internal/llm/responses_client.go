package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIResponsesClient sends requests to the OpenAI Responses API.
type OpenAIResponsesClient struct {
	cfg    ClientConfig
	client *http.Client
}

// NewOpenAIResponsesClient creates a new OpenAI Responses API client.
func NewOpenAIResponsesClient(cfg ClientConfig) *OpenAIResponsesClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	cfg.URL = ensureURLPathSuffix(cfg.URL, "/responses")
	return &OpenAIResponsesClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// CompletionsWithCtx sends a Responses API request with context support.
func (c *OpenAIResponsesClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	payload, err := c.buildRequestPayload(model, req)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", userAgent(""))
	c.setAuthHeader(httpReq)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, extractResponsesErrorMessage(bodyBytes))
	}

	chatResp, err := parseResponsesAPIResponse(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return chatResp, nil
}

func (c *OpenAIResponsesClient) buildRequestPayload(model string, req ChatRequest) ([]byte, error) {
	body := map[string]any{
		"model": model,
		"input": buildResponsesInput(req.Messages),
	}
	if len(req.Tools) > 0 {
		body["tools"] = buildResponsesTools(req.Tools)
	}
	if req.Temperature != nil {
		body["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_output_tokens"] = req.MaxTokens
	}
	for k, v := range c.cfg.ExtraBody {
		body[k] = v
	}
	return json.Marshal(body)
}

func (c *OpenAIResponsesClient) setAuthHeader(req *http.Request) {
	if c.cfg.APIKey == "" {
		return
	}
	if isAzureOpenAIEndpoint(c.cfg.URL) {
		req.Header.Set("api-key", c.cfg.APIKey)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
}

func buildResponsesInput(messages []Message) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "tool":
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  responseContentAsString(msg.Content),
			})
		case "assistant":
			content := msg.ExtractText()
			if content != "" {
				items = append(items, map[string]any{
					"role":    "assistant",
					"content": content,
				})
			}
			for _, tc := range msg.ToolCalls {
				items = append(items, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
		default:
			items = append(items, map[string]any{
				"role":    msg.Role,
				"content": responseContentAsString(msg.Content),
			})
		}
	}
	return items
}

func responseContentAsString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []ContentBlock:
		msg := Message{Content: v}
		return msg.ExtractText()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func buildResponsesTools(tools []ToolDef) []map[string]any {
	items := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		items = append(items, map[string]any{
			"type":        "function",
			"name":        t.Function.Name,
			"description": t.Function.Description,
			"parameters":  t.Function.Parameters,
			"strict":      false,
		})
	}
	return items
}

func parseResponsesAPIResponse(body []byte) (*ChatResponse, error) {
	type responseContent struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}
	type responseOutput struct {
		Type       string            `json:"type"`
		Role       string            `json:"role,omitempty"`
		Content    []responseContent `json:"content,omitempty"`
		ID         string            `json:"id,omitempty"`
		CallID     string            `json:"call_id,omitempty"`
		Name       string            `json:"name,omitempty"`
		Arguments  string            `json:"arguments,omitempty"`
		StopReason string            `json:"stop_reason,omitempty"`
	}
	var resp struct {
		ID                string           `json:"id"`
		Model             string           `json:"model"`
		Status            string           `json:"status,omitempty"`
		Output            []responseOutput `json:"output"`
		OutputText        string           `json:"output_text,omitempty"`
		IncompleteDetails struct {
			Reason string `json:"reason,omitempty"`
		} `json:"incomplete_details,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	var textParts []string
	var toolCalls []ToolCall
	role := "assistant"
	finishReason := ""
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			if item.Role != "" {
				role = item.Role
			}
			if item.StopReason != "" {
				finishReason = item.StopReason
			}
			for _, content := range item.Content {
				if content.Text != "" {
					textParts = append(textParts, content.Text)
				}
			}
		case "function_call":
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:   callID,
				Type: "function",
				Function: FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}
	if len(textParts) == 0 && resp.OutputText != "" {
		textParts = append(textParts, resp.OutputText)
	}

	var contentStr *string
	if len(textParts) > 0 {
		s := strings.Join(textParts, "\n")
		contentStr = &s
	}

	if finishReason == "" && resp.IncompleteDetails.Reason != "" {
		finishReason = resp.IncompleteDetails.Reason
	}
	if finishReason == "" && len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	if finishReason == "" {
		finishReason = "stop"
	}

	return &ChatResponse{
		ID:    resp.ID,
		Model: resp.Model,
		Choices: []Choice{{
			Message: ResponseMessage{
				Role:      role,
				Content:   contentStr,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
		Usage: resolveUsage(body),
	}, nil
}

func extractResponsesErrorMessage(body []byte) string {
	if len(body) == 0 {
		return "(empty body)"
	}
	var resp struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err == nil {
		switch e := resp.Error.(type) {
		case map[string]any:
			if msg, ok := e["message"].(string); ok && msg != "" {
				return msg
			}
		case string:
			if e != "" {
				return e
			}
		}
	}
	bodyText := string(body)
	if len(bodyText) > 512 {
		bodyText = bodyText[:512] + "... (truncated)"
	}
	return bodyText
}
