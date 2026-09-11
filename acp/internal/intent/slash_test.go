// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"testing"
)

func TestSlashCommands(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"review workspace", "/review", []string{"review", "--format", "json", "--audience", "human", "--color", "never"}},
		{"review range", "/review --from main --to feature", []string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"}},
		{"review range inline", "/review --from=main --to=feature", []string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"}},
		{"review commit", "/review --commit abc1234", []string{"review", "--commit", "abc1234", "--format", "json", "--audience", "human", "--color", "never"}},
		{"review effort", "/review --effort high", []string{"review", "--effort", "high", "--format", "json", "--audience", "human", "--color", "never"}},
		{"review bool flag", "/review --no-filter", []string{"review", "--no-filter", "--format", "json", "--audience", "human", "--color", "never"}},
		{"scan root", "/scan", []string{"scan", "--format", "json", "--audience", "human", "--color", "never"}},
		{"scan path", "/scan --path internal/agent", []string{"scan", "--path", "internal/agent", "--format", "json", "--audience", "human", "--color", "never"}},
		{"scan comma paths", "/scan --path a,b", []string{"scan", "--path", "a,b", "--format", "json", "--audience", "human", "--color", "never"}},
		{"scan batch", "/scan --batch by-language", []string{"scan", "--batch", "by-language", "--format", "json", "--audience", "human", "--color", "never"}},
		{"leading space is slash", "   /review", []string{"review", "--format", "json", "--audience", "human", "--color", "never"}},
		{"extra whitespace", "/review   --from  main   --to   feature", []string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &fakeLLM{call: intentCall(t, workspaceCallJSON)}
			p := newTestParser(llm, nil)
			r, err := p.Parse(context.Background(), tt.in, NewState())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := requireIntent(t, r); !equalArgs(got, tt.want) {
				t.Fatalf("argv = %v, want %v", got, tt.want)
			}
			if llm.calls != 0 {
				t.Fatalf("slash command called the LLM %d times", llm.calls)
			}
		})
	}
}

func TestSlashClarify(t *testing.T) {
	p := newTestParser(&fakeLLM{}, nil)
	st := NewState()
	r, err := p.Parse(context.Background(), "/review --from main", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireClarify(t, r, "to")
	pend := st.Pending()
	if pend == nil || pend.Action != "review" || pend.From != "main" || pend.ReviewType != "range" {
		t.Fatalf("pending = %+v", pend)
	}

	// The clarification answer completes the pending range.
	r, err = p.Parse(context.Background(), "/review --to feature", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"}
	if got := requireIntent(t, r); !equalArgs(got, want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	if st.Pending() != nil {
		t.Fatalf("pending not cleared after completion: %+v", st.Pending())
	}
}

func TestSlashClarifyMissingFrom(t *testing.T) {
	p := newTestParser(&fakeLLM{}, nil)
	r, err := p.Parse(context.Background(), "/review --to feature", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireClarify(t, r, "from")
}

func TestSlashRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unknown command", "/foo"},
		{"double dot", "/review main..feature"},
		{"double dot in from", "/review --from a..b --to c"},
		{"staged", "/review --staged"},
		{"path on review", "/review --path p"},
		{"from on scan", "/scan --from main"},
		{"unknown flag", "/review --bogus"},
		{"missing value", "/review --from"},
		{"missing path value", "/scan --path"},
		{"effort on scan", "/scan --effort high"},
		{"stray word", "/review main"},
		{"bad enum", "/review --effort ultra"},
		{"build cannot override format", "/review --format yaml"},
		{"repo flag", "/review --repo /tmp/x"},
		{"ocr binary flag", "/scan --ocr-binary /tmp/evil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &fakeLLM{call: intentCall(t, workspaceCallJSON)}
			p := newTestParser(llm, nil)
			r, err := p.Parse(context.Background(), tt.in, NewState())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			rej := requireReject(t, r)
			if rej.Hint == "" {
				t.Fatal("reject has no hint")
			}
			if llm.calls != 0 {
				t.Fatalf("reject path called the LLM %d times", llm.calls)
			}
		})
	}
}

func TestSlashNonSlashGoesToLLM(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, workspaceCallJSON)}
	p := newTestParser(llm, nil)
	r, err := p.Parse(context.Background(), "please use /review here", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireIntent(t, r)
	if llm.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", llm.calls)
	}
}

func TestEmptyInputRejected(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, workspaceCallJSON)}
	p := newTestParser(llm, nil)
	r, err := p.Parse(context.Background(), "   ", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireReject(t, r)
	if llm.calls != 0 {
		t.Fatalf("empty input called the LLM %d times", llm.calls)
	}
}
