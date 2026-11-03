// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNaturalResponseLanguageContract(t *testing.T) {
	for _, tc := range []struct {
		name, input, reply, message string
		kind                        Kind
	}{
		{"Chinese greeting", "\u4f60\u597d", `{"action":"reject","reason":"\u4f60\u597d\uff01","hint":"\u8bf7\u4f7f\u7528 /review \u6216 /scan\u3002"}`, "\u4f60\u597d\uff01", KindReject},
		{"English greeting", "hello", `{"action":"reject","reason":"Hello!","hint":"Use /review or /scan."}`, "Hello!", KindReject},
		{"Japanese clarification", "\u30b3\u30df\u30c3\u30c8\u3092\u30ec\u30d3\u30e5\u30fc", `{"action":"clarify","question":"\u3069\u306e\u30b3\u30df\u30c3\u30c8\u3067\u3059\u304b\uff1f","missing":["commit"]}`, "\u3069\u306e\u30b3\u30df\u30c3\u30c8\u3067\u3059\u304b\uff1f", KindClarify},
	} {
		t.Run(tc.name, func(t *testing.T) {
			llm := &fakeLLM{call: intentCall(t, tc.reply)}
			r, err := newTestParser(llm, nil).Parse(context.Background(), tc.input, &State{})
			if err != nil || r.Kind != tc.kind || llm.calls != 1 {
				t.Fatalf("unexpected parse: %+v %v", r, err)
			}
			if !strings.Contains(llm.lastReq.System, "language of the current user request") || !strings.Contains(llm.lastReq.System, "explicit request for a different response language") || !strings.Contains(llm.lastReq.User, tc.input) {
				t.Fatal("response-language instructions or original request missing")
			}
			if r.Kind == KindReject && (r.Reject.Reason != tc.message || !strings.Contains(r.Reject.Hint, "/review")) {
				t.Fatalf("localized rejection lost: %+v", r.Reject)
			}
			if r.Kind == KindClarify && (r.Clarify.Question != tc.message || r.Clarify.Missing[0] != "commit") {
				t.Fatalf("localized clarification changed slots: %+v", r.Clarify)
			}
		})
	}
}

func TestNaturalLatestCommitInstructionsAndValidation(t *testing.T) {
	for _, invalidHead := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalidHead=%v", invalidHead), func(t *testing.T) {
			input := "\u5ba1\u67e5\u4e00\u4e0b\u6700\u65b0\u63d0\u4ea4\u7684commit"
			llm := &fakeLLM{call: intentCall(t, `{"action":"review","review":{"type":"commit","commit":"HEAD"}}`)}
			repo := &fakeRepo{bad: map[string]bool{"HEAD": invalidHead}}
			state := NewState()
			state.setPending(&Pending{Action: actionReview, ReviewType: reviewCommit, Commit: "older-ref"})
			result, err := NewParser(llm, repo).Parse(context.Background(), input, state)
			if err != nil || llm.calls != 1 || !strings.Contains(llm.lastReq.User, input) {
				t.Fatalf("parse = %+v, %v; calls=%d", result, err, llm.calls)
			}
			// Verify the actual model request carries the relative-ref policy;
			// the canned response only tests deterministic validation and argv.
			for _, required := range []string{
				`"latest commit"`, `"most recent commit"`, `"last commit"`,
				`{"action":"review","review":{"type":"commit","commit":"HEAD"}}`,
				`/review --commit HEAD`, `Do not ask for a SHA`,
			} {
				if !strings.Contains(llm.lastReq.System, required) {
					t.Errorf("model instructions missing %q", required)
				}
			}
			if !strings.Contains(string(llm.lastReq.Tool.Parameters), "HEAD") {
				t.Error("commit field schema does not describe HEAD")
			}
			if !equalArgs(repo.calls, []string{"HEAD"}) {
				t.Fatalf("commit was not checked against the repository: %v", repo.calls)
			}
			if invalidHead {
				requireClarify(t, result, "ref")
				if pending := state.Pending(); pending == nil || pending.Commit != "HEAD" {
					t.Fatalf("invalid ref not retained for correction: %+v", pending)
				}
				return
			} else if got := requireIntent(t, result); !equalArgs(got, []string{"review", "--commit", "HEAD", "--format", "json", "--audience", "human", "--color", "never"}) {
				t.Fatalf("latest commit argv = %v", got)
			}
			if state.Pending() != nil {
				t.Fatalf("completed request retained pending state: %+v", state.Pending())
			}
		})
	}
}

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

