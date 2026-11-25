// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/contract"
)

// fakeRepo is an offline RepoContext. Refs listed in bad fail to resolve.
type fakeRepo struct {
	bad   map[string]bool
	calls []string
}

func (f *fakeRepo) ResolveCommit(_ context.Context, ref string) error {
	f.calls = append(f.calls, ref)
	if f.bad[ref] {
		return errors.New("unknown revision")
	}
	return nil
}

// fakeLLM is a scripted LLMClient: it returns one canned call or a canned error.
type fakeLLM struct {
	call    ToolCall
	err     error
	calls   int
	lastReq LLMRequest
	block   bool
}

func (f *fakeLLM) CallTool(ctx context.Context, req LLMRequest) (ToolCall, error) {
	f.calls++
	f.lastReq = req
	if f.block {
		<-ctx.Done()
		return ToolCall{}, ctx.Err()
	}
	return f.call, f.err
}

// workspaceCallJSON is the common canned reply: a workspace review.
const workspaceCallJSON = `{"action":"review","review":{"type":"workspace"}}`

// intentCall marshals a raw JSON intent object into a submit_intent ToolCall.
func intentCall(t *testing.T, raw string) ToolCall {
	t.Helper()
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("test intent is not valid JSON: %v", err)
	}
	return ToolCall{Name: submitIntentToolName, Arguments: []byte(raw)}
}

// newTestParser builds a parser with a permissive repo and the given LLM.
func newTestParser(llm LLMClient, repo RepoContext) *Parser {
	if repo == nil {
		repo = &fakeRepo{}
	}
	return NewParser(llm, repo)
}

// requireIntent fails unless r is a runnable intent and returns its argv.
func requireIntent(t *testing.T, r Result) []string {
	t.Helper()
	if r.Kind != KindIntent {
		t.Fatalf("Kind = %v, want KindIntent (clarify=%+v reject=%+v)", r.Kind, r.Clarify, r.Reject)
	}
	switch {
	case r.Review != nil:
		args, err := contract.BuildReviewArgs(r.Review)
		if err != nil {
			t.Fatalf("BuildReviewArgs: %v", err)
		}
		return args
	case r.Scan != nil:
		args, err := contract.BuildScanArgs(r.Scan)
		if err != nil {
			t.Fatalf("BuildScanArgs: %v", err)
		}
		return args
	default:
		t.Fatal("intent result has neither Review nor Scan")
		return nil
	}
}

func requireClarify(t *testing.T, r Result, wantMissing ...string) *Clarify {
	t.Helper()
	if r.Kind != KindClarify {
		t.Fatalf("Kind = %v, want KindClarify (reject=%+v)", r.Kind, r.Reject)
	}
	if r.Clarify == nil || r.Clarify.Question == "" {
		t.Fatal("clarify result has no question")
	}
	if len(wantMissing) > 0 {
		got := map[string]bool{}
		for _, m := range r.Clarify.Missing {
			got[m] = true
		}
		for _, m := range wantMissing {
			if !got[m] {
				t.Fatalf("clarify missing = %v, want it to contain %q", r.Clarify.Missing, m)
			}
		}
	}
	return r.Clarify
}

func requireReject(t *testing.T, r Result) *Reject {
	t.Helper()
	if r.Kind != KindReject {
		t.Fatalf("Kind = %v, want KindReject (clarify=%+v)", r.Kind, r.Clarify)
	}
	if r.Reject == nil || r.Reject.Reason == "" {
		t.Fatal("reject result has no reason")
	}
	return r.Reject
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
