// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestInlineReviewReplyRechecksTheAffectedPassages(t *testing.T) {
	const path = "labels.py"
	const current = "def print_label(parcel):\n    shade = parcel.shade or 'amber'\n    ticket = parcel.ticket\n    return draw(shade, ticket.serial)\n"
	const sourceQuote = "shade = parcel.shade or 'amber'"
	const line = "return draw(shade, ticket.serial)"
	const earlier = "Register the default shade at draw when no shade was supplied, or name the shade in the legend."
	const response = "Would a missing shade ever reach draw? The assignment supplies amber first. The legend could still help."
	const clause = "filling in a missing shade at draw, or "
	const kept = "name the shade in the legend."
	const fix = "return draw(shade or 'amber', ticket.serial)"
	const example = "\n\n**Suggestion:**\n```suggestion\n" + fix + "\n```"
	const archivedExample = "**Suggestion:**\n```suggestion\nreturn draw(default_shade(parcel), ticket.serial)\n```"
	mixed := model.LlmComment{Path: path, ExistingCode: line, StartLine: 4, EndLine: 4, Category: "maintainability", Severity: "low",
		Content: "**Palette:** consider " + clause + kept + example, SuggestionCode: fix}
	separate := model.LlmComment{Path: path, ExistingCode: line, StartLine: 4, Content: "A missing ticket raises an exception when its serial is read."}
	first := businessCheck{ID: "label-reply", Path: path, Line: 4, Quote: line, Outcome: "refuted", Candidate: "c-0",
		PriorClaim: "Register the default shade at draw when no shade was supplied", ReplyQuote: "The assignment supplies amber first.",
		RefutedText: clause, AlsoRefuted: []string{example}, DisputedSuggestion: fix, Trigger: "missing shade", Effect: "filling in",
		SourcePath: path, SourceQuote: sourceQuote, Reason: "The value is replaced before reaching the draw call."}
	readable := first
	readable.JoinWord, readable.JoinForm = "name", "naming"
	reworded := first
	reworded.JoinWord, reworded.JoinForm = "name", "hide"
	unrelated := first
	unrelated.Candidate, unrelated.RefutedText, unrelated.AlsoRefuted, unrelated.DisputedSuggestion = "c-1", separate.Content, nil, ""
	unrelated.Trigger, unrelated.Effect = "missing ticket", "raises an exception"
	overbroad := first
	overbroad.RefutedText, overbroad.AlsoRefuted = mixed.Content, nil
	bare := first
	bare.ReplyQuote = "approved for a future version"
	stale := strings.Replace(current, sourceQuote, "shade = parcel.shade", 1)
	control := first
	control.ReplyQuote = "Does this happen here?"
	same := first
	same.AlsoRefuted = nil
	same.DisputedSuggestion = ""
	same.RefutedText = "A missing shade reaches draw unchanged."
	same.Trigger, same.Effect = "missing shade", "reaches draw unchanged"
	full := mixed
	full.Content, full.SuggestionCode = same.RefutedText, ""
	withoutExample := mixed
	withoutExample.Content, withoutExample.SuggestionCode = strings.TrimSuffix(mixed.Content, example), ""
	publishedOnly := same
	publishedOnly.AlsoRefuted = []string{archivedExample}
	partial := first
	partial.AlsoRefuted, partial.DisputedSuggestion = nil, ""
	publishedPartial := partial
	publishedPartial.AlsoRefuted = []string{archivedExample}
	publishedPartial.JoinWord, publishedPartial.JoinForm = "name", "naming"
	for _, tc := range []struct {
		name, source, reply string
		draft               model.LlmComment
		first, second       []businessCheck
		want                string
		calls               int
	}{
		{"independent useful alternative", current, response, mixed, []businessCheck{first}, []businessCheck{readable, unrelated}, "**Palette:** consider naming the shade in the legend.", 2},
		{"the remaining suggestion cannot be replaced", current, response, mixed, []businessCheck{first}, []businessCheck{reworded}, mixed.Content, 2},
		{"two checks cannot expand a partial finding to the whole", current, response, mixed, []businessCheck{first}, []businessCheck{overbroad}, mixed.Content, 2},
		{"disagreement keeps both clauses", current, response, mixed, []businessCheck{first}, nil, mixed.Content, 2},
		{"complete current contradiction", current, response, full, []businessCheck{same}, []businessCheck{same}, "", 2},
		{"published example absent from the current claim", current, response, full, []businessCheck{publishedOnly}, []businessCheck{same}, "", 2},
		{"independent reviewer also cites a historical example", current, response, withoutExample, []businessCheck{partial}, []businessCheck{publishedPartial}, "**Palette:** consider naming the shade in the legend.", 2},
		{"current suggestion needs two matching decisions", current, response, mixed, []businessCheck{partial}, []businessCheck{readable}, mixed.Content, 2},
		{"old premise no longer holds", stale, response, full, []businessCheck{same}, []businessCheck{same}, full.Content, 1},
		{"a different reply has no quoted reason", current, response, mixed, []businessCheck{bare}, nil, mixed.Content, 1},
		{"acknowledgment has no independent nomination", current, "Thanks, that change belongs in the next release.", mixed, nil, []businessCheck{overbroad}, mixed.Content, 1},
		{"a question alone supplies no verified premise", current, "Does this happen here?", mixed, []businessCheck{control}, nil, mixed.Content, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replies := []string{`{"checks":[]}`, `{"checks":[]}`}
			if len(tc.first) != 0 {
				replies[0] = businessReply(t, tc.first...)
			}
			if len(tc.second) != 0 {
				replies[1] = businessReply(t, tc.second...)
			}
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &replies[0]}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &replies[1]}}}},
			}}
			a := New(Args{RepoDir: t.TempDir(), LLMClient: client, Template: template.Template{MaxTokens: 10000}})
			a.args.CommentCollector.Add(tc.draft)
			a.args.CommentCollector.Add(separate)
			f := knowledge.Fact{ID: first.ID, Scope: path, Source: path, Category: knowledge.CategoryDecision,
				Origin: "review-reply", Authority: knowledge.AuthorityHistorical, ReviewID: "labels-4", Finding: earlier + "\n\n" + archivedExample, Reason: tc.reply,
				ReviewReply: &knowledge.ReviewReply{ID: "comment-2", ReplyTo: "comment-1"},
				Citations:   []knowledge.Citation{{Kind: "code", CurrentPath: path, CurrentStartLine: 4, CurrentEndLine: 4, Quote: line}}}
			g := FileGroup{Diffs: []model.Diff{{NewPath: path, NewFileContent: tc.source, Diff: "@@ -4 +4 @@\n-    return draw(shade)\n+    " + line}}}
			snap := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusHistorical}})
			g.business = a.reviewBusinessContext(snap, g.Diffs)
			a.verifyBusinessFindings(t.Context(), g, "", g.Diffs[0].Diff)
			got := a.args.CommentCollector.CommentsForPath(path)
			if client.calls != tc.calls || len(got) == 0 || got[len(got)-1] != separate {
				t.Fatalf("calls=%d, comments=%+v", client.calls, got)
			}
			if tc.want == "" {
				if len(got) != 1 {
					t.Fatalf("fully refuted finding survived: %+v", got)
				}
			} else {
				want := tc.draft
				want.Content = tc.want
				if tc.want != tc.draft.Content {
					want.SuggestionCode = ""
				}
				if len(got) != 2 || got[0] != want {
					t.Fatalf("want %+v, got %+v", want, got)
				}
			}
			if tc.calls == 2 {
				record := a.session.GetOrCreateFileSession(fileGroupKey(g.Diffs)).TaskRecords[session.BusinessRefutationTask][0]
				if !strings.Contains(record.RequestMessages[0].ExtractText(), "mixed finding") ||
					strings.Contains(record.RequestMessages[0].ExtractText(), "unless you can disprove a complete finding") ||
					!strings.Contains(g.business.text, sourceQuote) || strings.Contains(record.RequestMessages[1].ExtractText(), first.Reason) ||
					strings.Contains(record.RequestMessages[1].ExtractText(), "default_shade(parcel)") {
					t.Fatal("independent review lacks the current guard, presupposes a verdict, or forbids narrowing")
				}
			}
		})
	}
}

