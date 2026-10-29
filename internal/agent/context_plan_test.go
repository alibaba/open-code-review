// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/chunk"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

func agentDiff(path, diff string) model.Diff {
	return model.Diff{NewPath: path, OldPath: path, Diff: diff}
}

// sizedDiff builds a diff of roughly the requested token size.
func sizedDiff(path string, lines int) model.Diff {
	var b strings.Builder
	b.WriteString("diff --git a/" + path + " b/" + path + "\n")
	b.WriteString("@@ -1,1 +1,1 @@\n")
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "+line %d of %s with payload text\n", i, path)
	}
	return agentDiff(path, b.String())
}

func newContextAgent(t *testing.T) *Agent {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Register(tool.NewFileReadDiff(tool.NewDiffMap(nil)))
	// A prompt ceiling small enough that the fixtures below actually straddle
	// the sharding threshold: the point of the feature is the boundary, and a
	// test that only ever exercises one side of it proves nothing.
	return New(Args{Tools: reg, Template: template.Template{MaxTokens: 6000, MaxCompletionTokens: 2000}})
}

func TestPlanReviewContext_SmallDiffStaysInline(t *testing.T) {
	a := newContextAgent(t)
	diffs := []model.Diff{sizedDiff("a.go", 5)}
	budget := a.contextBudget(500)

	rc := a.planReviewContext(context.Background(), "g1", diffs, budget)

	if rc.Sharded {
		t.Fatal("a change that fits the budget must not be sharded")
	}
	if !strings.Contains(rc.Text, `<file path="a.go">`) {
		t.Errorf("the inline path must be byte-for-byte what it always was: %q", rc.Text)
	}
	if len(a.chunks.ChunksOfGroup("g1")) != 0 {
		t.Error("an inline group must register no chunk: there is no manifest to account for")
	}
}

func TestPlanReviewContext_BigChangeBecomesAManifest(t *testing.T) {
	a := newContextAgent(t)
	diffs := []model.Diff{sizedDiff("a.go", 400), sizedDiff("b.go", 300)}
	budget := a.contextBudget(500)

	rc := a.planReviewContext(context.Background(), "g1", diffs, budget)

	if !rc.Sharded {
		t.Fatal("a change over the budget must be sharded")
	}
	if !strings.Contains(rc.Text, "<context_manifest") {
		t.Errorf("the first message must carry a manifest: %q", rc.Text[:min(200, len(rc.Text))])
	}
	for _, d := range diffs {
		if strings.Contains(rc.Text, "line 0 of "+d.NewPath) {
			t.Errorf("raw content of %s must not live in the frozen prefix", d.NewPath)
		}
	}
	if !strings.Contains(rc.Text, "file_read_diff") {
		t.Error("the manifest must tell the model how to read what it names")
	}
	if got := len(a.chunks.ChunksOfGroup("g1")); got < 2 {
		t.Errorf("expected the change to be cut into several units, got %d", got)
	}
}

func TestPlanReviewContext_ManifestIsDeterministic(t *testing.T) {
	diffs := []model.Diff{sizedDiff("a.go", 400)}

	first := newContextAgent(t)
	second := newContextAgent(t)
	b1 := first.contextBudget(500)
	b2 := second.contextBudget(500)

	r1 := first.planReviewContext(context.Background(), "g1", diffs, b1)
	r2 := second.planReviewContext(context.Background(), "g1", diffs, b2)
	if r1.Text != r2.Text {
		t.Error("the same change must produce a byte-identical manifest")
	}
}

func TestPlanReviewContext_UnusableBudgetKeepsTheDiffAndSaysWhy(t *testing.T) {
	a := newContextAgent(t)
	diffs := []model.Diff{sizedDiff("a.go", 400)}
	// A request limit the fixed prompt already consumes.
	budget := chunk.DeriveBudget(1000, 990, 500)

	rc := a.planReviewContext(context.Background(), "g1", diffs, budget)

	if rc.Sharded {
		t.Error("with no room for a chunk, sharding cannot help and must not pretend to")
	}
	if !strings.Contains(rc.Text, "line 0 of a.go") {
		t.Error("the diff must still be delivered: the invariant is never to drop content")
	}
	if !hasWarning(a.Warnings(), "context_budget_exhausted") {
		t.Errorf("a run that cannot shard must report it, warnings = %+v", a.Warnings())
	}
}

