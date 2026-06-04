// Package llm provides LLM client interfaces supporting multiple protocols.
// Supported protocols: Anthropic Messages API, OpenAI Chat Completions API.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"

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

// ChatRequest represents the payload for a chat completion call.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
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

// --- OpenAIClient ---

// OpenAIClient sends requests to an OpenAI-compatible chat completion API
// using the official openai-go SDK.
type OpenAIClient struct {
	client *openai.Client
	cfg    ClientConfig
}

// NewOpenAIClient creates a new OpenAI-compatible LLM client.
func NewOpenAIClient(cfg ClientConfig) *OpenAIClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}

	opts := []openaiopt.RequestOption{
		openaiopt.WithAPIKey(cfg.APIKey),
		openaiopt.WithBaseURL(cfg.URL),
		openaiopt.WithMaxRetries(maxRetries),
		openaiopt.WithRequestTimeout(cfg.Timeout),
	}

	return &OpenAIClient{
		client: openai.NewClient(opts...),
		cfg:    cfg,
	}
}

// NewClient is kept as an alias for backward compatibility during transition.
func NewClient(cfg ClientConfig) *OpenAIClient {
	return NewOpenAIClient(cfg)
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
		Model:    model,
		Messages: c.convertMessages(req.Messages),
	}

	if len(req.Tools) > 0 {
		params.Tools = c.convertTools(req.Tools)
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if len(c.cfg.ExtraBody) > 0 {
		params.SetExtraFields(c.cfg.ExtraBody)
	}

	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}

	return c.convertResponse(completion), nil
}

// StreamCompletion initiates a streaming chat completion. The callback is invoked per chunk.
func (c *OpenAIClient) StreamCompletion(req ChatRequest, cb func(chunk []byte) error) error {
	ctx := context.Background()
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: c.convertMessages(req.Messages),
	}
	if len(req.Tools) > 0 {
		params.Tools = c.convertTools(req.Tools)
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if len(c.cfg.ExtraBody) > 0 {
		params.SetExtraFields(c.cfg.ExtraBody)
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	for stream.Next() {
		chunk := stream.Current()
		data, err := json.Marshal(chunk)
		if err != nil {
			continue
		}
		if err := cb(data); err != nil {
			return err
		}
	}
	return stream.Err()
}

// convertMessages translates internal Message types to OpenAI SDK message params.
func (c *OpenAIClient) convertMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, msg := range msgs {
		content := fmt.Sprintf("%v", msg.Content)
		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(content))
		case "user":
			result = append(result, openai.UserMessage(content))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				var toolCalls []openai.ChatCompletionMessageToolCallUnion
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnion{
						ID:   tc.ID,
						Type: "function",
						Function: openai.ChatCompletionMessageToolCallFunction{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content: openai.ChatCompletionAssistantMessageParamContentUnion{
							OfString: openai.String(content),
						},
						ToolCalls: toolCalls,
					},
				})
			} else {
				result = append(result, openai.AssistantMessage(content))
			}
		case "tool":
			result = append(result, openai.ToolMessage(content, msg.ToolCallID))
		default:
			result = append(result, openai.UserMessage(content))
		}
	}
	return result
}

// convertTools translates internal ToolDef to OpenAI SDK tool params.
func (c *OpenAIClient) convertTools(tools []ToolDef) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		result = append(result, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        t.Function.Name,
			Description: openai.String(t.Function.Description),
			Parameters:  openai.FunctionParameters(t.Function.Parameters),
		}))
	}
	return result
}

// convertResponse translates an OpenAI SDK response to our internal ChatResponse.
func (c *OpenAIClient) convertResponse(completion *openai.ChatCompletion) *ChatResponse {
	choices := make([]Choice, 0, len(completion.Choices))
	for _, ch := range completion.Choices {
		var content *string
		if ch.Message.Content != "" {
			content = &ch.Message.Content
		}

		var toolCalls []ToolCall
		for _, tc := range ch.Message.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		choices = append(choices, Choice{
			Message: ResponseMessage{
				Role:      ch.Message.Role,
				Content:   content,
				ToolCalls: toolCalls,
			},
			FinishReason: string(ch.FinishReason),
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

	return &ChatResponse{
		ID:      completion.ID,
		Model:   completion.Model,
		Choices: choices,
		Usage:   usage,
	}
}

// --- AnthropicClient ---

// AnthropicClient implements the Anthropic Messages API using the official SDK.
type AnthropicClient struct {
	client *anthropic.Client
	cfg    ClientConfig
}

// NewAnthropicClient creates a new Anthropic Messages API client.
func NewAnthropicClient(cfg ClientConfig) *AnthropicClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.URL),
	}

	return &AnthropicClient{
		client: anthropic.NewClient(opts...),
		cfg:    cfg,
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

	messages, systemText := c.convertMessages(req.Messages)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}
	if params.MaxTokens <= 0 {
		params.MaxTokens = 8192 // Anthropic requires max_tokens
	}
	if systemText != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemText}}
	}
	if len(req.Tools) > 0 {
		params.Tools = c.convertTools(req.Tools)
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	message, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}

	return c.convertResponse(message), nil
}

