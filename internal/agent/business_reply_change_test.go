// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestChangedReplyRechecksTheOldPremiseWithoutAnOrdinaryFinding(t *testing.T) {
	const path = "fees.py"
	const before = "price = zone.quote"
	const after = "price = abs(zone.quote)"
	const operation = "return max(0, price)"
	const original = "The lower bound is redundant for every delivery quote."
	const reply = "A negative refund quote reaches checkout. The lower bound changes its value."
	const note = "A negative quote now passes through abs before checkout, so the lower bound has no effect. Return price directly."
	const input = "zone.quote=-4"
	const duplicate = "For a negative quote, the clamp can be removed because its input is already nonnegative."
	const different = "This return also drops the currency code for a foreign order."
	function := func(assignment string) string { return "def checkout(zone):\n    " + assignment + "\n    " + operation }
	cite := func(first, last int, code, rev string) knowledge.Citation {
		return knowledge.Citation{Kind: "code", Path: path, StartLine: first, EndLine: last, Quote: code, Revision: rev}
	}
	change := knowledge.ReviewReplyChange{Submitted: cite(1, 3, function(before), "submitted"), Current: cite(1, 3, function(after), "head"),
		Hunks: []knowledge.ReviewReplyHunk{{Submitted: cite(2, 2, "    "+before, "submitted"), Current: cite(2, 2, "    "+after, "head")}}}
	f := knowledge.Fact{ID: "checkout-reply", Scope: path, Source: path, Revision: "submitted", Category: knowledge.CategoryDecision,
		Origin: "review-reply", Authority: knowledge.AuthorityHistorical, ReviewID: "delivery", Finding: original, Reason: reply,
		ReviewReply: &knowledge.ReviewReply{ID: "second", ReplyTo: "first", Change: &change},
		Citations:   []knowledge.Citation{{Kind: "code", CurrentPath: path, CurrentStartLine: 3, CurrentEndLine: 3, Quote: operation}}}
	patch := "@@ -1,3 +1,3 @@\n def checkout(zone):\n-    " + before + "\n+    " + after + "\n     " + operation
	d := model.Diff{NewPath: path, NewFileContent: function(after) + "\n", Diff: patch}
	c := businessCheck{ID: f.ID, Outcome: "reopened", Severity: "medium", Path: path, Line: 3, Quote: operation,
		SourcePath: path, SubmittedQuote: before, SourceQuote: after, PriorClaim: original, ReplyQuote: reply, Trigger: input, ChangeEffect: "redundancy", Reason: note}
	overlap := `{"duplicates":[{"pair":"p-0","existing_trigger":"negative quote","existing_effect":"the clamp can be removed","additional_trigger":"negative quote","additional_effect":"the lower bound has no effect","reason":"Both identify the exact same redundant lower bound."}]}`
	for _, tc := range []struct {
		name, draft, overlap string
		stage                int
		key, value           string
		added                bool
	}{
		{name: "no ordinary candidates", added: true},
		{name: "same input with outer whitespace", stage: 2, key: "input", value: "\n" + input + " ", added: true},
		{name: "same operation with another finding", draft: different, overlap: `{"duplicates":[]}`, added: true},
		{name: "same operation already diagnosed", draft: duplicate, overlap: overlap},
		{name: "only a question in the discussion", stage: 1, key: "question"},
		{name: "acknowledgment supplies no prior explanation", stage: 1, key: "acknowledgment"},
		{name: "first reviewer quotes a different historical line", stage: 1, key: "old_line"},
		{name: "first reviewer names a different published revision", stage: 1, key: "wrong_revision"},
		{name: "changed current code was already present before this patch", stage: 1, key: "old_edit"},
		{name: "only a comment was edited", stage: 1, key: "comment"},
		{name: "independent reviewer refutes the proposal", stage: 2, key: "verdict", value: "refuted"},
		{name: "operation was already redundant at publication", stage: 2, key: "before_without", value: "0"},
		{name: "operation still matters for this input", stage: 2, key: "after_without", value: "-4"},
		{name: "all current branches must be checked", stage: 2, key: "all_inputs", value: ""},
		{name: "missing quote of the actual edit", stage: 2, key: "current_quote", value: operation},
		{name: "different independent test input", stage: 2, key: "input", value: "zone.quote=4"},
		{name: "an unrelated output line is insufficient", stage: 2, key: "output_quote", value: after},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fact, reviewed, nomination := f, d, c
			proof := map[string]any{"id": "e-0", "verdict": "confirmed", "input": input, "before": "0", "after": "4",
				"before_without": "-4", "after_without": "4", "all_inputs": "Every supported integer passes through abs before the return.",
				"reason": "The clamp changes negative values only in the earlier function.", "submitted_quote": before, "current_quote": after,
				"output_path": path, "output_line": 3, "output_quote": operation}
			wantCalls := 2
			switch tc.stage {
			case 1:
				switch tc.key {
				case "question":
					fact.Reason = "Does this happen?"
					nomination.ReplyQuote = "The lower bound changes its value."
					wantCalls = 1
				case "acknowledgment":
					fact.Reason = "Thanks for the review."
					nomination.ReplyQuote = fact.Reason
					proof["verdict"] = "unknown"
					proof["reason"] = "The reply supplies no factual reason to compare with the code."
				case "old_line":
					nomination.SubmittedQuote = operation
					wantCalls = 1
				case "wrong_revision":
					fact.Revision = "other-submission"
					wantCalls = 0
				case "old_edit":
					reviewed.Diff = "@@ -3,1 +3,2 @@\n     " + operation + "\n+    # Publish a receipt."
					reviewed.NewFileContent += "    # Publish a receipt.\n"
					wantCalls = 0
				case "comment":
					linked := *f.ReviewReply
					linked.Change = nil
					fact.ReviewReply = &linked
					reviewed.Diff = "@@ -1,3 +1,4 @@\n def checkout(zone):\n+    # Compute this quote.\n     " + before + "\n     " + operation
					reviewed.NewFileContent = strings.Replace(function(before), "    "+before, "    # Compute this quote.\n    "+before, 1)
					fact.Citations = []knowledge.Citation{{Kind: "code", CurrentPath: path, CurrentStartLine: 4, CurrentEndLine: 4, Quote: operation}}
					wantCalls = 0
				}
			case 2:
				proof[tc.key] = tc.value
			}
			proofJSON, err := json.Marshal(map[string]any{"checks": []map[string]any{proof}})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeAgentClient{}
			for _, resp := range []string{businessReply(t, nomination), string(proofJSON), tc.overlap} {
				client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &resp}}}})
			}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			ordinary := model.LlmComment{Path: path, StartLine: 3, ExistingCode: operation, Content: tc.draft}
			if tc.draft != "" {
				a.args.CommentCollector.Add(ordinary)
				wantCalls++
			}
			g := FileGroup{Diffs: []model.Diff{reviewed}}
			g.business = a.reviewBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: fact, Status: knowledge.StatusHistorical}}), g.Diffs)
			if len(g.business.rules) > 0 {
				a.verifyBusinessFindings(t.Context(), g, "", reviewed.Diff)
			}
			out := a.args.CommentCollector.CommentsForPath(path)
			if wantCalls != client.calls {
				t.Fatalf("wanted %d calls, got %d; comments=%+v", wantCalls, client.calls, out)
			}
			if tc.draft != "" {
				if len(out) == 0 || out[0] != ordinary {
					t.Fatalf("ordinary finding changed: %+v", out)
				}
				out = out[1:]
			}
			if !tc.added {
				if len(out) != 0 {
					t.Fatalf("unproven or already reported historical issue was added: %+v", out)
				}
				return
			}
			if len(out) != 1 || out[0].StartLine != 3 || out[0].ExistingCode != operation ||
				!strings.Contains(out[0].Content, note) || out[0].Severity != "low" || out[0].Category != "maintainability" {
				t.Fatalf("reopened a different finding: %+v", out)
			}
			record := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords[session.BusinessEffectTask][0]
			data := record.RequestMessages[1].ExtractText()
			var supplied struct {
				Patches map[string]string `json:"patches"`
			}
			if err := json.Unmarshal([]byte(data), &supplied); err != nil || supplied.Patches[path] != "@@ -2,1 +2,1 @@\n-    "+before+"\n+    "+after {
				t.Fatalf("independent reviewer did not receive the actual input change: %+v, %v", supplied.Patches, err)
			}
			for _, required := range []string{original, reply, before, after, operation, `"revision":"submitted"`, `"revision":"head"`} {
				if !strings.Contains(data, required) {
					t.Errorf("independent check lacks %q in %s", required, data)
				}
			}
		})
	}
}