func TestInlineReviewReplyProjectsOnlyVerbatimArchivedPassages(t *testing.T) {
	const current = "Keep the ordering in the printed ledger."
	const historical = "Sort before printing.\n\n**Suggestion:**\n```suggestion\nprint(sorted(rows))\n```"
	const archivedCode = "print(sorted(rows))"
	for _, tc := range []struct {
		name, extra, suggested string
		draft                  model.LlmComment
		want                   bool
		wantExtra              int
	}{
		{"historical example in a separate field", archivedCode, archivedCode, model.LlmComment{Content: current}, true, 0},
		{"the same current text still needs two assessments", archivedCode, "", model.LlmComment{Content: current + "\n" + archivedCode}, true, 1},
		{"invented historical text", "print(reverse(rows))", "", model.LlmComment{Content: current}, false, 0},
		{"different current code suggestion", archivedCode, archivedCode, model.LlmComment{Content: current, SuggestionCode: "print(rows)"}, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := businessCheck{RefutedText: current, AlsoRefuted: []string{tc.extra}, DisputedSuggestion: tc.suggested}
			ok := c.projectReplyPassages(historical, tc.draft)
			if ok != tc.want || ok && (len(c.AlsoRefuted) != tc.wantExtra || c.DisputedSuggestion != "") {
				t.Fatalf("projection: ok=%v, current passages=%q, suggestion=%q", ok, c.AlsoRefuted, c.DisputedSuggestion)
			}
		})
	}
}

