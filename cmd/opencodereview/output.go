// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/suggestdiff"
)

func outputText(comments []model.LlmComment) {
	if len(comments) == 0 {
		fmt.Println("No comments generated. Looks good to me.")
		return
	}
	for _, c := range comments {
		renderComment(c, os.Stdout)
	}
}

func hasSubtaskErrors(warnings []agent.AgentWarning) bool {
	for _, w := range warnings {
		if isSubtaskErrorType(w.Type) {
			return true
		}
	}
	return false
}

// warningsForOutput removes coverage-level subtask diagnostics once a manifest
// is present. Their classification and safe summary already live in the frozen
// coverage.failed set; retaining the original warning would duplicate that fact
// and could expose the provider's raw error text in JSON. Non-coverage warnings
// remain visible, and legacy output keeps its existing warning behavior.
func warningsForOutput(warnings []agent.AgentWarning, manifest *session.RunManifest) []agent.AgentWarning {
	if manifest == nil || len(warnings) == 0 {
		return warnings
	}
	filtered := make([]agent.AgentWarning, 0, len(warnings))
	for _, warning := range warnings {
		if !isSubtaskErrorType(warning.Type) {
			filtered = append(filtered, warning)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func isSubtaskErrorType(warningType string) bool {
	return warningType == "subtask_error" || warningType == "scan_subtask_error"
}

func outputTextWithWarnings(comments []model.LlmComment, warnings []agent.AgentWarning, manifest *session.RunManifest, out io.Writer) {
	if manifest != nil {
		fmt.Fprintln(out, manifestMessage(manifest, len(comments)))
		for _, c := range comments {
			renderComment(c, out)
		}
	} else if len(comments) == 0 {
		if hasSubtaskErrors(warnings) {
			fmt.Fprintln(out, "Some files could not be reviewed due to errors (see warnings below).")
		} else {
			fmt.Fprintln(out, "No comments generated. Looks good to me.")
		}
	} else {
		for _, c := range comments {
			renderComment(c, out)
		}
	}
	for _, w := range warnings {
		if isSubtaskErrorType(w.Type) {
			continue
		}
		fmt.Fprintf(os.Stderr, "[ocr] WARNING [%s] %s: %s\n", w.Type, sanitizeTerminal(w.File), sanitizeTerminal(w.Message))
	}
}

func renderComment(comment model.LlmComment, out io.Writer) {
	lines := buildDiffLines(comment)
	if len(lines) == 0 && comment.Content == "" {
		return
	}

	fmt.Fprintf(out, "\n%s\n", colorf("\033[2m", "─── %s:%d-%d ───", sanitizeTerminal(comment.Path), comment.StartLine, comment.EndLine))

	if comment.Content != "" {
		badge := buildBadge(comment)
		content := sanitizeTerminal(comment.Content)
		if badge != "" {
			// Prepend the plain badge text to the content so it wraps inline with
			// the first line, then colorize just the badge prefix after wrapping.
			content = badge + " " + content
		}
		lines := wrapByRunes(content, 100)
		for i, ln := range lines {
			if i == 0 && badge != "" && strings.HasPrefix(ln, badge) {
				ln = colorize(severityColor(comment.Severity), badge) + ln[len(badge):]
			}
			fmt.Fprintf(out, "%s\n", ln)
		}
		fmt.Fprintln(out)
	}

	if len(lines) > 0 {
		for _, dl := range lines {
			switch dl.Type {
			case suggestdiff.DiffAdded:
				printDiffLine(out, "+", sanitizeTerminal(dl.Content), "\033[92m", "\033[48;2;0;60;0m")
			case suggestdiff.DiffDeleted:
				printDiffLine(out, "-", sanitizeTerminal(dl.Content), "\033[91m", "\033[48;2;70;0;0m")
			case suggestdiff.DiffContext:
				printDiffLine(out, " ", sanitizeTerminal(dl.Content), "\033[2m", "\033[48;2;38;38;38m")
			}
		}
	}

	fmt.Fprintln(out)
}

// buildBadge renders a compact "[category · severity]" tag for a finding. It returns
// an empty string when neither structured field is present, so text output for findings
// without metadata is unchanged.
func buildBadge(comment model.LlmComment) string {
	category := sanitizeTerminal(comment.Category)
	severity := sanitizeTerminal(comment.Severity)
	switch {
	case category != "" && severity != "":
		return fmt.Sprintf("[%s · %s]", category, severity)
	case category != "":
		return fmt.Sprintf("[%s]", category)
	case severity != "":
		return fmt.Sprintf("[%s]", severity)
	default:
		return ""
	}
}

// severityColor maps a finding severity to an ANSI color used for its badge.
// Unknown or empty severities fall back to dim.
func severityColor(severity string) string {
	switch severity {
	case "critical":
		return "\033[1;91m" // bold bright red
	case "high":
		return "\033[91m" // bright red
	case "medium":
		return "\033[93m" // bright yellow
	case "low":
		return "\033[94m" // bright blue
	default:
		return "\033[2m" // dim
	}
}

// printDiffLine renders a single diff line with colored prefix and background on
// content. With color disabled it emits "<prefix> <content>", which keeps the
// +/-/space gutter that carries the added/deleted/context meaning in plain text.
func printDiffLine(out io.Writer, prefix, content, fgColor, bgColor string) {
	if !colorOn() {
		fmt.Fprintf(out, "%s %s\n", prefix, content)
		return
	}
	fmt.Fprintf(out, "%s%s%s %s%s\n", fgColor+bgColor, prefix, ansiReset+bgColor, content, ansiReset)
}

// wrapByRunes splits text into lines that fit within maxWidth **rune** columns.
// Respects existing newlines and wraps at word boundaries.
func wrapByRunes(text string, maxW int) []string {
	if text == "" {
		return nil
	}
	var result []string
	for _, para := range strings.Split(text, "\n") {
		result = append(result, wrapSingleRuneLine(para, maxW)...)
	}
	return result
}

// wrapSingleRuneLine breaks one paragraph (no newlines) into rune-width-constrained lines.
func wrapSingleRuneLine(line string, maxW int) []string {
	runes := []rune(line)
	if visibleRunesLen(runes) <= maxW {
		return []string{line}
	}
	var result []string
	for len(runes) > 0 {
		cut := runeWrapCut(runes, maxW)
		result = append(result, string(runes[:cut]))
		runes = runes[cut:]
		// trim leading spaces of next segment
		for len(runes) > 0 && runes[0] == ' ' {
			runes = runes[1:]
		}
	}
	return result
}

// runeWrapCut returns a rune index suitable for breaking the line at ~maxW display width.
func runeWrapCut(runes []rune, maxW int) int {
	if visibleRunesLen(runes) <= maxW {
		return len(runes)
	}
	best := maxW
	if best >= len(runes) {
		return len(runes)
	}
	for i := best; i > 0; i-- {
		if runes[i] == ' ' || runes[i] == '\t' {
			return i
		}
	}
	return best
}

func visibleRunesLen(runes []rune) int {
	n := 0
	for _, r := range runes {
		if r >= 32 && r != 127 {
			n++
		}
	}
	return n
}

func sanitizeTerminal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitToLines(s string) []string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func buildDiffLines(comment model.LlmComment) []suggestdiff.DiffLine {
	if comment.SuggestionCode == "" || comment.ExistingCode == "" {
		return nil
	}
	oldLines := splitToLines(comment.ExistingCode)
	newLines := splitToLines(comment.SuggestionCode)
	return suggestdiff.ComputeLineDiff(oldLines, newLines)
}

type jsonSummary struct {
	FilesReviewed    int64  `json:"files_reviewed"`
	Comments         int64  `json:"comments"`
	TotalTokens      int64  `json:"total_tokens"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64  `json:"cache_write_tokens,omitempty"`
	Elapsed          string `json:"elapsed"`
	BudgetExceeded   bool   `json:"budget_exceeded,omitempty"`
}

type jsonToolCalls struct {
	Total  int64            `json:"total"`
	ByTool map[string]int64 `json:"by_tool"`
}

type jsonLLMIdentity struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
}

type jsonOutput struct {
	Status         string               `json:"status"`
	LLM            *jsonLLMIdentity     `json:"llm,omitempty"`
	TraceID        string               `json:"trace_id,omitempty"`
	Message        string               `json:"message,omitempty"`
	Summary        *jsonSummary         `json:"summary,omitempty"`
	ToolCalls      *jsonToolCalls       `json:"tool_calls"`
	Comments       []model.LlmComment   `json:"comments"`
	Warnings       []agent.AgentWarning `json:"warnings,omitempty"`
	ProjectSummary string               `json:"project_summary,omitempty"`
	Resume         *agent.ResumeInfo    `json:"resume,omitempty"`
	SessionID      string               `json:"session_id,omitempty"`
	Manifest       *session.RunManifest `json:"manifest,omitempty"`
	// RetryReport is the frozen LLM retry report (ocr.llm-retry-report/v1).
	// Reuses llm.RetryReport's own field/tag definitions rather than mirroring
	// them here, and sits last with omitempty so a first-try-success run emits
	// byte-identical JSON to before #368.
	RetryReport *llm.RetryReport `json:"retry_report,omitempty"`
}

func outputJSON(comments []model.LlmComment) error {
	out := jsonOutput{
		Status:   "success",
		Comments: comments,
	}
	if len(comments) == 0 {
		out.Message = "No comments generated. Looks good to me."
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputJSONWithWarnings(comments []model.LlmComment, warnings []agent.AgentWarning,
	filesReviewed, inputTokens, outputTokens, totalTokens, cacheReadTokens, cacheWriteTokens int64,
	duration time.Duration, projectSummary string, toolCalls map[string]int64, traceID string, resumeInfo *agent.ResumeInfo, sessionID string,
	manifest *session.RunManifest, budgetExceeded bool, llmIdentity *jsonLLMIdentity, out io.Writer,
	retryReport *llm.RetryReport) error {
	publishedWarnings := warningsForOutput(warnings, manifest)
	payload := jsonOutput{
		Status:   "success",
		LLM:      llmIdentity,
		TraceID:  traceID,
		Comments: comments,
		Summary: &jsonSummary{
			FilesReviewed:    filesReviewed,
			Comments:         int64(len(comments)),
			TotalTokens:      totalTokens,
			InputTokens:      inputTokens,
			OutputTokens:     outputTokens,
			CacheReadTokens:  cacheReadTokens,
			CacheWriteTokens: cacheWriteTokens,
			Elapsed:          duration.Round(time.Second).String(),
			BudgetExceeded:   budgetExceeded,
		},
		ProjectSummary: projectSummary,
		Resume:         resumeInfo,
		SessionID:      sessionID,
		Manifest:       manifest,
		RetryReport:    retryReport,
	}
	var total int64
	for _, v := range toolCalls {
		total += v
	}
	byTool := toolCalls
	if byTool == nil {
		byTool = make(map[string]int64)
	}
	payload.ToolCalls = &jsonToolCalls{
		Total:  total,
		ByTool: byTool,
	}
	if manifest != nil {
		payload.Status = string(manifest.TerminalState)
		payload.Message = manifestMessage(manifest, len(comments))
	} else if len(comments) == 0 {
		if hasSubtaskErrors(warnings) {
			payload.Message = "Some files could not be reviewed due to errors."
		} else {
			payload.Message = "No comments generated. Looks good to me."
		}
	}
	if len(publishedWarnings) > 0 {
		payload.Warnings = publishedWarnings
		if manifest == nil && hasSubtaskErrors(publishedWarnings) {
			payload.Status = "completed_with_errors"
		} else if manifest == nil {
			payload.Status = "completed_with_warnings"
		}
	}
	// budgetExceeded deliberately does NOT touch payload.Status. Reaching the
	// aggregate token budget is a controlled coverage truncation, so it is already
	// expressed in the manifest as failed(budget) on the items that never got
	// dispatched — which makes terminal_state read "partial" whenever anything was
	// covered. The status set above is therefore the single source of truth,
	// and the budget reason stays observable through three deterministic outlets:
	// summary.budget_exceeded, the token_budget_reached warning, and
	// coverage.failed[].classification == "budget".
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// retryStages maps internal task types to concise terminal labels. The slice
// order is also the display order.
var retryStages = []struct {
	taskType session.TaskType
	title    string
}{
	{session.PlanTask, "Review planning"},
	{session.MainTask, "Core review"},
	{session.MemoryCompressionTask, "Context compaction"},
	{session.ReLocationTask, "Comment re-location"},
	{session.ReviewFilterTask, "Comment filtering"},
}

// Keep the terminal concise; JSON retains every request.
const retryGroupListLimit = 5

// retryGroup is one review-stage bucket of listed requests.
type retryGroup struct {
	rank     int
	title    string
	requests []llm.RequestReport
}

func retryOutcomeInfo(o llm.Outcome) (summary string, rank int) {
	switch o {
	case llm.OutcomeFailed:
		return "failed", 0
	case llm.OutcomeCancelled:
		return "cancelled", 1
	case llm.OutcomeRecovered:
		return "recovered after retry", 2
	default:
		return "retried at provider request", 3
	}
}

// outputRetryReportText groups noteworthy requests by review stage. It
// renders only stable classes and status codes; raw provider errors stay out.
func outputRetryReportText(w io.Writer, rep *llm.RetryReport) {
	if rep == nil {
		return
	}
	groups := groupRetryRequests(rep.Requests)

	// The counts come from the listed requests, not from the report aggregates, so the
	// summary line can never disagree with the entries printed under it.
	byOutcome := make(map[llm.Outcome]int, 3)
	for _, r := range rep.Requests {
		byOutcome[r.Outcome]++
	}
	parts := make([]string, 0, 4)
	for _, o := range []llm.Outcome{llm.OutcomeFailed, llm.OutcomeCancelled, llm.OutcomeRecovered, llm.OutcomeSucceeded} {
		if n := byOutcome[o]; n > 0 {
			summary, _ := retryOutcomeInfo(o)
			parts = append(parts, fmt.Sprintf("%d %s %s", n, plural(n, "request"), summary))
		}
	}

	fmt.Fprintf(w, "\nLLM retry report summary: %d of %d %s affected",
		len(rep.Requests), rep.TotalRequests, plural(rep.TotalRequests, "request"))
	if len(parts) > 0 {
		fmt.Fprintf(w, " -- %s", strings.Join(parts, ", "))
	}
	fmt.Fprintln(w)

	for _, g := range groups {
		header := fmt.Sprintf("%s (%d %s", g.title,
			len(g.requests), plural(len(g.requests), "request"))
		fmt.Fprintf(w, "\n%s):\n", header)
		for i, r := range g.requests {
			if i == retryGroupListLimit {
				fmt.Fprintf(w, "- ... and %d more\n", len(g.requests)-i)
				break
			}
			fmt.Fprintf(w, "- %s: %s\n", sanitizeTerminal(r.FilePath), retryAttemptChain(r))
		}
	}

	fmt.Fprintf(w, "\nPer-attempt detail: --format json (retry_report).\n")
}

// groupRetryRequests buckets by review stage, then sorts for stable output.
func groupRetryRequests(requests []llm.RequestReport) []*retryGroup {
	byStage := make(map[string]*retryGroup)
	for _, r := range requests {
		g := byStage[r.TaskType]
		if g == nil {
			title, rank := retryStageInfo(r.TaskType)
			g = &retryGroup{rank: rank, title: title}
			byStage[r.TaskType] = g
		}
		g.requests = append(g.requests, r)
	}

	groups := make([]*retryGroup, 0, len(byStage))
	for _, g := range byStage {
		sort.Slice(g.requests, func(i, j int) bool {
			_, ri := retryOutcomeInfo(g.requests[i].Outcome)
			_, rj := retryOutcomeInfo(g.requests[j].Outcome)
			if ri != rj {
				return ri < rj
			}
			if g.requests[i].FilePath != g.requests[j].FilePath {
				return g.requests[i].FilePath < g.requests[j].FilePath
			}
			return g.requests[i].RequestNo < g.requests[j].RequestNo
		})
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].rank != groups[j].rank {
			return groups[i].rank < groups[j].rank
		}
		return groups[i].title < groups[j].title
	})
	return groups
}

func retryStageInfo(taskType string) (title string, rank int) {
	for i, stage := range retryStages {
		if string(stage.taskType) == taskType {
			return stage.title, i
		}
	}
	return sanitizeTerminal(taskType), len(retryStages)
}

// retryAttemptChain renders one logical request as a short human-readable chain.
func retryAttemptChain(r llm.RequestReport) string {
	parts := make([]string, 0, len(r.Attempts)+1)
	for i, a := range r.Attempts {
		if a.Outcome == llm.AttemptSuccess {
			if i < len(r.Attempts)-1 {
				parts = append(parts, "succeeded (provider asked to retry)")
				continue
			}
			parts = append(parts, "succeeded")
			continue
		}
		parts = append(parts, retryErrorPhrase(a))
	}
	lastCancelled := len(r.Attempts) > 0 &&
		r.Attempts[len(r.Attempts)-1].ErrorClass == llm.ErrorClassCancelled
	if r.Outcome == llm.OutcomeFailed ||
		(r.Outcome == llm.OutcomeCancelled && !lastCancelled) {
		parts = append(parts, string(r.Outcome))
	}
	return strings.Join(parts, " -> ")
}

func retryErrorPhrase(a llm.AttemptRecord) string {
	var phrase string
	switch a.ErrorClass {
	case llm.ErrorClassRateLimited:
		phrase = "rate limited"
	case llm.ErrorClassOverloaded:
		phrase = "provider overloaded"
	case llm.ErrorClassAuthentication:
		phrase = "authentication rejected"
	case llm.ErrorClassTimeout:
		phrase = "timed out"
	case llm.ErrorClassNetwork:
		phrase = "network error"
	case llm.ErrorClassProvider:
		phrase = "provider error"
		if a.StatusCode > 0 && a.StatusCode < 500 {
			phrase = "rejected by provider"
		}
	case llm.ErrorClassCancelled:
		phrase = "cancelled"
	default:
		phrase = "unclassified error"
	}
	if a.StatusCode > 0 {
		phrase = fmt.Sprintf("%s (HTTP %d)", phrase, a.StatusCode)
	}
	return phrase
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func manifestMessage(manifest *session.RunManifest, findings int) string {
	if manifest == nil {
		return ""
	}
	selected := len(manifest.Coverage.Selected)
	failed := len(manifest.Coverage.Failed)
	waived := len(manifest.Coverage.Waived)
	switch manifest.TerminalState {
	case session.StateComplete:
		if waived > 0 {
			return fmt.Sprintf("Review complete: %d finding(s) across %d selected item(s), including %d waived.", findings, selected, waived)
		}
		return fmt.Sprintf("Review complete: %d finding(s) across %d selected item(s).", findings, selected)
	case session.StatePartial:
		return fmt.Sprintf("Review partially complete: %d finding(s); %d of %d selected item(s) failed.", findings, failed, selected)
	case session.StateFailed:
		if manifest.RunFailure != nil {
			return fmt.Sprintf("Review failed (%s): %d finding(s); %d of %d selected item(s) failed.", manifest.RunFailure.Classification, findings, failed, selected)
		}
		return fmt.Sprintf("Review failed: %d finding(s); %d of %d selected item(s) failed.", findings, failed, selected)
	case session.StateSkipped:
		return "Review skipped: no items were selected."
	default:
		return fmt.Sprintf("Review finished with unknown manifest state %q.", manifest.TerminalState)
	}
}

func outputJSONNoFiles(traceID string, llmIdentity *jsonLLMIdentity, out io.Writer) error {
	payload := jsonOutput{
		Status:   "skipped",
		LLM:      llmIdentity,
		TraceID:  traceID,
		Message:  "No supported files changed.",
		Comments: []model.LlmComment{},
		ToolCalls: &jsonToolCalls{
			ByTool: map[string]int64{},
		},
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// emitFailureUsage writes a best-effort structured usage record to stderr when
// a review fails, so the outer caller still sees the cost of the failed attempt.
// It carries only token/tool-call tallies and elapsed, never credentials or
// prompts.
//
// A plain aggregate budget stop does NOT reach here: it is a controlled coverage
// truncation, so it yields terminal_state=partial and a nil error. It only
// arrives when the truncation left nothing covered at all (every selected item
// failed(budget) ⇒ terminal_state=failed), or alongside an unrelated failure.
// Whenever the manifest was constructed, stdout has already published the
// complete frozen result before this runs — so this record supplements it, never
// replaces it. We report the agent's actual BudgetExceeded() value rather than
// hardcoding false, so the record can never contradict the agent's state.
//
// In json format it emits a jsonOutput-shaped object to stderr (kept separate
// from stdout so it does not pollute the machine-readable result stream, which
// therefore always carries exactly one JSON document); otherwise a single
// human-readable [ocr] line. It must never return an error that masks the
// original failure — all writes are best-effort.
//
// retryReport must be nil whenever emitRunResult already ran: a constructed
// manifest is publishable even on a failed run, so both this record and the
// normal result exit can execute for the same run, and the report belongs to
// exactly one of them. Pass the frozen report here only when the normal exit
// was skipped, so the report is never duplicated and never silently dropped.
func emitFailureUsage(ag ResultProvider, duration time.Duration, outputFormat string, llmIdentity *jsonLLMIdentity,
	retryReport *llm.RetryReport) {
	var toolTotal int64
	for _, v := range ag.ToolCalls() {
		toolTotal += v
	}
	budgetExceeded := ag.BudgetExceeded()
	if outputFormat == "json" {
		out := jsonOutput{
			Status: "failed",
			LLM:    llmIdentity,
			Summary: &jsonSummary{
				FilesReviewed:    ag.FilesReviewed(),
				TotalTokens:      ag.TotalTokensUsed(),
				InputTokens:      ag.TotalInputTokens(),
				OutputTokens:     ag.TotalOutputTokens(),
				CacheReadTokens:  ag.TotalCacheReadTokens(),
				CacheWriteTokens: ag.TotalCacheWriteTokens(),
				Elapsed:          duration.Round(time.Second).String(),
				BudgetExceeded:   budgetExceeded,
			},
			ToolCalls: &jsonToolCalls{
				Total:  toolTotal,
				ByTool: ag.ToolCalls(),
			},
			SessionID:   ag.SessionID(),
			RetryReport: retryReport,
		}
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}
	fmt.Fprintf(os.Stderr, "[ocr] usage on failure: %d file(s), %d input + %d output = %d total tokens, %d tool calls, elapsed %s, budget_exceeded=%v",
		ag.FilesReviewed(), ag.TotalInputTokens(), ag.TotalOutputTokens(), ag.TotalTokensUsed(),
		toolTotal, duration.Round(time.Second).String(), budgetExceeded)
	if id := ag.SessionID(); id != "" {
		fmt.Fprintf(os.Stderr, ", session %s", id)
	}
	fmt.Fprintln(os.Stderr)
	// Text mode has no structured envelope, so the report follows the usage
	// line on the same stream.
	outputRetryReportText(os.Stderr, retryReport)
}

// outputPreview renders a preview in the requested output format. sarif is
// rejected with an error because a preview contains file/rule metadata, not
// review findings — there is no SARIF result to emit, and a differently-shaped
// document would confuse consumers expecting a SARIF report.
func outputPreview(p *agent.DiffPreview, outputFormat string, out io.Writer) error {
	outputFormat = strings.ToLower(strings.TrimSpace(outputFormat))
	if outputFormat == "sarif" {
		return fmt.Errorf("--format sarif is not supported with --preview: SARIF output requires completed review findings")
	}
	if outputFormat == "json" {
		return outputPreviewJSON(p, out)
	}
	outputPreviewText(p, out)
	// outputPreviewText drops fmt.Fprintf write errors; surface deferred
	// writer errors so a failed --output write fails the command non-zero.
	return writeOutError(out)
}

func outputPreviewJSON(p *agent.DiffPreview, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}

func outputPreviewText(p *agent.DiffPreview, out io.Writer) {
	if p.TotalFiles == 0 {
		fmt.Fprintln(out, "No files changed.")
		return
	}

	maxPathLen := 0
	for _, e := range p.Entries {
		if n := len(sanitizeTerminal(e.Path)); n > maxPathLen {
			maxPathLen = n
		}
	}
	if maxPathLen < 20 {
		maxPathLen = 20
	}
	pathFmt := fmt.Sprintf("%%-%ds", maxPathLen)

	fmt.Fprintf(out, "\nPreview: %d file(s) changed  |  %s  %s\n", p.TotalFiles,
		colorf("\033[32m", "+%d", p.TotalInsertions),
		colorf("\033[31m", "-%d", p.TotalDeletions))

	if p.ReviewableCount > 0 {
		fmt.Fprintf(out, "\n%s\n", colorf("\033[1m", "Will review (%d):", p.ReviewableCount))
		for _, e := range p.Entries {
			if !e.WillReview {
				continue
			}
			// The counts are padded before colorizing so the columns stay aligned
			// whether or not the escape sequences are present.
			fmt.Fprintf(out, "  %s  "+pathFmt+" %s %s\n",
				statusBadge(e.Status), sanitizeTerminal(e.Path),
				colorf("\033[32m", "+%-4d", e.Insertions),
				colorf("\033[31m", "-%-4d", e.Deletions))
		}
	}

	if p.ExcludedCount > 0 {
		fmt.Fprintf(out, "\n%s\n", colorf("\033[1m", "Excluded from review (%d):", p.ExcludedCount))
		for _, e := range p.Entries {
			if e.WillReview {
				continue
			}
			fmt.Fprintf(out, "  %s  "+pathFmt+" %s\n",
				statusBadge(e.Status), sanitizeTerminal(e.Path),
				colorf("\033[2m", "(%s)", sanitizeTerminal(string(e.ExcludeReason))))
		}
	}

	fmt.Fprintln(out)
}

// statusBadge renders the per-file status tag. The letter carries the meaning,
// so with color disabled the bare "[A]"/"[M]"/... form is still unambiguous.
func statusBadge(status string) string {
	switch status {
	case "added":
		return colorize("\033[32m", "[A]")
	case "modified":
		return colorize("\033[33m", "[M]")
	case "deleted":
		return colorize("\033[31m", "[D]")
	case "renamed":
		return colorize("\033[36m", "[R]")
	case "binary":
		return colorize("\033[35m", "[B]")
	case "scan":
		return colorize("\033[34m", "[S]")
	default:
		return "[?]"
	}
}