// StreamCompletion initiates a streaming chat completion. The callback is invoked per chunk.
func (c *AnthropicClient) StreamCompletion(req ChatRequest, cb func(chunk []byte) error) error {
	ctx := context.Background()
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	messages, systemText := c.convertMessages(req.Messages)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}
	if params.MaxTokens <= 0 {
		params.MaxTokens = 8192
	}
	if systemText != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemText}}
	}
	if len(req.Tools) > 0 {
		params.Tools = c.convertTools(req.Tools)
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	stream := c.client.Messages.NewStreaming(ctx, params)
	for stream.Next() {
		event := stream.Current()
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if err := cb(data); err != nil {
			return err
		}
	}
	return stream.Err()
}

// convertMessages translates internal Message types to Anthropic SDK message params.
// Returns the messages and any extracted system text (Anthropic uses a separate System field).
func (c *AnthropicClient) convertMessages(msgs []Message) ([]anthropic.MessageParam, string) {
	var result []anthropic.MessageParam
	var systemText string
	var pendingToolResults []Message

	flushToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		var blocks []anthropic.ContentBlockParamUnion
		for _, tr := range pendingToolResults {
			content := fmt.Sprintf("%v", tr.Content)
			blocks = append(blocks, anthropic.NewToolResultBlock(tr.ToolCallID, content, false))
		}
		result = append(result, anthropic.NewUserMessage(blocks...))
		pendingToolResults = nil
	}

	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			if s, ok := msg.Content.(string); ok {
				systemText = s
			}
			flushToolResults()
		case "tool":
			pendingToolResults = append(pendingToolResults, msg)
		case "assistant":
			flushToolResults()
			var blocks []anthropic.ContentBlockParamUnion
			if s, ok := msg.Content.(string); ok && s != "" {
				blocks = append(blocks, anthropic.NewTextBlock(s))
			}
			for _, tc := range msg.ToolCalls {
				var input json.RawMessage
				if tc.Function.Arguments != "" {
					input = json.RawMessage(tc.Function.Arguments)
				} else {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, tc.Function.Name, input))
			}
			if len(blocks) > 0 {
				result = append(result, anthropic.NewAssistantMessage(blocks...))
			}
		default:
			flushToolResults()
			content := fmt.Sprintf("%v", msg.Content)
			result = append(result, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
		}
	}
	flushToolResults()

	return result, systemText
}

// convertTools translates internal ToolDef to Anthropic SDK tool params.
func (c *AnthropicClient) convertTools(tools []ToolDef) []anthropic.ToolParam {
	result := make([]anthropic.ToolParam, 0, len(tools))
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

// convertResponse translates an Anthropic SDK response to our internal ChatResponse.
func (c *AnthropicClient) convertResponse(message *anthropic.Message) *ChatResponse {
	var textParts []string
	var toolCalls []ToolCall

	for _, block := range message.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			textParts = append(textParts, b.Text)
		case anthropic.ToolUseBlock:
			argsJSON, _ := json.Marshal(b.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   b.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      b.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	var content *string
	if len(textParts) > 0 {
		s := strings.Join(textParts, "\n")
		content = &s
	}

	finishReason := string(message.StopReason)
	if finishReason == "" {
		finishReason = "stop"
	}

	var usage *UsageInfo
	if message.Usage.InputTokens > 0 || message.Usage.OutputTokens > 0 {
		usage = &UsageInfo{
			PromptTokens:     message.Usage.InputTokens,
			CompletionTokens: message.Usage.OutputTokens,
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return &ChatResponse{
		ID:    message.ID,
		Model: string(message.Model),
		Choices: []Choice{{
			Message: ResponseMessage{
				Role:      string(message.Role),
				Content:   content,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
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

// --- Utilities ---

// stripThinkTags removes reasoning wrapper tags from content.
func stripThinkTags(s string) string {
	openBytes := []byte{0x3c, 't', 'h', 'i', 'n', 'k', 0x3e}
	closeBytes := []byte{0x3c, 0x2f, 't', 'h', 'i', 'n', 'k', 0x3e}
	s = strings.ReplaceAll(s, string(openBytes), "")
	s = strings.ReplaceAll(s, string(closeBytes), "")
	return s
}
