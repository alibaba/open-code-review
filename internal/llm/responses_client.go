// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/chatgpt"

	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
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
		openaiopt.WithHTTPClient(httpClientWithHeaderTimeout(cfg.Timeout)),
	}
	if cfg.ChatGPT {
		opts = append(opts, openaiopt.WithMaxRetries(0))
	}
	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, openaiopt.WithMiddleware(mw))
	}
	// Raw before the retry observer; see NewOpenAIClient for why order matters.
	if cfg.rawHolder != nil {
		opts = append(opts, openaiopt.WithMiddleware(newRawMiddleware(cfg.rawHolder)))
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
		err = describeTimeout(ctx, err)
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
	if c.cfg.ChatGPT {
		return c.chatGPTCompletion(ctx, params, sessionKey)
	}

	var opts []openaiopt.RequestOption
	for k, v := range expandSessionKeyInHeaders(c.cfg.ExtraHeaders, sessionKey) {
		opts = append(opts, openaiopt.WithHeader(k, v))
	}
	for k, v := range expandSessionKeyInBody(c.cfg.ExtraBody, sessionKey) {
		// This client is non-streaming: it calls Responses.New, which expects a
		// single JSON body. If a provider config sets extra_body.stream=true
		// (valid for the Chat Completions client, which switches to a streaming
		// path), forwarding it here makes the API answer with SSE and every
		// call fails to decode. Drop the key rather than forward it.
		if k == "stream" {
			continue
		}
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}

	sdkResp, err := c.sdk.Responses.New(ctx, params, opts...)
	if err != nil {
		return nil, withProviderErrorBody(err)
	}

	// The Responses API returns HTTP 200 even when the response object is in a
	// terminal failure state (failed/cancelled) or a non-terminal background
	// state (queued/in_progress). The SDK therefore returns a nil Go error in
	// those cases. Surface them as real errors so callers (ocr llm test, the
	// review loop) that branch on err != nil actually fail instead of treating
	// a dead response as success.
	switch sdkResp.Status {
	case responses.ResponseStatusFailed, responses.ResponseStatusCancelled:
		err = fmt.Errorf("openai-responses request did not complete: status=%s", sdkResp.Status)
	case responses.ResponseStatusQueued, responses.ResponseStatusInProgress:
		err = fmt.Errorf("openai-responses returned non-terminal status=%s (background/async mode is not supported)", sdkResp.Status)
	}
	if err != nil {
		// Correct the attempt here, where the status is known. The observer saw
		// only the HTTP 200 that carried this dead response object, and nothing
		// downstream would catch the omission: a request whose outcome is failed is
		// listed with no error attempt at all, producing self-consistent counts
		// over a record that misstates what happened.
		reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassProvider, FailurePhaseResponseStatus)
		return nil, err
	}

	return c.mapResponsesResponse(sdkResp), nil
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
				item := responses.ResponseInputItemParamOfFunctionCall(tc.Function.Arguments, tc.ID, tc.Function.Name)
				if c.cfg.ChatGPT {
					item.OfFunctionCall.Namespace = openai.String(chatGPTToolNamespace)
				}
				input = append(input, item)
			}
		case "tool":
			input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(msg.ToolCallID, content))
		default:
			input = append(input, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		}
	}

	instructions := strings.Join(systemParts, "\n\n")

	var tools []responses.ToolUnionParam
	if c.cfg.ChatGPT && len(req.Tools) > 0 {
		var functions []responses.NamespaceToolToolUnionParam
		for _, t := range req.Tools {
			functions = append(functions, responses.NamespaceToolToolUnionParam{OfFunction: &responses.NamespaceToolToolFunctionParam{
				Name: t.Function.Name, Parameters: t.Function.Parameters, Description: openai.String(t.Function.Description), Strict: openai.Bool(false),
			}})
		}
		tools = []responses.ToolUnionParam{responses.ToolParamOfNamespace("Local code review tools", chatGPTToolNamespace, functions)}
	} else {
		for _, t := range req.Tools {
			tool := responses.FunctionToolParam{
				Name:        t.Function.Name,
				Parameters:  t.Function.Parameters,
				Strict:      openai.Bool(false),
				Description: openai.String(t.Function.Description),
			}
			tools = append(tools, responses.ToolUnionParam{OfFunction: &tool})
		}
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
	if len(tools) > 0 {
		params.Tools = tools
		if req.ToolChoice == "required" || req.ToolChoice == "none" {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptions(req.ToolChoice)),
			}
		}
	}
	if req.MaxTokens > 0 && !c.cfg.ChatGPT {
		params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Temperature != nil && !c.cfg.ChatGPT {
		params.Temperature = openai.Float(*req.Temperature)
	}

	return params
}

