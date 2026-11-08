// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

const fileReadMaxLines = 500

// Byte bounds for the rendered tool result. A line-count cap alone cannot
// bound the result: committed minified or generated files (bundles, lockfiles,
// JSON exports) pack megabytes into one line, and 500 capped lines still add
// up.
const (
	// fileReadMaxLineBytes caps one rendered line, cutting at a UTF-8 rune
	// boundary with an explicit omission marker.
	fileReadMaxLineBytes = 2048
	// fileReadMaxResultBytes caps the total bytes a rendered result may hold,
	// counted over the fully formatted lines (line-number prefix, content and
	// trailing newline) after the per-line cap.
	fileReadMaxResultBytes = 256 * 1024
)

// FileReadProvider reads file content at a given path and optional line range.
type FileReadProvider struct {
	FileReader *FileReader
}

func NewFileRead(fr *FileReader) *FileReadProvider { return &FileReadProvider{FileReader: fr} }

func (p *FileReadProvider) Tool() Tool { return FileRead }

func (p *FileReadProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return "Error: file_path is required", nil
	}

	startLine, hasStart := args["start_line"].(float64)
	endLine, hasEnd := args["end_line"].(float64)
	if !hasStart || startLine <= 0 {
		startLine = 1
	}
	if !hasEnd || endLine <= 0 {
		endLine = 0
	}

	maxLines := fileReadMaxLines
	if endLine > 0 {
		requested := int(endLine) - int(startLine) + 1
		if requested <= 0 {
			return "", fmt.Errorf("invalid line range: start_line %d is greater than end_line %d", int(startLine), int(endLine))
		}
		if requested < maxLines {
			maxLines = requested
		}
	}

	lines, totalLines, err := p.FileReader.ReadLines(ctx, filePath, int(startLine), maxLines)
	if err != nil {
		return "", fmt.Errorf("file %q not found: %w", filePath, err)
	}

	if totalLines > 0 && int(startLine)-1 >= totalLines {
		return "", fmt.Errorf("file %q has only %d lines, requested range %d-%d", filePath, totalLines, int(startLine), int(endLine))
	}

	effectiveEnd := totalLines
	if endLine > 0 && int(endLine) < effectiveEnd {
		effectiveEnd = int(endLine)
	}
	fullRange := effectiveEnd - (int(startLine) - 1)
	truncated := fullRange > fileReadMaxLines

	// Render the lines under the byte bounds before assembling the header, so
	// LINE_RANGE and IS_TRUNCATED always describe the lines actually shown.
	var body strings.Builder
	shown, renderedBytes := 0, 0
	lineTruncated, outputTruncated := false, false
	for i, line := range lines {
		text, cut := capFileReadLine(line)
		if cut {
			lineTruncated = true
		}
		// Budget the fully formatted line: the line-number prefix and the
		// trailing newline are body bytes too, so counting only len(text)
		// would let the rendered body drift past the cap.
		entry := fmt.Sprintf("%d|%s\n", int(startLine)+i, text)
		if renderedBytes > 0 && renderedBytes+len(entry) > fileReadMaxResultBytes {
			outputTruncated = true
			break
		}
		renderedBytes += len(entry)
		shown = i + 1
		body.WriteString(entry)
	}
	truncated = truncated || lineTruncated || outputTruncated

	displayEnd := int(startLine) - 1 + shown

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (Total lines: %d)\n", filePath, totalLines))
	sb.WriteString(fmt.Sprintf("IS_TRUNCATED: %t\n", truncated))
	sb.WriteString(fmt.Sprintf("LINE_RANGE: %d-%d\n", int(startLine), displayEnd))
	sb.WriteString(body.String())
	if lineTruncated {
		sb.WriteString(fmt.Sprintf("\nNote: Lines longer than %d bytes were cut mid-line and the omitted bytes are not part of this result. Reason about the visible part only.\n", fileReadMaxLineBytes))
	}
	if outputTruncated {
		sb.WriteString(fmt.Sprintf("\nNote: Output truncated at %d bytes. Re-read the remaining lines with a narrower start_line/end_line range.\n", fileReadMaxResultBytes))
	}
	if fullRange > fileReadMaxLines {
		sb.WriteString(fmt.Sprintf("\nNote: Results truncated to %d lines. Please narrow your line range.\n", fileReadMaxLines))
	}
	return sb.String(), nil
}

// capFileReadLine caps one rendered line at fileReadMaxLineBytes. The cut
// backs off to a UTF-8 rune boundary so a multibyte character is never split
// mid-sequence, and the marker reports the omitted byte count so the model
// knows the line was not shown in full.
func capFileReadLine(s string) (string, bool) {
	if len(s) <= fileReadMaxLineBytes {
		return s, false
	}
	cut := fileReadMaxLineBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s …[line truncated, %d more bytes omitted]", s[:cut], len(s)-cut), true
}
