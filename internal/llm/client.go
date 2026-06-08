// Package llm provides LLM client interfaces supporting multiple protocols.
// Supported protocols: Anthropic Messages API, OpenAI Chat Completions API.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

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
	CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error)
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
	ID      string     `json:"-"`
	Model   string     `json:"-"`
	Choices []Choice   `json:"-"`
	Usage   *UsageInfo `json:"-"` // Token usage extracted from API response
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
	URL        string         // Full API endpoint URL
	APIKey     string         // Bearer token / API key
	Model      string         // Default model override
	AuthHeader string         // Anthropic auth header: "x-api-key" or "authorization"
	Timeout    time.Duration  // Request timeout
	ExtraBody  map[string]any // Vendor-specific fields merged into every request body
}

// --- Factory ---

// NewLLMClient creates the appropriate client based on the resolved endpoint protocol.
// protocol: "anthropic" -> AnthropicClient, anything else -> OpenAIClient.
func NewLLMClient(ep ResolvedEndpoint) LLMClient {
	cfg := ClientConfig{
		URL:        ep.URL,
		APIKey:     ep.Token,
		Model:      ep.Model,
		AuthHeader: ep.AuthHeader,
		ExtraBody:  ep.ExtraBody,
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

// --- OpenAIClient ---

// OpenAIClient sends requests to an OpenAI-compatible chat completion API using the official SDK.
type OpenAIClient struct {
	cfg ClientConfig
	sdk openai.Client
}

// NewOpenAIClient creates a new OpenAI-compatible LLM client.
func NewOpenAIClient(cfg ClientConfig) *OpenAIClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	baseURL := strings.TrimRight(cfg.URL, "/")
	if !strings.HasSuffix(baseURL, "/chat/completions") {
		cfg.URL = baseURL + "/chat/completions"
	}

	sdkBaseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/chat/completions")

	return &OpenAIClient{
		cfg: cfg,
		sdk: openai.NewClient(
			openaiopt.WithAPIKey(cfg.APIKey),
			openaiopt.WithBaseURL(sdkBaseURL),
			openaiopt.WithMaxRetries(5),
			openaiopt.WithHeader("User-Agent", userAgent("")),
			openaiopt.WithRequestTimeout(cfg.Timeout),
		),
	}
}

// ChatRequest represents the payload for a chat completion call.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// CompletionsWithCtx sends a chat completion request with context support for cancellation and timeout.
func (c *OpenAIClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := c.buildOpenAIParams(model, req)

	var opts []openaiopt.RequestOption
	for k, v := range c.cfg.ExtraBody {
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}

	sdkResp, err := c.sdk.Chat.Completions.New(ctx, params, opts...)
	if err != nil {
		return nil, err
	}

	return c.mapOpenAIResponse(sdkResp), nil
}

// buildOpenAIParams converts the shared ChatRequest into OpenAI SDK parameters.
func (c *OpenAIClient) buildOpenAIParams(model string, req ChatRequest) openai.ChatCompletionNewParams {
	var messages []openai.ChatCompletionMessageParamUnion

	for _, msg := range req.Messages {
		content := msg.ExtractText()

		switch msg.Role {
		case "system":
			messages = append(messages, openai.SystemMessage(content))
		case "user":
			messages = append(messages, openai.UserMessage(content))
		case "tool":
			messages = append(messages, openai.ToolMessage(content, msg.ToolCallID))
		case "assistant":
			if len(msg.ToolCalls) == 0 {
				messages = append(messages, openai.AssistantMessage(content))
			} else {
				asst := openai.ChatCompletionAssistantMessageParam{}
				if content != "" {
					asst.Content.OfString = openai.String(content)
				}
				for _, tc := range msg.ToolCalls {
					asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						},
					})
				}
				messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
			}
		default:
			messages = append(messages, openai.UserMessage(content))
		}
	}

	var tools []openai.ChatCompletionToolUnionParam
	for _, t := range req.Tools {
		tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        t.Function.Name,
			Description: openai.String(t.Function.Description),
			Parameters:  shared.FunctionParameters(t.Function.Parameters),
		}))
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: messages,
	}

	if len(tools) > 0 {
		params.Tools = tools
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	return params
}

