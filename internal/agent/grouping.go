// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/telemetry"
)

const maxFilesPerGroup = 10

// smallChangeSetLabel labels the single group a below-threshold change set is
// bundled into. Unlike an LLM-produced label it carries no semantics, because
// no partition was computed: every file simply went in together.
const smallChangeSetLabel = "small change set"

// FileGroup is a set of semantically related diffs to be reviewed in one LLM call.
type FileGroup struct {
	Label string
	Diffs []model.Diff
}

// FileGroupInfo is the exported, JSON-friendly representation of a file group.
type FileGroupInfo struct {
	Label string   `json:"label"`
	Files []string `json:"files"`
}

type groupingResponse struct {
	Label string   `json:"label"`
	Files []string `json:"files"`
}

// groupDiffsResult holds the grouping output and any LLM usage to record.
//
// failure is set only when an attempted LLM partition failed; the groups beside
// it are then the per-file fallback. The paths that decide a partition without a
// round trip leave it nil, so a caller acting on it cannot mistake a deliberate
// skip for a broken one. What to do about a failure is the caller's policy, not
// this function's: it always returns a usable partition.
type groupDiffsResult struct {
	groups  []FileGroup
	usage   *llm.UsageInfo
	failure error
}

// groupingSessionOpts carries optional session-recording context.
// When non-nil, callGroupingLLM writes an LLM request/response record into the
// session history so the grouping call is visible in retry reports and viewers.
type groupingSessionOpts struct {
	session  *session.SessionHistory
	provider string
	model    string
}

// groupDiffs calls the LLM with file metadata (no diff content) to produce
// semantic groups. Falls back to one-file-per-group on any error.
func groupDiffs(ctx context.Context, diffs []model.Diff, client llm.LLMClient, modelName string, tpl template.Template, tokenLimit int, sessOpts *groupingSessionOpts) groupDiffsResult {
	if len(diffs) <= 1 {
		// Nothing to partition, whatever the thresholds say. This short-circuit is
		// unconditional and must stay ahead of GroupingPlan, which returns
		// GroupingViaLLM once GroupingMinFiles is 0 — that would send a grouping
		// call for a single file. Keeping it here also keeps the group's label as
		// the file path rather than smallChangeSetLabel.
		//
		// The skip is still reported: someone counting skipped groupings must not
		// have to know about a hidden path. Telemetry only, though — it is
		// aggregated and has to be complete, whereas a reader of the terminal does
		// not need telling that one file needs no partition.
		if len(diffs) == 1 {
			totalChanged, _ := diffsChurn(diffs)
			emitGroupingSkipped(ctx, template.GroupingPerFile, len(diffs), totalChanged, tpl)
		}
		return groupDiffsResult{groups: toSingleFileGroups(diffs)}
	}

	// A small change set decides its partition without an LLM round trip: too
	// few files for the call to buy any information. Whether they can then share
	// one review is a separate question that churn answers. See
	// Template.GroupingPlan.
	totalChanged, _ := diffsChurn(diffs)
	if strategy := tpl.GroupingPlan(len(diffs), totalChanged); strategy != template.GroupingViaLLM {
		return groupDiffsResult{groups: groupWithoutLLM(ctx, diffs, strategy, tpl, totalChanged, tokenLimit)}
	}

	if tpl.GroupingTask == nil || len(tpl.GroupingTask.Messages) == 0 {
		return groupDiffsResult{groups: toSingleFileGroups(diffs)}
	}

	groups, usage, err := callGroupingLLM(ctx, diffs, client, modelName, tpl.GroupingTask, tpl.CompletionTokenLimit(), sessOpts)
	if err != nil {
		// The line stops at what happened: the caller decides, and prints,
		// whatever follows. It is not the only record either — the write goes to
		// io.Discard under --audience agent, so the failure also rides out on the
		// telemetry event and on the failure field below.
		fmt.Fprintf(stdout.Writer(), "[ocr] LLM grouping failed: %v\n", err)
		emitGroupingFailed(ctx, len(diffs), totalChanged, tpl, err)
		return groupDiffsResult{groups: toSingleFileGroups(diffs), usage: usage, failure: err}
	}

	groups = enforceGroupTokenBudget(groups, tokenLimit)
	return groupDiffsResult{groups: groups, usage: usage}
}

