// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package llm provides LLM client interfaces supporting multiple protocols.
// Supported protocols (canonical names, see protocol.go):
//   - "anthropic" — Anthropic Messages API
//   - "openai" — OpenAI Chat Completions API
//   - "openai-responses" — OpenAI Responses API
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
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

// ReasoningContent extracts the reasoning content of the first choice, if any.
func (r *ChatResponse) ReasoningContent() string {
	if len(r.Choices) == 0 {
		return ""
	}
	return r.Choices[0].Message.ReasoningContent
}

// ToolDef defines a tool/function available to the model.
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef specifies the metadata for a tool definition.
type FunctionDef struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Parameters    map[string]any  `json:"parameters"`
	RawDefinition json.RawMessage `json:"-"`
}

// ClientConfig holds configuration for connecting to an LLM service.
type ClientConfig struct {
	URL          string            // Full API endpoint URL
	APIKey       string            // Bearer token / API key
	Model        string            // Default model override
	AuthHeader   string            // Auth header name: "x-api-key", "authorization", or empty for protocol default
	Timeout      time.Duration     // Request timeout
	ExtraBody    map[string]any    // Vendor-specific fields merged into every request body
	ExtraHeaders map[string]string // Extra HTTP headers sent with every request
	RetryCodes   []int             // Additional HTTP status codes that trigger retry
	// SessionKey is the fallback prompt-cache affinity key
	// for requests whose context carries none (see ContextWithSessionKey).
	//
	// Review and scan runs tag every request context with the real session's ID,
	// so this fallback only serves session-less callers such as `ocr llm test`.
	//
	// Auto-generated when empty. The effective key replaces `SessionKeyTemplateVar` in Extra* values.
	SessionKey string

	// retryCollector receives one record per real HTTP attempt. It is
	// unexported because it is not configuration: it is a handle on the current
	// run, owned by llmRuntime and set only by NewLLMClient, and the three
	// exported constructors keep their signatures because of it.
	//
	// A nil collector is fully inert: no middleware is mounted and nothing about
	// the request path changes. That is the state for llm test, and for any
	// caller that builds a client without one.
	retryCollector *RetryCollector

	// AWSProfile and AWSRegion are used only by SigV4 providers (bedrock).
	// Empty means the standard AWS credential chain decides.
	AWSProfile string
	AWSRegion  string
}

// retryCodesMiddleware returns an HTTP middleware that forces the SDK to retry
// responses whose status code is in the given set, by injecting the
// x-should-retry: true response header. Returns nil when codes is empty.
// The returned function is structurally compatible with both option.Middleware
// (Anthropic SDK) and openaiopt.Middleware (OpenAI SDK).
func retryCodesMiddleware(codes []int) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if len(codes) == 0 {
		return nil
	}
	codeSet := make(map[int]bool, len(codes))
	for _, c := range codes {
		codeSet[c] = true
	}
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		resp, err := next(req)
		if err != nil {
			return resp, err
		}
		if codeSet[resp.StatusCode] {
			resp.Header.Set("x-should-retry", "true")
		}
		return resp, err
	}
}

// --- Factory ---

