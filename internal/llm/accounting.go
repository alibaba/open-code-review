// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// UsageStatus describes how token counts were obtained.
type UsageStatus string

const (
	UsageStatusActual    UsageStatus = "actual"
	UsageStatusEstimated UsageStatus = "estimated"
	UsageStatusUnknown   UsageStatus = "unknown"
)

var (
	// ErrTokenBudgetExceeded means the aggregate budget gate rejected a new
	// provider dispatch. The provider was not called.
	ErrTokenBudgetExceeded = errors.New("aggregate token budget exceeded")
	// ErrContextBudgetExceeded means the complete semantic request did not fit
	// within the one-time safety-margined context ceiling.
	ErrContextBudgetExceeded = errors.New("request context budget exceeded")
)

// RequestEstimate is the local, conservative estimate of a fully assembled
// ChatRequest. It describes provider input only; MaxTokens is an output limit
// and is intentionally excluded from InputTokens.
type RequestEstimate struct {
	Model             string
	Encoding          string
	ModelMappingKnown bool
	InputTokens       int64
	Status            UsageStatus
	Components        map[string]int64
}

// RequestEstimator estimates one fully assembled request. Returning
// UsageStatusUnknown tells the accounting layer that no fallback token value
// is available and that it must preserve the uncertainty.
type RequestEstimator func(ChatRequest) RequestEstimate

// TokenAccountingOptions configures the shared request seam.
type TokenAccountingOptions struct {
	ContextBudget   int64
	AggregateBudget int64
	Estimator       RequestEstimator
}

// TokenAccountingSnapshot is a race-safe point-in-time view of all completed
// requests handled by one review run.
type TokenAccountingSnapshot struct {
	InputTokens        int64
	OutputTokens       int64
	UnattributedTokens int64
	CacheReadTokens    int64
	CacheWriteTokens   int64
	TotalTokens        int64
	Requests           int64
	ActualRequests     int64
	EstimatedRequests  int64
	UnknownRequests    int64
	UsageStatus        UsageStatus
	// BudgetExceeded is set when the aggregate gate rejects a new dispatch.
	// Reaching the cap exactly does not set it until a later dispatch is tried.
	BudgetExceeded  bool
	ContextBudget   int64
	AggregateBudget int64
}

// UsageRecord is the normalized result of one request. A later actual value
// replaces the pending estimate before this record contributes to totals.
type UsageRecord struct {
	Status UsageStatus
	Usage  *UsageInfo
}

// RequestHandle identifies a request between Begin and Finish. It is opaque
// to callers and exists to make estimate-to-actual reconciliation explicit.
type RequestHandle struct{ id uint64 }

type pendingRequest struct {
	estimate RequestEstimate
	finished bool
	record   UsageRecord
}

// TokenAccounting owns aggregate totals and the pre-dispatch gates for every
// LLM phase in one review run.
type TokenAccounting struct {
	mu              sync.Mutex
	contextBudget   int64
	aggregateBudget int64
	estimator       RequestEstimator
	nextID          uint64
	pending         map[uint64]*pendingRequest
	completed       map[uint64]UsageRecord
	snapshot        TokenAccountingSnapshot
}

// NewTokenAccounting creates a shared accounting store. Zero aggregate budget
// keeps the explicit unlimited behavior.
func NewTokenAccounting(opts TokenAccountingOptions) *TokenAccounting {
	estimator := opts.Estimator
	if estimator == nil {
		estimator = EstimateChatRequest
	}
	return &TokenAccounting{
		contextBudget:   opts.ContextBudget,
		aggregateBudget: opts.AggregateBudget,
		estimator:       estimator,
		pending:         make(map[uint64]*pendingRequest),
		completed:       make(map[uint64]UsageRecord),
		snapshot: TokenAccountingSnapshot{
			ContextBudget:   opts.ContextBudget,
			AggregateBudget: opts.AggregateBudget,
		},
	}
}

// SetContextBudget updates the configured context budget for future requests.
func (a *TokenAccounting) SetContextBudget(maxTokens int64) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.contextBudget = maxTokens
	a.snapshot.ContextBudget = maxTokens
	a.mu.Unlock()
}

