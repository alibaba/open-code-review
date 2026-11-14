// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const fileReadMaxLines = 500

// numericToolArg reads an optional numeric tool argument. JSON decodes numbers
// as float64, but a model (or a gateway forwarding its raw completion text)
// can also send the digits as a string. Treating that string as "absent" would
// silently answer a different question than the one asked, so it is parsed,
// and a non-numeric value is an explicit error the model can act on.
func numericToolArg(args map[string]any, name string) (float64, bool, error) {
	value, present := args[name]
	if !present || value == nil {
		return 0, false, nil
	}
	switch v := value.(type) {
	case float64:
		return v, true, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false, nil
		}
		parsed, parseErr := strconv.ParseFloat(trimmed, 64)
		if parseErr != nil {
			return 0, false, fmt.Errorf("%s must be a number, got %q", name, v)
		}
		return parsed, true, nil
	default:
		return 0, false, fmt.Errorf("%s must be a number, got %T", name, value)
	}
}

// FileReadProvider reads file content at a given path and optional line range.
type FileReadProvider struct {
	FileReader *FileReader
	Access     *CodeAccess
}

func NewFileRead(fr *FileReader) *FileReadProvider { return &FileReadProvider{FileReader: fr} }

func (p *FileReadProvider) Tool() Tool { return FileRead }

func (p *FileReadProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return p.Access.execute(ctx, args, p.FileReader, func(reader *FileReader) (string, error) {
		selected := *p
		selected.FileReader = reader
		return selected.execute(ctx, args)
	})
}

func (p *FileReadProvider) execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return "Error: file_path is required", nil
	}

	startLine, hasStart, err := numericToolArg(args, "start_line")
	if err != nil {
		return "", err
	}
	endLine, hasEnd, err := numericToolArg(args, "end_line")
	if err != nil {
		return "", err
	}
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