// groupWithoutLLM partitions a below-threshold change set locally, in the shape
// GroupingPlan chose. Both shapes report the file count and churn that drove
// the decision but not the thresholds, which ride on the telemetry event
// instead: a threshold of 0 disables its step, and there is then no meaningful
// comparison for the log line to claim.
func groupWithoutLLM(ctx context.Context, diffs []model.Diff, strategy template.GroupingStrategy, tpl template.Template, totalChanged int64, tokenLimit int) []FileGroup {
	groups := toSingleFileGroups(diffs)
	if strategy == template.GroupingBundleAll {
		// enforceMaxFilesPerGroup guards a GroupingMinFiles raised past
		// maxFilesPerGroup. enforceGroupTokenBudget is the prompt-size valve: a
		// bundle that outgrows the limit degrades to per-file groups, which is
		// exactly the shape a grouping failure already falls back to.
		//
		// A known cost of bundling: the --max-tokens-budget look-ahead in
		// dispatchSubtasks is per group, so a bundle makes it all-or-nothing over
		// the whole change set — a rejected bundle covers no file at all, where
		// per-file groups would have covered as many as the budget allowed. It
		// also over-charges the bundle: that estimate sums estimateDiffFileTokens,
		// which pays the fixed prompt overhead once per file, while a bundle's
		// files share one conversation.
		groups = enforceMaxFilesPerGroup([]FileGroup{{Label: smallChangeSetLabel, Diffs: diffs}})
		groups = enforceGroupTokenBudget(groups, tokenLimit)
	}

	// The log describes the partition that came out, not the one GroupingPlan
	// asked for: a bundle that trips a size valve above ends up as several
	// groups, and a line still claiming "one group" would misdirect anyone
	// tracing why more than one review call appeared. Telemetry keeps reporting
	// the decision, since that is what the thresholds beside it explain.
	var shape string
	switch {
	case len(groups) == 1:
		shape = "reviewing as one group"
	case len(groups) == len(diffs):
		shape = "reviewing per file"
	default:
		shape = fmt.Sprintf("reviewing as %d groups", len(groups))
	}
	fmt.Fprintf(stdout.Writer(), "[ocr] Skipping LLM grouping for %d file(s), %d changed line(s) — %s\n",
		len(diffs), totalChanged, shape)
	emitGroupingSkipped(ctx, strategy, len(diffs), totalChanged, tpl)

	return groups
}

// emitGroupingSkipped records a partition that was decided without an LLM round
// trip. Both skip paths report through here so their attribute sets cannot drift
// apart.
//
// The count rides on file.count, not group.file_count: the event fires before any
// FileGroup exists, so it describes the change set the way review.started does,
// rather than the group-scoped spans that pair group.file_count with a
// group.label there is none of here.
func emitGroupingSkipped(ctx context.Context, strategy template.GroupingStrategy, fileCount int, totalChanged int64, tpl template.Template) {
	telemetry.Event(ctx, "grouping.skipped",
		telemetry.AnyToAttr("strategy", strategy.String()),
		telemetry.AnyToAttr("file.count", fileCount),
		telemetry.AnyToAttr("lines.changed", totalChanged),
		telemetry.AnyToAttr("threshold.files", tpl.GroupingMinFiles),
		telemetry.AnyToAttr("threshold.lines", tpl.GroupingBundleLineThreshold))
}

// emitGroupingFailed records a partition the LLM was asked for and could not
// deliver. It carries emitGroupingSkipped's attribute set so the two are
// comparable, with strategy pinned to the one that was attempted.
//
// ErrorEvent rather than Event: a failed grouping is an error, and only
// ErrorEvent sets the span status and calls RecordError, which is what makes it
// findable in a trace backend without knowing the event name.
func emitGroupingFailed(ctx context.Context, fileCount int, totalChanged int64, tpl template.Template, err error) {
	telemetry.ErrorEvent(ctx, "grouping.failed", err,
		telemetry.AnyToAttr("strategy", template.GroupingViaLLM.String()),
		telemetry.AnyToAttr("file.count", fileCount),
		telemetry.AnyToAttr("lines.changed", totalChanged),
		telemetry.AnyToAttr("threshold.files", tpl.GroupingMinFiles),
		telemetry.AnyToAttr("threshold.lines", tpl.GroupingBundleLineThreshold))
}

func callGroupingLLM(ctx context.Context, diffs []model.Diff, client llm.LLMClient, modelName string, task *template.LlmConversation, maxTokens int, sessOpts *groupingSessionOpts) (groups []FileGroup, usage *llm.UsageInfo, err error) {
	var rec *session.TaskRecord
	startTime := time.Now()
	defer func() {
		if r := recover(); r != nil {
			groups = nil
			err = fmt.Errorf("grouping LLM panicked: %v", r)
			if rec != nil {
				rec.Response = nil
				rec.SetError(err, time.Since(startTime))
			}
		}
	}()

	fileList := buildFileList(diffs)

	messages := make([]llm.Message, 0, len(task.Messages))
	for _, m := range task.Messages {
		content := strings.ReplaceAll(m.Content, "{{file_list}}", fileList)
		messages = append(messages, llm.NewTextMessage(m.Role, content))
	}

	const groupingFileKey = "__grouping__"

	if sessOpts != nil && sessOpts.session != nil {
		fs := sessOpts.session.GetOrCreateFileSession(groupingFileKey)
		rec = fs.AppendTaskRecord(session.GroupingTask, messages)
		ctx = llm.ContextWithSessionKey(ctx,
			llm.SessionTaskKey(sessOpts.session.SessionID, string(session.GroupingTask), groupingFileKey))
		ctx = llm.WithRequestMeta(ctx, llm.RequestMeta{
			Provider:  sessOpts.provider,
			Model:     sessOpts.model,
			FilePath:  groupingFileKey,
			TaskType:  string(session.GroupingTask),
			RequestNo: rec.RequestNo,
		})
	}

	if maxTokens <= 0 {
		maxTokens = 4096
	}

	resp, err := client.CompletionsWithCtx(ctx, llm.ChatRequest{
		Model:     modelName,
		Messages:  messages,
		MaxTokens: maxTokens,
	})
	duration := time.Since(startTime)

	if err != nil {
		if rec != nil {
			rec.SetError(err, duration)
		}
		return nil, nil, fmt.Errorf("grouping LLM call: %w", err)
	}

	usage = resp.Usage

	content := resp.Content()
	if content == "" {
		if rec != nil {
			rec.SetError(fmt.Errorf("grouping LLM returned empty response"), duration)
		}
		return nil, usage, fmt.Errorf("grouping LLM returned empty response")
	}

	groups, err = parseGroupingResponse(content, diffs)
	if rec != nil {
		if err != nil {
			rec.SetError(fmt.Errorf("grouping response parse failed: %w", err), duration)
		} else {
			rec.SetResponse(resp, duration)
		}
	}
	return groups, usage, err
}

