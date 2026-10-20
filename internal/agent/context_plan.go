// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/chunk"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
	"github.com/alibaba/open-code-review/internal/tool"
)

// A group's change context reaches the model one of two ways.
//
// Below the request budget it is inlined in the first user message, exactly as
// it always was: one file element per file, nothing to fetch, nothing to track.
// Above it, the same text is replaced by a manifest - one bounded row per chunk
// of context - and the model reads the diff on demand through file_read_diff.
//
// The threshold is the point of the whole design, and it is deliberately not a
// round number: the two paths differ only when the inline diff would not fit,
// so every review that fits today behaves byte-for-byte as it did before.

// reviewContext is the change context of one group as it appears in the first
// user message, plus the accounting needed to close the loop afterwards.
type reviewContext struct {
	// Text is what replaces {{diffs}} in the prompt: the inline diff, or the
	// manifest with its reading instructions.
	Text string
	// Sharded reports whether Text is a manifest. It is false on the common
	// path, and no coverage bookkeeping happens for a group that is not.
	Sharded bool
	// Budget is the budget the context was planned against.
	Budget chunk.Budget
}

// manifestPreamble explains the manifest contract to the model. It travels with
// the substituted value rather than living in a prompt template, so a run with
// a custom MAIN_TASK template gets it too - and so a run that inlines its diff
// does not carry instructions about a manifest it never received.
const manifestPreamble = `The change context below is a manifest, not the diff itself. The raw diff
of each file is served on demand, one bounded chunk at a time.

- Read a chunk: file_read_diff {"chunk_id": "<id>"}.
- The chunk ids and line ranges of one file: file_read_diff {"path": "<path>"}.
- Give every chunk a pass. A chunk you deliberately skip must be declared in
  task_done {"skipped_chunks": [{"chunk_id": "<id>", "reason": "<cause>"}]}.
- A chunk you neither read nor declared is reported as uninspected.
- To re-read a chunk you already have, call it again; the repeat is recorded.

`

// contextBudget derives the run's context budget from the request limit, the
// measured fixed overhead of the rendered prompt, and the configured completion
// ceiling. Nothing here is a constant that could drift away from the
// configuration: changing --max-tokens moves all three terms.
func (a *Agent) contextBudget(fixedOverhead int) chunk.Budget {
	return chunk.DeriveBudget(
		llmloop.PromptTokenLimit(a.args.Template.MaxTokens),
		fixedOverhead,
		a.args.Template.CompletionTokenLimit(),
	)
}

// measureFixedOverhead renders the group's prompt with an empty change context
// and returns what it costs. Measuring beats assuming: the system prompt, the
// checklist, the plan and the tool definitions all live here, and a template
// edit must move the number rather than silently eat the chunk budget.
func (a *Agent) measureFixedOverhead(rule, changeFiles, plan, confirmed string) int {
	empty := a.buildMainTaskMessages(rule, changeFiles, "", plan, confirmed)
	return llmloop.CountMessagesTokens(empty)
}

// planReviewContext decides how a group's change context is delivered.
func (a *Agent) planReviewContext(ctx context.Context, groupKey string, diffs []model.Diff, budget chunk.Budget) reviewContext {
	inline := buildConcatenatedDiffs(diffs)

	if !budget.Usable() {
		// The prompt's fixed overhead alone exhausts the request limit. Sharding
		// cannot fix that - a chunk needs room too - and dropping content is
		// not an option, so the inline path stays and the run reports why.
		msg := fmt.Sprintf("context budget exhausted: request limit %d leaves no room below %d fixed prompt tokens",
			budget.RequestLimit, budget.FixedOverhead)
		fmt.Fprintf(stdout.Writer(), "[ocr] %s for group %q; diff inlined unchanged\n", msg, groupKey)
		a.recordWarning("context_budget_exhausted", groupKey, msg)
		return reviewContext{Text: inline, Budget: budget}
	}

	if llm.CountTokens(inline) <= budget.Chunk() {
		return reviewContext{Text: inline, Budget: budget}
	}

	var chunks []chunk.Chunk
	for _, d := range diffs {
		cs := chunk.Split(d.NewPath, d.Diff, chunk.Options{Budget: budget.Chunk()})
		a.chunks.Add(groupKey, d.NewPath, cs)
		chunks = append(chunks, cs...)
	}
	m := chunk.NewManifest(groupKey, chunks, a.chunks.PackCount(groupKey))

	// Tighten the shared read ceiling to this group's budget before its main
	// loop can issue a read.
	a.lowerReadBudget(budget)

	fmt.Fprintf(stdout.Writer(), "[ocr] Sharded group %q into %d chunk(s) across %d pack(s) (budget %d tokens)\n",
		groupKey, len(chunks), m.PackCount, budget.Chunk())
	telemetry.Event(ctx, "context.sharded",
		telemetry.AnyToAttr("group.label", groupKey),
		telemetry.AnyToAttr("chunk.count", len(chunks)),
		telemetry.AnyToAttr("chunk.pack_count", m.PackCount),
		telemetry.AnyToAttr("chunk.budget", budget.Chunk()),
		telemetry.AnyToAttr("file.count", len(m.Files)))

	return reviewContext{
		Text:    manifestPreamble + chunk.RenderManifest(m, budget.Manifest()),
		Sharded: true,
		Budget:  budget,
	}
}

