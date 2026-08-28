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
)

const maxFilesPerGroup = 10

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
type groupDiffsResult struct {
	groups []FileGroup
	usage  *llm.UsageInfo
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
		return groupDiffsResult{groups: toSingleFileGroups(diffs)}
	}

	if tpl.GroupingTask == nil || len(tpl.GroupingTask.Messages) == 0 {
		return groupDiffsResult{groups: toSingleFileGroups(diffs)}
	}

	groups, usage, err := callGroupingLLM(ctx, diffs, client, modelName, tpl.GroupingTask, tpl.CompletionTokenLimit(), sessOpts)
	if err != nil {
		fmt.Fprintf(stdout.Writer(), "[ocr] LLM grouping failed (%v), falling back to per-file dispatch\n", err)
		return groupDiffsResult{groups: toSingleFileGroups(diffs), usage: usage}
	}

	groups = enforceGroupTokenBudget(groups, tokenLimit)
	return groupDiffsResult{groups: groups, usage: usage}
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
