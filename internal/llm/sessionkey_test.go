// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewSessionKey(t *testing.T) {
	k1 := NewSessionKey()
	k2 := NewSessionKey()
	if !uuidV4Re.MatchString(k1) {
		t.Errorf("NewSessionKey() = %q, not a UUIDv4", k1)
	}
	if k1 == k2 {
		t.Errorf("NewSessionKey() returned duplicate key %q", k1)
	}
}

func TestSessionKeyContext(t *testing.T) {
	ctx := context.Background()
	if got := SessionKeyFromContext(ctx); got != "" {
		t.Errorf("SessionKeyFromContext(empty) = %q, want \"\"", got)
	}
	if got := SessionKeyFromContext(ContextWithSessionKey(ctx, "key-1")); got != "key-1" {
		t.Errorf("SessionKeyFromContext = %q, want %q", got, "key-1")
	}
	// Empty keys are not stored.
	if got := SessionKeyFromContext(ContextWithSessionKey(ctx, "")); got != "" {
		t.Errorf("SessionKeyFromContext(empty key) = %q, want \"\"", got)
	}
	// A nested key overrides the outer one.
	nested := ContextWithSessionKey(ContextWithSessionKey(ctx, "outer"), "inner")
	if got := SessionKeyFromContext(nested); got != "inner" {
		t.Errorf("SessionKeyFromContext(nested) = %q, want %q", got, "inner")
	}
}

func TestSessionTaskKey(t *testing.T) {
	if got := SessionTaskKey("sess", "", ""); got != "sess" {
		t.Errorf("SessionTaskKey(sess,,) = %q, want %q", got, "sess")
	}
	if got := SessionTaskKey("sess", "plan_task", ""); got != "sess-plan_task" {
		t.Errorf("SessionTaskKey without scope = %q, want %q", got, "sess-plan_task")
	}
	if got := SessionTaskKey("01234567-89ab-4cde-8fab-0123456789ab", "", ""); got != "01234567" {
		t.Errorf("SessionTaskKey with UUID only = %q, want %q", got, "01234567")
	}

	k1 := SessionTaskKey("sess", "main_task", "a/b.go")
	k2 := SessionTaskKey("sess", "main_task", "a/b.go")
	if k1 != k2 {
		t.Errorf("SessionTaskKey not deterministic: %q != %q", k1, k2)
	}
	if !strings.HasPrefix(k1, "sess-main_task-") {
		t.Errorf("SessionTaskKey = %q, want prefix %q", k1, "sess-main_task-")
	}
	if k1 == SessionTaskKey("sess", "main_task", "a/c.go") {
		t.Error("different scopes must produce different keys")
	}
	if k1 == SessionTaskKey("sess", "plan_task", "a/b.go") {
		t.Error("different task types must produce different keys")
	}
	// Non-ASCII scopes must still yield a header-safe key.
	for _, r := range SessionTaskKey("sess", "main_task", "文件/경로.go") { // allow-non-english: fixture exercises non-ASCII path hashing
		if r <= ' ' || r > '~' {
			t.Errorf("SessionTaskKey produced non header-safe rune %q", r)
		}
	}
}

func TestSessionTaskKeyProviderSafeLength(t *testing.T) {
	sessionKey := "00000000-0000-4000-8000-000000000000"
	taskTypes := []string{
		"plan_task",
		"main_task",
		"memory_compression_task",
		"re_location_task",
		"review_filter_task",
	}

	seen := make(map[string]string, len(taskTypes))
	for _, taskType := range taskTypes {
		got := SessionTaskKey(sessionKey, taskType, "src/example.go")
		if len(got) > maxSessionTaskKeyLength {
			t.Errorf("SessionTaskKey(%s) length = %d, want <= %d", taskType, len(got), maxSessionTaskKeyLength)
		}
		if previous, ok := seen[got]; ok {
			t.Errorf("SessionTaskKey collision for %s and %s: %q", previous, taskType, got)
		}
		seen[got] = taskType
		if got != SessionTaskKey(sessionKey, taskType, "src/example.go") {
			t.Errorf("SessionTaskKey(%s) is not deterministic", taskType)
		}
		wantPrefix := sessionKey[:sessionTaskKeySessionPrefixLength] + "-" + taskType + "-"
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("SessionTaskKey(%s) = %q, want readable prefix %q", taskType, got, wantPrefix)
		}
	}
}

