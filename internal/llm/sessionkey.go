// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"time"
)

// SessionKeyTemplateVar is the placeholder users can embed in extra_headers
// and extra_body values. Clients replace it with the run's session key, so
// providers that route prompt-cache traffic by an explicit key (e.g.
// x-session-affinity headers on Cloudflare/Fireworks/Mistral-style gateways)
// can be configured without OCR knowing each provider's convention.
const SessionKeyTemplateVar = "{ocr_session_key}"

// sessionKeyCtxKey is the context key carrying the run's session key.
type sessionKeyCtxKey struct{}

// ContextWithSessionKey returns a context carrying the given session key.
// Review and scan runs bind their session history's SessionID at the top of
// Run as a base key, and each task refines it with SessionTaskKey where its
// conversation starts, so every LLM request is tagged with the real OCR
// session's key at the granularity prompt caches actually work at.
func ContextWithSessionKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionKeyCtxKey{}, key)
}

// SessionTaskKey derives the prompt-cache affinity key for one task
// conversation within a session. Prompt caches match on prefixes, and OCR's
// task types (plan, main tool-loop, compression, dedup, ...) use unrelated
// prompts — routing a whole run under one key would pin every concurrent
// per-file conversation to one cache node with no shared prefix to reuse,
// and hot keys degrade anyway (OpenAI reroutes a prompt_cache_key once it
// exceeds roughly 15 requests/minute). Scoping the key to one conversation
// — (session, task type, file or batch) — keeps each conversation's growing
// prefix on a consistent node instead.
//
// The scope (usually a file path) is hashed so the key stays header-safe;
// the session key and task type stay readable so provider-side logs can be
// correlated with OCR session records.
func SessionTaskKey(sessionKey, taskType, scope string) string {
	if taskType == "" && scope == "" {
		return sessionKey
	}
	if scope == "" {
		return sessionKey + "-" + taskType
	}
	sum := sha256.Sum256([]byte(scope))
	return fmt.Sprintf("%s-%s-%x", sessionKey, taskType, sum[:8])
}

// SessionKeyFromContext returns the session key carried by ctx, or "" when
// none was set.
func SessionKeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(sessionKeyCtxKey{}).(string)
	return key
}

// NewSessionKey returns a fresh UUIDv4 session key. Clients use it as a
// per-client fallback for requests whose context carries no session key
// (e.g. `ocr llm test`, which has no review session).
func NewSessionKey() string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// Fallback — extremely unlikely but keeps things working without panics.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// expandSessionKeyInHeaders returns a copy of headers with every occurrence
// of SessionKeyTemplateVar in the values replaced by key. The input map is
// never mutated; nil input returns nil.
func expandSessionKeyInHeaders(headers map[string]string, key string) map[string]string {
	if len(headers) == 0 {
		return headers
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = strings.ReplaceAll(v, SessionKeyTemplateVar, key)
	}
	return out
}

// expandSessionKeyInBody returns a copy of body with SessionKeyTemplateVar
// replaced by key in every string value, recursing into nested maps and
// slices. The input is never mutated; nil input returns nil.
func expandSessionKeyInBody(body map[string]any, key string) map[string]any {
	if len(body) == 0 {
		return body
	}
	out := make(map[string]any, len(body))
	for k, v := range body {
		out[k] = expandSessionKeyValue(v, key)
	}
	return out
}

func expandSessionKeyValue(v any, key string) any {
	switch val := v.(type) {
	case string:
		return strings.ReplaceAll(val, SessionKeyTemplateVar, key)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, nested := range val {
			out[k] = expandSessionKeyValue(nested, key)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, nested := range val {
			out[i] = expandSessionKeyValue(nested, key)
		}
		return out
	default:
		return v
	}
}
