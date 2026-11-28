// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestAcceptedCallerEffectChecksActualInputAndOutput(t *testing.T) {
	const impl, caller, unrelated = "rules.py", "consumer.py", "migrate.py"
	const before = "def normalize(n):\n    value = n\n    return value\n\ndef preview(n):\n    return n\n"
	const after = "def normalize(n):\n    value = n + 1\n    return value\n\ndef preview(n):\n    return normalize(n) + 8\n"
	const flow = "from .rules import normalize as transform\n\ndef write(n):\n    if n < 0:\n        return 'ignored'\n    value = transform(n)\n    return save(value)\n\ndef save(n):\n    return backend.write(n)\n"
	const migration = "def migrate(n):\n    return normalize(n)\n"
	dir := initPreviewRepo(t)
	commitIn(t, dir, impl, before, "introduce the accepted calculation")
	commitIn(t, dir, caller, flow, "store the calculation result")
	commitIn(t, dir, unrelated, "def migrate(n):\n    return n\n", "introduce the old migration")
	ctx := t.Context()
	git := gitcmd.New(1)
	output := func(args ...string) string {
		t.Helper()
		out, err := git.Output(ctx, dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	base := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	claim := "The writer saves the original non-negative input."
	f, err := base.Prepare(ctx, knowledge.Fact{Scope: caller, Source: impl, Origin: "discovery", Claim: claim,
		Category: knowledge.CategoryBusinessRule, Authority: knowledge.AuthorityInferred, Identifiers: []string{"normalize", "write"},
		Citations: []knowledge.Citation{{Kind: "code", Path: impl, StartLine: 2, EndLine: 3}, {Kind: "code", Path: caller, StartLine: 7, EndLine: 7}}})
	if err != nil {
		t.Fatal(err)
	}
	// This is the on-disk shape of an older discovery before it kept both names.
	f.Citations[1].Symbol = "write"
	commitIn(t, dir, impl, after, "change two functions in the same module")
	commitIn(t, dir, unrelated, migration, "edit the unrelated migration")
	head := knowledge.Tree{Repo: dir, Git: git, Commit: strings.TrimSpace(output("rev-parse", "HEAD"))}
	checked, err := head.CheckReview(ctx, base.Commit, []knowledge.Fact{f})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := knowledge.ReviewSnapshot(checked)
	if err = snapshot.DescribeReviewChanges(ctx, base, head, checked); err != nil {
		t.Fatal(err)
	}
	want := knowledge.AcceptedCall{CallerPath: caller, Caller: "write", CalleePath: impl, Callee: "normalize"}
	if len(snapshot) != 1 || snapshot[0].AcceptedCall == nil || *snapshot[0].AcceptedCall != want || snapshot[0].ContextStatus() != knowledge.StatusTransition {
		t.Fatalf("missing accepted relation after the callee changed: %+v", snapshot)
	}
	diffs := []model.Diff{{OldPath: impl, NewPath: impl, NewFileContent: after, Diff: output("diff", base.Commit, head.Commit, "--", impl)},
		{OldPath: unrelated, NewPath: unrelated, NewFileContent: migration, Diff: output("diff", base.Commit, head.Commit, "--", unrelated)}}
	g := FileGroup{Diffs: diffs, business: selectBusinessContext(snapshot, diffs)}
	intermediate, err := head.Cite(ctx, impl, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := head.Cite(ctx, caller, 9, 10)
	if err != nil {
		t.Fatal(err)
	}
	g.business.lookupEvidence = []knowledge.Citation{intermediate, downstream}
	key := fileGroupKey(diffs)
	first := businessCheck{ID: snapshot[0].ID, Outcome: "violation", Path: impl, SourcePath: caller, Quote: "value = n + 1",
		SourceQuote: "return save(value)", Trigger: "n=2", Reason: "Calling write(2) saves 3.", Severity: "medium"}
	guard := first
	guard.Trigger, guard.Reason = "n=-1", "Calling write(-1) saves 0."
	other := first
	other.Quote = "return normalize(n) + 8"
	moved := first
	moved.Path, moved.Quote = unrelated, "return normalize(n)"
	payload, _ := json.Marshal(map[string]any{"checks": []businessCheck{first, guard, other, moved}})
	verify := callerEffectResult{ID: "0", Verdict: "confirmed", Input: "n=2", Reachable: true, Reachability: "2 passes the nonnegative guard.",
		BaseResult: "2", HeadResult: "3", Expected: "saved 2", Before: "saved 2", After: "saved 3", Reason: "The writer passes the changed return value to save.",
		BaseQuote: "return value", HeadQuote: "return value", CallQuote: "value = transform(n)", UseQuote: "return save(value)",
		OutputPath: caller, OutputLine: 7, OutputQuote: "return save(value)"}
	refuted := callerEffectResult{ID: "1", Verdict: "refuted", Input: "n=-1", Reachability: "The caller returns ignored before invoking the calculation."}
	for _, tc := range []struct {
		name string
		edit func(*callerEffectResult)
		keep bool
	}{
		{"reachable", func(*callerEffectResult) {}, true},
		{"reached same-file output", func(r *callerEffectResult) {
			r.OutputLine, r.OutputQuote = 10, "return backend.write(n)"
		}, true},
		{"invented same-file output", func(r *callerEffectResult) {
			r.OutputLine, r.OutputQuote = 10, "return backend.write(0)"
		}, false},
		{"wrong input", func(r *callerEffectResult) { r.Input = "n=7" }, false},
		{"caller excludes input", func(r *callerEffectResult) { r.Reachable = false }, false},
		{"invocation alone", func(r *callerEffectResult) { r.OutputLine, r.OutputQuote = 6, r.CallQuote }, false},
		{"only the callee changed", func(r *callerEffectResult) { r.After = r.Before }, false},
		{"unproven use", func(r *callerEffectResult) { r.UseQuote = "return save(n)" }, false},
		{"unrelated output", func(r *callerEffectResult) { r.OutputPath = unrelated }, false},
		{"supplemental quote from the intermediate calculation", func(r *callerEffectResult) {
			r.OutputPath, r.OutputLine, r.OutputQuote = impl, 3, "return value"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := verify
			tc.edit(&r)
			answer, _ := json.Marshal(map[string]any{"checks": []callerEffectResult{r, refuted}})
			review, effect := string(payload), string(answer)
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &review}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &effect}}}},
			}}
			a := New(Args{RepoDir: dir, KnowledgeTree: &head, LLMClient: client, Template: template.Template{MaxTokens: 16000}, SkipFilter: true})
			a.inputResolution.ResolvedBase = base.Commit
			a.verifyBusinessFindings(ctx, g, "", diffs[0].Diff+"\n"+diffs[1].Diff)
			out := a.args.CommentCollector.CommentsForPath(impl)
			if (len(out) == 1) != tc.keep || len(out) > 1 || len(out) == 1 && !strings.Contains(out[0].Content, first.Reason) ||
				len(a.args.CommentCollector.CommentsForPath(unrelated)) != 0 {
				t.Fatalf("added findings: %+v", out)
			}
			records := a.session.GetOrCreateFileSession(key).TaskRecords[session.BusinessEffectTask]
			if len(records) != 1 || client.calls != 2 {
				t.Fatalf("independent outcome requests: %d; client calls: %d", len(records), client.calls)
			}
			var evidence struct {
				Proposals []callerEffectProposal `json:"proposals"`
				Functions []knowledge.Citation   `json:"functions"`
			}
			if err := json.Unmarshal([]byte(records[0].RequestMessages[1].ExtractText()), &evidence); err != nil {
				t.Fatal(err)
			}
			if len(evidence.Proposals) != 2 || len(evidence.Functions) != 3 {
				t.Fatalf("unrelated proposals or missing functions: %+v", evidence)
			}
			p := evidence.Proposals[0]
			consumer := evidence.Functions[p.Head.Caller].Quote
			if p.Rule != claim || !strings.Contains(consumer, "if n < 0:") || !strings.Contains(consumer, "return save(value)") ||
				!strings.Contains(evidence.Functions[p.Base.Callee].Quote, "value = n\n") || !strings.Contains(evidence.Functions[p.Head.Callee].Quote, "value = n + 1\n") {
				t.Fatalf("missing base, guard, change or destination: %+v", evidence)
			}
			if tc.keep {
				old := evidence.Functions[p.Base.Caller]
				old.Quote = strings.Replace(old.Quote, "value = transform(n)", "value = transform(abs(n))", 1)
				p.Base.Caller = len(evidence.Functions)
				evidence.Functions = append(evidence.Functions, old)
				if r.confirms(p, evidence.Functions, nil) {
					t.Fatal("a different accepted invocation had no supporting quote")
				}
				r.BaseCallQuote, r.BaseUseQuote = "value = transform(abs(n))", "return save(value)"
				if !r.confirms(p, evidence.Functions, g.business.lookupEvidence) {
					t.Fatal("both versions of the invocation and result use were verified")
				}
			}
		})
	}
}

