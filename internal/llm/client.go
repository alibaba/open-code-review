// Package llm provides LLM client interfaces supporting multiple protocols.
// Supported protocols: Anthropic Messages API, OpenAI Chat Completions API.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	tiktoken "github.com/pkoukk/tiktoken-go"

	"github.com/open-code-review/open-code-review/internal/stdout"
)

const maxRetries = 10 // Maximum number of retry attempts with exponential backoff.

var AppVersion = "dev"

func userAgent(provider string) string {
	ua := "open-code-review/" + AppVersion
	if provider != "" {
		ua += " | " + provider
	}
	return ua
}

// LLMClient is the unified interface for all LLM protocol implementations.
type LLMClient interface {
	Completions(req ChatRequest) (*ChatResponse, error)
	CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	StreamCompletion(req ChatRequest, cb func(chunk []byte) error) error
}

// --- Shared data types ---

// Message represents a single message in a chat conversation.
// Content can be either plain string (for system/user/assistant/tool messages)
// or an array of content blocks (used by Claude for multi-part content).
// ToolCallID is used by OpenAI-format APIs to identify which tool call this result responds to.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`                // string or []ContentBlock
	ToolCallID string     `json:"tool_call_id,omitempty"` // OpenAI tool call identifier
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant tool invocations
}

// ContentBlock represents a single block within a multi-part message content.
// Used by Claude's Messages API for tool results and multimodal content.
type ContentBlock struct {
	Type      string         `json:"type"`                  // "text" or "tool_result"
	Text      string         `json:"text,omitempty"`        // for type="text"
	ToolUseID string         `json:"tool_use_id,omitempty"` // for type="tool_result"
	Content   []ContentBlock `json:"content,omitempty"`     // nested text blocks inside tool_result
}

// NewTextMessage creates a message with simple string content.
func NewTextMessage(role, content string) Message {
	return Message{Role: role, Content: content}
}

// NewToolCallMessage creates an assistant message with text content and tool invocations.
func NewToolCallMessage(content string, toolCalls []ToolCall) Message {
	var tc []ToolCall
	if len(toolCalls) > 0 {
		tc = make([]ToolCall, len(toolCalls))
		copy(tc, toolCalls)
	}
	return Message{Role: "assistant", Content: content, ToolCalls: tc}
}

// NewToolResultMessage creates a tool-role message with the given result.
// Uses the OpenAI Chat Completions format: role="tool" with tool_call_id and plain string content.
func NewToolResultMessage(toolCallID, result string) Message {
	return Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	}
}

// ExtractText returns the concatenated text content from a Message's Content field.
// Handles both plain string and content block array formats.
func (m *Message) ExtractText() string {
	switch v := m.Content.(type) {
	case string:
		return v
	case []ContentBlock:
		var sb strings.Builder
		for _, block := range v {
			sb.WriteString(extractBlockText(block))
		}
		return sb.String()
	default:
		return ""
	}
}

func extractBlockText(block ContentBlock) string {
	if block.Text != "" {
		return block.Text
	}
	var sb strings.Builder
	for _, nested := range block.Content {
		sb.WriteString(extractBlockText(nested))
	}
	return sb.String()
}

// Choice holds a single choice from the response.
type Choice struct {
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the name and arguments of a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string
}

