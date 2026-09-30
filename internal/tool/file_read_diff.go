// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
)

const (
	// fileReadDiffMaxLines is the default per-call line budget for file_read_diff.
	fileReadDiffMaxLines = 500
	// fileReadDiffMaxLinesCap is the hard ceiling for a caller-requested max_lines.
	fileReadDiffMaxLinesCap = 5000
	// fileReadDiffMaxBytes is the per-call byte budget. It is a backstop for
	// pathologically long lines, such as minified sources, that a line budget
	// alone cannot bound.
	fileReadDiffMaxBytes = 1 << 20
)

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

// diffBlock records where one file's slice starts in the assembled output.
type diffBlock struct {
	header    string
	startLine int
}

// Execute returns the diff for the requested paths, bounded per call.
//
// The response carries at most maxLines lines (default fileReadDiffMaxLines,
// hard ceiling fileReadDiffMaxLinesCap) and fileReadDiffMaxBytes bytes. A
// bounded response reports IS_TRUNCATED and the NEXT_OFFSET the caller passes
// back as offset to keep paging; consecutive pages reassemble byte for byte
// into the unbounded output. A page that opens in the middle of a file repeats
// that file's header line so the page stays self-describing. A single diff line
// larger than the whole byte budget cannot be returned at all and is reported
// as an error instead of being silently dropped.
func (p *FileReadDiffProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	pathArray, _ := args["path_array"].([]any)
	if len(pathArray) == 0 {
		return "Error: no files found", nil
	}

	offset, err := parseToolInt(args, "offset", 0)
	if err != nil {
		return "", err
	}
	maxLines, err := parseToolInt(args, "max_lines", fileReadDiffMaxLines)
	if err != nil {
		return "", err
	}
	if offset < 0 {
		return "", fmt.Errorf("offset must be >= 0, got %d", offset)
	}
	if maxLines <= 0 {
		return "", fmt.Errorf("max_lines must be > 0, got %d", maxLines)
	}
	if maxLines > fileReadDiffMaxLinesCap {
		maxLines = fileReadDiffMaxLinesCap
	}

	full, blocks := p.assemble(pathArray)
	if full == "" {
		return "Error: diff not found for the requested paths", nil
	}
	// Every block ends with a newline, so the trailing empty element a split
	// would produce is not a line: the real line count is the newline count.
	total := strings.Count(full, "\n")

	var (
		sb       strings.Builder
		lineIdx  int
		emitted  int
		size     int
		blockIdx int
	)
	for pos := 0; pos < len(full); {
		nl := strings.IndexByte(full[pos:], '\n')
		line := full[pos : pos+nl]

		if lineIdx >= offset {
			// Stop at the requested page boundary before the next line is
			// validated: a line beyond the page belongs to a later request,
			// and failing on it here would discard the collected prefix.
			if emitted >= maxLines {
				break
			}
			// The next requested line cannot be emitted at all, so the
			// oversized-line error is only surfaced for content the caller
			// actually asked for.
			if len(line)+1 > fileReadDiffMaxBytes {
				return "", fmt.Errorf("the diff line at offset %d is %d bytes, over the %d-byte per-call limit; request fewer paths or read that file with file_read", lineIdx, len(line), fileReadDiffMaxBytes)
			}
			if size+len(line)+1 > fileReadDiffMaxBytes {
				break
			}
			for blockIdx+1 < len(blocks) && lineIdx >= blocks[blockIdx+1].startLine {
				blockIdx++
			}
			if emitted == 0 && lineIdx > blocks[blockIdx].startLine {
				sb.WriteString(blocks[blockIdx].header)
				sb.WriteByte('\n')
			}
			sb.WriteString(line)
			sb.WriteByte('\n')
			emitted++
			size += len(line) + 1
		}
		lineIdx++
		pos += nl + 1
	}

	if emitted == 0 {
		return "", fmt.Errorf("offset %d is past the end of the diff output (%d lines total)", offset, total)
	}

	truncated := offset+emitted < total
	var meta strings.Builder
	meta.WriteString(fmt.Sprintf("IS_TRUNCATED: %t\n", truncated))
	if truncated {
		meta.WriteString(fmt.Sprintf("NEXT_OFFSET: %d\n", offset+emitted))
	}
	return meta.String() + sb.String(), nil
}

// assemble concatenates the requested paths into the unbounded output and
// records the line index each file's block starts at.
func (p *FileReadDiffProvider) assemble(pathArray []any) (string, []diffBlock) {
	var (
		sb     strings.Builder
		blocks []diffBlock
		line   int
	)
	for _, item := range pathArray {
		path, ok := item.(string)
		if !ok {
			continue
		}
		diff, exists := p.diffMap.Get(path)
		if !exists {
			continue
		}
		header := "==== FILE: " + path + " ===="
		text := header + "\n" + diff + "\n"
		blocks = append(blocks, diffBlock{header: header, startLine: line})
		sb.WriteString(text)
		line += strings.Count(text, "\n")
	}
	return sb.String(), blocks
}

// parseToolInt reads an integer tool argument, falling back to def when the
// key is absent. JSON decoding yields float64; direct Go callers may pass int.
func parseToolInt(args map[string]any, key string, def int) (int, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return def, nil
	}
	switch v := raw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", key, raw)
	}
}
