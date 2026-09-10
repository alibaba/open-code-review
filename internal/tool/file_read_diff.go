// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
)

const fileReadDiffMaxLines = 500

// DiffMap is a read-only snapshot of parsed diffs, keyed by file path.
// Safe for concurrent reads after construction via NewDiffMap.
type DiffMap struct {
	m map[string]string
}

// NewDiffMap creates a frozen, read-only DiffMap from a plain map.
func NewDiffMap(m map[string]string) DiffMap {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return DiffMap{m: cp}
}

// Get returns the diff text for path.
func (d DiffMap) Get(path string) (string, bool) {
	v, ok := d.m[path]
	return v, ok
}

// FileReadDiffProvider retrieves diff content by file path from an already-parsed diff set.
type FileReadDiffProvider struct {
	diffMap DiffMap
}

func NewFileReadDiff(dm DiffMap) *FileReadDiffProvider {
	return &FileReadDiffProvider{diffMap: dm}
}

// SetDiffMap replaces the diff snapshot. Must be called before concurrent access begins.
func (p *FileReadDiffProvider) SetDiffMap(dm DiffMap) {
	p.diffMap = dm
}

func (p *FileReadDiffProvider) Tool() Tool { return FileReadDiff }

func (p *FileReadDiffProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	pathArray, _ := args["path_array"].([]any)
	if len(pathArray) == 0 {
		return "Error: no files found", nil
	}

	// start_line is a 1-based index into the diff lines of path_array taken
	// in order, file headers not counted, so a truncated result can be
	// continued from exactly where it stopped.
	skip := 0
	if v, ok := args["start_line"].(float64); ok && v > 1 {
		skip = int(v) - 1
	}

	var content strings.Builder
	totalDiffLines, shown := 0, 0
	truncated := false
	hasFoundDiff := false

outer:
	for _, item := range pathArray {
		path, ok := item.(string)
		if !ok {
			continue
		}
		d, exists := p.diffMap.Get(path)
		if !exists {
			continue
		}
		hasFoundDiff = true
		d = strings.TrimRight(d, "\n")
		if d == "" {
			// Binary files and pure renames have no diff body; list them on
			// the first page, as before the cap, without spending budget.
			if skip == 0 {
				fmt.Fprintf(&content, "==== FILE: %s ====\n", path)
			}
			continue
		}
		lines := strings.Split(d, "\n")
		from := max(skip-totalDiffLines, 0)
		totalDiffLines += len(lines)
		if from >= len(lines) {
			continue
		}
		if shown >= fileReadDiffMaxLines {
			truncated = true
			break
		}
		fmt.Fprintf(&content, "==== FILE: %s ====\n", path)
		for _, line := range lines[from:] {
			if shown >= fileReadDiffMaxLines {
				truncated = true
				break outer
			}
			content.WriteString(line)
			content.WriteString("\n")
			shown++
		}
	}

	if !hasFoundDiff {
		return "Error: diff not found for the requested paths", nil
	}
	if content.Len() == 0 {
		return fmt.Sprintf("Error: start_line %d is past the end of the diff (%d lines)", skip+1, totalDiffLines), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "IS_TRUNCATED: %t\n", truncated)
	sb.WriteString(content.String())
	if truncated {
		fmt.Fprintf(&sb, "\nNote: Results truncated to %d lines. Call file_read_diff again with the same path_array and start_line=%d to read the rest.\n", fileReadDiffMaxLines, skip+shown+1)
	}

	return sb.String(), nil
}
