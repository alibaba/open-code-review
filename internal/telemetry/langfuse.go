// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package telemetry

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"

	"github.com/alibaba/open-code-review/internal/llm"
)

// activeCfg is the config resolved once by Init. It is nil only when
// telemetry state was set without Init (tests), in which case helpers fall
// back to resolving from the environment per call.
var activeCfg atomic.Pointer[Config]

// sessionIDCtxKey carries the Langfuse session ID down to every span started
// under a review conversation. Concurrent subtasks each scope their own
// context, so per-run session IDs never share mutable global state.
type sessionIDCtxKey struct{}

// WithSessionID returns a context whose descendants stamp sessionID as
// langfuse.session.id on their spans when OCR_TRACE_SESSION does not pin one.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDCtxKey{}, sessionID)
}

func sessionIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sessionIDCtxKey{}).(string)
	return v
}

// resolvedConfig returns the config captured at Init, or a live resolve when
// Init never ran.
func resolvedConfig() Config {
	if cfg := activeCfg.Load(); cfg != nil {
		return *cfg
	}
	return ResolveConfig("")
}

// applyTraceAttrs stamps the configured OCR_TRACE_* attributes on every span.
// Langfuse accepts trace-level attributes (name, session, release,
// environment, tags) from any span in the trace, so applying them broadly is
// the simplest way to keep them consistent.
func applyTraceAttrs(ctx context.Context, span trace.Span) {
	cfg := resolvedConfig()
	if cfg.TraceName != "" {
		SetAttr(span, "langfuse.trace.name", cfg.TraceName)
	}
	if cfg.TraceEnv != "" {
		SetAttr(span, "langfuse.environment", cfg.TraceEnv)
	}
	session := cfg.TraceSession
	if session == "" {
		session = sessionIDFromContext(ctx)
	}
	if session != "" {
		SetAttr(span, "langfuse.session.id", session)
	}
	if cfg.TraceRelease != "" {
		SetAttr(span, "langfuse.release", cfg.TraceRelease)
	}
	if cfg.TraceTags != "" {
		SetAttr(span, "langfuse.tags", strings.Split(cfg.TraceTags, ","))
	}
}

// SpanContentEnabled reports whether span-level content capture is on
// (OCR_TRACE_CONTENT=1). Content attributes carry prompts, completions and
// tool payloads — off by default since they can be large and sensitive.
func SpanContentEnabled() bool {
	if !IsEnabled() {
		return false
	}
	return resolvedConfig().SpanContent
}

func setJSONAttr(span trace.Span, key string, v any) {
	if v == nil {
		return
	}
	b, err := json.Marshal(v)
	if err == nil {
		SetAttr(span, key, string(b))
	}
}

// RecordLLMInput marks an llm.request span as a Langfuse generation and
// attaches the outgoing messages as its input. sessionID stamps
// langfuse.session.id on this span only when OCR_TRACE_SESSION does not pin
// one; callers that need the ID on sibling spans should carry it on the
// context via WithSessionID.
func RecordLLMInput(span trace.Span, sessionID, model string, input any) {
	if span == nil || !SpanContentEnabled() {
		return
	}
	SetAttr(span, "langfuse.observation.type", "generation")
	SetAttr(span, "langfuse.observation.model.name", model)
	SetAttr(span, "gen_ai.request.model", model)
	if sessionID != "" && resolvedConfig().TraceSession == "" {
		SetAttr(span, "langfuse.session.id", sessionID)
	}
	setJSONAttr(span, "langfuse.observation.input", input)
}

// RecordLLMOutput attaches the completion — content, tool calls, reasoning
// and usage — to an llm.request span after the response arrives.
func RecordLLMOutput(span trace.Span, content string, toolCalls any, reasoning string, usage *llm.UsageInfo) {
	if span == nil || !SpanContentEnabled() {
		return
	}
	out := map[string]any{"content": content}
	if toolCalls != nil {
		out["tool_calls"] = toolCalls
	}
	setJSONAttr(span, "langfuse.observation.output", out)
	if reasoning != "" {
		SetAttr(span, "langfuse.observation.metadata.reasoning_content", reasoning)
	}
	if usage != nil {
		SetAttr(span, "gen_ai.usage.input_tokens", usage.PromptTokens)
		SetAttr(span, "gen_ai.usage.output_tokens", usage.CompletionTokens)
		SetAttr(span, "gen_ai.usage.total_tokens", usage.TotalTokens)
		setJSONAttr(span, "langfuse.observation.usage_details", map[string]int64{
			"input":                usage.PromptTokens,
			"output":               usage.CompletionTokens,
			"total":                usage.TotalTokens,
			"cache_read_input":     usage.CacheReadTokens,
			"cache_creation_input": usage.CacheWriteTokens,
		})
	}
}

// RecordToolInput marks a tool span as a Langfuse tool observation and
// attaches its arguments. Call right after StartToolSpan so the input is
// recorded even when execution fails.
func RecordToolInput(span trace.Span, input any) {
	if span == nil || !SpanContentEnabled() {
		return
	}
	SetAttr(span, "langfuse.observation.type", "tool")
	setJSONAttr(span, "langfuse.observation.input", input)
}

// RecordToolOutput attaches a tool result (or error string) to a tool span.
func RecordToolOutput(span trace.Span, output any) {
	if span == nil || !SpanContentEnabled() {
		return
	}
	setJSONAttr(span, "langfuse.observation.output", output)
}