// NewLLMClient creates the appropriate client based on the resolved endpoint protocol.
// protocol dispatch (canonical names from protocol.go):
//   - ProtocolAnthropic ("anthropic") -> AnthropicClient
//   - ProtocolOpenAIResponses ("openai-responses") -> OpenAIResponsesClient
//   - ProtocolOpenAIChatCompletions ("openai") or anything else -> OpenAIClient
//
// The defensive default keeps legacy callers that somehow bypass resolver
// normalization working (they previously got OpenAIClient for any non-anthropic
// protocol).
//
// collector observes every HTTP attempt the returned client makes; pass nil to
// build a client that is not observed. It is a parameter rather than a field on
// ResolvedEndpoint because it belongs to the run, not to the endpoint.
func NewLLMClient(ep ResolvedEndpoint, collector *RetryCollector) LLMClient {
	cfg := ClientConfig{
		URL:            ep.URL,
		APIKey:         ep.Token,
		Model:          ep.Model,
		AuthHeader:     ep.AuthHeader,
		Timeout:        ep.Timeout,
		ExtraBody:      ep.ExtraBody,
		ExtraHeaders:   ep.ExtraHeaders,
		RetryCodes:     ep.RetryCodes,
		retryCollector: collector,
		AWSProfile:     ep.AWSProfile,
		AWSRegion:      ep.AWSRegion,
	}
	switch ep.Protocol {
	case ProtocolAnthropic:
		return NewAnthropicClient(cfg)
	case ProtocolAnthropicBedrock:
		return NewAnthropicBedrockClient(cfg)
	case ProtocolOpenAIResponses:
		return NewOpenAIResponsesClient(cfg)
	default:
		return NewOpenAIClient(cfg)
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

// --- OpenAIClient ---

// OpenAIClient sends requests to an OpenAI-compatible chat completion API using the official SDK.
type OpenAIClient struct {
	cfg ClientConfig
	sdk openai.Client
}

// NewOpenAIClient creates a new OpenAI-compatible LLM client.
// ExtraHeaders are applied per request (not baked into the SDK client) so
// SessionKeyTemplateVar can expand to the session key each request carries.
func NewOpenAIClient(cfg ClientConfig) *OpenAIClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.SessionKey == "" {
		cfg.SessionKey = NewSessionKey()
	}
	baseURL := strings.TrimRight(cfg.URL, "/")
	if !strings.HasSuffix(baseURL, "/chat/completions") {
		cfg.URL = baseURL + "/chat/completions"
	}

	sdkBaseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/chat/completions")

	opts := []openaiopt.RequestOption{
		openaiopt.WithAPIKey(cfg.APIKey),
		openaiopt.WithBaseURL(sdkBaseURL),
		openaiopt.WithMaxRetries(5),
		openaiopt.WithHeader("User-Agent", userAgent("")),
		openaiopt.WithRequestTimeout(cfg.Timeout),
	}
	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, openaiopt.WithMiddleware(mw))
	}
	if cfg.retryCollector != nil {
		opts = append(opts, openaiopt.WithMiddleware(newRetryObserver(cfg.retryCollector)))
	}

	return &OpenAIClient{
		cfg: cfg,
		sdk: openai.NewClient(opts...),
	}
}

// ChatRequest represents the payload for a chat completion call.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"` // "auto", "required", or "none"; empty means provider default
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	SessionID   string    `json:"-"` // per-file agent loop session ID; used as prompt_cache_key by the Responses API client
}

// CompletionsWithCtx sends a chat completion request with context support for cancellation and timeout.
//
// The deferred finalizeRequest is the client boundary for the retry report: it is
// the only place that knows the logical request is over, and it covers every exit
// path including the streaming branch, the EOF recovery and a panic. Results are
// named so the defer can read the error actually returned.
func (c *OpenAIClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (resp *ChatResponse, err error) {
	defer func() {
		// A panic still has to finalize, or the entry stays unfinalized and Freeze
		// drops the whole run's report. The panic value itself is re-raised
		// unchanged so agent.go's per-file recovery behaves exactly as before.
		if r := recover(); r != nil {
			finalizeRequest(ctx, c.cfg.retryCollector, errRequestPanicked)
			panic(r)
		}
		finalizeRequest(ctx, c.cfg.retryCollector, err)
	}()

	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := c.buildOpenAIParams(model, req)

	sessionKey := c.cfg.SessionKey
	if k := SessionKeyFromContext(ctx); k != "" {
		sessionKey = k
	}

	var opts []openaiopt.RequestOption
	for k, v := range expandSessionKeyInHeaders(c.cfg.ExtraHeaders, sessionKey) {
		opts = append(opts, openaiopt.WithHeader(k, v))
	}
	for k, v := range expandSessionKeyInBody(c.cfg.ExtraBody, sessionKey) {
		// Skip the "stream" key here. The streaming decision below uses a
		// dedicated boolean check, and when streaming is enabled the SDK's
		// NewStreaming method sets stream=true on the wire itself. When
		// streaming is NOT enabled, leaving the key in the body would make
		// the API answer with text/event-stream and the non-streaming path
		// fails to decode (see issue #647).
		if k == "stream" {
			continue
		}
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}
	if stream, ok := c.cfg.ExtraBody["stream"].(bool); ok && stream {
		return c.completionsStreaming(ctx, params, opts...)
	}

	sdkResp, err := c.sdk.Chat.Completions.New(ctx, params, opts...)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		// The truncated response was observed as HTTP 200, so it is corrected here
		// rather than in the defer: this SDK call ends now, and a second one is
		// about to append its own attempts under the same logical request.
		//
		// Both corrections sit ahead of their ctx early return. Placing them after
		// would leave a truncated attempt recorded as a success whenever the parent
		// context was cancelled in between — no invariant would complain, since a
		// cancelled request needs no error attempt, but the record would be wrong.
		reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassNetwork, FailurePhaseResponseDecode)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		retryResp, retryErr := c.sdk.Chat.Completions.New(ctx, params, opts...)
		if retryErr == nil {
			sdkResp = retryResp
			err = nil
		} else {
			if errors.Is(retryErr, io.ErrUnexpectedEOF) {
				reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassNetwork, FailurePhaseResponseDecode)
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if !errors.Is(retryErr, io.ErrUnexpectedEOF) {
				err = retryErr
			}
		}
	}
	if err != nil {
		return nil, err
	}

	return c.mapOpenAIResponse(sdkResp), nil
}

