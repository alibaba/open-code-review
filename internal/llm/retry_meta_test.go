// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"strings"
	"testing"
)

func testMeta() RequestMeta {
	return RequestMeta{
		Provider:  "anthropic",
		Model:     "claude-sonnet-4-6",
		FilePath:  "payment.go",
		TaskType:  "main_task",
		RequestNo: 1,
	}
}

func TestRequestMetaValid(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*RequestMeta)
		want bool
	}{
		{"complete", func(*RequestMeta) {}, true},
		{"empty provider is valid", func(m *RequestMeta) { m.Provider = "" }, true},
		{"missing model", func(m *RequestMeta) { m.Model = "" }, false},
		{"missing file path", func(m *RequestMeta) { m.FilePath = "" }, false},
		{"missing task type", func(m *RequestMeta) { m.TaskType = "" }, false},
		{"zero request no", func(m *RequestMeta) { m.RequestNo = 0 }, false},
		{"negative request no", func(m *RequestMeta) { m.RequestNo = -1 }, false},
		{"NUL in provider", func(m *RequestMeta) { m.Provider = "anth\x00ropic" }, false},
		{"NUL in model", func(m *RequestMeta) { m.Model = "cla\x00ude" }, false},
		{"NUL in file path", func(m *RequestMeta) { m.FilePath = "pay\x00.go" }, false},
		{"NUL in task type", func(m *RequestMeta) { m.TaskType = "main\x00" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := testMeta()
			tc.mut(&m)
			if got := m.valid(); got != tc.want {
				t.Fatalf("valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLogicalRequestIDIsDeterministic(t *testing.T) {
	m := testMeta()
	if a, b := m.logicalRequestID("run-1"), m.logicalRequestID("run-1"); a != b {
		t.Fatalf("same input produced different IDs: %s vs %s", a, b)
	}
	if len(m.logicalRequestID("run-1")) != 64 {
		t.Fatalf("expected a 64-char hex digest, got %q", m.logicalRequestID("run-1"))
	}
}

// TestLogicalRequestIDSeparatesFields is the reason the encoding is canonical:
// a plain concatenation would give these metas the same digest.
func TestLogicalRequestIDSeparatesFields(t *testing.T) {
	base := testMeta()

	shifted := base
	shifted.Provider = "anthropi"
	shifted.Model = "cclaude-sonnet-4-6"

	swapped := base
	swapped.FilePath, swapped.TaskType = base.TaskType, base.FilePath

	cases := map[string]RequestMeta{
		"byte shifted between provider and model": shifted,
		"file path and task type swapped":         swapped,
	}
	want := base.logicalRequestID("run-1")
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if got := m.logicalRequestID("run-1"); got == want {
				t.Fatalf("distinct meta produced the same ID %s", got)
			}
		})
	}
}

func TestLogicalRequestIDVariesWithRunIDAndRequestNo(t *testing.T) {
	m := testMeta()
	base := m.logicalRequestID("run-1")

	if m.logicalRequestID("run-2") == base {
		t.Fatal("different run_id produced the same ID")
	}
	other := m
	other.RequestNo = 2
	if other.logicalRequestID("run-1") == base {
		t.Fatal("different request_no produced the same ID")
	}

	// The separator in front of request_no matters too: without it, task_type
	// "main_task1" with request_no 2 and task_type "main_task" with request_no 12
	// would hash the same bytes.
	a := m
	a.TaskType, a.RequestNo = "main_task1", 2
	b := m
	b.TaskType, b.RequestNo = "main_task", 12
	if a.logicalRequestID("run-1") == b.logicalRequestID("run-1") {
		t.Fatal("task_type and request_no are not separated")
	}
}

// The encoding is sha256 over version, run_id, provider, model, file_path,
// task_type and decimal request_no, each followed by a NUL — the trailing field
// included. The digest is pinned because that layout is not observable from any
// other assertion: with the current field set the ID stays collision-free with or
// without the last terminator, so dropping it would silently reintroduce the
// collision that appending a future field would cause.
//
// IDs only have to be stable within one run, so updating this constant is
// allowed. It just has to be a deliberate edit and not a silent side effect.
func TestLogicalRequestIDCanonicalEncoding(t *testing.T) {
	const want = "14e212a5316c922ea2e0758da1a243255ac33f6360fd0d4e70af90ad1441516c"
	if got := testMeta().logicalRequestID("run-1"); got != want {
		t.Fatalf("canonical encoding changed:\n got %s\nwant %s", got, want)
	}
}

func TestRequestMetaDescribe(t *testing.T) {
	got := testMeta().describe()
	for _, want := range []string{"file=payment.go", "task=main_task", "request_no=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("describe() = %q, missing %q", got, want)
		}
	}
}

func TestWithRequestMeta(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		m := testMeta()
		ctx := WithRequestMeta(context.Background(), m)
		got, ok := RequestMetaFromContext(ctx)
		if !ok || got != m {
			t.Fatalf("got (%+v, %v), want (%+v, true)", got, ok, m)
		}
	})

	t.Run("invalid meta is not attached", func(t *testing.T) {
		m := testMeta()
		m.Model = ""
		ctx := WithRequestMeta(context.Background(), m)
		if _, ok := RequestMetaFromContext(ctx); ok {
			t.Fatal("invalid meta was attached to the context")
		}
	})

	t.Run("bare context carries nothing", func(t *testing.T) {
		if _, ok := RequestMetaFromContext(context.Background()); ok {
			t.Fatal("empty context reported a meta")
		}
	})

	// Both directions tolerate a nil context instead of panicking, so a call site
	// that never set one up degrades to "no identity, no report" rather than
	// failing the review.
	t.Run("nil context", func(t *testing.T) {
		//nolint:staticcheck // deliberately passing a nil context
		if got := WithRequestMeta(nil, testMeta()); got != nil {
			t.Fatalf("WithRequestMeta(nil, ...) = %v, want nil", got)
		}
		//nolint:staticcheck // deliberately passing a nil context
		if m, ok := RequestMetaFromContext(nil); ok || m != (RequestMeta{}) {
			t.Fatalf("got (%+v, %v), want (zero, false)", m, ok)
		}
	})
}