func TestNaturalClarifyChainPreservesPartialSlots(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, `{"action":"clarify","question":"Which head?","missing":["to"],"review":{"type":"range","from":"main"}}`)}
	p := newTestParser(llm, nil)
	st := NewState()
	if r, _ := p.Parse(context.Background(), "compare", st); r.Kind != KindClarify {
		t.Fatal(r)
	}
	llm.call = intentCall(t, `{"action":"clarify","question":"Which head?","missing":["to"],"review":{"type":"range"}}`)
	if r, _ := p.Parse(context.Background(), "still deciding", st); r.Kind != KindClarify {
		t.Fatal(r)
	}
	if st.Pending().From != "main" {
		t.Fatalf("pending lost from: %+v", st.Pending())
	}
	llm.call = intentCall(t, `{"action":"review","review":{"type":"range","to":"feature"}}`)
	r, err := p.Parse(context.Background(), "feature", st)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"review", "--from", "main", "--to", "feature", "--format", "json", "--audience", "human", "--color", "never"}
	if got := requireIntent(t, r); !equalArgs(got, want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	if st.Pending() != nil {
		t.Fatal("pending not cleared")
	}
}

func TestNaturalClarifyTypeSwitchDoesNotLeakReview(t *testing.T) {
	llm := &fakeLLM{call: intentCall(t, `{"action":"clarify","question":"Which ref?","missing":["commit"],"review":{"type":"commit"}}`)}
	p := newTestParser(llm, nil)
	st := NewState()
	st.setPending(&Pending{Action: actionReview, ReviewType: reviewRange, From: "main", To: "feature"})
	if _, err := p.Parse(context.Background(), "commit", st); err != nil {
		t.Fatal(err)
	}
	if got := st.Pending(); got == nil || got.From != "" || got.To != "" {
		t.Fatalf("cross-type leak: %+v", got)
	}
}

func TestClarificationMergeCompatibilityAndLists(t *testing.T) {
	for _, tc := range []struct {
		name, reply      string
		previous         Pending
		from, to, commit string
		extra, paths     []string
	}{
		{name: "new range value wins", previous: Pending{Action: actionReview, ReviewType: reviewRange, From: "old", To: "tip", Extra: []string{"--no-filter"}}, reply: `{"action":"clarify","question":"Confirm?","review":{"type":"range","from":"new"}}`, from: "new", to: "tip", extra: []string{"--no-filter"}},
		{name: "explicit empty extra clears", previous: Pending{Action: actionReview, ReviewType: reviewRange, From: "main", Extra: []string{"--no-filter"}}, reply: `{"action":"clarify","question":"Head?","review":{"type":"range"},"extra":[]}`, from: "main"},
		{name: "extra replaces", previous: Pending{Action: actionReview, ReviewType: reviewRange, Extra: []string{"--no-filter"}}, reply: `{"action":"clarify","question":"Refs?","review":{"type":"range"},"extra":["--effort","low"]}`, extra: []string{"--effort", "low"}},
		{name: "commit preserved", previous: Pending{Action: actionReview, ReviewType: reviewCommit, Commit: "abc"}, reply: `{"action":"clarify","question":"Confirm?","review":{"type":"commit"}}`, commit: "abc"},
		{name: "commit replaced", previous: Pending{Action: actionReview, ReviewType: reviewCommit, Commit: "abc"}, reply: `{"action":"clarify","question":"Confirm?","review":{"type":"commit","commit":"def"}}`, commit: "def"},
		{name: "commit to range", previous: Pending{Action: actionReview, ReviewType: reviewCommit, Commit: "abc", Extra: []string{"--no-filter"}}, reply: `{"action":"clarify","question":"Head?","review":{"type":"range","from":"main"}}`, from: "main"},
		{name: "scan to review", previous: Pending{Action: actionScan, Paths: []string{"a.go"}, Extra: []string{"--no-plan"}}, reply: `{"action":"clarify","question":"Head?","review":{"type":"range","from":"main"}}`, from: "main"},
		{name: "review to scan", previous: Pending{Action: actionReview, ReviewType: reviewRange, From: "main", Extra: []string{"--effort", "high"}}, reply: `{"action":"clarify","question":"Files?","scan":{}}`},
		{name: "scan omitted lists retained", previous: Pending{Action: actionScan, Paths: []string{"a.go"}, Extra: []string{"--no-plan"}}, reply: `{"action":"clarify","question":"Confirm?","scan":{}}`, paths: []string{"a.go"}, extra: []string{"--no-plan"}},
		{name: "scan empty lists clear", previous: Pending{Action: actionScan, Paths: []string{"a.go"}, Extra: []string{"--no-plan"}}, reply: `{"action":"clarify","question":"Confirm?","scan":{"paths":[]},"extra":[]}`},
		{name: "scan lists replace", previous: Pending{Action: actionScan, Paths: []string{"a.go"}, Extra: []string{"--no-plan"}}, reply: `{"action":"clarify","question":"Confirm?","scan":{"paths":["b.go"]},"extra":["--no-summary"]}`, paths: []string{"b.go"}, extra: []string{"--no-summary"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := NewState()
			st.setPending(&tc.previous)
			llm := &fakeLLM{call: intentCall(t, tc.reply)}
			p := newTestParser(llm, nil)
			r, err := p.Parse(context.Background(), "follow up", st)
			if err != nil || r.Kind != KindClarify {
				t.Fatalf("%+v %v", r, err)
			}
			got := st.Pending()
			if got == nil || got.From != tc.from || got.To != tc.to || got.Commit != tc.commit || !equalArgs(got.Extra, tc.extra) || !equalArgs(got.Paths, tc.paths) {
				t.Fatalf("pending=%+v", got)
			}
		})
	}
}

