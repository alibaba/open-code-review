// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestBusinessOverlapDropsOnlyAnAlreadyDiagnosedAddition(t *testing.T) {
	g := routingReviewGroup(knowledge.StatusCurrent)
	group := fileGroupKey(g.Diffs)
	const diagnosed = "Ordinary orders now enter the rush queue instead of the regular queue."
	const additional = "The unconditional call sends a standard order into the express queue."
	check := businessCheck{ID: "express-policy", Path: routingReviewPath, Outcome: "violation", Line: 11, Quote: dispatchQuote,
		SourcePath: routingPolicy, SourceQuote: routingSource, Severity: "high", Reason: additional}
	first := businessReply(t, check)
	proof := map[string]string{
		"pair": "p-0", "existing_trigger": "Ordinary orders", "existing_effect": "enter the rush queue instead of the regular queue",
		"additional_trigger": "a standard order", "additional_effect": "into the express queue",
		"reason": "Both state that the same non-priority order is routed to expedited processing.",
	}
	asJSON := func(rows ...map[string]string) string {
		t.Helper()
		b, err := json.Marshal(map[string]any{"duplicates": rows})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	invalid := map[string]string{}
	for k, v := range proof {
		invalid[k] = v
	}
	invalid["additional_effect"] = "the queue discards the order"
	unknown := asJSON(proof)
	unknown = strings.Replace(unknown, `"pair":"p-0"`, `"pair":"p-1"`, 1)
	for _, tc := range []struct {
		name, draft, reply string
		skip               bool
		want               int
	}{
		{"established same consequence", diagnosed, asJSON(proof), false, 1},
		{"unresolved applicability", "Does this path ever receive an ordinary order? Verify the configured route.", `{"duplicates":[]}`, false, 2},
		{"different failure at same call", "A retry after a disconnect publishes the same rush order twice.", `{"duplicates":[]}`, false, 2},
		{"proof belongs to another original", "Will the caller use this queue when the feature is enabled?", asJSON(proof), false, 2},
		{"invented effect", diagnosed, asJSON(invalid), false, 2},
		{"unknown pair", diagnosed, unknown, false, 2},
		{"inconsistent pair repetitions", diagnosed, asJSON(proof, proof), false, 2},
		{"malformed judgment", diagnosed, "not a judgment", false, 2},
		{"user disables filtering", diagnosed, asJSON(proof), true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &first}}}},
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &tc.reply}}}},
			}}
			a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: 10000}, LLMClient: client, SkipFilter: tc.skip})
			original := model.LlmComment{Path: routingReviewPath, StartLine: 10, EndLine: 12, ExistingCode: dispatchQuote, Content: tc.draft, Severity: "high", Category: "bug"}
			a.args.CommentCollector.Add(original)
			a.verifyBusinessFindings(t.Context(), g, "Check dispatch behavior.", g.Diffs[0].Diff)
			got := a.args.CommentCollector.CommentsForPath(routingReviewPath)
			if len(got) != tc.want || got[0] != original || len(got) == 2 && !strings.Contains(got[1].Content, additional) {
				t.Fatalf("altered original or incorrectly filtered a new finding: %+v", got)
			}
			records := a.session.GetOrCreateFileSession(group).TaskRecords[session.BusinessOverlapTask]
			if tc.skip {
				if len(records) != 0 || client.calls != 1 {
					t.Fatalf("comparison ran with filtering disabled: %d calls", client.calls)
				}
				return
			}
			if len(records) != 1 || client.calls != 2 {
				t.Fatalf("expected one independent comparison: %d records, %d total requests", len(records), client.calls)
			}
			msgs := records[0].RequestMessages
			if len(msgs) != 2 || msgs[0].Role != "system" || !strings.HasPrefix(msgs[0].ExtractText(), "Compare an additional code-review finding with an existing one") ||
				msgs[1].Role != "user" || !strings.Contains(msgs[1].ExtractText(), original.Content) || !strings.Contains(msgs[1].ExtractText(), additional) ||
				strings.Contains(msgs[0].ExtractText(), tc.draft) || strings.Contains(msgs[1].ExtractText(), g.Diffs[0].Diff) {
				t.Fatalf("comparison did not independently receive the two complete findings: %+v", msgs)
			}
		})
	}
}