// mapOpenAIResponse converts the SDK response into ChatResponse.
func (c *OpenAIClient) mapOpenAIResponse(sdkResp *openai.ChatCompletion) *ChatResponse {
	rawJSON := sdkResp.RawJSON()

	usage := resolveUsage([]byte(rawJSON))
	if usage == nil {
		u := sdkResp.Usage
		if u.PromptTokens > 0 || u.CompletionTokens > 0 {
			usage = &UsageInfo{
				PromptTokens:     u.PromptTokens,
				CompletionTokens: u.CompletionTokens,
				TotalTokens:      u.TotalTokens,
			}
		}
	}

	var choices []Choice
	for _, ch := range sdkResp.Choices {
		var toolCalls []ToolCall
		for _, tc := range ch.Message.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		content := ch.Message.Content
		var contentPtr *string
		if content != "" {
			contentPtr = &content
		}

		var reasoningContent string
		if extra, ok := ch.Message.JSON.ExtraFields["reasoning_content"]; ok && extra.Valid() {
			if err := json.Unmarshal([]byte(extra.Raw()), &reasoningContent); err != nil {
				reasoningContent = extra.Raw()
			}
		}

		choices = append(choices, Choice{
			Message: ResponseMessage{
				Role:             "assistant",
				Content:          contentPtr,
				ReasoningContent: reasoningContent,
				ToolCalls:        toolCalls,
			},
			FinishReason: ch.FinishReason,
		})
	}

	return &ChatResponse{
		ID:      sdkResp.ID,
		Model:   sdkResp.Model,
		Choices: choices,
		Usage:   usage,
	}
}

// --- AnthropicClient ---

// AnthropicClient implements the Anthropic Messages API using the official SDK.
type AnthropicClient struct {
	cfg ClientConfig
	sdk anthropic.Client
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

	sdkBaseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/v1/messages")
	authHeader := normalizeAuthHeader(cfg.AuthHeader)
	if authHeader == "" {
		authHeader = "authorization"
	}
	cfg.AuthHeader = authHeader

	opts := []option.RequestOption{
		option.WithBaseURL(sdkBaseURL),
		option.WithMaxRetries(5),
		option.WithHeader("User-Agent", userAgent("claude")),
		option.WithRequestTimeout(cfg.Timeout),
	}

	switch authHeader {
	case "authorization":
		opts = append(opts, option.WithHeaderDel("X-Api-Key"), option.WithAuthToken(cfg.APIKey))
	case "x-api-key":
		opts = append(opts, option.WithHeaderDel("Authorization"), option.WithAPIKey(cfg.APIKey))
	default:
		opts = append(opts,
			option.WithHeaderDel("Authorization"),
			option.WithHeaderDel("X-Api-Key"),
			option.WithHeader(authHeader, cfg.APIKey),
		)
	}

	return &AnthropicClient{
		cfg: cfg,
		sdk: anthropic.NewClient(opts...),
	}
}

// CompletionsWithCtx sends a chat completion request with context support.
func (c *AnthropicClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := c.buildAnthropicParams(model, req)

	var opts []option.RequestOption
	for k, v := range c.cfg.ExtraBody {
		opts = append(opts, option.WithJSONSet(k, v))
	}

	sdkResp, err := c.sdk.Messages.New(ctx, params, opts...)
	if err != nil {
		return nil, err
	}

	return c.mapAnthropicResponse(sdkResp), nil
}