// ResponseMessage extends Message with optional reasoning content.
type ResponseMessage struct {
	Role             string     `json:"role"`
	Content          *string    `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

// ChatResponse is the parsed result of a completion request.
type ChatResponse struct {
	ID      string      `json:"-"`
	Model   string      `json:"-"`
	Choices []Choice    `json:"-"`
	Headers http.Header `json:"-"` // Raw response headers (may contain session IDs, etc.)
	Usage   *UsageInfo  `json:"-"` // Token usage extracted from API response
}

// Content extracts the text content from the first choice, falling back to reasoning content.
func (r *ChatResponse) Content() string {
	if len(r.Choices) == 0 {
		return ""
	}
	msg := r.Choices[0].Message
	if msg.Content != nil && *msg.Content != "" {
		cleaned := stripThinkTags(*msg.Content)
		return strings.TrimSpace(cleaned)
	}
	return msg.ReasoningContent
}

// ToolCalls extracts tool calls from the first choice.
func (r *ChatResponse) ToolCalls() []ToolCall {
	if len(r.Choices) == 0 {
		return nil
	}
	return r.Choices[0].Message.ToolCalls
}

// ToolDef defines a tool/function available to the model.
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef specifies the metadata for a tool definition.
type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ClientConfig holds configuration for connecting to an LLM service.
type ClientConfig struct {
	URL       string         // Full API endpoint URL
	APIKey    string         // Bearer token / API key
	Model     string         // Default model override
	Timeout   time.Duration  // Request timeout
	ExtraBody map[string]any // Vendor-specific fields merged into every request body
}

// --- Factory ---

// NewLLMClient creates the appropriate client based on the resolved endpoint protocol.
// protocol: "anthropic" -> AnthropicClient, anything else -> OpenAIClient.
func NewLLMClient(ep ResolvedEndpoint) LLMClient {
	cfg := ClientConfig{
		URL:       ep.URL,
		APIKey:    ep.Token,
		Model:     ep.Model,
		ExtraBody: ep.ExtraBody,
	}
	if ep.Protocol == "anthropic" {
		return NewAnthropicClient(cfg)
	}
	return NewOpenAIClient(cfg)
}

// --- Token counting with tiktoken ---

// modelTokenizerCache caches initialized tiktoken encoders keyed by encoding name.
type modelTokenizerCache struct {
	mu    sync.RWMutex
	cache map[string]*tiktoken.Tiktoken
}

func newModelTokenizerCache() *modelTokenizerCache {
	return &modelTokenizerCache{cache: make(map[string]*tiktoken.Tiktoken)}
}

func (c *modelTokenizerCache) getOrLoad(encName string) (*tiktoken.Tiktoken, error) {
	c.mu.RLock()
	if tke, ok := c.cache[encName]; ok {
		c.mu.RUnlock()
		return tke, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if tke, ok := c.cache[encName]; ok {
		return tke, nil
	}
	enc, err := tiktoken.GetEncoding(encName)
	if err != nil {
		return nil, fmt.Errorf("get tiktoken encoding %q: %w", encName, err)
	}
	c.cache[encName] = enc
	return enc, nil
}

var defaultTokenizer = newModelTokenizerCache()

func countTokensWithEncoding(text string, encName string) int {
	tke, err := defaultTokenizer.getOrLoad(encName)
	if err != nil {
		return len([]byte(text)) / 4
	}
	return len(tke.Encode(text, nil, nil))
}

func CountTokens(text string) int {
	return CountTokensForModel(text, "")
}

func CountTokensForModel(text string, modelName string) int {
	if text == "" {
		return 0
	}
	encName := encodingForModel(modelName)
	return countTokensWithEncoding(text, encName)
}

func encodingForModel(modelName string) string {
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "o1") || strings.Contains(lower, "o3") || strings.Contains(lower, "o4"):
		return "o200k_base"
	default:
		return "cl100k_base"
	}
}

// --- OpenAI SDK translation functions ---

// messagesToOpenAI converts internal Message slice to OpenAI SDK message params.
func messagesToOpenAI(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion
	for _, m := range msgs {
		switch m.Role {
		case "system", "developer":
			text := extractMessageText(m)
			result = append(result, openai.DeveloperMessage(text))
		case "user":
			text := extractMessageText(m)
			result = append(result, openai.UserMessage(text))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
				for _, tc := range m.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						},
					})
				}
				msg := openai.AssistantMessage(extractMessageText(m))
				msg.OfAssistant.ToolCalls = toolCalls
				result = append(result, msg)
			} else {
				result = append(result, openai.AssistantMessage(extractMessageText(m)))
			}
		case "tool":
			result = append(result, openai.ToolMessage(m.Content.(string), m.ToolCallID))
		}
	}
	return result
}

// extractMessageText returns the text content from a Message.
func extractMessageText(m Message) string {
	switch v := m.Content.(type) {
	case string:
		return v
	default:
		b, _ := json.Marshal(m.Content)
		return string(b)
	}
}

// toolsToOpenAI converts internal ToolDef slice to OpenAI SDK tool params.
func toolsToOpenAI(tools []ToolDef) []openai.ChatCompletionToolUnionParam {
	var result []openai.ChatCompletionToolUnionParam
	for _, t := range tools {
		result = append(result, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Function.Name,
			Description: openai.String(t.Function.Description),
			Parameters:  openai.FunctionParameters(t.Function.Parameters),
		}))
	}
	return result
}

// openAIToChatResponse converts OpenAI SDK response to internal ChatResponse.
func openAIToChatResponse(id, model, finishReason, content string, toolCalls []ToolCall, usage *UsageInfo) *ChatResponse {
	respMsg := ResponseMessage{
		Role:      "assistant",
		Content:   &content,
		ToolCalls: toolCalls,
	}
	return &ChatResponse{
		ID:    id,
		Model: model,
		Choices: []Choice{{
			Message:      respMsg,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}

// --- Anthropic SDK translation functions ---

// messagesToAnthropic converts internal Messages to Anthropic SDK params.
// Returns system prompt blocks and message params separately.
func messagesToAnthropic(msgs []Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var system []anthropic.TextBlockParam
	var result []anthropic.MessageParam

	for _, m := range msgs {
		switch m.Role {
		case "system":
			text := extractMessageText(m)
			system = append(system, anthropic.TextBlockParam{Text: text})
		case "user":
			text := extractMessageText(m)
			result = append(result, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				if text := extractMessageText(m); text != "" {
					blocks = append(blocks, anthropic.NewTextBlock(text))
				}
				for _, tc := range m.ToolCalls {
					var input any
					json.Unmarshal([]byte(tc.Function.Arguments), &input)
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    tc.ID,
							Name:  tc.Function.Name,
							Input: input,
						},
					})
				}
				result = append(result, anthropic.NewAssistantMessage(blocks...))
			} else {
				text := extractMessageText(m)
				result = append(result, anthropic.NewAssistantMessage(anthropic.NewTextBlock(text)))
			}
		case "tool":
			result = append(result, anthropic.NewUserMessage(
				anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: m.ToolCallID,
						Content: []anthropic.ToolResultBlockParamContentUnion{{
							OfText: &anthropic.TextBlockParam{Text: extractMessageText(m)},
						}},
					},
				},
			))
		}
	}
	return system, result
}

// toolsToAnthropic converts internal ToolDef slice to Anthropic SDK tool params.
func toolsToAnthropic(tools []ToolDef) []anthropic.ToolParam {
	var result []anthropic.ToolParam
	for _, t := range tools {
		result = append(result, anthropic.ToolParam{
			Name:        t.Function.Name,
			Description: anthropic.String(t.Function.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: t.Function.Parameters,
			},
		})
	}
	return result
}

// anthropicToChatResponse converts Anthropic SDK response to internal ChatResponse.
func anthropicToChatResponse(id, model, stopReason, content string, toolCalls []ToolCall, usage *UsageInfo) *ChatResponse {
	// Map Anthropic stop reasons to OpenAI-style finish reasons
	finishReason := stopReason
	if stopReason == "tool_use" {
		finishReason = "tool_calls"
	}

	respMsg := ResponseMessage{
		Role:      "assistant",
		Content:   &content,
		ToolCalls: toolCalls,
	}
	return &ChatResponse{
		ID:    id,
		Model: model,
		Choices: []Choice{{
			Message:      respMsg,
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}

// --- OpenAIClient ---

// OpenAIClient sends requests to an OpenAI-compatible chat completion API.
type OpenAIClient struct {
	cfg    ClientConfig
	client *openai.Client
}

// NewOpenAIClient creates a new OpenAI-compatible LLM client using the official SDK.
func NewOpenAIClient(cfg ClientConfig) *OpenAIClient {
	opts := []openaiopt.RequestOption{
		openaiopt.WithAPIKey(cfg.APIKey),
	}
	if cfg.URL != "" {
		opts = append(opts, openaiopt.WithBaseURL(cfg.URL))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, openaiopt.WithRequestTimeout(cfg.Timeout))
	}
	client := openai.NewClient(opts...)
	return &OpenAIClient{
		cfg:    cfg,
		client: &client,
	}
}

// ChatRequest represents the payload for a chat completion call.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Completions sends a chat completion request and returns the parsed response.
func (c *OpenAIClient) Completions(req ChatRequest) (*ChatResponse, error) {
	return c.CompletionsWithCtx(context.Background(), req)
}

// CompletionsWithCtx sends a chat completion request with context support.
func (c *OpenAIClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := openai.ChatCompletionNewParams{
		Model:               openai.ChatModel(model),
		Messages:            messagesToOpenAI(req.Messages),
		MaxCompletionTokens: openai.Int(int64(req.MaxTokens)),
	}
	if len(req.Tools) > 0 {
		params.Tools = toolsToOpenAI(req.Tools)
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	// Apply ExtraBody via request options.
	var opts []openaiopt.RequestOption
	for k, v := range c.cfg.ExtraBody {
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}

	var result *ChatResponse
	err := c.withRetryCtx(ctx, func() error {
		completion, err := c.client.Chat.Completions.New(ctx, params, opts...)
		if err != nil {
			return fmt.Errorf("llm request failed: %w", err)
		}

		if len(completion.Choices) == 0 {
			return fmt.Errorf("llm response: no choices returned")
		}

		choice := completion.Choices[0]
		var toolCalls []ToolCall
		for _, tc := range choice.Message.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		var usage *UsageInfo
		if completion.Usage.TotalTokens > 0 {
			usage = &UsageInfo{
				TotalTokens:      completion.Usage.TotalTokens,
				PromptTokens:     completion.Usage.PromptTokens,
				CompletionTokens: completion.Usage.CompletionTokens,
			}
		}

		result = openAIToChatResponse(
			completion.ID,
			completion.Model,
			choice.FinishReason,
			choice.Message.Content,
			toolCalls,
			usage,
		)
		return nil
	})
	return result, err
}

// StreamCompletion initiates a streaming chat completion. The callback is invoked per chunk.
func (c *OpenAIClient) StreamCompletion(req ChatRequest, cb func(chunk []byte) error) error {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := openai.ChatCompletionNewParams{
		Model:               openai.ChatModel(model),
		Messages:            messagesToOpenAI(req.Messages),
		MaxCompletionTokens: openai.Int(int64(req.MaxTokens)),
	}
	if len(req.Tools) > 0 {
		params.Tools = toolsToOpenAI(req.Tools)
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	var opts []openaiopt.RequestOption
	for k, v := range c.cfg.ExtraBody {
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}

	stream := c.client.Chat.Completions.NewStreaming(context.Background(), params, opts...)
	for stream.Next() {
		evt := stream.Current()
		chunkBytes, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		if err := cb(chunkBytes); err != nil {
			return err
		}
	}
	return stream.Err()
}

// --- AnthropicClient ---

const anthropicVersion = "2023-06-01"

// AnthropicClient implements the Anthropic Messages API.
type AnthropicClient struct {
	cfg    ClientConfig
	client *http.Client
}

// NewAnthropicClient creates a new Anthropic Messages API client.
func NewAnthropicClient(cfg ClientConfig) *AnthropicClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if !strings.HasSuffix(cfg.URL, "/v1/messages") && !strings.HasSuffix(cfg.URL, "/v1/messages/") {
		baseURL := strings.TrimRight(cfg.URL, "/")
		if !strings.HasSuffix(baseURL, "/v1/messages") {
			cfg.URL = baseURL + "/v1/messages"
		}
	}
	return &AnthropicClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Completions sends a chat completion request and returns the parsed response.
func (c *AnthropicClient) Completions(req ChatRequest) (*ChatResponse, error) {
	return c.CompletionsWithCtx(context.Background(), req)
}

// CompletionsWithCtx sends a chat completion request with context support.
func (c *AnthropicClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	var result *ChatResponse
	err := c.withRetryCtx(ctx, func() error {
		resp, err := c.doRequestCtx(ctx, model, req)
		if err != nil {
			return err
		}
		result = resp
		return nil
	})
	return result, err
}

// StreamCompletion initiates a streaming chat completion using SSE. The callback
// is invoked per chunk with raw JSON data stripped of the "data: " prefix.
func (c *AnthropicClient) StreamCompletion(req ChatRequest, cb func(chunk []byte) error) error {
	req.Stream = true

	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	return c.withRetry(func() error {
		body := c.buildRequestBody(model, req)
		body.Stream = true

		payload, err := mergeExtraBody(body, c.cfg.ExtraBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}

		httpReq, err := http.NewRequest(http.MethodPost, c.cfg.URL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", c.cfg.APIKey)
		httpReq.Header.Set("anthropic-version", anthropicVersion)
		httpReq.Header.Set("User-Agent", userAgent("claude"))

		resp, err := c.client.Do(httpReq)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if isRetryableStatus(resp.StatusCode) {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
		}
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error %d: %s (non-retryable)", resp.StatusCode, string(bodyBytes))
		}

		scanner := bufio.NewScanner(resp.Body)
		var eventType string

		for scanner.Scan() {
			line := scanner.Text()

			// Capture event type line: "event: message_delta"
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
				continue
			}

			// Skip empty lines and non-data lines
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			// message_stop signals end of stream
			if eventType == "message_stop" {
				break
			}

			if err := cb([]byte(data)); err != nil {
				return err
			}
		}
		return scanner.Err()
	})
}

// anthropicRequest is the request body for Anthropic Messages API.
type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	System      string          `json:"system,omitempty"`
	Messages    []anthroMessage `json:"messages"`
	Tools       []anthroTool    `json:"tools,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
}

type anthroMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []interface{}
}

// anthropicToolUseBlock represents a tool_use content block in Anthropic's Messages API.
type anthropicToolUseBlock struct {
	Type  string         `json:"type"`  // "tool_use"
	ID    string         `json:"id"`    // tool use ID
	Name  string         `json:"name"`  // function name
	Input map[string]any `json:"input"` // function arguments (parsed as object)
}

type anthroTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// doRequestCtx builds and sends an Anthropic Messages API request.
func (c *AnthropicClient) doRequestCtx(ctx context.Context, model string, req ChatRequest) (*ChatResponse, error) {
	if model == "" {
		model = c.cfg.Model
	}

	body := c.buildRequestBody(model, req)
	payload, err := mergeExtraBody(body, c.cfg.ExtraBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("User-Agent", userAgent("claude"))

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
		detail := extractErrorMessage(bodyBytes)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, detail)
	}

	chatResp, err := c.parseResponse(bodyBytes, resp.Header)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return chatResp, nil
}

// buildRequestBody converts the shared ChatRequest into Anthropic format.
func (c *AnthropicClient) buildRequestBody(model string, req ChatRequest) anthropicRequest {
	messages := make([]anthroMessage, 0, len(req.Messages))
	var systemMsg string

	var pendingToolResults []Message // collect consecutive tool messages

	flushToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		// Merge all pending tool results into a single user message
		var blocks []interface{}
		for _, tr := range pendingToolResults {
			blocks = append(blocks, ContentBlock{
				Type:      "tool_result",
				ToolUseID: tr.ToolCallID,
				Content: []ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("%v", tr.Content),
				}},
			})
		}
		messages = append(messages, anthroMessage{Role: "user", Content: blocks})
		pendingToolResults = nil
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			if s, ok := msg.Content.(string); ok {
				systemMsg = s
			}
			flushToolResults()
		case "tool":
			pendingToolResults = append(pendingToolResults, msg)
		case "assistant":
			flushToolResults()
			// Build Anthropic content blocks from text + tool calls
			var blocks []interface{}
			if s, ok := msg.Content.(string); ok && s != "" {
				blocks = append(blocks, ContentBlock{Type: "text", Text: s})
			}
			for _, tc := range msg.ToolCalls {
				argsMap := map[string]any{}
				if tc.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsMap); err != nil {
						fmt.Fprintf(stdout.Writer(), "[llm] WARNING: failed to parse tool call arguments JSON for %q: %v\n", tc.ID, err)
					}
				}
				blocks = append(blocks, anthropicToolUseBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: argsMap,
				})
			}
			if len(blocks) > 0 {
				messages = append(messages, anthroMessage{Role: "assistant", Content: blocks})
			} else {
				s, _ := msg.Content.(string)
				messages = append(messages, anthroMessage{Role: "assistant", Content: s})
			}
		default:
			// user or other roles: flush tool results first
			flushToolResults()
			content := msg.Content
			if blkArr, ok := content.([]ContentBlock); ok {
				converted := make([]ContentBlock, len(blkArr))
				for i, b := range blkArr {
					converted[i] = ContentBlock{
						Type:      b.Type,
						Text:      b.Text,
						ToolUseID: b.ToolUseID,
						Content:   b.Content,
					}
				}
				content = converted
			}
			messages = append(messages, anthroMessage{Role: msg.Role, Content: content})
		}
	}
	flushToolResults() // flush any remaining tool results at the end

	tools := make([]anthroTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, anthroTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192 // Anthropic default
	}

	return anthropicRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		System:      systemMsg,
		Messages:    messages,
		Tools:       tools,
		Stream:      false,
		Temperature: req.Temperature,
	}
}