// SetAggregateBudget updates the explicit aggregate cap for future requests.
// Zero and negative values remain unlimited for compatibility with the CLI.
func (a *TokenAccounting) SetAggregateBudget(maxTokens int64) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.aggregateBudget = maxTokens
	a.snapshot.AggregateBudget = maxTokens
	a.mu.Unlock()
}

// Estimate returns the configured estimator's result without dispatching.
func (a *TokenAccounting) Estimate(req ChatRequest) RequestEstimate {
	if a == nil {
		return RequestEstimate{Status: UsageStatusUnknown}
	}
	a.mu.Lock()
	estimator := a.estimator
	a.mu.Unlock()
	return normalizeEstimate(estimator(req))
}

// Begin runs the aggregate and context gates and records the request's
// preflight estimate without adding it to the completed totals yet.
func (a *TokenAccounting) Begin(req ChatRequest) (RequestHandle, error) {
	if a == nil {
		return RequestHandle{}, nil
	}
	// Only completed usage contributes to the aggregate gate. Requests already
	// admitted may finish concurrently; the next Begin observes their actual or
	// fallback usage before allowing another dispatch.
	a.mu.Lock()
	if a.aggregateBudget > 0 && a.snapshot.TotalTokens >= a.aggregateBudget {
		a.snapshot.BudgetExceeded = true
		a.mu.Unlock()
		return RequestHandle{}, ErrTokenBudgetExceeded
	}
	estimator := a.estimator
	contextBudget := a.contextBudget
	a.mu.Unlock()

	estimate := normalizeEstimate(estimator(req))
	if contextBudget > 0 && estimate.Status != UsageStatusUnknown && estimate.InputTokens > EffectivePreflightCeiling(contextBudget) {
		return RequestHandle{}, fmt.Errorf("%w: estimated input %d exceeds ceiling %d", ErrContextBudgetExceeded, estimate.InputTokens, EffectivePreflightCeiling(contextBudget))
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.aggregateBudget > 0 && a.snapshot.TotalTokens >= a.aggregateBudget {
		a.snapshot.BudgetExceeded = true
		return RequestHandle{}, ErrTokenBudgetExceeded
	}
	a.nextID++
	handle := RequestHandle{id: a.nextID}
	a.pending[handle.id] = &pendingRequest{estimate: estimate}
	return handle, nil
}

// Abort discards a request that never produced a response. Failed provider
// calls do not fabricate a successful token total.
func (a *TokenAccounting) Abort(handle RequestHandle) {
	if a == nil || handle.id == 0 {
		return
	}
	a.mu.Lock()
	delete(a.pending, handle.id)
	a.mu.Unlock()
}

// Finish reconciles one request. Provider usage wins; only when it is absent
// does the complete request/response estimator become the fallback.
func (a *TokenAccounting) Finish(handle RequestHandle, resp *ChatResponse) UsageRecord {
	if a == nil || handle.id == 0 {
		return UsageRecord{Status: UsageStatusUnknown}
	}

	a.mu.Lock()
	req, ok := a.pending[handle.id]
	if !ok {
		record, completed := a.completed[handle.id]
		if !completed {
			record = UsageRecord{Status: UsageStatusUnknown}
		}
		if completed && record.Status == "" {
			record.Status = UsageStatusUnknown
		}
		if completed && record.Status != UsageStatusActual && resp != nil && resp.Usage != nil {
			a.removeRecordLocked(record)
			record = UsageRecord{Status: UsageStatusActual, Usage: cloneUsage(resp.Usage)}
			a.completed[handle.id] = record
			a.addRecordLocked(record)
		}
		a.mu.Unlock()
		if resp != nil {
			resp.UsageStatus = record.Status
			if record.Usage != nil {
				resp.Usage = cloneUsage(record.Usage)
			}
		}
		return record
	}
	if req.finished {
		record := req.record
		a.mu.Unlock()
		return record
	}

	record := UsageRecord{Status: UsageStatusUnknown}
	if resp != nil && resp.Usage != nil {
		record = UsageRecord{Status: UsageStatusActual, Usage: cloneUsage(resp.Usage)}
	} else if resp != nil && req.estimate.Status == UsageStatusEstimated {
		output := EstimateChatResponseTokens(resp, req.estimate.Model)
		usage := &UsageInfo{
			PromptTokens:     req.estimate.InputTokens,
			CompletionTokens: output,
			TotalTokens:      req.estimate.InputTokens + output,
		}
		record = UsageRecord{Status: UsageStatusEstimated, Usage: usage}
	}

	req.finished = true
	req.record = record
	a.completed[handle.id] = record
	delete(a.pending, handle.id)
	a.addRecordLocked(record)
	a.mu.Unlock()

	if resp != nil {
		resp.UsageStatus = record.Status
		if record.Usage != nil {
			resp.Usage = cloneUsage(record.Usage)
		}
	}
	return record
}

// Reconcile replaces an earlier estimated record with provider usage obtained
// later for the same request. The replacement adjusts totals in place, so the
// request is never counted twice.
func (a *TokenAccounting) Reconcile(handle RequestHandle, usage *UsageInfo) bool {
	if a == nil || handle.id == 0 || usage == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	record, ok := a.completed[handle.id]
	if !ok || record.Status == UsageStatusActual {
		return false
	}
	a.removeRecordLocked(record)
	replacement := UsageRecord{Status: UsageStatusActual, Usage: cloneUsage(usage)}
	a.completed[handle.id] = replacement
	a.addRecordLocked(replacement)
	return true
}

// RecordExternal keeps compatibility with callers that receive a response
// outside the shared seam. New review phases should call the AccountingClient
// instead; this method is useful for legacy tests and integrations.
func (a *TokenAccounting) RecordExternal(usage *UsageInfo) {
	if a == nil || usage == nil {
		return
	}
	a.mu.Lock()
	a.addRecordLocked(UsageRecord{Status: UsageStatusActual, Usage: cloneUsage(usage)})
	a.mu.Unlock()
}

func (a *TokenAccounting) addRecordLocked(record UsageRecord) {
	a.snapshot.Requests++
	switch record.Status {
	case UsageStatusActual:
		a.snapshot.ActualRequests++
	case UsageStatusEstimated:
		a.snapshot.EstimatedRequests++
	default:
		a.snapshot.UnknownRequests++
	}
	if record.Usage != nil {
		a.snapshot.InputTokens += record.Usage.PromptTokens
		a.snapshot.OutputTokens += record.Usage.CompletionTokens
		known := record.Usage.PromptTokens + record.Usage.CompletionTokens
		if record.Usage.TotalTokens > known {
			a.snapshot.UnattributedTokens += record.Usage.TotalTokens - known
		}
		a.snapshot.CacheReadTokens += record.Usage.CacheReadTokens
		a.snapshot.CacheWriteTokens += record.Usage.CacheWriteTokens
		a.snapshot.TotalTokens = a.snapshot.InputTokens + a.snapshot.OutputTokens + a.snapshot.UnattributedTokens
	}
	a.snapshot.UsageStatus = aggregateUsageStatus(a.snapshot)
}

func (a *TokenAccounting) removeRecordLocked(record UsageRecord) {
	if a.snapshot.Requests > 0 {
		a.snapshot.Requests--
	}
	switch record.Status {
	case UsageStatusActual:
		if a.snapshot.ActualRequests > 0 {
			a.snapshot.ActualRequests--
		}
	case UsageStatusEstimated:
		if a.snapshot.EstimatedRequests > 0 {
			a.snapshot.EstimatedRequests--
		}
	default:
		if a.snapshot.UnknownRequests > 0 {
			a.snapshot.UnknownRequests--
		}
	}
	if record.Usage != nil {
		a.snapshot.InputTokens -= record.Usage.PromptTokens
		a.snapshot.OutputTokens -= record.Usage.CompletionTokens
		known := record.Usage.PromptTokens + record.Usage.CompletionTokens
		if record.Usage.TotalTokens > known {
			a.snapshot.UnattributedTokens -= record.Usage.TotalTokens - known
		}
		a.snapshot.CacheReadTokens -= record.Usage.CacheReadTokens
		a.snapshot.CacheWriteTokens -= record.Usage.CacheWriteTokens
		a.snapshot.TotalTokens = a.snapshot.InputTokens + a.snapshot.OutputTokens + a.snapshot.UnattributedTokens
	}
	a.snapshot.UsageStatus = aggregateUsageStatus(a.snapshot)
}

// Snapshot returns a copy of the current totals.
func (a *TokenAccounting) Snapshot() TokenAccountingSnapshot {
	if a == nil {
		return TokenAccountingSnapshot{UsageStatus: UsageStatusUnknown}
	}
	a.mu.Lock()
	snapshot := a.snapshot
	a.mu.Unlock()
	return snapshot
}

func aggregateUsageStatus(snapshot TokenAccountingSnapshot) UsageStatus {
	if snapshot.UnknownRequests > 0 {
		return UsageStatusUnknown
	}
	if snapshot.EstimatedRequests > 0 {
		return UsageStatusEstimated
	}
	if snapshot.ActualRequests > 0 {
		return UsageStatusActual
	}
	return UsageStatusUnknown
}

func normalizeEstimate(est RequestEstimate) RequestEstimate {
	if est.Status == "" {
		est.Status = UsageStatusUnknown
	}
	if est.Components == nil {
		est.Components = make(map[string]int64)
	}
	if est.InputTokens < 0 {
		est.InputTokens = 0
	}
	return est
}

func cloneUsage(usage *UsageInfo) *UsageInfo {
	if usage == nil {
		return nil
	}
	copy := *usage
	return &copy
}

// AccountingClient is the one request seam used by all review phases. It
// performs preflight/gating before the provider call and usage normalization
// after the response returns.
type AccountingClient struct {
	inner      LLMClient
	accounting *TokenAccounting
}

// NewAccountingClient wraps an existing protocol client with shared token
// accounting. The wrapper still satisfies LLMClient for legacy call sites.
func NewAccountingClient(inner LLMClient, accounting *TokenAccounting) *AccountingClient {
	return &AccountingClient{inner: inner, accounting: accounting}
}

// CompletionsWithCtx applies the shared pre/post request contract.
func (c *AccountingClient) CompletionsWithCtx(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if c == nil || c.accounting == nil {
		if c == nil || c.inner == nil {
			return nil, errors.New("nil LLM client")
		}
		return c.inner.CompletionsWithCtx(ctx, req)
	}
	handle, err := c.accounting.Begin(req)
	if err != nil {
		return nil, err
	}
	if c.inner == nil {
		c.accounting.Abort(handle)
		return nil, errors.New("nil LLM client")
	}
	resp, err := c.inner.CompletionsWithCtx(ctx, req)
	if err != nil {
		c.accounting.Abort(handle)
		return nil, err
	}
	c.accounting.Finish(handle, resp)
	return resp, nil
}

// Accounting returns the shared store for result aggregation and diagnostics.
func (c *AccountingClient) Accounting() *TokenAccounting { return c.accounting }

// EffectivePreflightCeiling applies the one agreed 15% safety margin. The
// configured context budget itself is never changed.
func EffectivePreflightCeiling(maxContextTokens int64) int64 {
	if maxContextTokens <= 0 {
		return 0
	}
	whole, remainder := maxContextTokens/100, maxContextTokens%100
	return whole*85 + remainder*85/100
}

// EstimateChatRequest counts semantic request fields, including role/framing,
// tools and schemas, tool calls/results, Responses replay items and phase
// metadata. It deliberately does not count JSON transport syntax or the output
// limit, and it never claims provider-exactness.
func EstimateChatRequest(req ChatRequest) RequestEstimate {
	est := RequestEstimate{
		Model:      req.Model,
		Components: make(map[string]int64),
		Status:     UsageStatusEstimated,
	}
	encoding, mappingKnown := tokenizerEncodingForModel(req.Model)
	est.Encoding = encoding
	est.ModelMappingKnown = mappingKnown

	canonical := []struct {
		name  string
		value string
	}{{"request", req.Model}}
	components := make([]struct {
		name  string
		value string
	}, 0, len(req.Messages)*4+len(req.Tools))

	type messagePayload struct {
		Role          string            `json:"role"`
		Content       any               `json:"content"`
		ToolCallID    string            `json:"tool_call_id,omitempty"`
		ToolCalls     []ToolCall        `json:"tool_calls,omitempty"`
		Phase         string            `json:"phase,omitempty"`
		ResponseItems []json.RawMessage `json:"response_items,omitempty"`
	}
	for _, message := range req.Messages {
		payload, err := json.Marshal(messagePayload{
			Role:          message.Role,
			Content:       message.Content,
			ToolCallID:    message.ToolCallID,
			ToolCalls:     message.ToolCalls,
			Phase:         message.Phase,
			ResponseItems: message.ResponseItems,
		})
		if err != nil {
			est.Status = UsageStatusUnknown
			continue
		}
		canonical = append(canonical, struct {
			name  string
			value string
		}{"messages", string(payload)})
		if message.Role != "" {
			components = append(components, struct {
				name  string
				value string
			}{"roles", message.Role})
		}
		if message.Role == "system" {
			components = append(components, struct {
				name  string
				value string
			}{"instructions", message.ExtractText()})
		}
		if len(message.ToolCalls) > 0 {
			components = append(components, struct {
				name  string
				value string
			}{"tool_calls", string(payload)})
		}
		if message.Role == "tool" || message.ToolCallID != "" {
			components = append(components, struct {
				name  string
				value string
			}{"tool_results", string(payload)})
		}
		if len(message.ResponseItems) > 0 {
			components = append(components, struct {
				name  string
				value string
			}{"response_items", string(payload)})
		}
		if message.Phase != "" {
			components = append(components, struct {
				name  string
				value string
			}{"phase_metadata", message.Phase})
		}
	}

	for _, tool := range req.Tools {
		var rawSchema json.RawMessage
		if len(tool.Function.Parameters) == 0 && len(tool.Function.RawDefinition) > 0 {
			rawSchema = tool.Function.RawDefinition
		}
		payload, err := json.Marshal(struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  map[string]any  `json:"parameters"`
			RawSchema   json.RawMessage `json:"raw_schema,omitempty"`
		}{
			Type:        tool.Type,
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
			RawSchema:   rawSchema,
		})
		if err != nil {
			est.Status = UsageStatusUnknown
			continue
		}
		canonical = append(canonical, struct {
			name  string
			value string
		}{"tools", string(payload)})
	}

	for _, part := range canonical {
		tokens, ok, _ := countTokensForModelExact(part.value, req.Model)
		if !ok {
			est.Status = UsageStatusUnknown
			continue
		}
		est.InputTokens += int64(tokens)
		est.Components[part.name] += int64(tokens)
	}
	for _, part := range components {
		tokens, ok, _ := countTokensForModelExact(part.value, req.Model)
		if !ok {
			est.Status = UsageStatusUnknown
			continue
		}
		est.Components[part.name] += int64(tokens)
	}
	return est
}

// EstimateChatResponseTokens estimates response semantic fields for fallback
// output accounting when the provider omits usage.
func EstimateChatResponseTokens(resp *ChatResponse, model string) int64 {
	if resp == nil || len(resp.Choices) == 0 {
		return 0
	}
	choice := resp.Choices[0]
	payload, err := json.Marshal(struct {
		Content          *string           `json:"content,omitempty"`
		ReasoningContent string            `json:"reasoning_content,omitempty"`
		ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
		Phase            string            `json:"phase,omitempty"`
		ResponseItems    []json.RawMessage `json:"response_items,omitempty"`
	}{
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
		ToolCalls:        choice.Message.ToolCalls,
		Phase:            choice.Message.Phase,
		ResponseItems:    choice.Message.ResponseItems,
	})
	if err != nil {
		return 0
	}
	tokens, ok, _ := countTokensForModelExact(string(payload), model)
	if !ok {
		return 0
	}
	return int64(tokens)
}
