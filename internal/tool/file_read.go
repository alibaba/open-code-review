// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

const fileReadMaxLines = 500

const fileReadNotFoundMessage = "Use file_find to locate the exact path, then retry file_read."

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
		errText := strings.ToLower(err.Error())
		if errors.Is(err, os.ErrNotExist) || strings.Contains(errText, "does not exist in") || strings.Contains(errText, "exists on disk, but not in") {
			return "", fmt.Errorf(`file not found: %q. %s`, filePath, fileReadNotFoundMessage)
		}
		return "", fmt.Errorf("could not read file %q: %w", filePath, err)
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