func mergeExtraBody(base any, extraBody map[string]any) ([]byte, error) {
	if len(extraBody) == 0 {
		return json.Marshal(base)
	}
	b, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range extraBody {
		m[k] = v
	}
	return json.Marshal(m)
}

// parseResponse converts Anthropic JSON response into ChatResponse.
func (c *AnthropicClient) parseResponse(body []byte, headers http.Header) (*ChatResponse, error) {
	type contentBlockResp struct {
		Type  string `json:"type"`
		Text  string `json:"text,omitempty"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Input any    `json:"input,omitempty"`
	}

	type anthropicUsageRaw struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	}

	var resp struct {
		ID         string             `json:"id"`
		Model      string             `json:"model"`
		Type       string             `json:"type"`
		Role       string             `json:"role"`
		Content    []contentBlockResp `json:"content"`
		Usage      anthropicUsageRaw  `json:"usage"`
		StopReason string             `json:"stop_reason,omitempty"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	// Build the response message from content blocks.
	var textParts []string
	var toolCalls []ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	var contentStr *string
	if len(textParts) > 0 {
		s := strings.Join(textParts, "\n")
		contentStr = &s
	}

	finishReason := resp.StopReason
	if finishReason == "" {
		finishReason = "stop"
	}

	var usage *UsageInfo
	if u := resp.Usage; u.InputTokens > 0 || u.OutputTokens > 0 {
		usage = &UsageInfo{
			PromptTokens:     u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens,
			CompletionTokens: u.OutputTokens,
			CacheReadTokens:  u.CacheReadInputTokens,
			CacheWriteTokens: u.CacheCreationInputTokens,
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return &ChatResponse{
		ID:    resp.ID,
		Model: resp.Model,
		Choices: []Choice{{
			Message: ResponseMessage{
				Role:      resp.Role,
				Content:   contentStr,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
		Headers: headers,
		Usage:   usage,
	}, nil
}

// --- Retry logic ---

func retryWithCtx(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !isRetryable(lastErr) {
			return lastErr
		}

		if attempt < maxRetries {
			sleepWithBackoff(attempt)
		}
	}
	return fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
}

func (c *OpenAIClient) withRetry(fn func() error) error {
	return retryWithCtx(context.Background(), fn)
}

func (c *OpenAIClient) withRetryCtx(ctx context.Context, fn func() error) error {
	return retryWithCtx(ctx, fn)
}

func (c *AnthropicClient) withRetry(fn func() error) error {
	return retryWithCtx(context.Background(), fn)
}

func (c *AnthropicClient) withRetryCtx(ctx context.Context, fn func() error) error {
	return retryWithCtx(ctx, fn)
}

// isRetryable determines whether an error is transient and worth retrying.
func isRetryable(err error) bool {
	msg := err.Error()
	// 429 (rate limit) and 5xx server errors are retryable.
	if strings.Contains(msg, "API error 429:") {
		return true
	}
	for code := 500; code <= 599; code++ {
		if strings.Contains(msg, fmt.Sprintf("API error %d:", code)) {
			return true
		}
	}
	// Network-level errors (timeout, connection refused, DNS failure, etc.) are retryable.
	if strings.Contains(msg, "request failed:") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "EOF") {
		return true
	}
	return false
}

// isRetryableStatus returns true for HTTP status codes that should trigger a retry.
func isRetryableStatus(status int) bool {
	return status == 429 || (status >= 500 && status <= 599)
}

// sleepWithBackoff sleeps for baseDelay * 2^attempt + jitter, capped at 60s.
// Jitter spreads retries randomly within ±50% of the computed delay.
func sleepWithBackoff(attempt int) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 60 * time.Second
	)

	delay := baseDelay << uint(min(attempt, 6)) // 1s, 2s, 4s, 8s, 16s, 32s, 64s→capped
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add random jitter: [delay*0.5, delay*1.5]
	jitter := time.Duration(rand.Int63n(int64(delay))) - delay/2
	delay += jitter

	fmt.Fprintf(stdout.Writer(), "[llm] Retrying in %v (attempt info)... \n", delay)
	time.Sleep(delay)
}

// stripThinkTags removes reasoning wrapper tags from content.
func stripThinkTags(s string) string {
	// Construct tag strings from individual bytes.
	openBytes := []byte{0x3c, 't', 'h', 'i', 'n', 'k', 0x3e}
	closeBytes := []byte{0x3c, 0x2f, 't', 'h', 'i', 'n', 'k', 0x3e}
	s = strings.ReplaceAll(s, string(openBytes), "")
	s = strings.ReplaceAll(s, string(closeBytes), "")
	return s
}

// extractErrorMessage attempts to pull a human-readable error message from
// a JSON API error response body. Falls back to truncating the raw body if
// the structure is not recognised or decoding fails.
func extractErrorMessage(body []byte) string {
	type openAIError struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	type anthropicError struct {
		Type  string `json:"type"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if len(body) == 0 {
		return "(empty body)"
	}

	var oe openAIError
	if err := json.Unmarshal(body, &oe); err == nil && oe.Error.Message != "" {
		return oe.Error.Message
	}
	var ae anthropicError
	if err := json.Unmarshal(body, &ae); err == nil && ae.Error.Message != "" {
		return ae.Error.Message
	}

	// Truncate raw body to avoid excessively noisy errors.
	bodyText := string(body)
	if len(bodyText) > 512 {
		bodyText = bodyText[:512] + "... (truncated)"
	}
	return bodyText
}
