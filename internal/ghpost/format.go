// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/model"
)

const (
	summaryFragmentBytes = 4 * 1024
	categoryBadgeColor   = "blue"
)

var severityBadgeColors = map[string]string{
	"critical": "darkred",
	"high":     "red",
	"medium":   "orange",
	"low":      "green",
}

func buildBadgeImage(comment model.LlmComment) string {
	category := sanitizeMetadata(comment.Category)
	severity := sanitizeMetadata(comment.Severity)
	if category == "" && severity == "" {
		return ""
	}
	alt := category
	if category != "" && severity != "" {
		alt = category + " · " + severity
	} else if severity != "" {
		alt = severity
	}
	normalizedCategory := strings.ToLower(strings.TrimSpace(category))
	normalizedSeverity := strings.ToLower(strings.TrimSpace(severity))
	color := severityBadgeColors[normalizedSeverity]
	if color == "" {
		color = categoryBadgeColor
	}
	label := shieldsEscape(normalizedCategory)
	if normalizedCategory != "" && normalizedSeverity != "" {
		label += "-" + shieldsEscape(normalizedSeverity)
	} else if normalizedSeverity != "" {
		label = shieldsEscape(normalizedSeverity)
	}
	return fmt.Sprintf("![%s](https://img.shields.io/badge/%s-%s)", escapeMarkdownAlt(alt), label, color)
}

func sanitizeMetadata(value string) string {
	return strings.Map(func(r rune) rune {
		if r <= 0x1f || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, value)
}

func shieldsEscape(value string) string {
	value = strings.ReplaceAll(value, "-", "--")
	value = strings.ReplaceAll(value, "_", "__")
	return url.PathEscape(value)
}

func escapeMarkdownAlt(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "[", "\\[")
	return strings.ReplaceAll(value, "]", "\\]")
}

func formatCommentBody(comment model.LlmComment) string {
	var body strings.Builder
	if badge := buildBadgeImage(comment); badge != "" {
		body.WriteString(badge)
		body.WriteString("\n\n")
	}
	body.WriteString(comment.Content)
	if comment.SuggestionCode == "" {
		return body.String()
	}
	if strings.Contains(comment.SuggestionCode, "```") {
		body.WriteString("\n\n**Suggested code:**\n\n")
		for _, line := range strings.Split(comment.SuggestionCode, "\n") {
			body.WriteString("    ")
			body.WriteString(line)
			body.WriteByte('\n')
		}
		return body.String()
	}
	body.WriteString("\n\n**Suggestion:**\n```suggestion\n")
	body.WriteString(comment.SuggestionCode)
	if !strings.HasSuffix(comment.SuggestionCode, "\n") {
		body.WriteByte('\n')
	}
	body.WriteString("```")
	return body.String()
}

func splitSummaryText(text string) []string {
	if text == "" {
		return []string{""}
	}
	parts := make([]string, 0, (len(text)+summaryFragmentBytes-1)/summaryFragmentBytes)
	for len(text) > summaryFragmentBytes {
		end := summaryFragmentBytes
		for end > 0 && !utf8.ValidString(text[:end]) {
			end--
		}
		if end == 0 {
			end = summaryFragmentBytes
		}
		parts = append(parts, text[:end])
		text = text[end:]
	}
	return append(parts, text)
}
