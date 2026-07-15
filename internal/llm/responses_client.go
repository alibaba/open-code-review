package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
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
func NewOpenAIResponsesClient(cfg ClientConfig) *OpenAIResponsesClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
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
	for k, v := range cfg.ExtraHeaders {
		opts = append(opts, openaiopt.WithHeader(k, v))
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
func (c *OpenAIResponsesClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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
		return nil, fmt.Errorf("openai-responses request did not complete: status=%s", sdkResp.Status)
	case responses.ResponseStatusQueued, responses.ResponseStatusInProgress:
		return nil, fmt.Errorf("openai-responses returned non-terminal status=%s (background/async mode is not supported)", sdkResp.Status)
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
		params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
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
	for _, item := range sdkResp.Output {
		// TODO(phase): ResponseOutputMessage.Phase (commentary/final_answer) is
		// currently dropped. For gpt-5.3-codex+ models, preserve and resend
		// Phase on assistant messages to avoid performance degradation. See
		// DESIGN_STATE_CACHE_PHASE.md §3.
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
		case "reasoning":
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
