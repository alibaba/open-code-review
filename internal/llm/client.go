// Package llm provides LLM client interfaces supporting multiple protocols.
// Supported protocols: Anthropic Messages API, OpenAI Chat Completions API.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropt "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	tiktoken "github.com/pkoukk/tiktoken-go"

	"github.com/open-code-review/open-code-review/internal/stdout"
)

const maxRetries = 10 // Maximum number of retry attempts with exponential backoff.

var AppVersion = "dev"

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
		case "system":
			text := extractMessageText(m)
			result = append(result, openai.SystemMessage(text))
		case "developer":
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
			result = append(result, openai.ToolMessage(extractMessageText(m), m.ToolCallID))
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
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
						input = map[string]any{}
					}
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
func toolsToAnthropic(tools []ToolDef) []anthropic.ToolUnionParam {
	var result []anthropic.ToolUnionParam
	for _, t := range tools {
		toolParam := anthropic.ToolParam{
			Name:        t.Function.Name,
			Description: anthropic.String(t.Function.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: t.Function.Parameters,
			},
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &toolParam})
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

	// TODO: LLMClient.StreamCompletion interface does not accept context.Context;
	// using context.Background() as a workaround. Consider updating the interface
	// to support context propagation for cancellation and tracing.
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

// AnthropicClient sends requests to the Anthropic Messages API.
type AnthropicClient struct {
	cfg    ClientConfig
	client *anthropic.Client
}

// NewAnthropicClient creates a new Anthropic LLM client using the official SDK.
func NewAnthropicClient(cfg ClientConfig) *AnthropicClient {
	opts := []anthropt.RequestOption{
		anthropt.WithAPIKey(cfg.APIKey),
	}
	if cfg.URL != "" {
		opts = append(opts, anthropt.WithBaseURL(cfg.URL))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, anthropt.WithRequestTimeout(cfg.Timeout))
	}
	client := anthropic.NewClient(opts...)
	return &AnthropicClient{
		cfg:    cfg,
		client: &client,
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

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	system, messages := messagesToAnthropic(req.Messages)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  messages,
	}
	if len(system) > 0 {
		params.System = system
	}
	if len(req.Tools) > 0 {
		params.Tools = toolsToAnthropic(req.Tools)
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	// Apply ExtraBody via request options.
	var opts []anthropt.RequestOption
	for k, v := range c.cfg.ExtraBody {
		opts = append(opts, anthropt.WithJSONSet(k, v))
	}

	message, err := c.client.Messages.New(ctx, params, opts...)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}

	// Extract text and tool calls from response content blocks
	var textContent string
	var toolCalls []ToolCall
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}

	var usage *UsageInfo
	if message.Usage.InputTokens > 0 || message.Usage.OutputTokens > 0 {
		usage = &UsageInfo{
			PromptTokens:     message.Usage.InputTokens + message.Usage.CacheReadInputTokens + message.Usage.CacheCreationInputTokens,
			CompletionTokens: message.Usage.OutputTokens,
			CacheReadTokens:  message.Usage.CacheReadInputTokens,
			CacheWriteTokens: message.Usage.CacheCreationInputTokens,
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return anthropicToChatResponse(
		message.ID,
		string(message.Model),
		string(message.StopReason),
		textContent,
		toolCalls,
		usage,
	), nil
}

// StreamCompletion initiates a streaming chat completion.
func (c *AnthropicClient) StreamCompletion(req ChatRequest, cb func(chunk []byte) error) error {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	system, messages := messagesToAnthropic(req.Messages)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		Messages:  messages,
	}
	if len(system) > 0 {
		params.System = system
	}
	if len(req.Tools) > 0 {
		params.Tools = toolsToAnthropic(req.Tools)
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	// TODO: LLMClient.StreamCompletion interface does not accept context.Context;
	// using context.Background() as a workaround. Consider updating the interface
	// to support context propagation for cancellation and tracing.
	stream := c.client.Messages.NewStreaming(context.Background(), params)
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

func (c *OpenAIClient) withRetryCtx(ctx context.Context, fn func() error) error {
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
