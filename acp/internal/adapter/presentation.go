// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func formatResult(r *orchestrator.Result) string {
	return formatResultAtRoot(r, "")
}

func formatResultAtRoot(r *orchestrator.Result, root string) string {
	if r == nil || r.Review == nil && r.Scan == nil {
		return "OCR returned no result."
	}
	var b strings.Builder
	if r.Review != nil {
		review := r.Review
		writeReport(&b, root, "review", review.Status, review.Message, review.Summary, review.Comments, review.Manifest)
	}
	if r.Scan != nil {
		scan := r.Scan
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		writeReport(&b, root, "scan", scan.Status, scan.Message, scan.Summary, scan.Comments, nil)
	}
	return b.String()
}

func resultStatus(status string) string {
	switch status {
	case contract.StatusSuccess, contract.StatusComplete:
		return "Complete"
	case contract.StatusPartial:
		return "Partial"
	case contract.StatusFailed:
		return "Failed"
	case contract.StatusSkipped:
		return "Skipped"
	case contract.StatusCompletedWithWarnings:
		return "Completed with warnings"
	case contract.StatusCompletedWithErrors:
		return "Completed with errors"
	case "":
		return "Status unavailable"
	default:
		return markdownLabel(status)
	}
}

// reviewStatus uses the manifest's terminal state when the CLI provides one.
func reviewStatus(status string, manifest *contract.Manifest) string {
	if manifest != nil && manifest.TerminalState != "" {
		return manifest.TerminalState
	}
	return status
}

func writeReport(b *strings.Builder, root, operation, status, message string, summary *contract.Summary, comments []contract.Comment, manifest *contract.Manifest) {
	fmt.Fprintf(b, "## OCR %s · %s\n\n", operation, resultStatus(reviewStatus(status, manifest)))
	count := int64(len(comments))
	if manifest != nil && manifest.Coverage != nil && manifest.Coverage.Selected != nil {
		coverage := manifest.Coverage
		fmt.Fprintf(b, "%d/%d files completed · ", len(coverage.Completed), len(coverage.Selected))
		for _, group := range []struct {
			count int
			label string
		}{
			{len(coverage.Reused), "reused"}, {len(coverage.Failed), "failed"}, {len(coverage.Waived), "waived"},
		} {
			if group.count > 0 {
				fmt.Fprintf(b, "%d %s · ", group.count, group.label)
			}
		}
	} else if summary != nil {
		fileNoun := "files"
		if summary.FilesReviewed == 1 {
			fileNoun = "file"
		}
		verb := "processed"
		if state := reviewStatus(status, manifest); state == contract.StatusSuccess || state == contract.StatusComplete {
			verb = "reviewed"
		}
		fmt.Fprintf(b, "%d %s %s · ", summary.FilesReviewed, fileNoun, verb)
	}
	noun := "findings"
	if count == 1 {
		noun = "finding"
	}
	fmt.Fprintf(b, "%d %s", count, noun)
	writeSeverityCounts(b, comments)
	if message != "" {
		fmt.Fprintf(b, "\n\n%s", message)
	}
	if summary != nil && summary.BudgetExceeded {
		b.WriteString("\n\n**Token budget exceeded.** Coverage may be incomplete.")
	}
	if manifest != nil {
		if manifest.TerminalState != "" && resultStatus(manifest.TerminalState) != resultStatus(status) {
			fmt.Fprintf(b, "\n\nReported status: %s", resultStatus(status))
		}
		if manifest.RunFailure != nil {
			fmt.Fprintf(b, "\n\nRun failure [%s]: %s", manifest.RunFailure.Classification, manifest.RunFailure.Reason)
		}
		if manifest.Coverage != nil {
			for _, failure := range manifest.Coverage.Failed {
				fmt.Fprintf(b, "\n\nFailed file %s [%s]: %s", markdownLabel(failure.Path), failure.Classification, failure.Reason)
			}
		}
	}
	for index, comment := range comments {
		writeFinding(b, index+1, comment, findingLocation(root, comment))
	}
	if summary != nil {
		var usage []string
		if summary.TotalTokens > 0 {
			usage = append(usage, fmt.Sprintf("%d tokens", summary.TotalTokens))
		}
		if summary.Elapsed != "" {
			usage = append(usage, markdownLabel(summary.Elapsed))
		}
		if len(usage) > 0 {
			fmt.Fprintf(b, "\n\nUsage: %s", strings.Join(usage, " · "))
		}
	}
}

func severityLabel(severity string) string {
	switch severity {
	case "critical":
		return "Critical"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	case "low":
		return "Low"
	case "":
		return "Unspecified"
	default:
		return markdownLabel(severity)
	}
}

func writeSeverityCounts(b *strings.Builder, comments []contract.Comment) {
	if len(comments) == 0 {
		return
	}
	counts := make(map[string]int)
	for _, comment := range comments {
		counts[comment.Severity]++
	}
	labels := make([]string, 0, len(counts))
	for _, severity := range []string{"critical", "high", "medium", "low"} {
		if count := counts[severity]; count > 0 {
			labels = append(labels, fmt.Sprintf("%s %d", severityLabel(severity), count))
			delete(counts, severity)
		}
	}
	unknown := make([]string, 0, len(counts))
	for severity := range counts {
		if severity != "" {
			unknown = append(unknown, severity)
		}
	}
	sort.Strings(unknown)
	for _, severity := range unknown {
		labels = append(labels, fmt.Sprintf("%s %d", severityLabel(severity), counts[severity]))
	}
	if counts[""] > 0 {
		labels = append(labels, fmt.Sprintf("Unspecified %d", counts[""]))
	}
	fmt.Fprintf(b, "\n\n%s", strings.Join(labels, " · "))
}

func writeFinding(b *strings.Builder, number int, c contract.Comment, location *acp.ToolCallLocation) {
	fmt.Fprintf(b, "\n\n### %d. %s", number, severityLabel(c.Severity))
	label := c.Path
	if c.StartLine > 0 {
		label += fmt.Sprintf(":%d", c.StartLine)
		if c.EndLine > c.StartLine {
			label += fmt.Sprintf("-%d", c.EndLine)
		}
	}
	if label != "" {
		b.WriteString(" · ")
		if location != nil {
			fmt.Fprintf(b, "[%s](<%s>)", markdownLabel(label), fileURLString(location.Path, location.Line))
		} else {
			b.WriteString(markdownLabel(label))
		}
	}
	if c.Category != "" {
		fmt.Fprintf(b, "\n\n**Category:** %s", markdownLabel(c.Category))
	}
	fmt.Fprintf(b, "\n\n%s", c.Content)
	if c.ExistingCode != "" {
		b.WriteString("\n\n**Existing code:**\n\n")
		writeCodeBlock(b, c.Path, c.ExistingCode)
	}
	if c.SuggestionCode != "" {
		b.WriteString("\n\n**Suggested code (not applied):**\n\n")
		writeCodeBlock(b, c.Path, c.SuggestionCode)
	}
}