func TestChangedReplyRequiresANewFailureForTheSameSchedule(t *testing.T) {
	const path = "schedule.py"
	const submitted = "ready = [task for task in jobs if task.zone == zone]"
	const current = "ready = list(jobs)"
	const operation = "enqueue(task)"
	const original = "A job from a different time zone can be queued here."
	const reply = "The ready list includes only jobs from the selected zone."
	const input = "one job in a different zone"
	function := func(line string) string {
		return "def run(jobs, zone):\n    " + line + "\n    for task in ready:\n        " + operation
	}
	citation := func(code, revision string, first, last int) knowledge.Citation {
		return knowledge.Citation{Kind: "code", Path: path, Revision: revision, Quote: code, StartLine: first, EndLine: last}
	}
	change := &knowledge.ReviewReplyChange{
		Submitted: citation(function(submitted), "then", 1, 4), Current: citation(function(current), "now", 1, 4),
		Hunks: []knowledge.ReviewReplyHunk{{Submitted: citation(submitted, "then", 2, 2), Current: citation(current, "now", 2, 2)}},
	}
	f := knowledge.Fact{ID: "dispatch-reply", Revision: "then", Scope: path, Source: path, ReviewID: "dispatch",
		Category: knowledge.CategoryDecision, Origin: "review-reply", Authority: knowledge.AuthorityHistorical, Finding: original, Reason: reply,
		ReviewReply: &knowledge.ReviewReply{ID: "answer", ReplyTo: "comment", Change: change},
		Citations:   []knowledge.Citation{{Kind: "code", CurrentPath: path, CurrentStartLine: 4, CurrentEndLine: 4, Quote: operation}}}
	d := model.Diff{NewPath: path, NewFileContent: function(current), Diff: "@@ -1,4 +1,4 @@\n def run(jobs, zone):\n-    " + submitted + "\n+    " + current + "\n     for task in ready:\n         " + operation}
	c := businessCheck{ID: f.ID, Outcome: "reopened", Severity: "medium", Path: path, Line: 2, Quote: current,
		SourcePath: path, SourceQuote: current, SubmittedQuote: submitted, PriorClaim: original, ReplyQuote: reply, Trigger: input, ChangeEffect: "failure",
		Reason: "The scheduler now queues a job even when its zone differs from the requested zone. Filter the ready list before dispatch."}
	for _, tc := range []struct {
		name                    string
		before, after, expected any
		report                  bool
	}{
		{"one newly queued job outside the selected zone", "0", "1", "0", true},
		{"old behavior already violates the alleged requirement", "1", "2", "0", false},
		{"same observable result", "0", "0", "0", false},
		{"number of foreign jobs as JSON numbers", 0, 1, 0, true},
		{"presence of a foreign job as JSON booleans", false, true, "false", true},
		{"complete integer precision", json.Number("90071992547409930"), json.Number("90071992547409931"), "90071992547409930", true},
		{"zero is different from missing", nil, 1, 0, false},
		{"an array is not a measured result", 0, []int{1}, 0, false},
		{"an object is not a measured result", map[string]int{"count": 0}, 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proof := map[string]any{"id": "e-0", "input": input, "verdict": "confirmed", "reason": "Count the foreign-zone jobs passed to enqueue.",
				"before": tc.before, "after": tc.after, "expected": tc.expected, "output_path": path, "output_line": 4, "output_quote": operation,
				"submitted_quote": submitted, "current_quote": current}
			if tc.before == nil {
				delete(proof, "before")
			}
			payload, err := json.Marshal(map[string]any{"checks": []map[string]any{proof}})
			if err != nil {
				t.Fatal(err)
			}
			client := &fakeAgentClient{}
			for _, response := range []string{businessReply(t, c), string(payload)} {
				client.responses = append(client.responses, &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &response}}}})
			}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 16000}})
			g := FileGroup{Diffs: []model.Diff{d}}
			g.business = a.reviewBusinessContext(knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusHistorical}}), g.Diffs)
			a.verifyBusinessFindings(t.Context(), g, "", d.Diff)
			out := a.args.CommentCollector.CommentsForPath(path)
			if client.calls != 2 || (len(out) == 1) != tc.report || len(out) > 0 && (out[0].Category != "bug" || out[0].Severity != "medium" || out[0].StartLine != 2) {
				t.Fatalf("calls=%d, comments=%+v", client.calls, out)
			}
		})
	}
}

