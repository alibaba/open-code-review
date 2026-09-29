// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
)

const fileReadMaxLines = 500

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
		if int(endLine) < int(startLine) {
			// Models sometimes derive start_line from the old side of a diff
			// hunk while end_line comes from the new file, or simply swap the
			// two. Serve the swapped range instead of failing the call; the
			// total-line count in the response lets the caller recover.
			startLine, endLine = endLine, startLine
		}
		if requested := int(endLine) - int(startLine) + 1; requested < maxLines {
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

	displayEnd := int(startLine) - 1 + len(lines)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (Total lines: %d)\n", filePath, totalLines))
	sb.WriteString(fmt.Sprintf("IS_TRUNCATED: %t\n", truncated))
	sb.WriteString(fmt.Sprintf("LINE_RANGE: %d-%d\n", int(startLine), displayEnd))
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%d|%s\n", int(startLine)+i, line))
	}
	if truncated {
		sb.WriteString(fmt.Sprintf("\nNote: Results truncated to %d lines. Please narrow your line range.\n", fileReadMaxLines))
	}
	return sb.String(), nil
}
