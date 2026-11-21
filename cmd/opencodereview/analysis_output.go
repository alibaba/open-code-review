// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var analysisCitationPattern = regexp.MustCompile(`[ \t]*\[\[obs-[0-9]+\]\](?:[ \t]*\[\[obs-[0-9]+\]\])*`)

func outputAnalysisText(writer io.Writer, output *analysisOutput) {
	report := output.Report
	fmt.Fprintln(writer, "\n──────── Analysis Report ────────")
	oldSource, newSource := shortAnalysisRevision(report.Comparison.Old), shortAnalysisRevision(report.Comparison.Head)
	if report.Comparison.BaseIsEmpty {
		oldSource = "empty tree"
	}
	fmt.Fprintf(writer, "Comparison: %s -> %s\n", sanitizeTerminal(oldSource), sanitizeTerminal(newSource))
	if output.ReviewStatus == "partial" || output.ReviewStatus == "failed" {
		fmt.Fprintf(writer, "Review: %s\n", output.ReviewStatus)
	}
	if report.Status != "completed" {
		fmt.Fprintf(writer, "\nAnalysis: %s\n", sanitizeTerminal(report.Status))
	}
	stopReason := analysisStopReason(report.StopReason)
	if stopReason != "" {
		fmt.Fprintf(writer, "Reason: %s\n", sanitizeTerminal(stopReason))
	}
	for _, failure := range report.Errors {
		fmt.Fprintf(writer, "Error: %s\n", sanitizeTerminal(failure))
	}
	for _, warning := range report.Warnings {
		switch warning.Type {
		case "observation_error", "observation_truncated", "observation_replay_truncated":
			continue
		}
		if warning.Type == "behavior_limit_reached" && warning.Message == stopReason {
			continue
		}
		fmt.Fprintf(writer, "Warning: %s\n", sanitizeTerminal(warning.Message))
	}
	if report.Markdown == "" {
		fmt.Fprintln(writer, "\nNo analysis report was accepted.")
	} else {
		if report.DeclaredCompletion == "partial" {
			fmt.Fprintln(writer, "\nInvestigation incomplete (model self-assessment); see the report for unresolved questions.")
		}
		markdown := strings.TrimRight(analysisMarkdownForDisplay(report.Markdown), "\r\n")
		fmt.Fprintf(writer, "\n%s\n", markdownForOutput(writer, markdown))
	}
	if report.Status == "completed" {
		fmt.Fprintln(writer, "\nAnalysis: completed")
	}
	if usage := report.Usage; usage.TotalTokens > 0 {
		fmt.Fprintf(writer, "Analysis tokens: ~%d", usage.TotalTokens)
		if usage.InputTokens > 0 || usage.OutputTokens > 0 {
			fmt.Fprintf(writer, " (input: ~%d, output: ~%d)", usage.InputTokens, usage.OutputTokens)
		}
		if usage.CacheReadTokens > 0 || usage.CacheWriteTokens > 0 {
			fmt.Fprintf(writer, ", cache(read: ~%d, write: ~%d)", usage.CacheReadTokens, usage.CacheWriteTokens)
		}
		fmt.Fprintln(writer)
	}
	if report.ArtifactDir != "" && (report.Status == "failed" || report.Status == "partial" || report.DeclaredCompletion == "partial") {
		fmt.Fprintf(writer, "Evidence: %s\n", sanitizeTerminal(report.ArtifactDir))
	}
}

func shortAnalysisRevision(revision string) string {
	if len(revision) == 40 || len(revision) == 64 {
		if _, err := hex.DecodeString(revision); err == nil {
			return revision[:8]
		}
	}
	return revision
}

func analysisStopReason(reason string) string {
	if reason == "" || reason == "none" {
		return ""
	}
	for _, stop := range []llmloop.MainLoopStop{llmloop.StopMaxRounds, llmloop.StopEmptyRounds, llmloop.StopCompression, llmloop.StopTokenBudget} {
		if reason == stop.String() {
			return stop.Reason()
		}
	}
	return reason
}

func analysisMarkdownForDisplay(markdown string) string {
	if !analysisCitationPattern.MatchString(markdown) {
		return markdown
	}
	contentStarts, closingDelimiters := analysisMarkdownBoundaries(markdown)
	lines := strings.Split(markdown, "\n")
	offset := 0
	for lineIndex, line := range lines {
		lineOffset := offset
		offset += len(line) + 1
		matches := analysisCitationPattern.FindAllStringIndex(line, -1)
		if len(matches) == 0 {
			continue
		}
		var cleaned strings.Builder
		previousEnd := 0
		for _, match := range matches {
			start, end := match[0], match[1]
			cleaned.WriteString(line[previousEnd:start])
			markerStart := start + strings.Index(line[start:end], "[[")
			leading := line[start:markerStart]
			next, _ := utf8.DecodeRuneInString(line[end:])
			trimLeading := closingDelimiters[lineOffset+end] || strings.ContainsRune(" \t.,;:!?)]}\u3002\uff0c\uff1b\uff1a\uff01\uff1f\uff09\u3011", next)
			if contentStarts[lineOffset+markerStart] {
				cleaned.WriteString(leading)
				for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
					end++
				}
			} else if strings.TrimSpace(line[end:]) != "" && (start == 0 || !trimLeading) {
				cleaned.WriteString(leading)
			}
			previousEnd = end
		}
		cleaned.WriteString(line[previousEnd:])
		lines[lineIndex] = cleaned.String()
	}
	return strings.Join(lines, "\n")
}

func analysisMarkdownBoundaries(markdown string) (map[int]bool, map[int]bool) {
	source := []byte(markdown)
	document := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList)).Parser().Parse(text.NewReader(source))
	contentStarts := map[int]bool{}
	closingDelimiters := map[int]bool{}
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node.Type() == ast.TypeBlock {
			lines := node.Lines()
			for lineIndex := 0; lineIndex < lines.Len(); lineIndex++ {
				contentStarts[lines.At(lineIndex).Start] = true
			}
		}
		switch node.(type) {
		case *ast.Emphasis, *ast.CodeSpan, *extensionast.Strikethrough:
			if first, ok := node.FirstChild().(*ast.Text); ok {
				contentStarts[first.Segment.Start] = true
			}
			if last, ok := node.LastChild().(*ast.Text); ok {
				closingDelimiters[last.Segment.Stop] = true
			}
		}
		return ast.WalkContinue, nil
	})
	return contentStarts, closingDelimiters
}