func TestChangedReplyIntersectsTheCurrentExecutableEdit(t *testing.T) {
	const path = "fees.py"
	head := knowledge.Citation{StartLine: 1, EndLine: 6}
	old := knowledge.Citation{StartLine: 2, EndLine: 3, Quote: "    rate = quote\n    # Morning rate."}
	current := knowledge.Citation{StartLine: 2, EndLine: 3, Quote: "    rate = abs(quote)\n    # Weekend rate."}
	change := knowledge.ReviewReplyChange{Current: head, Hunks: []knowledge.ReviewReplyHunk{{Submitted: old, Current: current}}}
	comments := "@@ -3 +3 @@\n-    # Evening rate.\n+    # Weekend rate.\n@@ -5 +5 @@\n-    emit(rate)\n+    return rate"
	if got := reviewedReplyChanges(model.Diff{NewPath: path, Diff: comments}, &change); len(got) != 0 {
		t.Fatalf("an old price change was reopened by editing its neighboring comment: %+v", got)
	}
	patch := "@@ -2 +2 @@\n-    rate = quote\n+    rate = abs(quote)"
	got := reviewedReplyChanges(model.Diff{NewPath: path, Diff: patch}, &change)
	if len(got) != 1 || got[0].Current.StartLine != 2 || got[0].Current.EndLine != 2 || got[0].Current.Quote != "    rate = abs(quote)" ||
		strings.Contains(got[0].patch, "Weekend") {
		t.Fatalf("historical lines outside the actual patch can prove a current edit: %+v", got)
	}
	deletion := knowledge.ReviewReplyHunk{Submitted: knowledge.Citation{StartLine: 2, EndLine: 2, Quote: "    rate = quote"}}
	change.Hunks = []knowledge.ReviewReplyHunk{deletion}
	got = reviewedReplyChanges(model.Diff{NewPath: path, Diff: "@@ -2 +1,0 @@\n-    rate = quote"}, &change)
	if len(got) != 1 || !strings.Contains(got[0].patch, "-    rate = quote") {
		t.Fatalf("missing removed executable premise: %+v", got)
	}
	move := knowledge.ReviewReplyHunk{
		Submitted: knowledge.Citation{StartLine: 3, EndLine: 3, Quote: "        audit(order)"},
		Current:   knowledge.Citation{StartLine: 3, EndLine: 3, Quote: "    audit(order)"}, LayoutChanged: true,
	}
	change.Hunks = []knowledge.ReviewReplyHunk{move}
	actual := model.Diff{NewPath: path, Diff: "@@ -3 +3 @@\n-        audit(order)\n+    audit(order)"}
	if moved := reviewedReplyChanges(actual, &change); len(moved) != 1 {
		t.Fatal("lost the exact edit that moved the operation out of its guard")
	}
	change.Hunks[0].LayoutChanged = false
	if cosmetic := reviewedReplyChanges(actual, &change); len(cosmetic) != 0 {
		t.Fatal("reopened a formatting change without a function-level change in control flow")
	}
}
