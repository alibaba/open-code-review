// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"sort"
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

// diffNotFoundMaxListed bounds how many available paths the not-found message
// names before it summarizes the rest. The list exists to let a model correct a
// wrong path, so it has to stay short enough to read in one glance.
const diffNotFoundMaxListed = 10

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

// Paths returns the snapshot's paths in sorted order, so a message built from
// it reads the same way on every run.
func (d DiffMap) Paths() []string {
	paths := make([]string, 0, len(d.m))
	for p := range d.m {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
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
//
// A requested path that has no diff does not shorten the page: the paths that
// were served are returned as usual and the missing ones are named after them,
// so the caller learns which path to correct instead of losing the diffs it
// did get.
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

	full, blocks, missing := p.assemble(pathArray)
	if full == "" {
		if len(missing) > 0 {
			return p.notFoundMessage(missing), nil
		}
		return "Error: no files found", nil
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
	out := meta.String() + sb.String()
	if len(missing) > 0 {
		// The missing-path notice follows the diffs it qualifies, so a partial
		// hit reads as the diff first and the reason it is short second.
		out += "\n" + p.notFoundMessage(missing)
	}
	return out, nil
}

// assemble concatenates the requested paths into the unbounded output and
// records the line index each file's block starts at. The paths without a diff
// are returned separately: they are not part of the output, but the caller
// still has to be told which ones were dropped.
func (p *FileReadDiffProvider) assemble(pathArray []any) (string, []diffBlock, []string) {
	var (
		sb      strings.Builder
		blocks  []diffBlock
		line    int
		missing []string
	)
	for _, item := range pathArray {
		path, ok := item.(string)
		if !ok {
			continue
		}
		diff, exists := p.diffMap.Get(path)
		if !exists {
			missing = append(missing, path)
			continue
		}
		header := "==== FILE: " + path + " ===="
		text := header + "\n" + diff + "\n"
		blocks = append(blocks, diffBlock{header: header, startLine: line})
		sb.WriteString(text)
		line += strings.Count(text, "\n")
	}
	return sb.String(), blocks, missing
}

// notFoundMessage names the paths that had no diff and, when the snapshot
// holds other paths, names those too: a request for a path outside the change
// set reads exactly like a diff that does not exist, and only the list of paths
// that do have diffs lets the model correct the request instead of re-reading
// the same file under another spelling.
//
// Naming file_read_diff here would send the model back to the call that just
// failed, so the way to reach a file outside the change set is file_read.
func (p *FileReadDiffProvider) notFoundMessage(missing []string) string {
	var sb strings.Builder
	sb.WriteString("Error: diff not found for: ")
	sb.WriteString(strings.Join(missing, ", "))
	sb.WriteString("\n")

	available := p.diffMap.Paths()
	if len(available) == 0 {
		// Nothing to point at. State only what the snapshot says and leave the
		// mode out of it: the message cannot tell why the snapshot is empty.
		sb.WriteString("This run has no diffs. Read the file with file_read instead.\n")
		return sb.String()
	}

	listed := available
	if len(listed) > diffNotFoundMaxListed {
		listed = listed[:diffNotFoundMaxListed]
	}
	sb.WriteString("Diffs exist for: ")
	sb.WriteString(strings.Join(listed, ", "))
	if rest := len(available) - len(listed); rest > 0 {
		fmt.Fprintf(&sb, " (and %d more)", rest)
	}
	sb.WriteString("\nUse one of those paths, or file_read for a file outside the reviewed set.\n")
	return sb.String()
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
