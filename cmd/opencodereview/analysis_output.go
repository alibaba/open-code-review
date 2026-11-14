// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/session"
)

func (output *analysisOutput) status() string {
	if (output.ReviewStatus == string(session.StateComplete) || output.ReviewStatus == string(session.StateSkipped)) && output.Report.Status == "completed" {
		return "completed"
	}
	if output.ReviewStatus == "failed" && output.Report.Status == "failed" {
		return "failed"
	}
	return "partial"
}

func outputAnalysisText(writer io.Writer, output *analysisOutput) {
	report := output.Report
	fmt.Fprintf(writer, "\n──────── Analysis Report ────────\n\nOverall execution: %s | Review: %s | Analysis execution: %s\n", output.status(), sanitizeTerminal(output.ReviewStatus), sanitizeTerminal(report.Status))
	fmt.Fprintf(writer, "Model-declared completeness: %s (not a quality guarantee)\n", sanitizeTerminal(report.DeclaredCompletion))
	oldSource, newSource := report.Comparison.Old, report.Comparison.Head
	if report.Comparison.BaseIsEmpty {
		oldSource = "empty tree"
	}
	if report.Comparison.Mode == "workspace" {
		newSource = "workspace (live)"
	}
	fmt.Fprintf(writer, "Comparison: %s -> %s\nStop: %s | Submission: %s\n", sanitizeTerminal(oldSource), sanitizeTerminal(newSource), sanitizeTerminal(report.StopReason), sanitizeTerminal(report.SubmissionStatus))
	if report.Markdown == "" {
		fmt.Fprintln(writer, "\nNo analysis report was accepted.")
	} else {
		fmt.Fprintf(writer, "\n%s\n", markdownForOutput(writer, report.Markdown))
	}
	for _, failure := range report.Errors {
		fmt.Fprintf(writer, "\nError: %s\n", sanitizeTerminal(failure))
	}
	for _, warning := range report.Warnings {
		switch warning.Type {
		case "observation_error", "observation_truncated", "observation_replay_truncated":
			continue
		}
		fmt.Fprintf(writer, "Warning: %s\n", sanitizeTerminal(warning.Message))
	}
	fmt.Fprintf(writer, "\nAnalysis tokens: %d input + %d output\nArtifacts: %s\n", report.Usage.InputTokens, report.Usage.OutputTokens, sanitizeTerminal(report.ArtifactDir))
	outputRetryReportText(writer, report.RetryReport)
}