func TestFinalizeContextCoverage_ReportsUninspectedChunks(t *testing.T) {
	a := newContextAgent(t)
	diffs := []model.Diff{sizedDiff("a.go", 400)}
	rc := a.planReviewContext(context.Background(), "g1", diffs, a.contextBudget(500))
	chunks := a.chunks.ChunksOfGroup("g1")
	if len(chunks) < 2 {
		t.Fatalf("fixture should split, got %d", len(chunks))
	}
	a.chunks.RecordFetch(chunks[0].ID)

	folded := false
	a.finalizeContextCoverage(context.Background(), "g1", rc, &folded)
	if !folded {
		t.Error("the group must be marked as accounted for")
	}
	if !hasWarning(a.Warnings(), "context_chunks_uninspected") {
		t.Errorf("a chunk nobody read must be reported, warnings = %+v", a.Warnings())
	}
	st := runnerStats(t, a)
	if st.UniqueContextChunks != int64(len(chunks)) {
		t.Errorf("UniqueContextChunks = %d, want %d", st.UniqueContextChunks, len(chunks))
	}
	if st.ChunkFetchCount != 1 {
		t.Errorf("ChunkFetchCount = %d, want 1", st.ChunkFetchCount)
	}

	// Folding twice would double-count a group.
	a.finalizeContextCoverage(context.Background(), "g1", rc, &folded)
	if got := runnerStats(t, a).UniqueContextChunks; got != int64(len(chunks)) {
		t.Errorf("UniqueContextChunks = %d after a second fold, want %d", got, len(chunks))
	}
}

func TestMarkChunksSkipped_ClosesTheLoopOnADeclaredSkip(t *testing.T) {
	a := newContextAgent(t)
	diffs := []model.Diff{sizedDiff("a.go", 400)}
	rc := a.planReviewContext(context.Background(), "g1", diffs, a.contextBudget(500))
	chunks := a.chunks.ChunksOfGroup("g1")

	if len(chunks) < 3 {
		t.Fatalf("fixture should split into at least 3, got %d", len(chunks))
	}
	// One chunk read, the rest declared skipped with a cause: a model that says
	// what it left alone must not be reported as having left something unread.
	a.chunks.RecordFetch(chunks[0].ID)
	declared := make([]llmloop.SkippedChunk, 0, len(chunks)-1)
	for _, c := range chunks[1:] {
		declared = append(declared, llmloop.SkippedChunk{ID: c.ID, Reason: "generated code"})
	}
	a.markChunksSkipped("g1", declared)

	folded := false
	a.finalizeContextCoverage(context.Background(), "g1", rc, &folded)
	if hasWarning(a.Warnings(), "context_chunks_uninspected") {
		t.Errorf("every chunk was read or declared: warnings = %+v", a.Warnings())
	}
}

func TestReviewFilterContext_DoesNotReTransmitTheWholeChange(t *testing.T) {
	// A sharded group's filter is shown the chunks its candidates point at, not
	// the whole change again: re-sending it is the replay this feature exists
	// to stop.
	a := newContextAgent(t)
	group := FileGroup{Label: "g1", Diffs: []model.Diff{sizedDiff("a.go", 300), sizedDiff("b.go", 300)}}
	a.planReviewContext(context.Background(), "g1", group.Diffs, a.contextBudget(500))
	budget := a.contextBudget(500)

	got := a.reviewFilterContext("g1", group, []string{"a.go"}, budget)
	if strings.Contains(got, "FILE: b.go") {
		t.Error("the filter must not receive a file no candidate comment points at")
	}
	if !strings.Contains(got, "FILE: a.go") {
		t.Errorf("the filter must still receive the file its comments are about: %q", got)
	}
	if !strings.Contains(got, "chunk_id") {
		t.Error("the filter must be told it can re-read anything else by chunk id")
	}
}

