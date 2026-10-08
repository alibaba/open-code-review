// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
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
	cfg          ClientConfig
	sdk          openai.Client
	chatGPTToken func(context.Context) (string, error)
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

	httpClient := httpClientWithHeaderTimeout(cfg.Timeout)
	retries := 5
	if cfg.ChatGPTPlan {
		retries = 0
		httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	opts := []openaiopt.RequestOption{
		openaiopt.WithAPIKey(cfg.APIKey),
		openaiopt.WithBaseURL(sdkBaseURL),
		openaiopt.WithMaxRetries(retries),
		openaiopt.WithHeader("User-Agent", userAgent("")),
		openaiopt.WithRequestTimeout(cfg.Timeout),
		openaiopt.WithHTTPClient(httpClient),
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

	client := &OpenAIResponsesClient{
		cfg: cfg,
		sdk: openai.NewClient(opts...),
	}
	if cfg.ChatGPTPlan {
		client.chatGPTToken = func(ctx context.Context) (string, error) {
			auth, err := chatGPTCredentials(ctx, cfg.ChatGPTAccount)
			if err != nil {
				return "", err
			}
			return auth.AccessToken, nil
		}
	}
	return client
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

	var planToken string
	if c.cfg.ChatGPTPlan {
		if c.cfg.URL != chatgptauth.Resource+"/responses" || !c.cfg.RequiresStreaming {
			return nil, errors.New("chatgpt preview requires the public OpenAI Responses endpoint and streaming")
		}
		if err := validateChatGPTBody(c.cfg.ExtraBody); err != nil {
			return nil, err
		}
		for key := range c.cfg.ExtraHeaders {
			if reservedHeaders[strings.ToLower(key)] || strings.EqualFold(key, "Host") {
				return nil, fmt.Errorf("chatgpt cannot override header %s", key)
			}
		}
		planToken, err = c.chatGPTToken(ctx)
		if err != nil {
			return nil, err
		}
	}
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
	if c.cfg.ChatGPTPlan {
		opts = append(opts, openaiopt.WithAPIKey(planToken))
	}
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
		return nil, withProviderErrorBody(err)
	}

	if err = checkResponseStatus(sdkResp); err != nil {
		reviseAttempt(ctx, c.cfg.retryCollector, ErrorClassProvider, FailurePhaseResponseStatus)
		return nil, err
	}

	if c.cfg.ChatGPTPlan {
		for _, item := range sdkResp.Output {
			if item.Type == "function_call" && item.AsFunctionCall().Namespace != "ocr" {
				return nil, errors.New("chatgpt returned a function call outside the offered OCR namespace")
			}
		}
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

	accumulator := responseStreamAccumulator{requireCompleted: c.cfg.ChatGPTPlan}
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

const maxResponseStreamOutputBytes = 16 << 20

type responseStreamAccumulator struct {
	items            []indexedResponseOutputItem
	outputBytes      int
	terminal         *responses.Response
	requireCompleted bool
	terminalType     string
}

func (a *responseStreamAccumulator) add(event responses.ResponseStreamEventUnion) error {
	switch event.Type {
	case "response.output_item.done":
		done := event.AsResponseOutputItemDone()
		raw := done.Item.RawJSON()
		// Include per-item bookkeeping so a stream of tiny items is bounded too.
		const itemOverhead = 64
		if len(raw)+itemOverhead > maxResponseStreamOutputBytes-a.outputBytes {
			return &responseStreamEventError{code: "output_limit_exceeded", message: "output exceeds 16 MiB stream limit"}
		}
		a.outputBytes += len(raw) + itemOverhead
		a.items = append(a.items, indexedResponseOutputItem{
			outputIndex: done.OutputIndex,
			raw:         json.RawMessage(raw),
		})
	case "response.completed", "response.failed", "response.incomplete":
		if a.requireCompleted && event.Type != "response.completed" {
			var envelope struct {
				Error struct {
					Param string `json:"param"`
				} `json:"error"`
				Incomplete json.RawMessage `json:"incomplete_details"`
			}
			if err := json.Unmarshal([]byte(event.Response.RawJSON()), &envelope); err != nil {
				return err
			}
			return &responseStreamEventError{code: string(event.Response.Error.Code), param: envelope.Error.Param, message: fmt.Sprintf("%s: %s %s; Manage usage: %s", event.Type, event.Response.Error.Message, envelope.Incomplete, chatgptauth.UsageURL)}
		}
		a.terminalType = event.Type
		response := event.Response
		if len(response.RawJSON()) > maxResponseStreamOutputBytes-a.outputBytes {
			return &responseStreamEventError{code: "output_limit_exceeded", message: "output exceeds 16 MiB stream limit"}
		}
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
	if a.requireCompleted {
		if a.terminalType != "response.completed" || a.terminal.Status != responses.ResponseStatusCompleted {
			return nil, &responseStatusError{message: "chatgpt stream did not reach response.completed"}
		}
		if len(a.terminal.Output) > 0 {
			return a.terminal, nil
		}
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
// events and the terminal response envelope, including streams whose terminal
// output array is empty after emitting complete output items.
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
		case "developer":
			role := responses.EasyInputMessageRoleUser
			if c.cfg.ChatGPTPlan {
				role = responses.EasyInputMessageRoleDeveloper
			}
			input = append(input, responses.ResponseInputItemParamOfMessage(content, role))
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
				if c.cfg.ChatGPTPlan {
					item.OfFunctionCall.Namespace = openai.String("ocr")
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

	if c.cfg.ChatGPTPlan && len(req.Tools) > 0 {
		var functions []responses.NamespaceToolToolUnionParam
		for _, t := range req.Tools {
			functions = append(functions, responses.NamespaceToolToolUnionParam{OfFunction: &responses.NamespaceToolToolFunctionParam{
				Name: t.Function.Name, Parameters: t.Function.Parameters, Strict: openai.Bool(false), Description: openai.String(t.Function.Description),
			}})
		}
		tools = []responses.ToolUnionParam{{OfNamespace: &responses.NamespaceToolParam{Name: "ocr", Description: "OpenCodeReview local review tools", Tools: functions}}}
	}
	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if req.SessionID != "" {
		params.PromptCacheKey = openai.String(req.SessionID)
	}
	if len(tools) > 0 {
		params.Tools = tools
		if req.ToolChoice == "required" {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
			}
		}
	}
	if !c.cfg.RejectsSamplingParams && !c.cfg.ChatGPTPlan {
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

	var rawUsageJSON json.RawMessage
	if c.cfg.ChatGPTPlan {
		var envelope struct {
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal([]byte(sdkResp.RawJSON()), &envelope) == nil {
			rawUsageJSON = envelope.Usage
		}
	}
	return &ChatResponse{
		ID:       sdkResp.ID,
		Model:    string(sdkResp.Model),
		RawUsage: rawUsageJSON,
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