// completionsStreaming consumes an SSE completion and corrects the retry report
// when the stream fails after it was already established.
//
// The correction lives in this one wrapper instead of at the inner function's
// four error returns: a mid-stream failure is invisible to the observer, which
// saw only the HTTP 200 that opened the stream. It must not finalize the logical
// request — CompletionsWithCtx returns this call directly, so its defer already
// does, and a second Finalize would be recorded as a violation.
//
// A stream that never opened needs nothing here: stream.Err() then carries the
// non-2xx *apierror.Error, the observer already recorded that attempt as an
// error, and ReviseLastAttempt's precondition makes this a no-op.
func (c *OpenAIClient) completionsStreaming(ctx context.Context, params openai.ChatCompletionNewParams, opts ...openaiopt.RequestOption) (*ChatResponse, error) {
	resp, err := c.completionsStreamingInner(ctx, params, opts...)
	if err != nil {
		class, phase := classifyStreamError(err)
		reviseAttempt(ctx, c.cfg.retryCollector, class, phase)
		return nil, err
	}
	return resp, nil
}

func (c *OpenAIClient) completionsStreamingInner(ctx context.Context, params openai.ChatCompletionNewParams, opts ...openaiopt.RequestOption) (*ChatResponse, error) {
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params, opts...)
	defer stream.Close()

	accumulator := openai.ChatCompletionAccumulator{}
	reasoningByChoice := make(map[int64]*strings.Builder)
	seenChoices := make(map[int64]bool)
	finishedChoices := make(map[int64]bool)
	var choiceOrder []int64
	var usage *UsageInfo
	for stream.Next() {
		chunk := stream.Current()
		if chunk.JSON.Usage.Valid() {
			if chunkUsage := resolveUsage([]byte(chunk.RawJSON())); chunkUsage != nil {
				usage = chunkUsage
			}
		}
		for _, choice := range chunk.Choices {
			if !seenChoices[choice.Index] {
				seenChoices[choice.Index] = true
				choiceOrder = append(choiceOrder, choice.Index)
			}
			if choice.FinishReason != "" {
				finishedChoices[choice.Index] = true
			}

			extra, ok := choice.Delta.JSON.ExtraFields["reasoning_content"]
			if !ok {
				continue
			}

			var reasoningContent string
			if err := json.Unmarshal([]byte(extra.Raw()), &reasoningContent); err != nil {
				reasoningContent = extra.Raw()
			}
			builder := reasoningByChoice[choice.Index]
			if builder == nil {
				builder = &strings.Builder{}
				reasoningByChoice[choice.Index] = builder
			}
			builder.WriteString(reasoningContent)
		}
		if !accumulator.AddChunk(chunk) {
			return nil, &streamIntegrityError{reason: "contained inconsistent chunks"}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if len(choiceOrder) == 0 {
		return nil, &streamIntegrityError{reason: "contained no choices"}
	}
	for _, index := range choiceOrder {
		if !finishedChoices[index] {
			return nil, &streamIntegrityError{reason: fmt.Sprintf("ended before choice %d finished", index)}
		}
	}

	resp := c.mapOpenAIResponse(&accumulator.ChatCompletion)
	if usage != nil {
		resp.Usage = usage
	}
	for i := range resp.Choices {
		builder := reasoningByChoice[accumulator.Choices[i].Index]
		if builder != nil {
			resp.Choices[i].Message.ReasoningContent = builder.String()
		}
	}

	return resp, nil
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
	if req.ToolChoice != "" && len(tools) > 0 {
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String(req.ToolChoice),
		}
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

	// initErr defers a construction failure to the first request. The client
	// factory returns an LLMClient with no error channel, and the alternative —
	// panicking, as the SDK's own bedrock helper does — would surface a Go
	// stack trace to someone whose real problem is an expired AWS session.
	initErr error
}

// NewAnthropicClient creates a new Anthropic Messages API client.
// The Anthropic API manages prompt-cache affinity server-side, so unlike the
// OpenAI client no session key body field is injected; the key is still
// available to ExtraHeaders/ExtraBody via SessionKeyTemplateVar, applied per
// request so it can expand to the session key each request carries.
func NewAnthropicClient(cfg ClientConfig) *AnthropicClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.SessionKey == "" {
		cfg.SessionKey = NewSessionKey()
	}
	if !strings.HasSuffix(cfg.URL, "/v1/messages") && !strings.HasSuffix(cfg.URL, "/v1/messages/") {
		baseURL := strings.TrimRight(cfg.URL, "/")
		if !strings.HasSuffix(baseURL, "/v1/messages") {
			cfg.URL = baseURL + "/v1/messages"
		}
	}

	sdkBaseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/v1/messages")
	authHeader, _ := NormalizeAuthHeader(cfg.AuthHeader)
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

	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}
	if cfg.retryCollector != nil {
		opts = append(opts, option.WithMiddleware(newRetryObserver(cfg.retryCollector)))
	}

	return &AnthropicClient{
		cfg: cfg,
		sdk: anthropic.NewClient(opts...),
	}
}

