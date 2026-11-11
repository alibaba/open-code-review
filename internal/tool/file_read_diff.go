// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

func (p *FileReadDiffProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	pathArray, _ := args["path_array"].([]any)
	if len(pathArray) == 0 {
		return "Error: no files found", nil
	}

	var sb strings.Builder
	var missing []string
	for _, item := range pathArray {
		path, ok := item.(string)
		if !ok {
			continue
		}
		if d, exists := p.diffMap.Get(path); exists {
			sb.WriteString("==== FILE: ")
			sb.WriteString(path)
			sb.WriteString(" ====\n")
			sb.WriteString(d)
			sb.WriteString("\n")
			continue
		}
		missing = append(missing, path)
	}

	if len(missing) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(p.notFoundMessage(missing))
	}

	result := sb.String()
	if result == "" {
		return "Error: diff not found for the requested paths", nil
	}
	return result, nil
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
