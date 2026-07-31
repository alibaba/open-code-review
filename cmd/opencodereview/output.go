package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/alibaba/open-code-review/internal/agent"
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
		renderComment(c)
	}
}

func hasSubtaskErrors(warnings []agent.AgentWarning) bool {
	for _, w := range warnings {
		if w.Type == "subtask_error" {
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
		if warning.Type != "subtask_error" {
			filtered = append(filtered, warning)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func outputTextWithWarnings(comments []model.LlmComment, warnings []agent.AgentWarning, manifest *session.RunManifest) {
	if manifest != nil {
		fmt.Println(manifestMessage(manifest, len(comments)))
		for _, c := range comments {
			renderComment(c)
		}
	} else if len(comments) == 0 {
		if hasSubtaskErrors(warnings) {
			fmt.Println("Some files could not be reviewed due to errors (see warnings below).")
		} else {
			fmt.Println("No comments generated. Looks good to me.")
		}
	} else {
		for _, c := range comments {
			renderComment(c)
		}
	}
	for _, w := range warnings {
		if w.Type == "subtask_error" {
			continue
		}
		fmt.Fprintf(os.Stderr, "[ocr] WARNING [%s] %s: %s\n", w.Type, sanitizeTerminal(w.File), sanitizeTerminal(w.Message))
	}
}

func renderComment(comment model.LlmComment) {
	lines := buildDiffLines(comment)
	if len(lines) == 0 && comment.Content == "" {
		return
	}

	fmt.Printf("\n\033[2m─── %s:%d-%d ───\033[0m\n", sanitizeTerminal(comment.Path), comment.StartLine, comment.EndLine)

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
				color := severityColor(comment.Severity)
				ln = color + badge + "\033[0m" + ln[len(badge):]
			}
			fmt.Printf("%s\n", ln)
		}
		fmt.Println()
	}

	if len(lines) > 0 {
		for _, dl := range lines {
			switch dl.Type {
			case suggestdiff.DiffAdded:
				printDiffLine("+", sanitizeTerminal(dl.Content), "\033[92m", "\033[48;2;0;60;0m")
			case suggestdiff.DiffDeleted:
				printDiffLine("-", sanitizeTerminal(dl.Content), "\033[91m", "\033[48;2;70;0;0m")
			case suggestdiff.DiffContext:
				printDiffLine(" ", sanitizeTerminal(dl.Content), "\033[2m", "\033[48;2;38;38;38m")
			}
		}
	}

	fmt.Println()
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

// printDiffLine renders a single diff line with colored prefix and background on content.
func printDiffLine(prefix, content, fgColor, bgColor string) {
	fmt.Printf("%s%s%s %s%s\033[0m\n", fgColor+bgColor, prefix, "\033[0m"+bgColor, content, "\033[0m")
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

type jsonOutput struct {
	Status         string               `json:"status"`
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
	manifest *session.RunManifest, budgetExceeded bool) error {
	publishedWarnings := warningsForOutput(warnings, manifest)
	out := jsonOutput{
		Status:   "success",
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
	}
	var total int64
	for _, v := range toolCalls {
		total += v
	}
	byTool := toolCalls
	if byTool == nil {
		byTool = make(map[string]int64)
	}
	out.ToolCalls = &jsonToolCalls{
		Total:  total,
		ByTool: byTool,
	}
	if manifest != nil {
		out.Status = string(manifest.TerminalState)
		out.Message = manifestMessage(manifest, len(comments))
	} else if len(comments) == 0 {
		if hasSubtaskErrors(warnings) {
			out.Message = "Some files could not be reviewed due to errors."
		} else {
			out.Message = "No comments generated. Looks good to me."
		}
	}
	if len(publishedWarnings) > 0 {
		out.Warnings = publishedWarnings
		if manifest == nil && hasSubtaskErrors(publishedWarnings) {
			out.Status = "completed_with_errors"
		} else if manifest == nil {
			out.Status = "completed_with_warnings"
		}
	}
	// budgetExceeded deliberately does NOT touch out.Status. Reaching the
	// aggregate token budget is a controlled coverage truncation, so it is already
	// expressed in the manifest as failed(budget) on the items that never got
	// dispatched — which makes terminal_state read "partial" whenever anything was
	// covered. The status set above is therefore the single source of truth,
	// and the budget reason stays observable through three deterministic outlets:
	// summary.budget_exceeded, the token_budget_reached warning, and
	// coverage.failed[].classification == "budget".
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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

func outputJSONNoFiles(traceID string) error {
	out := jsonOutput{
		Status:   "skipped",
		TraceID:  traceID,
		Message:  "No supported files changed.",
		Comments: []model.LlmComment{},
		ToolCalls: &jsonToolCalls{
			ByTool: map[string]int64{},
		},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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
func emitFailureUsage(ag ResultProvider, duration time.Duration, outputFormat string) {
	var toolTotal int64
	for _, v := range ag.ToolCalls() {
		toolTotal += v
	}
	budgetExceeded := ag.BudgetExceeded()
	if outputFormat == "json" {
		out := jsonOutput{
			Status: "failed",
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
			SessionID: ag.SessionID(),
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
}

func outputPreviewText(p *agent.DiffPreview) {
	if p.TotalFiles == 0 {
		fmt.Println("No files changed.")
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

	fmt.Printf("\nPreview: %d file(s) changed  |  \033[32m+%d\033[0m  \033[31m-%d\033[0m\n",
		p.TotalFiles, p.TotalInsertions, p.TotalDeletions)

	if p.ReviewableCount > 0 {
		fmt.Printf("\n\033[1mWill review (%d):\033[0m\n", p.ReviewableCount)
		for _, e := range p.Entries {
			if !e.WillReview {
				continue
			}
			fmt.Printf("  %s  "+pathFmt+" \033[32m+%-4d\033[0m \033[31m-%-4d\033[0m\n",
				statusBadge(e.Status), sanitizeTerminal(e.Path), e.Insertions, e.Deletions)
		}
	}

	if p.ExcludedCount > 0 {
		fmt.Printf("\n\033[1mExcluded from review (%d):\033[0m\n", p.ExcludedCount)
		for _, e := range p.Entries {
			if e.WillReview {
				continue
			}
			fmt.Printf("  %s  "+pathFmt+" \033[2m(%s)\033[0m\n",
				statusBadge(e.Status), sanitizeTerminal(e.Path), sanitizeTerminal(string(e.ExcludeReason)))
		}
	}

	fmt.Println()
}

func statusBadge(status string) string {
	switch status {
	case "added":
		return "\033[32m[A]\033[0m"
	case "modified":
		return "\033[33m[M]\033[0m"
	case "deleted":
		return "\033[31m[D]\033[0m"
	case "renamed":
		return "\033[36m[R]\033[0m"
	case "binary":
		return "\033[35m[B]\033[0m"
	case "scan":
		return "\033[34m[S]\033[0m"
	default:
		return "[?]"
	}
}