// lowerReadBudget pushes a group's read ceiling down to the shared read tool.
// The tool only ever tightens, so a group reading with a budget at least as
// large as its own; see FileReadDiffProvider.LowerReadBudget.
func (a *Agent) lowerReadBudget(budget chunk.Budget) {
	if !budget.Usable() {
		return
	}
	if p, ok := a.args.Tools.Get(tool.FileReadDiff.Name()); ok {
		if frd, ok := p.(*tool.FileReadDiffProvider); ok {
			frd.LowerReadBudget(budget.Read())
		}
	}
}

// finalizeContextCoverage closes the loop on a sharded group: it folds the
// group's context accounting into the run and reports what was never read.
//
// An uninspected chunk is reported, never failed. The model chooses the order
// it reads context in and may legitimately stop early on a round budget; a
// chunk nobody opened is a fact about coverage, and the honest place for that
// fact is the run's own record rather than a file marked failed on a guess.
func (a *Agent) finalizeContextCoverage(ctx context.Context, groupKey string, rc reviewContext, folded *bool) {
	if !rc.Sharded || folded == nil || *folded {
		return
	}
	*folded = true

	st := a.chunks.GroupStats(groupKey)
	a.runner.RecordChunkStats(llmloop.ChunkRunStats{
		UniqueChunks: int64(st.UniqueChunks),
		FetchCount:   int64(st.FetchCount),
		RefetchCount: int64(st.RefetchCount),
		ResendTokens: int64(st.ResendTokens),
	})

	telemetry.Event(ctx, "context.accounted",
		telemetry.AnyToAttr("group.label", groupKey),
		telemetry.AnyToAttr("chunk.unique", st.UniqueChunks),
		telemetry.AnyToAttr("chunk.fetch_count", st.FetchCount),
		telemetry.AnyToAttr("chunk.refetch_count", st.RefetchCount),
		telemetry.AnyToAttr("chunk.uninspected", len(st.Uninspected)))

	if len(st.Uninspected) == 0 {
		return
	}
	msg := fmt.Sprintf("%d of %d chunk(s) were never read (first: %s)",
		len(st.Uninspected), st.UniqueChunks, st.Uninspected[0])
	fmt.Fprintf(stdout.Writer(), "[ocr] WARNING: %s for group %q\n", msg, groupKey)
	a.recordWarning("context_chunks_uninspected", groupKey, msg)
}

// reviewFilterContext is the change context handed to the review filter.
//
// When the group was sharded, the filter does not get the whole change again:
// it gets the chunks its candidate comments point at, bounded the same way. A
// filter verdict is a judgement about specific comments, so the diffs of files
// no candidate touches are context it never needed - and re-sending them is
// exactly the replay this feature exists to stop. Anything it does need beyond
// that it can re-read by chunk id.
func (a *Agent) reviewFilterContext(groupKey string, g FileGroup, paths []string, budget chunk.Budget) string {
	inline := buildConcatenatedDiffs(g.Diffs)
	if !budget.Usable() || !a.groupSharded(groupKey) {
		return inline
	}

	var b strings.Builder
	b.WriteString("Only the chunks referenced by the candidate comments follow. Read any further context with file_read_diff {\"chunk_id\": \"...\"}.\n\n")
	b.WriteString(a.chunks.RenderPaths(paths, budget.Read()))
	return b.String()
}

// groupSharded reports whether a group was delivered as a manifest.
func (a *Agent) groupSharded(groupKey string) bool {
	return len(a.chunks.ChunksOfGroup(groupKey)) > 0
}

// markChunksSkipped closes the coverage of the chunks a model declared skipped
// at task_done. Unknown ids are ignored rather than reported: the model was
// reasoning over ids it was shown, and a typo must not become a coverage
// failure.
func (a *Agent) markChunksSkipped(groupKey string, skipped []llmloop.SkippedChunk) {
	if len(skipped) == 0 {
		return
	}
	byCause := make(map[string][]string, len(skipped))
	var order []string
	for _, s := range skipped {
		cause := strings.TrimSpace(s.Reason)
		if cause == "" {
			cause = "no cause given"
		}
		if _, seen := byCause[cause]; !seen {
			order = append(order, cause)
		}
		byCause[cause] = append(byCause[cause], s.ID)
	}
	for _, cause := range order {
		a.chunks.MarkSkippedInGroup(groupKey, byCause[cause], cause)
	}
}