// NewAnthropicBedrockClient creates a client for Anthropic models served by AWS
// Bedrock.
//
// The wire format is the Messages API, so this reuses AnthropicClient wholesale;
// the bedrock middleware from the official SDK handles what differs — SigV4
// signing, moving the model from the body into the URL path, injecting
// anthropic_version, and deriving the host from the region.
//
// No api_key is involved. Credentials come from the standard AWS chain
// (AWS_PROFILE, SSO cache, instance role, AWS_ACCESS_KEY_ID…), or from
// AWS_BEARER_TOKEN_BEDROCK if set. Region comes from AWS_REGION or the active
// profile.
func NewAnthropicBedrockClient(cfg ClientConfig) *AnthropicClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.SessionKey == "" {
		cfg.SessionKey = NewSessionKey()
	}

	// The middleware only rewrites requests whose path is already /v1/messages,
	// so keep cfg.URL in the same shape the plain Anthropic client uses. It is
	// not turned into a base URL: the region decides the host.
	if cfg.URL != "" && !strings.HasSuffix(strings.TrimRight(cfg.URL, "/"), "/v1/messages") {
		cfg.URL = strings.TrimRight(cfg.URL, "/") + "/v1/messages"
	}

	opts := []option.RequestOption{
		option.WithMaxRetries(5),
		option.WithHeader("User-Agent", userAgent("claude")),
		option.WithRequestTimeout(cfg.Timeout),
		// Bedrock authenticates by SigV4 signature, added by the middleware
		// below at transport time. Any API-key header the SDK would otherwise
		// attach — including an empty one — is rejected outright with
		// "Invalid API Key format: Must start with pre-defined prefix", so both
		// are removed here, before signing.
		option.WithHeaderDel("Authorization"),
		option.WithHeaderDel("X-Api-Key"),
	}
	// ExtraHeaders are applied per request in CompletionsWithCtx, where the
	// session key template can expand — same as the plain Anthropic client.
	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}
	if cfg.retryCollector != nil {
		opts = append(opts, option.WithMiddleware(newRetryObserver(cfg.retryCollector)))
	}

	// Load the AWS config here rather than calling bedrock.WithLoadDefaultConfig,
	// which panics on failure.
	var loadOpts []func(*awsconfig.LoadOptions) error
	if cfg.AWSProfile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(cfg.AWSProfile))
	}
	if cfg.AWSRegion != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.AWSRegion))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return &AnthropicClient{
			cfg: cfg,
			initErr: fmt.Errorf("bedrock: could not load AWS configuration: %w\n"+
				"  bedrock uses the standard AWS credential chain — set AWS_PROFILE, or run `aws sso login --profile <name>`", err),
		}
	}
	if awsCfg.Region == "" {
		return &AnthropicClient{
			cfg: cfg,
			initErr: fmt.Errorf("bedrock: no AWS region resolved\n" +
				"  set AWS_REGION, or give the active profile a region — the region decides which bedrock-runtime host is used"),
		}
	}

	// Force SigV4 unless a Bedrock bearer token was set deliberately.
	//
	// bedrock.WithConfig prefers bearer auth over SigV4 whenever
	// cfg.BearerAuthTokenProvider is non-nil, and LoadDefaultConfig populates
	// that provider from the SSO token cache — the OIDC access token, which is
	// for identity services, not Bedrock. So an SSO-authenticated caller
	// (i.e. most enterprise setups) silently sends `Authorization: Bearer
	// <sso-token>` and Bedrock answers 403 "Invalid API Key format: Must start
	// with pre-defined prefix". Clearing it here restores the SigV4 path.
	// WithConfig re-reads AWS_BEARER_TOKEN_BEDROCK when the provider is nil, so
	// an explicitly configured bearer token still wins.
	if os.Getenv("AWS_BEARER_TOKEN_BEDROCK") == "" {
		awsCfg.BearerAuthTokenProvider = nil
	}

	// Appended after the options above so its base URL and middleware win.
	opts = append(opts, bedrock.WithConfig(awsCfg))

	return &AnthropicClient{
		cfg: cfg,
		sdk: anthropic.NewClient(opts...),
	}
}