// buildAnthropicParams converts the shared ChatRequest into Anthropic SDK parameters.
func (c *AnthropicClient) buildAnthropicParams(model string, req ChatRequest) anthropic.MessageNewParams {
	var systemBlocks []anthropic.TextBlockParam
	var messages []anthropic.MessageParam
	var pendingToolResults []Message

	flushToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		var blocks []anthropic.ContentBlockParamUnion
		for _, tr := range pendingToolResults {
			blocks = append(blocks, anthropic.NewToolResultBlock(
				tr.ToolCallID,
				fmt.Sprintf("%v", tr.Content),
				false,
			))
		}
		messages = append(messages, anthropic.NewUserMessage(blocks...))
		pendingToolResults = nil
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			if s, ok := msg.Content.(string); ok {
				systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: s})
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
				argsMap := map[string]any{}
				if tc.Function.Arguments != "" {
					json.Unmarshal([]byte(tc.Function.Arguments), &argsMap)
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, argsMap, tc.Function.Name))
			}
			if len(blocks) > 0 {
				messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			} else {
				s, _ := msg.Content.(string)
				messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(s)))
			}
		default:
			flushToolResults()
			switch content := msg.Content.(type) {
			case string:
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
			case []ContentBlock:
				var blocks []anthropic.ContentBlockParamUnion
				for _, b := range content {
					if b.Type == "tool_result" {
						blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, extractBlockText(b), false))
					} else {
						blocks = append(blocks, anthropic.NewTextBlock(b.Text))
					}
				}
				if len(blocks) > 0 {
					messages = append(messages, anthropic.NewUserMessage(blocks...))
				}
			}
		}
	}
	flushToolResults()

	var tools []anthropic.ToolUnionParam
	for _, t := range req.Tools {
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Function.Name,
				Description: anthropic.String(t.Function.Description),
				InputSchema: buildToolInputSchema(t.Function.Parameters),
			},
		})
	}

	maxTokens := int64(req.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
		params.System = systemBlocks
	}
	if len(tools) > 0 {
		tools[len(tools)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		params.Tools = tools
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	return params
}

func buildToolInputSchema(params map[string]any) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{}
	if props, ok := params["properties"]; ok {
		schema.Properties = props
	}
	if req, ok := params["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	for k, v := range params {
		if k == "type" || k == "properties" || k == "required" {
			continue
		}
		if schema.ExtraFields == nil {
			schema.ExtraFields = make(map[string]any)
		}
		schema.ExtraFields[k] = v
	}
	return schema
}

// mapAnthropicResponse converts the SDK response into ChatResponse.
func (c *AnthropicClient) mapAnthropicResponse(sdkResp *anthropic.Message) *ChatResponse {
	var textParts []string
	var thinkingParts []string
	var toolCalls []ToolCall

	for _, block := range sdkResp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "thinking":
			if block.Thinking != "" {
				thinkingParts = append(thinkingParts, block.Thinking)
			}
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

	var contentStr *string
	if len(textParts) > 0 {
		s := strings.Join(textParts, "\n")
		contentStr = &s
	}

	var reasoningContent string
	if len(thinkingParts) > 0 {
		reasoningContent = strings.Join(thinkingParts, "\n")
	}

	finishReason := string(sdkResp.StopReason)
	if finishReason == "" {
		finishReason = "stop"
	}

	var usage *UsageInfo
	u := sdkResp.Usage
	if u.InputTokens > 0 || u.OutputTokens > 0 {
		usage = &UsageInfo{
			PromptTokens:     u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens,
			CompletionTokens: u.OutputTokens,
			CacheReadTokens:  u.CacheReadInputTokens,
			CacheWriteTokens: u.CacheCreationInputTokens,
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	} else {
		usage = resolveUsage([]byte(sdkResp.RawJSON()))
	}

	return &ChatResponse{
		ID:    sdkResp.ID,
		Model: string(sdkResp.Model),
		Choices: []Choice{{
			Message: ResponseMessage{
				Role:             "assistant",
				Content:          contentStr,
				ReasoningContent: reasoningContent,
				ToolCalls:        toolCalls,
			},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
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
