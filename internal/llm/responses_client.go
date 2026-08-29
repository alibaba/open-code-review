// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/sjson"
)

// --- OpenAIResponsesClient ---

// OpenAIResponsesClient speaks the OpenAI Responses API (/v1/responses) using
// the official SDK. It is stateless: every request carries the full input
// history (no previous_response_id), so the agent loop does not need to track
// server-side response IDs. See DESIGN_STATE_CACHE_PHASE.md for the rationale.
type OpenAIResponsesClient struct {
	cfg ClientConfig
	sdk openai.Client
}

// NewOpenAIResponsesClient creates a client for the OpenAI Responses API.
// URL normalization mirrors NewOpenAIClient: cfg.URL is forced to end in
// /responses, and that suffix is stripped to derive the SDK base URL (the SDK
// appends "responses" itself).
// ExtraHeaders are applied per request (not baked into the SDK client)
// so SessionKeyTemplateVar can expand to the session key each request carries.
func NewOpenAIResponsesClient(cfg ClientConfig) *OpenAIResponsesClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.SessionKey == "" {
		cfg.SessionKey = NewSessionKey()
	}
	ensureResponsesEndpoint(&cfg)
	sdkBaseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/responses")

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
	if cfg.DetailErrorEnvelope {
		opts = append(opts, openaiopt.WithMiddleware(rewriteDetailErrorMiddleware))
	}
	if cfg.retryCollector != nil {
		opts = append(opts, openaiopt.WithMiddleware(newRetryObserver(cfg.retryCollector)))
	}

	return &OpenAIResponsesClient{
		cfg: cfg,
		sdk: openai.NewClient(opts...),
	}
}

// ensureResponsesEndpoint normalizes cfg.URL to end with /responses. The
// trailing /responses is kept on cfg.URL (so tests and logs see the full
// endpoint) and the SDK base URL is derived by stripping it.
//
// Contract (mirrors TestNewOpenAIClient_URLNormalization):
//
//	https://api.openai.com/v1             -> https://api.openai.com/v1/responses
//	https://api.openai.com/v1/            -> https://api.openai.com/v1/responses
//	https://api.openai.com/v1/responses   -> https://api.openai.com/v1/responses
//	https://api.openai.com/v1/responses/  -> https://api.openai.com/v1/responses
//	https://api.openai.com                -> https://api.openai.com/responses
func ensureResponsesEndpoint(cfg *ClientConfig) {
	baseURL := strings.TrimRight(cfg.URL, "/")
	if !strings.HasSuffix(baseURL, "/responses") {
		baseURL = baseURL + "/responses"
	}
	cfg.URL = baseURL
}

