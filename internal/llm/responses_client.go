// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// --- OpenAIResponsesClient ---

// OpenAIResponsesClient speaks the OpenAI Responses API (/v1/responses) using
// the official SDK. It is stateless: every request carries the full input
// history (no previous_response_id), so the agent loop does not need to track
// server-side response IDs. See DESIGN_STATE_CACHE_PHASE.md for the rationale.
type OpenAIResponsesClient struct {
	cfg   ClientConfig
	sdk   openai.Client
	oauth *openAIAccountSession
}

type openAIAccountSession struct {
	mu          sync.Mutex
	credentials OpenAIAccountCredentials
}

// NewOpenAIResponsesClient creates a client for the OpenAI Responses API.
// URL normalization mirrors NewOpenAIClient: cfg.URL is forced to end in
// /responses, and that suffix is stripped to derive the SDK base URL (the SDK
// appends "responses" itself).
func NewOpenAIResponsesClient(cfg ClientConfig) *OpenAIResponsesClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	ensureResponsesEndpoint(&cfg)

	oauth := newOpenAIAccountSession(cfg.OpenAIAccount)

	return &OpenAIResponsesClient{
		cfg:   cfg,
		sdk:   newOpenAIResponsesSDK(cfg, cfg.APIKey, accountID(cfg.OpenAIAccount)),
		oauth: oauth,
	}
}

func newOpenAIAccountSession(credentials *OpenAIAccountCredentials) *openAIAccountSession {
	if credentials == nil {
		return nil
	}
	return &openAIAccountSession{credentials: *credentials}
}

func accountID(credentials *OpenAIAccountCredentials) string {
	if credentials == nil {
		return ""
	}
	return credentials.AccountID
}

func newOpenAIResponsesSDK(cfg ClientConfig, token, accountID string) openai.Client {
	sdkBaseURL := strings.TrimSuffix(strings.TrimRight(cfg.URL, "/"), "/responses")
	opts := []openaiopt.RequestOption{
		openaiopt.WithAPIKey(token),
		openaiopt.WithBaseURL(sdkBaseURL),
		openaiopt.WithMaxRetries(0),
		openaiopt.WithHeader("User-Agent", userAgent("")),
		openaiopt.WithRequestTimeout(cfg.Timeout),
	}
	if cfg.OpenAIAccount != nil {
		opts = append(opts, openaiopt.WithHeader("originator", OpenAIAccountOriginator))
		if accountID != "" {
			opts = append(opts, openaiopt.WithHeader("chatgpt-account-id", accountID))
		}
	}
	for k, v := range cfg.ExtraHeaders {
		opts = append(opts, openaiopt.WithHeader(k, v))
	}
	if mw := retryCodesMiddleware(cfg.RetryCodes); mw != nil {
		opts = append(opts, openaiopt.WithMiddleware(mw))
	}
	if cfg.retryCollector != nil {
		opts = append(opts, openaiopt.WithMiddleware(newRetryObserver(cfg.retryCollector)))
	}
	return openai.NewClient(opts...)
}

func (s *openAIAccountSession) credentialsForRequest(ctx context.Context) (OpenAIAccountCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := EnsureOpenAIAccountCredentials(ctx, s.credentials)
	if err != nil {
		return OpenAIAccountCredentials{}, err
	}
	s.credentials = credentials
	return credentials, nil
}

