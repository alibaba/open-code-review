// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNaturalIntents(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  []string
	}{
		{
			"workspace",
			`{"action":"review","review":{"type":"workspace"}}`,
			[]string{"review", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			"range",
			`{"action":"review","review":{"type":"range","from":"main","to":"feature"}}`,
			[]string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			"commit",
			`{"action":"review","review":{"type":"commit","commit":"abc1234"}}`,
			[]string{"review", "--commit", "abc1234", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			"scan paths",
			`{"action":"scan","scan":{"paths":["internal/agent"]}}`,
			[]string{"scan", "--path", "internal/agent", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			"scan root",
			`{"action":"scan","scan":{}}`,
			[]string{"scan", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			"review with extra",
			`{"action":"review","review":{"type":"workspace"},"extra":["--effort","high"]}`,
			[]string{"review", "--effort", "high", "--format", "json", "--audience", "human", "--color", "never"},
		},
		{
			"scan with extra",
			`{"action":"scan","scan":{"paths":["pkg"]},"extra":["--batch","by-language"]}`,
			[]string{"scan", "--path", "pkg", "--batch", "by-language", "--format", "json", "--audience", "human", "--color", "never"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &fakeLLM{call: intentCall(t, tt.reply)}
			p := newTestParser(llm, nil)
			r, err := p.Parse(context.Background(), "a natural language request", NewState())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := requireIntent(t, r); !equalArgs(got, tt.want) {
				t.Fatalf("argv = %v, want %v", got, tt.want)
			}
			if llm.calls != 1 {
				t.Fatalf("LLM calls = %d, want 1", llm.calls)
			}
		})
	}
}

func TestNaturalClarify(t *testing.T) {
	reply := `{"action":"clarify","question":"Which two refs?","missing":["from","to"],"review":{"type":"range"}}`
	p := newTestParser(&fakeLLM{call: intentCall(t, reply)}, nil)
	st := NewState()
	r, err := p.Parse(context.Background(), "compare two branches", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireClarify(t, r, "from", "to")
	if r.Clarify.Question != "Which two refs?" {
		t.Fatalf("question = %q", r.Clarify.Question)
	}
	pend := st.Pending()
	if pend == nil || pend.Action != "review" || pend.ReviewType != "range" {
		t.Fatalf("pending = %+v", pend)
	}
}

func TestNaturalClarifyWithoutPartialSlots(t *testing.T) {
	reply := `{"action":"clarify","question":"What should I review?"}`
	p := newTestParser(&fakeLLM{call: intentCall(t, reply)}, nil)
	st := NewState()
	st.setPending(&Pending{Action: actionReview, ReviewType: reviewRange, From: "main", Missing: []string{"to"}})
	r, err := p.Parse(context.Background(), "do the thing", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireClarify(t, r)
	if st.Pending() != nil {
		t.Fatalf("pending should be cleared, got %+v", st.Pending())
	}
}

func TestNaturalFollowUpMergesPendingSlots(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, `{"action":"clarify","question":"Which head ref?","missing":["to"],"review":{"type":"range","from":"main"}}`)}
	p := newTestParser(llm, nil)
	st := NewState()
	if r, _ := p.Parse(context.Background(), "compare branches", st); r.Kind != KindClarify {
		t.Fatalf("first parse = %+v", r)
	}
	llm.call = intentCall(t, `{"action":"review","review":{"type":"range","to":"feature"}}`)
	r, err := p.Parse(context.Background(), "feature", st)
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

func TestNaturalCommitClarificationPreservesTypeAndQuestion(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, `{"action":"review","review":{"type":"commit"}}`)}
	p := newTestParser(llm, nil)
	st := NewState()
	r, err := p.Parse(context.Background(), "review a commit", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	c := requireClarify(t, r, "commit")
	if c.Question != "Which commit should I review? Add --commit <sha>." {
		t.Fatalf("question = %q", c.Question)
	}
	if pending := st.Pending(); pending == nil || pending.ReviewType != reviewCommit {
		t.Fatalf("pending = %+v", pending)
	}
	llm.call = intentCall(t, `{"action":"review","review":{"type":"commit","commit":"abc1234"}}`)
	r, err = p.Parse(context.Background(), "abc1234", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Review == nil || r.Review.Commit != "abc1234" || st.Pending() != nil {
		t.Fatalf("unexpected result or pending: %+v / %+v", r.Review, st.Pending())
	}
}

func TestNaturalReject(t *testing.T) {
	reply := `{"action":"reject","reason":"the adapter only reviews","hint":"Use /review"}`
	p := newTestParser(&fakeLLM{call: intentCall(t, reply)}, nil)
	st := NewState()
	st.setPending(&Pending{Action: "review", From: "main", Missing: []string{"to"}})
	r, err := p.Parse(context.Background(), "fix these bugs", st)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rej := requireReject(t, r)
	if rej.Hint != "Use /review" {
		t.Fatalf("hint = %q", rej.Hint)
	}
	if st.Pending() != nil {
		t.Fatal("reject did not clear pending")
	}
}

func TestNaturalReviewMissingSlots(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		missing string
	}{
		{"range missing to", `{"action":"review","review":{"type":"range","from":"main"}}`, "to"},
		{"commit missing sha", `{"action":"review","review":{"type":"commit"}}`, "commit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestParser(&fakeLLM{call: intentCall(t, tt.reply)}, nil)
			r, err := p.Parse(context.Background(), "x", NewState())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			requireClarify(t, r, tt.missing)
		})
	}
}

func TestNaturalRefValidation(t *testing.T) {
	repo := &fakeRepo{bad: map[string]bool{"nope": true}}

	p := newTestParser(&fakeLLM{call: intentCall(t, `{"action":"review","review":{"type":"range","from":"nope","to":"main"}}`)}, repo)
	r, err := p.Parse(context.Background(), "x", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireClarify(t, r, "ref")
	if len(repo.calls) != 1 || repo.calls[0] != "nope" {
		t.Fatalf("repo calls = %v", repo.calls)
	}

	p = newTestParser(&fakeLLM{call: intentCall(t, `{"action":"review","review":{"type":"commit","commit":"nope"}}`)}, repo)
	r, err = p.Parse(context.Background(), "x", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireClarify(t, r, "ref")

	p = newTestParser(&fakeLLM{call: intentCall(t, `{"action":"review","review":{"type":"range","from":"main","to":"feature"}}`)}, repo)
	r, err = p.Parse(context.Background(), "x", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireIntent(t, r)
}

func TestNaturalForbiddenExtra(t *testing.T) {
	tests := []string{
		`{"action":"review","review":{"type":"workspace"},"extra":["--format","yaml"]}`,
		`{"action":"scan","scan":{},"extra":["--ocr-binary","/tmp/evil"]}`,
		`{"action":"review","review":{"type":"workspace"},"extra":["--repo","/tmp/x"]}`,
		`{"action":"review","review":{"type":"workspace"},"extra":["--effort","ultra"]}`,
	}
	for i, reply := range tests {
		p := newTestParser(&fakeLLM{call: intentCall(t, reply)}, nil)
		r, err := p.Parse(context.Background(), "x", NewState())
		if err != nil {
			t.Fatalf("case %d Parse: %v", i, err)
		}
		requireReject(t, r)
	}
}

func TestNaturalLLMFailures(t *testing.T) {
	tests := []struct {
		name  string
		llm   LLMClient
		parse func(t *testing.T, p *Parser) Result
	}{
		{"llm error", &fakeLLM{err: errors.New("boom")}, nil},
		{"nil llm", nil, nil},
		{"wrong tool", &fakeLLM{call: ToolCall{Name: "other", Arguments: []byte(`{}`)}}, nil},
		{"malformed json", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{not json`)}}, nil},
		{"trailing data", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"review"}{"x":1}`)}}, nil},
		{"unknown field", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"review","bogus":1}`)}}, nil},
		{"unknown action", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"dance"}`)}}, nil},
		{"review missing object", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"review"}`)}}, nil},
		{"scan missing object", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"scan"}`)}}, nil},
		{"unknown review type", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"review","review":{"type":"sideways"}}`)}}, nil},
		{"clarify no question", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"clarify"}`)}}, nil},
		{"reject no reason", &fakeLLM{call: ToolCall{Name: submitIntentToolName, Arguments: []byte(`{"action":"reject"}`)}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := tt.llm
			if tt.name == "nil llm" {
				llm = nil
			}
			p := newTestParser(llm, nil)
			r, err := p.Parse(context.Background(), "x", NewState())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			requireReject(t, r)
		})
	}
}

func TestNaturalTimeout(t *testing.T) {
	llm := &fakeLLM{block: true}
	p := newTestParser(llm, nil).WithTimeout(20 * time.Millisecond)
	r, err := p.Parse(context.Background(), "x", NewState())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	requireReject(t, r)
}

func TestNaturalRequestShape(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, workspaceCallJSON)}
	p := newTestParser(llm, nil)
	st := NewState()
	st.setPending(&Pending{Action: "review", ReviewType: "range", From: "main", Missing: []string{"to"}})
	if _, err := p.Parse(context.Background(), "feature is the head", st); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	req := llm.lastReq
	if req.Tool.Name != submitIntentToolName {
		t.Fatalf("tool name = %q", req.Tool.Name)
	}
	if req.System == "" {
		t.Fatal("system prompt is empty")
	}
	if !strings.Contains(req.User, "feature is the head") {
		t.Fatalf("user prompt missing request text: %q", req.User)
	}
	if !strings.Contains(req.User, `"From":"main"`) {
		t.Fatalf("user prompt missing pending slots: %q", req.User)
	}
	if !strings.Contains(req.User, "Unfinished request") {
		t.Fatalf("user prompt missing pending explanation: %q", req.User)
	}
}