func (c *OpenAIResponsesClient) chatGPTCompletion(ctx context.Context, params responses.ResponseNewParams, sessionKey string) (*ChatResponse, error) {
	if c.cfg.tokenSource == nil {
		return nil, fmt.Errorf("ChatGPT OAuth token source is unavailable; run 'ocr llm login openai-chatgpt'")
	}
	if err := checkChatGPTExtraBody(c.cfg.ExtraBody); err != nil {
		return nil, err
	}
	token, err := c.cfg.tokenSource(ctx)
	if err != nil {
		return nil, err
	}
	// The SDK picks up OPENAI_ORG_ID / OPENAI_PROJECT_ID from the environment for
	// API-key users; those platform scopes must not ride along with a plan token.
	opts := []openaiopt.RequestOption{
		openaiopt.WithAPIKey(token),
		openaiopt.WithHeaderDel("OpenAI-Organization"),
		openaiopt.WithHeaderDel("OpenAI-Project"),
	}
	for k, v := range expandSessionKeyInBody(c.cfg.ExtraBody, sessionKey) {
		opts = append(opts, openaiopt.WithJSONSet(k, v))
	}
	stream := c.sdk.Responses.NewStreaming(ctx, params, opts...)
	defer stream.Close()
	var completed *responses.Response
	items := make(map[int64]responses.ResponseOutputItemUnion)
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_item.done":
			done := event.AsResponseOutputItemDone()
			if done.OutputIndex < 0 {
				return nil, c.chatGPTStreamFailure(ctx, "returned an invalid output index")
			}
			items[done.OutputIndex] = done.Item
		case "response.completed":
			r := event.AsResponseCompleted().Response
			if r.Status != responses.ResponseStatusCompleted || r.ID == "" {
				return nil, c.chatGPTStreamFailure(ctx, "returned an invalid completed response")
			}
			completed = &r
		case "response.failed", "response.incomplete", "error":
			code, detail := chatGPTStreamEventDetail(event)
			failure := c.chatGPTStreamFailure(ctx, "did not complete ("+detail+")")
			if mapped := chatGPTSubscriptionError(code, failure); mapped != nil {
				return nil, mapped
			}
			return nil, failure
		}
	}
	if err := stream.Err(); err != nil {
		class, phase := classifyStreamError(err)
		reviseAttempt(ctx, c.cfg.retryCollector, class, phase)
		return nil, chatGPTRequestError(err)
	}
	if completed == nil {
		return nil, c.chatGPTStreamFailure(ctx, "ended before response.completed")
	}
	// Subscription streams can leave output empty on the terminal response.
	// Only finalized items carry complete tool arguments and encrypted reasoning.
	if len(items) > 0 {
		for i, item := range completed.Output {
			if _, exists := items[int64(i)]; !exists {
				items[int64(i)] = item
			}
		}
		indices := make([]int64, 0, len(items))
		for index := range items {
			indices = append(indices, index)
		}
		slices.Sort(indices)
		completed.Output = nil
		for position, index := range indices {
			if index != int64(position) {
				return nil, c.chatGPTStreamFailure(ctx, "returned non-contiguous output items")
			}
			completed.Output = append(completed.Output, items[index])
		}
	}
	if len(completed.Output) == 0 {
		return nil, c.chatGPTStreamFailure(ctx, "completed without any output items")
	}
	return c.mapResponsesResponse(completed), nil
}

func (c *OpenAIResponsesClient) chatGPTStreamFailure(ctx context.Context, reason string) error {
	reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassProvider, FailurePhaseStream)
	return &streamIntegrityError{reason: reason}
}

// chatGPTStreamEventDetail extracts the provider's reason from a terminal
// failure event, so an in-stream quota or eligibility error is not reduced to
// its event type.
func chatGPTStreamEventDetail(event responses.ResponseStreamEventUnion) (code, detail string) {
	var message string
	switch event.Type {
	case "error":
		e := event.AsError()
		code, message = e.Code, e.Message
	case "response.failed":
		e := event.AsResponseFailed().Response.Error
		code, message = string(e.Code), e.Message
	case "response.incomplete":
		message = event.AsResponseIncomplete().Response.IncompleteDetails.Reason
	}
	detail = event.Type
	for _, part := range []string{code, message} {
		if part != "" {
			detail += ": " + part
		}
	}
	return code, detail
}

func chatGPTRequestError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		if mapped := chatGPTSubscriptionError(apiErr.Code, err); mapped != nil {
			return mapped
		}
	}
	return withProviderErrorBody(err)
}

// chatGPTSubscriptionError turns a plan-usage error code into an actionable
// message, or returns nil for any other code.
func chatGPTSubscriptionError(code string, err error) error {
	switch code {
	case "subscription_sharing_usage_limit_exceeded":
		return fmt.Errorf("ChatGPT plan usage limit reached; manage app usage at %s: %w", chatgpt.UsageURL, err)
	case "subscription_sharing_user_not_eligible":
		return fmt.Errorf("this ChatGPT account or workspace is not eligible for plan usage: %w", err)
	case "subscription_sharing_invalid_user":
		return fmt.Errorf("ChatGPT could not validate this session; run 'ocr llm login openai-chatgpt': %w", err)
	}
	return nil
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