// CompletionsWithCtx sends a chat completion request with context support.
//
// The deferred finalizeRequest is this client's boundary for the retry report;
// see the OpenAI counterpart for why it is deferred and why the results are
// named. A parameter-building failure returns before any HTTP attempt, so
// Finalize finds no entry and the request stays out of the report entirely.
func (c *AnthropicClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (resp *ChatResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			finalizeRequest(ctx, c.cfg.retryCollector, errRequestPanicked)
			panic(r)
		}
		finalizeRequest(ctx, c.cfg.retryCollector, err)
	}()

	if c.initErr != nil {
		return nil, c.initErr
	}

	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params, err := c.buildAnthropicParams(model, req)
	if err != nil {
		return nil, err
	}

	sessionKey := c.cfg.SessionKey
	if k := SessionKeyFromContext(ctx); k != "" {
		sessionKey = k
	}

	var opts []option.RequestOption
	for k, v := range expandSessionKeyInHeaders(c.cfg.ExtraHeaders, sessionKey) {
		opts = append(opts, option.WithHeader(k, v))
	}
	for k, v := range expandSessionKeyInBody(c.cfg.ExtraBody, sessionKey) {
		// This client is non-streaming: it calls Messages.New, which expects a
		// single JSON body. If a provider config sets extra_body.stream=true,
		// forwarding it here makes the API answer with SSE and every call fails
		// to decode. Drop the key rather than forward it.
		if k == "stream" {
			continue
		}
		opts = append(opts, option.WithJSONSet(k, v))
	}

	sdkResp, err := c.sdk.Messages.New(ctx, params, opts...)
	if err != nil {
		return nil, err
	}

	return c.mapAnthropicResponse(sdkResp), nil
}

// buildAnthropicParams converts the shared ChatRequest into Anthropic SDK parameters.
func (c *AnthropicClient) buildAnthropicParams(model string, req ChatRequest) (anthropic.MessageNewParams, error) {
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
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsMap); err != nil {
						return anthropic.MessageNewParams{}, fmt.Errorf("invalid tool call arguments for %s: %w", tc.Function.Name, err)
					}
					if argsMap == nil {
						// null arguments → empty map; Anthropic API rejects
						// null input (#382). Same guard as llmloop.parseToolArgs.
						argsMap = map[string]any{}
					}
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
		if req.ToolChoice == "required" {
			params.ToolChoice = anthropic.ToolChoiceUnionParam{
				OfAny: &anthropic.ToolChoiceAnyParam{},
			}
		}
	}
	// Dynamic breakpoint on the latest message so multi-turn history is
	// cached incrementally: read the full previous prefix, write only the delta.
	if len(messages) > 0 {
		last := &messages[len(messages)-1]
		if len(last.Content) > 0 {
			if cc := last.Content[len(last.Content)-1].GetCacheControl(); cc != nil {
				*cc = anthropic.NewCacheControlEphemeralParam()
			}
		}
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	return params, nil
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