func TestScanClarificationChainCompletesWithMergedLists(t *testing.T) {
	for _, explicitEmpty := range []bool{false, true} {
		llm := &fakeLLM{call: intentCall(t, `{"action":"clarify","question":"Confirm?","scan":{"paths":["a.go"]},"extra":["--no-plan"]}`)}
		p := newTestParser(llm, nil)
		st := NewState()
		if _, err := p.Parse(context.Background(), "scan files", st); err != nil {
			t.Fatal(err)
		}
		llm.call = intentCall(t, `{"action":"clarify","question":"Confirm again?","scan":{}}`)
		if r, err := p.Parse(context.Background(), "continue", st); err != nil || r.Kind != KindClarify {
			t.Fatalf("%+v %v", r, err)
		}
		llm.call = intentCall(t, `{"action":"scan","scan":{}}`)
		want := []string{"scan", "--path", "a.go", "--no-plan", "--format", "json", "--audience", "human", "--color", "never"}
		if explicitEmpty {
			llm.call = intentCall(t, `{"action":"scan","scan":{"paths":[]},"extra":[]}`)
			want = []string{"scan", "--format", "json", "--audience", "human", "--color", "never"}
		}
		r, err := p.Parse(context.Background(), "confirmed", st)
		if err != nil {
			t.Fatal(err)
		}
		if got := requireIntent(t, r); !equalArgs(got, want) {
			t.Fatalf("argv=%v want=%v", got, want)
		}
		if st.Pending() != nil {
			t.Fatal("pending not cleared")
		}
	}
}

func TestWorkspaceClarificationOnlyPreservesCompatibleOptions(t *testing.T) {
	st := NewState()
	st.setPending(&Pending{Action: actionReview, ReviewType: reviewWorkspace, Extra: []string{"--no-filter"}})
	llm := &fakeLLM{call: intentCall(t, `{"action":"clarify","question":"Confirm?","review":{"type":"workspace"}}`)}
	p := newTestParser(llm, nil)
	if _, err := p.Parse(context.Background(), "confirm later", st); err != nil {
		t.Fatal(err)
	}
	if got := st.Pending(); !equalArgs(got.Extra, []string{"--no-filter"}) || got.From != "" || got.Commit != "" {
		t.Fatalf("pending=%+v", got)
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

type diagnosticTimeoutLLM struct{}

func (diagnosticTimeoutLLM) CallTool(ctx context.Context, _ LLMRequest) (ToolCall, error) {
	<-ctx.Done()
	return ToolCall{}, fmt.Errorf("waiting for response headers: %w", ctx.Err())
}

func TestNaturalTimeoutDiagnostics(t *testing.T) {
	for _, name := range []string{"parser timeout", "parent cancellation", "parent deadline"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			switch name {
			case "parent cancellation":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "parent deadline":
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			p := newTestParser(diagnosticTimeoutLLM{}, nil).WithTimeout(time.Millisecond)
			r, err := p.Parse(ctx, "review my changes", NewState())
			if err != nil {
				t.Fatal(err)
			}
			requireReject(t, r)
			if !strings.Contains(r.Reject.Reason, "waiting for response headers:") {
				t.Fatalf("lost underlying diagnostic: %+v", r.Reject)
			}
			if !strings.Contains(r.Reject.Hint, "/review") || !strings.Contains(r.Reject.Hint, "/scan") {
				t.Fatalf("missing direct command alternatives: %+v", r.Reject)
			}
			ownTimeout := strings.Contains(r.Reject.Reason, "configured parser timeout of 1ms")
			if ownTimeout != (name == "parser timeout") {
				t.Fatalf("incorrect timeout attribution: %+v", r.Reject)
			}
			if name == "parser timeout" && !strings.Contains(r.Reject.Reason, "OCR has not started") {
				t.Fatalf("missing execution stage: %+v", r.Reject)
			}
		})
	}
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