func (s *openAIAccountSession) forceRefresh(ctx context.Context) (OpenAIAccountCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := RefreshOpenAIAccountCredentials(ctx, s.credentials)
	if err != nil {
		return OpenAIAccountCredentials{}, err
	}
	s.credentials = credentials
	return credentials, nil
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
		finalizeRequest(ctx, c.cfg.retryCollector, err)
	}()

	model := req.Model
	if model == "" {
		model = c.cfg.Model
	}

	params := c.buildResponsesParams(model, req)

	var opts []openaiopt.RequestOption
	for k, v := range c.cfg.ExtraBody {
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
	if c.oauth != nil {
		return c.completionsWithOpenAIAccount(ctx, params, opts)
	}

	return withLLMRetry(ctx, func(ctx context.Context) (*ChatResponse, error) {
		sdkResp, err := c.sdk.Responses.New(ctx, params, opts...)
		if err != nil {
			return nil, err
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
	})
}

func (c *OpenAIResponsesClient) completionsWithOpenAIAccount(ctx context.Context, params responses.ResponseNewParams, opts []openaiopt.RequestOption) (*ChatResponse, error) {
	return withLLMRetry(ctx, func(ctx context.Context) (*ChatResponse, error) {
		credentials, err := c.oauth.credentialsForRequest(ctx)
		if err != nil {
			return nil, err
		}
		response, err := c.streamOpenAIAccountResponse(ctx, credentials, params, opts)
		if err == nil {
			return response, nil
		}
		if !isOpenAIAccountAuthError(err) || credentials.RefreshToken == "" {
			return nil, err
		}
		refreshed, refreshErr := c.oauth.forceRefresh(ctx)
		if refreshErr != nil {
			return nil, fmt.Errorf("refresh OpenAI account credentials after HTTP authorization failure: %w", refreshErr)
		}
		return c.streamOpenAIAccountResponse(ctx, refreshed, params, opts)
	})
}

func (c *OpenAIResponsesClient) streamOpenAIAccountResponse(ctx context.Context, credentials OpenAIAccountCredentials, params responses.ResponseNewParams, opts []openaiopt.RequestOption) (*ChatResponse, error) {
	cfg := c.cfg
	sdk := newOpenAIResponsesSDK(cfg, credentials.AccessToken, credentials.AccountID)
	stream := sdk.Responses.NewStreaming(ctx, params, opts...)
	defer stream.Close()

	var completed *responses.Response
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "error":
			return nil, fmt.Errorf("OpenAI account stream failed: %s", event.Message)
		case "response.failed", "response.cancelled":
			response := event.Response
			return nil, fmt.Errorf("OpenAI account response did not complete: status=%s", response.Status)
		case "response.queued", "response.in_progress":
			return nil, fmt.Errorf("OpenAI account returned non-terminal status=%s", event.Type)
		case "response.completed", "response.incomplete":
			response := event.Response
			completed = &response
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if completed == nil {
		return nil, errors.New("OpenAI account stream ended without response.completed")
	}
	if completed.Status == responses.ResponseStatusFailed || completed.Status == responses.ResponseStatusCancelled {
		return nil, fmt.Errorf("OpenAI account response did not complete: status=%s", completed.Status)
	}
	if completed.Status == responses.ResponseStatusQueued || completed.Status == responses.ResponseStatusInProgress {
		return nil, fmt.Errorf("OpenAI account returned non-terminal status=%s", completed.Status)
	}
	return c.mapResponsesResponse(completed), nil
}

func isOpenAIAccountAuthError(err error) bool {
	var accountErr *OpenAIAccountHTTPError
	if errors.As(err, &accountErr) {
		return accountErr.StatusCode == http.StatusUnauthorized || accountErr.StatusCode == http.StatusForbidden
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
	}
	return false
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
			for _, raw := range msg.ResponseItems {
				var item responses.ResponseInputItemUnionParam
				if err := json.Unmarshal(raw, &item); err == nil {
					input = append(input, item)
				}
			}
			if content != "" {
				assistantMessage := responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleAssistant)
				if assistantMessage.OfMessage != nil && msg.Phase != "" {
					assistantMessage.OfMessage.Phase = responses.EasyInputMessagePhase(msg.Phase)
				}
				input = append(input, assistantMessage)
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
		Store: openai.Bool(false),
	}

	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if req.SessionID != "" {
		params.PromptCacheKey = openai.String(req.SessionID)
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	if req.MaxTokens > 0 {
		if c.oauth == nil {
			params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
		}
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if c.cfg.ReasoningEffort != "" && c.cfg.ReasoningEffort != "none" {
		params.Reasoning.Effort = shared.ReasoningEffort(c.cfg.ReasoningEffort)
	}
	if c.cfg.ServiceTier != "" {
		params.ServiceTier = responses.ResponseNewParamsServiceTier(c.cfg.ServiceTier)
	}
	if c.oauth != nil {
		params.ParallelToolCalls = openai.Bool(false)
		params.Include = []responses.ResponseIncludable{"reasoning.encrypted_content"}
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
	var responseItems []json.RawMessage
	var phase string
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
		case "message":
			message := item.AsMessage()
			if message.Phase != "" {
				phase = string(message.Phase)
			}
		case "reasoning":
			if raw := item.RawJSON(); raw != "" {
				responseItems = append(responseItems, json.RawMessage(raw))
			}
			// Best-effort: aggregate every summary entry's Text (not just the
			// first) so multi-paragraph reasoning isn't truncated.
			r := item.AsReasoning()
			for _, s := range r.Summary {
				if s.Text != "" {
					reasoningParts = append(reasoningParts, s.Text)
				}
			}
		}
	}

	var reasoningContent string
	if len(reasoningParts) > 0 {
		reasoningContent = strings.Join(reasoningParts, "\n")
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
				Phase:            phase,
				ResponseItems:    responseItems,
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