// rewriteDetailErrorMiddleware translates the Codex gateway's detail-only
// error envelope into the shape the OpenAI SDK parses.
func rewriteDetailErrorMiddleware(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	resp, err := next(req)
	if err != nil || resp == nil || resp.Body == nil || resp.StatusCode < http.StatusBadRequest {
		return resp, err
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return resp, fmt.Errorf("read Codex error response: %w", readErr)
	}
	if closeErr != nil {
		return resp, fmt.Errorf("close Codex error response: %w", closeErr)
	}
	setResponseBody(resp, body)

	var envelope struct {
		Detail string          `json:"detail"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Detail == "" || len(envelope.Error) > 0 {
		return resp, nil
	}

	rewritten, err := json.Marshal(struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: struct {
			Message string `json:"message"`
		}{Message: envelope.Detail},
	})
	if err != nil {
		return resp, fmt.Errorf("rewrite Codex error response: %w", err)
	}
	setResponseBody(resp, rewritten)
	return resp, nil
}

func setResponseBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
}

// CompletionsWithCtx sends a Responses API request and maps the result back to
// the shared ChatResponse shape.
//
// The deferred finalizeRequest is this client's boundary for the retry report;
// see the OpenAI Chat Completions counterpart for why it is deferred and why the
// results are named.
func (c *OpenAIResponsesClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (resp *ChatResponse, err error) {
	defer func() {
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

	params := c.buildResponsesParams(model, req)

	sessionKey := c.cfg.SessionKey
	if k := SessionKeyFromContext(ctx); k != "" {
		sessionKey = k
	}

	var opts []openaiopt.RequestOption
	for k, v := range expandSessionKeyInHeaders(c.cfg.ExtraHeaders, sessionKey) {
		opts = append(opts, openaiopt.WithHeader(k, v))
	}
	for k, v := range expandSessionKeyInBody(c.cfg.ExtraBody, sessionKey) {
		// Streaming is selected only by the resolved provider. Forwarding an
		// extra_body.stream setting could conflict with that selection or make an
		// ordinary Responses.New call receive SSE, so it is always dropped here.
		if k == "stream" {
			continue
		}
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}

	var sdkResp *responses.Response
	if c.cfg.RequiresStreaming {
		sdkResp, err = c.responsesStreaming(ctx, params, opts...)
	} else {
		sdkResp, err = c.sdk.Responses.New(ctx, params, opts...)
	}
	if err != nil {
		return nil, err
	}

	if err = checkResponseStatus(sdkResp); err != nil {
		reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassProvider, FailurePhaseResponseStatus)
		return nil, err
	}

	return c.mapResponsesResponse(sdkResp), nil
}

type responseStatusError struct {
	message string
}

func (e *responseStatusError) Error() string { return e.message }

// checkResponseStatus turns unsuccessful response objects into errors. The
// Responses API can return these states with HTTP 200, so the SDK does not
// reject them itself.
func checkResponseStatus(resp *responses.Response) error {
	switch resp.Status {
	case responses.ResponseStatusFailed, responses.ResponseStatusCancelled:
		return &responseStatusError{message: fmt.Sprintf("openai-responses request did not complete: status=%s", resp.Status)}
	case responses.ResponseStatusQueued, responses.ResponseStatusInProgress:
		return &responseStatusError{message: fmt.Sprintf("openai-responses returned non-terminal status=%s (background/async mode is not supported)", resp.Status)}
	default:
		return nil
	}
}

type responseStreamEventError struct {
	code    string
	message string
	param   string
}

func (e *responseStreamEventError) Error() string {
	message := "openai-responses stream error"
	if e.code != "" {
		message += ": code=" + e.code
	}
	if e.message != "" {
		message += ": " + e.message
	}
	if e.param != "" {
		message += " (param=" + e.param + ")"
	}
	return message
}

func (c *OpenAIResponsesClient) responsesStreaming(ctx context.Context, params responses.ResponseNewParams, opts ...openaiopt.RequestOption) (*responses.Response, error) {
	resp, err := c.responsesStreamingInner(ctx, params, opts...)
	if err == nil {
		return resp, nil
	}

	var statusErr *responseStatusError
	if errors.As(err, &statusErr) {
		reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassProvider, FailurePhaseResponseStatus)
	} else {
		class, phase := classifyStreamError(err)
		reviseAttempt(ctx, c.cfg.retryCollector, class, phase)
	}
	return nil, err
}

func (c *OpenAIResponsesClient) responsesStreamingInner(ctx context.Context, params responses.ResponseNewParams, opts ...openaiopt.RequestOption) (*responses.Response, error) {
	stream := c.sdk.Responses.NewStreaming(ctx, params, opts...)
	defer stream.Close()

	var accumulator responseStreamAccumulator
	for stream.Next() {
		if err := accumulator.add(stream.Current()); err != nil {
			return nil, err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return accumulator.response()
}

type indexedResponseOutputItem struct {
	outputIndex int64
	raw         json.RawMessage
}

type responseStreamAccumulator struct {
	items    []indexedResponseOutputItem
	terminal *responses.Response
}

func (a *responseStreamAccumulator) add(event responses.ResponseStreamEventUnion) error {
	switch event.Type {
	case "response.output_item.done":
		done := event.AsResponseOutputItemDone()
		a.items = append(a.items, indexedResponseOutputItem{
			outputIndex: done.OutputIndex,
			raw:         json.RawMessage(done.Item.RawJSON()),
		})
	case "response.completed", "response.failed", "response.incomplete":
		response := event.Response
		a.terminal = &response
	case "error":
		streamErr := event.AsError()
		return &responseStreamEventError{
			code:    streamErr.Code,
			message: streamErr.Message,
			param:   streamErr.Param,
		}
	}
	return nil
}

func (a *responseStreamAccumulator) response() (*responses.Response, error) {
	if a.terminal == nil {
		return nil, &streamIntegrityError{reason: "ended before a terminal event"}
	}

	sort.SliceStable(a.items, func(i, j int) bool {
		return a.items[i].outputIndex < a.items[j].outputIndex
	})
	rawItems := make([]json.RawMessage, len(a.items))
	for i := range a.items {
		rawItems[i] = a.items[i].raw
	}
	itemsJSON, err := json.Marshal(rawItems)
	if err != nil {
		return nil, fmt.Errorf("marshal openai-responses stream output: %w", err)
	}

	merged, err := sjson.SetRawBytes([]byte(a.terminal.RawJSON()), "output", itemsJSON)
	if err != nil {
		return nil, fmt.Errorf("merge openai-responses stream output: %w", err)
	}
	var full responses.Response
	if err := full.UnmarshalJSON(merged); err != nil {
		return nil, fmt.Errorf("unmarshal accumulated openai-responses stream: %w", err)
	}
	if err := checkResponseStatus(&full); err != nil {
		return nil, err
	}
	if full.Status != responses.ResponseStatusCompleted && full.Status != responses.ResponseStatusIncomplete {
		return nil, &responseStatusError{message: fmt.Sprintf(
			"openai-responses terminal event carried unexpected status=%q", full.Status)}
	}
	return &full, nil
}

// accumulateResponseStream rebuilds a complete response from output-item done
// events and the terminal response envelope. Codex leaves the terminal
// envelope's output array empty even though it emitted complete output items.
func accumulateResponseStream(events []responses.ResponseStreamEventUnion) (*responses.Response, error) {
	var accumulator responseStreamAccumulator
	for _, event := range events {
		if err := accumulator.add(event); err != nil {
			return nil, err
		}
	}
	return accumulator.response()
}

// buildResponsesParams converts the shared ChatRequest into Responses API
// parameters. Mapping notes:
//
//   - Multiple system messages are concatenated into Instructions (\n\n joined).
//     Responses API exposes a single top-level Instructions field.
//   - assistant messages with ToolCalls are split: an optional assistant message
//     item carries any text, then each ToolCall becomes a function_call item
//     keyed by the tool call's ID (the CallID the loop pairs results against).
//   - role=tool messages (ToolCallID set) become function_call_output items.
//   - store is forced to false (stateless, privacy-preserving; see
//     DESIGN_STATE_CACHE_PHASE.md §4).
//   - PromptCacheKey is set from req.SessionID when non-empty. The caller
//     generates a random UUID per file session so that all turns within one
//     file's agent loop share a cache bucket. Only set when non-empty.
//     An explicit extra_body.prompt_cache_key entry is applied afterwards as a JSON patch.
func (c *OpenAIResponsesClient) buildResponsesParams(model string, req ChatRequest) responses.ResponseNewParams {
	var systemParts []string
	var input []responses.ResponseInputItemUnionParam

	for _, msg := range req.Messages {
		content := msg.ExtractText()
		switch msg.Role {
		case "system":
			if content != "" {
				systemParts = append(systemParts, content)
			}
		case "user":
			input = append(input, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		case "assistant":
			// Reuse native output items to preserve reasoning/encrypted_content.
			if items, ok := msg.Native.Payload.([]responses.ResponseInputItemUnionParam); ok && len(items) > 0 {
				input = append(input, items...)
				continue
			}
			if content != "" {
				input = append(input, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleAssistant))
			}
			for _, tc := range msg.ToolCalls {
				input = append(input, responses.ResponseInputItemParamOfFunctionCall(tc.Function.Arguments, tc.ID, tc.Function.Name))
			}
		case "tool":
			input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(msg.ToolCallID, content))
		default:
			input = append(input, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		}
	}

	instructions := strings.Join(systemParts, "\n\n")

	var tools []responses.ToolUnionParam
	for _, t := range req.Tools {
		tool := responses.FunctionToolParam{
			Name:        t.Function.Name,
			Parameters:  t.Function.Parameters,
			Strict:      openai.Bool(false),
			Description: openai.String(t.Function.Description),
		}
		tools = append(tools, responses.ToolUnionParam{OfFunction: &tool})
	}

	params := responses.ResponseNewParams{
		Model: openai.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Store:   openai.Bool(false),
		Include: []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent},
	}

	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if req.SessionID != "" {
		params.PromptCacheKey = openai.String(req.SessionID)
	}
	// The API default is "in_memory", which is short lived. A review issues
	// dozens of turns against one growing conversation, so ask for the longer
	// retention class where the provider supports it.
	if c.cfg.PromptCacheRetention != "" {
		params.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetention(c.cfg.PromptCacheRetention)
	}
	if len(tools) > 0 {
		params.Tools = tools
		if req.ToolChoice == "required" {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
			}
		}
	}
	if !c.cfg.RejectsSamplingParams {
		if req.MaxTokens > 0 {
			params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
		}
		if req.Temperature != nil {
			params.Temperature = openai.Float(*req.Temperature)
		}
	}

	return params
}

// mapResponsesResponse converts the SDK Response into the shared ChatResponse.
// Text output is read via the SDK's OutputText() helper (it walks all output
// items and aggregates type=="output_text" content). Function calls become
// ToolCalls keyed by CallID so the agent loop's NewToolResultMessage(call.ID,
// ...) pairs correctly.
func (c *OpenAIResponsesClient) mapResponsesResponse(sdkResp *responses.Response) *ChatResponse {
	var contentPtr *string
	if text := sdkResp.OutputText(); text != "" {
		cleaned := stripThinkTags(text)
		contentPtr = &cleaned
	}

	var toolCalls []ToolCall
	var reasoningParts []string
	var nativeItems []responses.ResponseInputItemUnionParam
	// hasActionableItem gates Native: a lone reasoning item (no message or
	// function_call) is not valid standalone input and risks a 400 on replay.
	var hasActionableItem bool
	for _, item := range sdkResp.Output {
		switch item.Type {
		case "function_call":
			fc := item.AsFunctionCall()
			toolCalls = append(toolCalls, ToolCall{
				ID:   fc.CallID,
				Type: "function",
				Function: FunctionCall{
					Name:      fc.Name,
					Arguments: fc.Arguments,
				},
			})
			p := fc.ToParam()
			nativeItems = append(nativeItems, responses.ResponseInputItemUnionParam{OfFunctionCall: &p})
			hasActionableItem = true
		case "reasoning":
			// Best-effort: aggregate every summary entry's Text (not just the
			// first) so multi-paragraph reasoning isn't truncated.
			r := item.AsReasoning()
			for _, s := range r.Summary {
				if s.Text != "" {
					reasoningParts = append(reasoningParts, s.Text)
				}
			}
			p := r.ToParam()
			nativeItems = append(nativeItems, responses.ResponseInputItemUnionParam{OfReasoning: &p})
		case "message":
			m := item.AsMessage()
			p := m.ToParam()
			nativeItems = append(nativeItems, responses.ResponseInputItemUnionParam{OfOutputMessage: &p})
			hasActionableItem = true
		}
	}

	var reasoningContent string
	if len(reasoningParts) > 0 {
		reasoningContent = strings.Join(reasoningParts, "\n")
	}

	var native NativeTurn
	if hasActionableItem && len(nativeItems) > 0 {
		native = NativeTurn{Family: "openai-responses", Payload: nativeItems}
	}

	finishReason := mapResponsesFinishReason(string(sdkResp.Status), toolCalls)

	var usage *UsageInfo
	rawUsage := resolveUsage([]byte(sdkResp.RawJSON()))
	if rawUsage != nil {
		usage = rawUsage
	} else {
		u := sdkResp.Usage
		if u.InputTokens > 0 || u.OutputTokens > 0 || u.TotalTokens > 0 {
			usage = &UsageInfo{
				PromptTokens:     u.InputTokens,
				CompletionTokens: u.OutputTokens,
				CacheReadTokens:  u.InputTokensDetails.CachedTokens,
				TotalTokens:      u.TotalTokens,
			}
		}
	}

	return &ChatResponse{
		ID:    sdkResp.ID,
		Model: string(sdkResp.Model),
		Choices: []Choice{{
			Message: ResponseMessage{
				Role:             "assistant",
				Content:          contentPtr,
				ReasoningContent: reasoningContent,
				ToolCalls:        toolCalls,
				Native:           native,
			},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}
}

// mapResponsesFinishReason applies the coarse-grained mapping from decision 8:
//   - completed -> stop
//   - incomplete -> length
//   - failed/cancelled -> error
//   - any tool calls present -> tool_calls (overrides status, since a model
//     that emitted function calls is mid-tool-loop regardless of API status)
//   - otherwise -> stop (defensive default; keeps the loop progressing)
func mapResponsesFinishReason(status string, toolCalls []ToolCall) string {
	if len(toolCalls) > 0 {
		return "tool_calls"
	}
	switch status {
	case string(responses.ResponseStatusIncomplete):
		return "length"
	case string(responses.ResponseStatusFailed), string(responses.ResponseStatusCancelled):
		return "error"
	default:
		return "stop"
	}
}