func TestInlineReviewReplyTargetsMovedAndInsertedLines(t *testing.T) {
	const path = "ledger.py"
	const anchor = "def record(batch):\n    # order 42\n    return store(batch)"
	const current = "def elsewhere():\n    return store(batch)\n\n" + anchor + "\n"
	f := knowledge.Fact{ID: "reply-1", Source: path, Scope: path, Origin: "review-reply", Category: knowledge.CategoryDecision,
		Finding: "Attribute this transfer to calendar order 17.", Reason: "Order 42 defines this transfer.",
		ReviewReply: &knowledge.ReviewReply{SourceQuote: "def record(batch):\n    return store(batch)"},
		Citations:   []knowledge.Citation{{Kind: "code", Path: path, StartLine: 1, EndLine: 2, CurrentPath: path, CurrentStartLine: 4, CurrentEndLine: 6, Quote: anchor}}}
	outside := f
	outside.ID = "unrelated-reply"
	outside.Citations = []knowledge.Citation{{Kind: "code", CurrentPath: path, CurrentStartLine: 1, CurrentEndLine: 1, Quote: "def elsewhere():"}}
	a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: 10000}})
	a.args.CommentCollector.Add(model.LlmComment{Path: path, ExistingCode: "# order 42", Content: "Use order 17 for this transfer."})
	a.args.CommentCollector.Add(model.LlmComment{Path: path, ExistingCode: "return store(batch)", Content: "This code occurs in both functions."})
	change := model.Diff{NewPath: path, NewFileContent: current, Diff: "@@ -4,2 +4,3 @@\n def record(batch):\n+    # order 42\n     return store(batch)"}
	records := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusHistorical}, {Fact: outside, Status: knowledge.StatusHistorical}})
	b := a.reviewBusinessContext(records, []model.Diff{change})
	if len(b.rules) != 1 || len(b.replies) != 1 || !slices.Equal(b.replies[0].Candidates, []reviewReplyCandidate{{ID: "c-0", Start: 5, End: 5}}) ||
		strings.Contains(b.text, outside.ID) || !strings.Contains(b.text, "# order 42") {
		t.Fatalf("current reply targets: %+v", b.replies)
	}
	c := businessCheck{ID: f.ID, Candidate: "c-0", Path: path, Quote: "# order 42", Line: 5, Outcome: "refuted",
		SourcePath: path, SourceQuote: "    # order 42\n    return store(batch)", PriorClaim: "calendar order 17", ReplyQuote: f.Reason,
		RefutedText: "Use order 17 for this transfer.", Trigger: "this transfer", Effect: "Use order 17", Reason: "The current marker names the transfer's governing order."}
	drafts, _ := a.businessDrafts([]model.Diff{change})
	_, proposed, err := groundBusinessChecks(businessReply(t, c), FileGroup{Diffs: []model.Diff{change}, business: b}, drafts)
	if err != nil || len(proposed) != 1 {
		t.Fatalf("the inserted middle line was lost: %v, %+v", err, proposed)
	}
	a.args.SkipFilter = true
	if got := a.reviewBusinessContext(records, []model.Diff{change}); len(got.rules) != 0 {
		t.Fatalf("skip-filter still scheduled a reply-based reassessment: %+v", got)
	}
}

