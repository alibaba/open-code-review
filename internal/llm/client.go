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
	"strings"
	"sync"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
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
	Role       string         `json:"role"`
	Content    any            `json:"content"`                // string or []ContentBlock
	ToolCallID string         `json:"tool_call_id,omitempty"` // OpenAI tool call identifier
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`   // assistant tool invocations
	replay     replayEnvelope `json:"-"`                      // provider-owned continuation state
}

// replayEnvelope is intentionally private: orchestration can carry a
// provider's continuation state but cannot inspect or reconstruct it.
type replayEnvelope interface {
	isReplayEnvelope()
	tokenCount() int
}

// openAIReplay is the OpenAI Chat Completions adapter's native continuation
// envelope. It owns the complete assistant message used by the next provider
// request; its fields never enter normalized messages or persisted logs.
type openAIReplay struct {
	assistantMessage json.RawMessage
	approxTokenCount int
}

func (openAIReplay) isReplayEnvelope() {}
func (r openAIReplay) tokenCount() int { return r.approxTokenCount }

func newOpenAIReplay(assistantMessage json.RawMessage) replayEnvelope {
	if len(assistantMessage) == 0 || !openAIMessageHasReplayState(assistantMessage) {
		return nil
	}
	if !openAIReplayToolCallsAreValid(assistantMessage) {
		return nil
	}
	return openAIReplay{
		assistantMessage: append(json.RawMessage(nil), assistantMessage...),
		approxTokenCount: CountTokens(string(assistantMessage)),
	}
}

// AssistantTurn is the finalized assistant response exposed to the loop.
// Content and ToolCalls are the normalized view used for tool execution;
// Message retains the adapter-owned continuation state for the next request.
type AssistantTurn struct {
	content   string
	toolCalls []ToolCall
	replay    replayEnvelope
}

// Content returns visible assistant text without substituting hidden reasoning.
func (t AssistantTurn) Content() string { return t.content }

// IsEmpty reports whether the provider returned no visible content, tool
// calls, or continuation state worth replaying.
func (t AssistantTurn) IsEmpty() bool {
	return t.content == "" && len(t.toolCalls) == 0 && t.replay == nil
}

// ToolCalls returns a copy of the assistant's tool calls.
func (t AssistantTurn) ToolCalls() []ToolCall {
	if len(t.toolCalls) == 0 {
		return nil
	}
	calls := make([]ToolCall, len(t.toolCalls))
	copy(calls, t.toolCalls)
	return calls
}

// Message returns the complete assistant turn for replay. The continuation
// state is not part of the normalized JSON projection of Message.
func (t AssistantTurn) Message() Message {
	m := NewToolCallMessage(t.content, t.toolCalls)
	m.replay = t.replay
	return m
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

// Clone copies a message while retaining opaque provider replay state.
func (m Message) Clone() Message {
	cp := m
	cp.ToolCalls = append([]ToolCall(nil), m.ToolCalls...)
	if blocks, ok := m.Content.([]ContentBlock); ok {
		cp.Content = cloneContentBlocks(blocks)
	}
	return cp
}

func cloneContentBlocks(blocks []ContentBlock) []ContentBlock {
	cloned := append([]ContentBlock(nil), blocks...)
	for i := range cloned {
		if len(cloned[i].Content) > 0 {
			cloned[i].Content = cloneContentBlocks(cloned[i].Content)
		}
	}
	return cloned
}

// CloneMessages copies a conversation while retaining adapter-owned replay
// envelopes and isolating mutable normalized slices.
func CloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	for i, message := range messages {
		cloned[i] = message.Clone()
	}
	return cloned
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

// ResponseMessage extends Message with optional reasoning content. Refusal
// text is not normalized: it stays inside the adapter-owned replay envelope,
// which is its only consumer.
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
	turn    *AssistantTurn
}

// AssistantTurn returns the complete first-choice assistant turn. Responses
// produced by an adapter carry its finalized replay state; the fallback keeps
// custom LLMClient implementations backward compatible.
func (r *ChatResponse) AssistantTurn() AssistantTurn {
	if r == nil {
		return AssistantTurn{}
	}
	if r.turn != nil {
		return *r.turn
	}
	if len(r.Choices) == 0 {
		return AssistantTurn{}
	}
	msg := r.Choices[0].Message
	var content string
	if msg.Content != nil && *msg.Content != "" {
		content = strings.TrimSpace(stripThinkTags(*msg.Content))
	} else {
		// Parity with the pre-turn history rebuild (resp.Content()): a turn
		// whose only text lives in the reasoning channel replays that text,
		// since nothing else carries it forward.
		content = msg.ReasoningContent
	}
	return AssistantTurn{
		content:   content,
		toolCalls: append([]ToolCall(nil), msg.ToolCalls...),
	}
}

// ApproxCompletionTokenCount estimates the complete first-choice response,
// preferring provider-owned replay state when an adapter supplied it. This
// keeps hidden reasoning and tool-call payloads in accounting without exposing
// them through the normalized response projection.
func (r *ChatResponse) ApproxCompletionTokenCount() int {
	if r == nil || len(r.Choices) == 0 {
		return 0
	}
	if r.turn != nil {
		if r.turn.replay != nil {
			return r.turn.replay.tokenCount()
		}
		if r.turn.IsEmpty() {
			return 0
		}
	}
	raw, err := json.Marshal(r.Choices[0].Message)
	if err != nil {
		return CountTokens(r.Content())
	}
	return CountTokens(string(raw))
}

// ApproxTokenCount returns a rough request-token estimate while keeping
// provider-native replay text opaque to callers.
func (m Message) ApproxTokenCount() int {
	if m.replay != nil {
		return m.replay.tokenCount()
	}
	return CountTokens(m.ExtractText())
}

// ApproxMessagesTokenCount returns the rough token count of a conversation,
// including provider-owned replay text without exposing that text to callers.
func ApproxMessagesTokenCount(messages []Message) int {
	var total int
	for _, message := range messages {
		total += message.ApproxTokenCount()
	}
	return total
}

// IsReplayable reports whether the message carries an assistant continuation
// that must remain intact across provider requests.
func (m Message) IsReplayable() bool {
	return m.replay != nil || len(m.ToolCalls) > 0
}

// HasReplayState reports whether the message carries an opaque provider
// replay envelope. Unlike IsReplayable, a plain normalized tool-call message
// does not qualify: it can be rebuilt or summarized without corrupting a
// provider contract.
func (m Message) HasReplayState() bool {
	return m.replay != nil
}

// NewReplayStateMessageForTesting returns an assistant message carrying a
// synthetic opaque replay envelope. It exists so packages that consume replay
// semantics (e.g. llmloop compression) can exercise envelope-dependent paths
// without a provider round trip; production envelopes are only ever attached
// by protocol adapters.
func NewReplayStateMessageForTesting(content string, toolCalls []ToolCall) Message {
	m := NewToolCallMessage(content, toolCalls)
	raw, _ := json.Marshal(map[string]any{"role": "assistant", "content": content})
	m.replay = openAIReplay{assistantMessage: raw, approxTokenCount: CountTokens(content)}
	return m
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

// AssistantReplayNative opts a configured endpoint into provider-native
// assistant turn replay: the complete assistant message a provider returned
// is carried back verbatim on the next tool turn instead of being rebuilt
// from the normalized projection. Any other value keeps the rebuild.
const AssistantReplayNative = "native"

// ClientConfig holds configuration for connecting to an LLM service.
type ClientConfig struct {
	URL             string            // Full API endpoint URL
	APIKey          string            // Bearer token / API key
	Model           string            // Default model override
	Provider        string            // Resolved provider name (e.g. "deepseek"); informs provider-specific wire quirks
	AssistantReplay string            // AssistantReplayNative enables native turn replay; default keeps the normalized rebuild
	AuthHeader      string            // Auth header name: "x-api-key", "authorization", or empty for protocol default
	Timeout         time.Duration     // Request timeout
	ExtraBody       map[string]any    // Vendor-specific fields merged into every request body
	ExtraHeaders    map[string]string // Extra HTTP headers sent with every request
	RetryCodes      []int             // Additional HTTP status codes that trigger retry
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
func NewLLMClient(ep ResolvedEndpoint) LLMClient {
	cfg := ClientConfig{
		URL:             ep.URL,
		APIKey:          ep.Token,
		Model:           ep.Model,
		Provider:        ep.Provider,
		AssistantReplay: ep.AssistantReplay,
		AuthHeader:      ep.AuthHeader,
		Timeout:         ep.Timeout,
		ExtraBody:       ep.ExtraBody,
		ExtraHeaders:    ep.ExtraHeaders,
		RetryCodes:      ep.RetryCodes,
	}
	switch ep.Protocol {
	case ProtocolAnthropic:
		return NewAnthropicClient(cfg)
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
func NewOpenAIClient(cfg ClientConfig) *OpenAIClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
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
	for k, v := range cfg.ExtraHeaders {
		opts = append(opts, openaiopt.WithHeader(k, v))
	}
	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, openaiopt.WithMiddleware(mw))
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
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	SessionID   string    `json:"-"` // per-file agent loop session ID; used as prompt_cache_key by the Responses API client
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
		// Skip the "stream" key here. The streaming decision below uses a
		// dedicated boolean check, and when streaming is enabled the SDK's
		// NewStreaming method sets stream=true on the wire itself. When
		// streaming is NOT enabled, leaving the key in the body would make
		// the API answer with text/event-stream and the non-streaming path
		// fails to decode (see issue #647). "stream_options" is owned by the
		// streaming branch below for the same reason: providers reject it
		// unless stream is true.
		if k == "stream" || k == "stream_options" {
			continue
		}
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}
	if stream, ok := c.cfg.ExtraBody["stream"].(bool); ok && stream {
		if streamOptions, ok := c.cfg.ExtraBody["stream_options"]; !ok {
			// OpenAI-compatible servers omit token usage from streams unless
			// asked, silently losing cost accounting for streamed requests.
			// Ask for the final usage chunk by default.
			params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
		} else if streamOptions != nil {
			// An explicit stream_options in extra_body replaces the default,
			// but usage stays requested unless include_usage itself is spelled
			// out: configuring an unrelated stream option must not silently
			// disable cost accounting. An explicit null suppresses the field
			// entirely, for gateways that reject stream_options.
			if object, ok := streamOptions.(map[string]any); ok {
				if _, has := object["include_usage"]; !has {
					merged := make(map[string]any, len(object)+1)
					for key, value := range object {
						merged[key] = value
					}
					merged["include_usage"] = true
					streamOptions = merged
				}
			}
			opts = append(opts, openaiopt.WithJSONSet("stream_options", streamOptions))
		}
		return c.completionsStreaming(ctx, params, opts...)
	}

	sdkResp, err := c.sdk.Chat.Completions.New(ctx, params, opts...)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		retryResp, retryErr := c.sdk.Chat.Completions.New(ctx, params, opts...)
		if retryErr == nil {
			sdkResp = retryResp
			err = nil
		} else {
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

func (c *OpenAIClient) completionsStreaming(ctx context.Context, params openai.ChatCompletionNewParams, opts ...openaiopt.RequestOption) (*ChatResponse, error) {
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params, opts...)
	defer stream.Close()

	accumulator := openai.ChatCompletionAccumulator{}
	choiceStates := make(map[int64]*openAIStreamChoiceState)
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
			state := choiceStates[choice.Index]
			if state == nil {
				state = &openAIStreamChoiceState{}
				choiceStates[choice.Index] = state
				choiceOrder = append(choiceOrder, choice.Index)
			}
			if choice.FinishReason != "" {
				state.finished = true
			}
			if err := state.addDelta(choice.Delta.RawJSON()); err != nil {
				return nil, fmt.Errorf("accumulate OpenAI streaming choice %d: %w", choice.Index, err)
			}
		}
		if !accumulator.AddChunk(chunk) {
			return nil, fmt.Errorf("OpenAI streaming response contained inconsistent chunks")
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if len(choiceOrder) == 0 {
		return nil, fmt.Errorf("OpenAI streaming response contained no choices")
	}
	for _, index := range choiceOrder {
		if !choiceStates[index].finished {
			return nil, fmt.Errorf("OpenAI streaming response ended before choice %d finished", index)
		}
	}

	resp := c.mapOpenAIResponseNormalized(&accumulator.ChatCompletion)
	if usage != nil {
		resp.Usage = usage
	}
	var firstReplayMessage json.RawMessage
	for i := range resp.Choices {
		index := accumulator.Choices[i].Index
		state := choiceStates[index]
		if state == nil {
			continue
		}
		state.applyNormalizedFields(&resp.Choices[i].Message)
		if i == 0 && c.nativeReplayEnabled() {
			var err error
			firstReplayMessage, err = state.finalize(accumulator.Choices[i].Message, c.replayPolicy())
			if err != nil {
				return nil, fmt.Errorf("finalize OpenAI streaming assistant message: %w", err)
			}
		}
	}
	if len(resp.Choices) > 0 {
		resp.turn = openAIAssistantTurnFromResponse(resp, firstReplayMessage)
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
			if replay, ok := msg.replay.(openAIReplay); ok {
				asst := param.Override[openai.ChatCompletionAssistantMessageParam](replay.assistantMessage)
				messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
				continue
			}
			asst := openai.ChatCompletionAssistantMessageParam{}
			// Content may only be omitted on tool-call messages; a bare
			// {"role":"assistant"} is rejected by strict servers.
			if content != "" || len(msg.ToolCalls) == 0 {
				asst.Content.OfString = openai.String(content)
			}
			if len(msg.ToolCalls) > 0 {
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
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
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

// mapOpenAIResponse converts the SDK response into ChatResponse, attaching
// the provider-native replay envelope when the endpoint opted into it.
func (c *OpenAIClient) mapOpenAIResponse(sdkResp *openai.ChatCompletion) *ChatResponse {
	resp := c.mapOpenAIResponseNormalized(sdkResp)
	var firstReplayMessage json.RawMessage
	if c.nativeReplayEnabled() && len(sdkResp.Choices) > 0 && sdkResp.Choices[0].Message.RawJSON() != "" {
		if replayMessage, err := openAIReplayMessageFromResponse(sdkResp.Choices[0].Message, c.replayPolicy()); err == nil {
			firstReplayMessage = replayMessage
		}
	}
	resp.turn = openAIAssistantTurnFromResponse(resp, firstReplayMessage)
	return resp
}

// mapOpenAIResponseNormalized converts the SDK response into the normalized
// ChatResponse without attaching a turn: the streaming path finalizes its own
// per-choice state and envelope on top of this.
func (c *OpenAIClient) mapOpenAIResponseNormalized(sdkResp *openai.ChatCompletion) *ChatResponse {
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

		var contentPtr *string
		if rawContent := ch.Message.JSON.Content.Raw(); rawContent != "" && rawContent != "null" {
			var content string
			if err := json.Unmarshal([]byte(rawContent), &content); err == nil {
				contentPtr = &content
			}
		} else if ch.Message.Content != "" {
			content := ch.Message.Content
			contentPtr = &content
		}

		var reasoningContent string
		if extra, ok := ch.Message.JSON.ExtraFields["reasoning_content"]; ok {
			rawReasoning := extra.Raw()
			if rawReasoning != "" && rawReasoning != "null" {
				reasoningContent = decodeReasoningContent([]byte(rawReasoning))
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

func decodeReasoningContent(raw []byte) string {
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return string(raw)
	}
	return content
}

func openAIAssistantTurnFromResponse(resp *ChatResponse, rawMessage json.RawMessage) *AssistantTurn {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	msg := resp.Choices[0].Message
	replay := newOpenAIReplay(rawMessage)
	// The normalized view follows the same contract as the generic fallback:
	// think tags stripped, and — only when no opaque envelope carries the
	// turn — reasoning substituted for empty content, matching the
	// pre-envelope history rebuild.
	var content string
	if msg.Content != nil && *msg.Content != "" {
		content = strings.TrimSpace(stripThinkTags(*msg.Content))
	} else if replay == nil {
		content = msg.ReasoningContent
	}
	return &AssistantTurn{
		content:   content,
		toolCalls: append([]ToolCall(nil), msg.ToolCalls...),
		replay:    replay,
	}
}

func openAIMessageHasReplayState(rawMessage json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rawMessage, &fields); err != nil {
		return false
	}
	for name, raw := range fields {
		if name == "role" {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return true
		}
		switch v := value.(type) {
		case nil:
		case string:
			if v != "" {
				return true
			}
		case []any:
			if len(v) > 0 {
				return true
			}
		case map[string]any:
			if len(v) > 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
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

	for k, v := range cfg.ExtraHeaders {
		opts = append(opts, option.WithHeader(k, v))
	}
	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
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

	params, err := c.buildAnthropicParams(model, req)
	if err != nil {
		return nil, err
	}

	var opts []option.RequestOption
	for k, v := range c.cfg.ExtraBody {
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