func buildFileList(diffs []model.Diff) string {
	var sb strings.Builder
	for _, d := range diffs {
		sb.WriteString(formatDiffEntry(d))
		sb.WriteString("\n")
	}
	return sb.String()
}

func parseGroupingResponse(content string, diffs []model.Diff) ([]FileGroup, error) {
	content = strings.TrimSpace(content)
	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		content = strings.Join(lines, "\n")
	}

	var resp []groupingResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parse grouping JSON: %w", err)
	}

	diffByPath := make(map[string]model.Diff, len(diffs))
	for _, d := range diffs {
		diffByPath[d.NewPath] = d
	}

	seen := make(map[string]bool, len(diffs))
	var groups []FileGroup

	for _, g := range resp {
		var gDiffs []model.Diff
		for _, f := range g.Files {
			if seen[f] {
				// Skip duplicate — file already assigned to an earlier group
				continue
			}
			d, ok := diffByPath[f]
			if !ok {
				// Skip unknown file path
				continue
			}
			seen[f] = true
			gDiffs = append(gDiffs, d)
		}
		if len(gDiffs) > 0 {
			groups = append(groups, FileGroup{Label: g.Label, Diffs: gDiffs})
		}
	}

	// Files not covered by any group get their own single-file group
	for _, d := range diffs {
		if !seen[d.NewPath] {
			groups = append(groups, FileGroup{Label: d.NewPath, Diffs: []model.Diff{d}})
		}
	}

	// Enforce max files per group
	groups = enforceMaxFilesPerGroup(groups)

	return groups, nil
}

// enforceMaxFilesPerGroup splits groups that exceed maxFilesPerGroup into smaller chunks.
func enforceMaxFilesPerGroup(groups []FileGroup) []FileGroup {
	var result []FileGroup
	for _, g := range groups {
		if len(g.Diffs) <= maxFilesPerGroup {
			result = append(result, g)
			continue
		}
		for i := 0; i < len(g.Diffs); i += maxFilesPerGroup {
			end := i + maxFilesPerGroup
			if end > len(g.Diffs) {
				end = len(g.Diffs)
			}
			result = append(result, FileGroup{
				Label: g.Label,
				Diffs: g.Diffs[i:end],
			})
		}
	}
	return result
}

// enforceGroupTokenBudget splits groups whose combined diffs exceed the token limit.
func enforceGroupTokenBudget(groups []FileGroup, tokenLimit int) []FileGroup {
	if tokenLimit <= 0 {
		return groups
	}
	var result []FileGroup
	for _, g := range groups {
		total := int64(0)
		for _, d := range g.Diffs {
			total += int64(llm.CountTokens(d.Diff))
		}
		if total <= int64(tokenLimit) {
			result = append(result, g)
		} else {
			for _, d := range g.Diffs {
				result = append(result, FileGroup{
					Label: g.Label + " (split: " + d.NewPath + ")",
					Diffs: []model.Diff{d},
				})
			}
		}
	}
	return result
}

// fileGroupKey returns a deterministic key for a file group: sorted paths joined by comma.
func fileGroupKey(diffs []model.Diff) string {
	if len(diffs) == 1 {
		return diffs[0].NewPath
	}
	paths := make([]string, len(diffs))
	for i, d := range diffs {
		paths[i] = d.NewPath
	}
	sort.Strings(paths)
	return strings.Join(paths, ",")
}

func toSingleFileGroups(diffs []model.Diff) []FileGroup {
	groups := make([]FileGroup, 0, len(diffs))
	for _, d := range diffs {
		groups = append(groups, FileGroup{
			Label: d.NewPath,
			Diffs: []model.Diff{d},
		})
	}
	return groups
}