func TestBusinessOverlapKeepsEveryUncomparedAddition(t *testing.T) {
	newFinding := model.LlmComment{Path: "log.py", StartLine: 4, EndLine: 4, Content: "Publishing an empty batch loses its receipt."}
	original := model.LlmComment{Path: "log.py", StartLine: 4, Content: "An empty publish no longer returns the receipt."}
	for _, tc := range []struct {
		name         string
		scope        string
		line, end    int
		remove       bool
		budget       int64
		contextLimit int
		wantCalls    int
	}{
		{"ordinary was independently withdrawn", "log.py", 4, 0, true, 0, 10000, 0},
		{"other path", "other.py", 4, 4, false, 0, 10000, 0},
		{"adjacent ranges", "log.py", 5, 7, false, 0, 10000, 0},
		{"missing location", "log.py", 0, 7, false, 0, 10000, 0},
		{"invalid range", "log.py", 4, 3, false, 0, 10000, 0},
		{"aggregate budget exhausted", "log.py", 4, 0, false, 1, 10000, 0},
		{"insufficient context", "log.py", 4, 0, false, 0, 10, 0},
		{"independent provider fails", "log.py", 4, 0, false, 0, 10000, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAgentClient{responses: []*llm.ChatResponse{nil}}
			a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: tc.contextLimit}, LLMClient: client, MaxTokensBudget: tc.budget})
			draft := original
			draft.Path, draft.StartLine, draft.EndLine = tc.scope, tc.line, tc.end
			drafts := []model.LlmComment{draft}
			additions := []model.LlmComment{newFinding}
			got := a.uniqueBusinessAdditions(t.Context(), "log.py", additions, drafts, map[int]bool{0: tc.remove})
			if !slices.Equal(got, additions) || got[0] != newFinding || drafts[0] != draft || client.calls != tc.wantCalls {
				t.Fatalf("calls=%d additions=%+v drafts=%+v", client.calls, got, drafts)
			}
		})
	}
}

func TestBusinessOverlapBindsEachProofToItsPair(t *testing.T) {
	const reply = `{"duplicates":[{"pair":"p-0","existing_trigger":"empty batch","existing_effect":"loses the receipt","additional_trigger":"zero items","additional_effect":"drops the confirmation","reason":"Both confirm the identical missing receipt for empty input."},{"pair":"p-1","existing_trigger":"empty batch","existing_effect":"loses the receipt","additional_trigger":"zero items","additional_effect":"drops the confirmation","reason":"Copied from the other pair."}]}`
	answer := reply
	client := &fakeAgentClient{responses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &answer}}}}}}
	a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: 10000}, LLMClient: client})
	cm := func(body string) model.LlmComment {
		return model.LlmComment{Path: "queue.py", StartLine: 8, Content: body}
	}
	ordinary := []model.LlmComment{cm("This original was withdrawn."), cm("An empty batch loses the receipt.")}
	unique := cm("For retries the unconditional enqueue can create two shipments.")
	got := a.uniqueBusinessAdditions(t.Context(), "queue.py", []model.LlmComment{cm("Publishing zero items drops the confirmation."), unique}, ordinary, map[int]bool{0: true})
	rs := a.session.GetOrCreateFileSession("queue.py").TaskRecords[session.BusinessOverlapTask]
	if len(rs) != 1 || len(got) != 1 || got[0] != unique || strings.Contains(rs[0].RequestMessages[1].ExtractText(), ordinary[0].Content) {
		t.Fatalf("proof removed another addition or compared a withdrawn original: %+v", got)
	}
}

func TestBusinessOverlapKeepsAllFindingsBeyondBriefComparison(t *testing.T) {
	a := New(Args{RepoDir: t.TempDir(), Template: template.Template{MaxTokens: 10000}, LLMClient: &fakeAgentClient{}})
	cm := model.LlmComment{Path: "dispatch.py", StartLine: 18, Content: "An empty queue loses its pending receipt."}
	drafts := slices.Repeat([]model.LlmComment{cm}, 13)
	added := []model.LlmComment{{Path: cm.Path, StartLine: cm.StartLine, Content: "Closing the empty queue discards the acknowledgement."}}
	got := a.uniqueBusinessAdditions(t.Context(), cm.Path, added, drafts, nil)
	tasks := a.session.GetOrCreateFileSession(cm.Path).TaskRecords
	if !slices.Equal(got, added) || len(tasks[session.BusinessOverlapTask]) != 0 || len(a.Warnings()) == 0 {
		t.Fatalf("oversized comparison was treated as a confirmed duplicate: %+v", got)
	}
}