func TestInlineReviewReplyUsesTheActualHunkAndCandidateRange(t *testing.T) {
	const path = "charging.rb"
	const anchor = "    fee = cents.round"
	const checkFee = "    assert_equal(5, fee)"
	const body = "def charge_preview\n" + anchor + "\n" + checkFee + "\nend\narchive(previous)\n"
	const old = "    fee = cents.floor"
	const patch = "@@ -1,4 +1,4 @@\n def charge_preview\n-" + old + "\n+" + anchor + "\n " + checkFee + "\n end\n"
	const earlier = "The preview accepts a fractional fee."
	f := knowledge.Fact{ID: "charge-1", Scope: path, Source: path, Origin: "review-reply", Category: knowledge.CategoryDecision,
		Finding: earlier, Reason: "The preview rounds the fee before checking it.", ReviewReply: &knowledge.ReviewReply{ID: "comment-2"},
		Citations: []knowledge.Citation{{Kind: "test", CurrentPath: path, CurrentStartLine: 2, CurrentEndLine: 2, Quote: anchor}}}
	a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: 10000}})
	a.args.CommentCollector.Add(model.LlmComment{Path: path, ExistingCode: anchor, Content: earlier})
	snapshot := knowledge.ReviewSnapshot([]knowledge.Checked{{Fact: f, Status: knowledge.StatusHistorical}})
	change := model.Diff{NewPath: path, NewFileContent: body, Diff: patch}
	g := FileGroup{Diffs: []model.Diff{change}}
	g.business = a.reviewBusinessContext(snapshot, g.Diffs)
	drafts, _ := a.businessDrafts(g.Diffs)
	c := businessCheck{ID: f.ID, Candidate: "c-0", Path: path, Outcome: "refuted", Line: 3, Quote: anchor,
		PriorClaim: earlier, ReplyQuote: f.Reason, RefutedText: earlier, Trigger: "preview", Effect: "accepts a fractional fee",
		SourcePath: path, SourceQuote: anchor + "\n" + checkFee, Reason: "The preview rounds the fractional input to an integer before the assertion."}
	_, proposed, err := groundBusinessChecks(businessReply(t, c), g, drafts)
	if err != nil || len(proposed) != 1 || proposed[0].Line != 2 || !strings.Contains(g.business.text, `"current_start_line":2`) {
		t.Fatalf("unique current quote and its adjacent witness were lost: %+v, %v", proposed, err)
	}
	block := c
	block.Line, block.Quote = 2, anchor+"\n"+checkFee
	if l := g.business.checkLocation(&block, reviewedLines(g.Diffs)[path]); l != 0 {
		t.Fatalf("a quote extending beyond the candidate located a finding at %d", l)
	}
	g.business.replies[0].Candidates[0].End = 3
	if l := g.business.checkLocation(&block, reviewedLines(g.Diffs)[path]); l != 2 || block.Quote != strings.TrimSpace(anchor) {
		t.Fatalf("verbatim current multiline candidate: line=%d, quote=%q", l, block.Quote)
	}
	block.Quote = anchor + "\n    print(fee)"
	if l := g.business.checkLocation(&block, reviewedLines(g.Diffs)[path]); l != 0 {
		t.Fatalf("a partly invented block located a finding at %d", l)
	}
	g.business.replies[0].Candidates[0].End = 2
	// The same numerical hint explicitly selecting a duplicate, or two possible
	// in-range occurrences, cannot relocate a check to a different operation.
	duplicate := []changedLine{{number: 2, text: strings.TrimSpace(anchor)}, {number: 3, text: strings.TrimSpace(anchor)}}
	c.Line = g.business.checkLocation(&c, duplicate)
	if _, _, ok := g.business.replyAmendment(g.business.rules[0], &c, drafts, g.Diffs); ok {
		t.Fatal("explicitly quoted different occurrence reached a withdrawal")
	}
	g.business.replies[0].Candidates[0].End = 3
	c.Line = 1
	if got := g.business.checkLocation(&c, duplicate); got != 0 {
		t.Fatalf("ambiguous current quote resolved to %d", got)
	}
	change.NewFileContent = strings.Replace(body, checkFee, "    assert_positive(fee)", 1)
	g.Diffs[0] = change
	g.business = a.reviewBusinessContext(snapshot, g.Diffs)
	_, proposed, _ = groundBusinessChecks(businessReply(t, c), g, drafts)
	if len(proposed) != 0 {
		t.Fatal("stale patch text was accepted as a current assertion")
	}
}
