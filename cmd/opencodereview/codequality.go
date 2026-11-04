// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"io"
	"strconv"

	"github.com/alibaba/open-code-review/internal/model"
)

// --- GitLab Code Quality report (Code Climate issue subset) ---
//
// GitLab renders a Code Quality artifact natively in the merge request
// widget (every tier; diff annotations need Ultimate), with no API token and
// no posting script. The report is a JSON array of issues; GitLab requires
// description, check_name, fingerprint, severity, location.path and
// location.lines.begin on every entry.
// Spec: https://docs.gitlab.com/ci/testing/code_quality/#code-quality-report-format

type codeQualityIssue struct {
	Type        string              `json:"type"`
	CheckName   string              `json:"check_name"`
	Description string              `json:"description"`
	Categories  []string            `json:"categories"`
	Severity    string              `json:"severity"`
	Fingerprint string              `json:"fingerprint"`
	Location    codeQualityLocation `json:"location"`
}

type codeQualityLocation struct {
	Path  string           `json:"path"`
	Lines codeQualityLines `json:"lines"`
}

type codeQualityLines struct {
	Begin int `json:"begin"`
	End   int `json:"end,omitempty"`
}

// outputCodeQuality writes a GitLab Code Quality report. Findings without a
// file path are omitted because GitLab rejects issues lacking location.path;
// they remain available through --format json. An empty review produces "[]"
// (never "null"), which GitLab reads as "no new issues".
func outputCodeQuality(comments []model.LlmComment, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(codeQualityIssues(comments))
}

func codeQualityIssues(comments []model.LlmComment) []codeQualityIssue {
	issues := make([]codeQualityIssue, 0, len(comments))
	seen := make(map[string]int, len(comments))
	for _, c := range comments {
		if c.Path == "" {
			continue
		}
		category := c.Category
		if category == "" {
			category = "other"
		}
		// Reuse the SARIF fingerprint so both reports track a finding with the
		// same identity across runs (path + category + code, not LLM prose).
		fp := sarifFingerprints(c, category)[sarifFingerprintKey]
		n := seen[fp]
		seen[fp] = n + 1
		if n > 0 {
			fp += "#" + strconv.Itoa(n)
		}

		begin, end := 1, 0
		if c.StartLine > 0 {
			begin = c.StartLine
			if c.EndLine > c.StartLine {
				end = c.EndLine
			}
		}

		issues = append(issues, codeQualityIssue{
			Type:        "issue",
			CheckName:   "ocr/" + category,
			Description: c.Content,
			Categories:  []string{codeQualityCategory(category)},
			Severity:    codeQualitySeverity(c.Severity),
			Fingerprint: fp,
			Location: codeQualityLocation{
				Path:  c.Path,
				Lines: codeQualityLines{Begin: begin, End: end},
			},
		})
	}
	return issues
}

// codeQualitySeverity maps OCR severities onto GitLab's five levels
// (info, minor, major, critical, blocker). "blocker" is reserved for tools
// that gate merges on it, so OCR never emits it. Unknown values fall back to
// "info", mirroring the SARIF mapping that downgrades unknown to "note".
func codeQualitySeverity(severity string) string {
	switch severity {
	case "critical":
		return "critical"
	case "high":
		return "major"
	case "medium":
		return "minor"
	default:
		return "info"
	}
}

// codeQualityCategory maps OCR categories onto the Code Climate category
// vocabulary GitLab understands.
func codeQualityCategory(category string) string {
	switch category {
	case "bug", "test":
		return "Bug Risk"
	case "security":
		return "Security"
	case "performance":
		return "Performance"
	case "maintainability":
		return "Complexity"
	case "style":
		return "Style"
	case "documentation":
		return "Clarity"
	default:
		return "Bug Risk"
	}
}