func TestCallerEffectSplitsIndividuallyFittingProposals(t *testing.T) {
	one := func(p string) businessAddition {
		caller := knowledge.Citation{Path: p, StartLine: 1, EndLine: 303,
			Quote: "def finish(n):\n    value = convert(n)\n" + strings.Repeat("    progress(n)\n", 300) + "    return save(value)"}
		callee := knowledge.Citation{Path: "source_" + p, StartLine: 1, EndLine: 2, Quote: "def convert(n):\n    return n"}
		base := knowledge.CallerRevision{Revision: "base", Caller: caller, Callee: callee, CallLine: 2, ResultLine: 303}
		head := base
		head.Revision, head.Callee.Quote = "head", "def convert(n):\n    return n + 1"
		return businessAddition{check: businessCheck{Path: callee.Path, Reason: "The changed return reaches the writer.",
			caller: &knowledge.ReviewedCall{Rule: "Save the input.", Base: base, Head: head}}}
	}
	additions := []businessAddition{one("left.py"), one("right.py")}
	requests := func(limit int, batch []businessAddition) []int {
		t.Helper()
		empty := `{"checks":[]}`
		reply := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &empty}}}}
		client := &fakeAgentClient{responses: []*llm.ChatResponse{reply, reply}}
		a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: limit}})
		a.verifyCallerEffects(t.Context(), "group", batch, nil)
		var sizes []int
		for _, call := range a.session.GetOrCreateFileSession("group").TaskRecords[session.BusinessEffectTask] {
			sizes = append(sizes, llmloop.CountMessagesTokens(call.RequestMessages))
		}
		return sizes
	}
	full, single := requests(32000, additions), requests(32000, additions[:1])
	if len(full) != 1 || len(single) != 1 || full[0] <= single[0] {
		t.Fatalf("unexpected original prompt sizes: %v, %v", full, single)
	}
	// Place the prompt limit between the measured single and combined requests.
	window := (single[0] + full[0]) * 5 / 8
	fit := llmloop.PromptTokenLimit(window)
	if sizes := requests(window, additions); len(sizes) != 2 || sizes[0] > fit || sizes[1] > fit {
		t.Fatalf("individual requests %v under limit %d", sizes, fit)
	}
}
