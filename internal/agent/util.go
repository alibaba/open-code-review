// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// planBlockPattern matches the "Review Plan" section in a MAIN_TASK
// template user message: a header line beginning with "### " whose text
// contains "Review Plan" (with an optional "(Optional)" suffix), the
// {{plan_guidance}} placeholder on its own line, and one trailing blank line.
var planBlockPattern = regexp.MustCompile(
	`(?m)^### [^\n]*Review Plan[^\n]*\n\{\{plan_guidance\}\}\n\n?`)

// stripEmptyPlanBlock removes the "### Review Directive\n{{plan_guidance}}\n\n"
// wrapper from a MAIN_TASK user message when the plan phase produced no
// guidance. Strip is a no-op when the wrapper is absent.
func stripEmptyPlanBlock(content string) string {
	return planBlockPattern.ReplaceAllString(content, "")
}

// confirmedBlockPattern matches the "Previously Confirmed Findings" section:
// a header line containing "Confirmed Findings", the {{confirmed_comments}}
// placeholder on its own line, and one trailing blank line.
var confirmedBlockPattern = regexp.MustCompile(
	`(?m)^### [^\n]*Confirmed Findings[^\n]*\n\{\{confirmed_comments\}\}\n\n?`)

// stripEmptyConfirmedBlock removes the confirmed-findings wrapper when no
// round has produced confirmed comments yet (round 1). Must run before
// ReplaceAll of {{confirmed_comments}}.
func stripEmptyConfirmedBlock(content string) string {
	return confirmedBlockPattern.ReplaceAllString(content, "")
}

const (
	confirmedMaxExistingCode = 200
	confirmedMaxContent      = 300
	confirmedCap             = 30
)

// buildConfirmedCommentsBlock renders confirmed findings from prior rounds into
// a compact text block for injection into the next round's prompt. Returns ""
// for an empty slice, which triggers stripEmptyConfirmedBlock.
func buildConfirmedCommentsBlock(comments []model.LlmComment) string {
	if len(comments) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("The following issues were already identified and confirmed in a prior review pass. " +
		"Do not repeat them. " +
		"Continue reviewing all files in <review_files> and report any other real issues you find.\n\n")
	sb.WriteString("<confirmed_findings>\n")
	for i, cm := range comments {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, cm.Path))
		if cm.ExistingCode != "" {
			code := flattenOneLine(cm.ExistingCode)
			if runes := []rune(code); len(runes) > confirmedMaxExistingCode {
				code = string(runes[:confirmedMaxExistingCode]) + "..."
			}
			sb.WriteString(fmt.Sprintf("   code: %s\n", code))
		}
		content := flattenOneLine(cm.Content)
		if runes := []rune(content); len(runes) > confirmedMaxContent {
			content = string(runes[:confirmedMaxContent]) + "..."
		}
		sb.WriteString(fmt.Sprintf("   issue: %s\n", content))
	}
	sb.WriteString("</confirmed_findings>")
	return sb.String()
}

// flattenOneLine collapses a multi-line string into a single line.
func flattenOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

func reviewModeString(from, to, commit string) string {
	if commit != "" {
		return session.ReviewModeCommit
	}
	if from != "" && to != "" {
		return session.ReviewModeRange
	}
	return session.ReviewModeWorkspace
}

// detectGitBranch returns the current git branch name for the given repo, or empty string on failure.
func detectGitBranch(ctx context.Context, repoDir string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}