func TestReviewFilterContext_UnshardedGroupIsUnchanged(t *testing.T) {
	a := newContextAgent(t)
	group := FileGroup{Label: "g1", Diffs: []model.Diff{sizedDiff("a.go", 5)}}

	got := a.reviewFilterContext("g1", group, []string{"a.go"}, a.contextBudget(500))
	if !strings.Contains(got, `<file path="a.go">`) {
		t.Errorf("an unsharded group must reach the filter exactly as before: %q", got)
	}
}

func TestContextBudget_FollowsTheConfiguredCeiling(t *testing.T) {
	a := newContextAgent(t)
	a.args.Template.MaxTokens = 200000
	big := a.contextBudget(4000)
	a.args.Template.MaxTokens = 26000
	small := a.contextBudget(4000)

	if big.Chunk() <= small.Chunk() {
		t.Errorf("the chunk budget must follow --max-tokens: %d vs %d", big.Chunk(), small.Chunk())
	}
}

func TestMeasureFixedOverhead_ScalesWithThePrompt(t *testing.T) {
	a := newContextAgent(t)
	a.args.Template.MainTask = templateConversation("short {{diffs}}")
	short := a.measureFixedOverhead("rule", "files", "", "")

	a.args.Template.MainTask = templateConversation("short {{diffs}} " + strings.Repeat("extra checklist text ", 200))
	long := a.measureFixedOverhead("rule", "files", "", "")

	if long <= short {
		t.Errorf("a bigger prompt must measure bigger: %d vs %d", long, short)
	}
}

func templateConversation(content string) template.LlmConversation {
	return template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: content}}}
}

func hasWarning(ws []AgentWarning, kind string) bool {
	for _, w := range ws {
		if w.Type == kind {
			return true
		}
	}
	return false
}

func runnerStats(t *testing.T, a *Agent) llmloop.RunContextStats {
	t.Helper()
	if a.runner == nil {
		t.Fatal("agent has no runner")
	}
	return a.runner.ContextStats()
}

func TestPlanReviewContext_TheAssembledRequestStaysUnderTheDerivedBound(t *testing.T) {
	// The end-to-end bound the whole design rests on: after sharding, the first
	// request - system prompt, manifest, whatever else the template carries -
	// must fit the provider ceiling with the reserves left intact. Sharding that
	// merely moves the overflow from one field to another has failed.
	a := newContextAgent(t)
	a.args.Template.MainTask = templateConversation(
		"system {{system_rule}} change files {{change_files}}\n{{diffs}}\nplan {{plan_guidance}}")

	diffs := []model.Diff{sizedDiff("a.go", 900), sizedDiff("b.go", 700), sizedDiff("c.go", 500)}
	budget := a.contextBudget(a.measureFixedOverhead("a long system rule", "a.go\nb.go\nc.go", "a plan", ""))

	rc := a.planReviewContext(context.Background(), "g1", diffs, budget)
	if !rc.Sharded {
		t.Fatal("fixture should straddle the threshold")
	}

	messages := a.buildMainTaskMessages("a long system rule", "a.go\nb.go\nc.go", rc.Text, "a plan", "")
	raw := llmloop.CountMessagesTokens(messages)

	// request_limit = fixed_overhead + the assembled context must leave the
	// conversation and output reserves intact.
	reserves := llmloop.PromptTokenLimit(a.args.Template.MaxTokens) - budget.FixedOverhead - budget.Chunk()
	if raw > budget.FixedOverhead+budget.Chunk() {
		t.Errorf("assembled request is %d tokens, above the %d the derived budget allows for it",
			raw, budget.FixedOverhead+budget.Chunk())
	}
	if raw >= llmloop.PromptTokenLimit(a.args.Template.MaxTokens)-reserves {
		t.Errorf("assembled request %d has eaten into the %d tokens reserved for the conversation and the reply", raw, reserves)
	}

	// And it is far smaller than the change it replaced: that is the saving.
	inline := buildConcatenatedDiffs(diffs)
	if got, want := raw*4, llm.CountTokens(inline); got >= want {
		t.Errorf("sharded prompt is %d tokens against an inline %d: no saving was made", got, want)
	}
}