func TestSessionTaskKeyTruncatesLongSessionComponent(t *testing.T) {
	baseKey := strings.Repeat("x", maxSessionTaskKeyLength+1)
	got := SessionTaskKey(baseKey, "", "")
	want := strings.Repeat("x", sessionTaskKeySessionPrefixLength)
	if got != want {
		t.Fatalf("SessionTaskKey(long base key) = %q, want %q", got, want)
	}
}

func TestSessionTaskKeyTruncatesLongTaskType(t *testing.T) {
	sessionKey := "01234567-89ab-4cde-8fab-0123456789ab"
	taskType := strings.Repeat("t", maxSessionTaskKeyTaskTypeLength+1)
	got := SessionTaskKey(sessionKey, taskType, "src/example.go")
	if len(got) != maxSessionTaskKeyLength {
		t.Fatalf("SessionTaskKey(long task type) length = %d, want %d: %q", len(got), maxSessionTaskKeyLength, got)
	}
	wantPrefix := sessionKey[:sessionTaskKeySessionPrefixLength] + "-" + taskType[:maxSessionTaskKeyTaskTypeLength] + "-"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("SessionTaskKey(long task type) = %q, want readable prefix %q", got, wantPrefix)
	}
}

func TestSessionTaskKeySessionPrefixCollisionBoundary(t *testing.T) {
	scope := "src/example.go"
	first := SessionTaskKey("01234567-0000-4000-8000-000000000000", "main_task", scope)
	samePrefix := SessionTaskKey("01234567-ffff-4fff-bfff-ffffffffffff", "main_task", scope)
	if first != samePrefix {
		t.Errorf("sessions with the same eight-character prefix produced different keys: %q != %q", first, samePrefix)
	}

	differentPrefix := SessionTaskKey("89abcdef-0000-4000-8000-000000000000", "main_task", scope)
	if first == differentPrefix {
		t.Errorf("sessions with different eight-character prefixes produced the same key: %q", first)
	}
}

func TestExpandSessionKeyInHeaders(t *testing.T) {
	in := map[string]string{
		"x-session-affinity": "{ocr_session_key}",
		"X-Tenant":           "tenant-{ocr_session_key}-suffix",
		"X-Static":           "unchanged",
	}
	out := expandSessionKeyInHeaders(in, "key-123")

	if got := out["x-session-affinity"]; got != "key-123" {
		t.Errorf("x-session-affinity = %q, want %q", got, "key-123")
	}
	if got := out["X-Tenant"]; got != "tenant-key-123-suffix" {
		t.Errorf("X-Tenant = %q, want %q", got, "tenant-key-123-suffix")
	}
	if got := out["X-Static"]; got != "unchanged" {
		t.Errorf("X-Static = %q, want %q", got, "unchanged")
	}
	// The input map must not be mutated.
	if in["x-session-affinity"] != "{ocr_session_key}" {
		t.Error("input map was mutated")
	}
	if expandSessionKeyInHeaders(nil, "key") != nil {
		t.Error("nil input should return nil")
	}
}

func TestExpandSessionKeyInBody(t *testing.T) {
	in := map[string]any{
		"prompt_cache_key": "{ocr_session_key}",
		"count":            42,
		"nested": map[string]any{
			"session": "{ocr_session_key}",
		},
		"list": []any{"{ocr_session_key}", 1, true},
	}
	out := expandSessionKeyInBody(in, "key-456")

	if got := out["prompt_cache_key"]; got != "key-456" {
		t.Errorf("prompt_cache_key = %v, want %q", got, "key-456")
	}
	if got := out["count"]; got != 42 {
		t.Errorf("count = %v, want 42", got)
	}
	nested, ok := out["nested"].(map[string]any)
	if !ok || nested["session"] != "key-456" {
		t.Errorf("nested.session = %v, want %q", out["nested"], "key-456")
	}
	list, ok := out["list"].([]any)
	if !ok || list[0] != "key-456" || list[1] != 1 || list[2] != true {
		t.Errorf("list = %v, want [key-456 1 true]", out["list"])
	}
	// The input map must not be mutated.
	if in["prompt_cache_key"] != "{ocr_session_key}" {
		t.Error("input map was mutated")
	}
	if in["nested"].(map[string]any)["session"] != "{ocr_session_key}" {
		t.Error("nested input map was mutated")
	}
	if expandSessionKeyInBody(nil, "key") != nil {
		t.Error("nil input should return nil")
	}
}
