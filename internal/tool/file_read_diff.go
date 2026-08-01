package tool

import (
	"context"
	"strings"
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

func (p *FileReadDiffProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	pathArray, _ := args["path_array"].([]any)
	if len(pathArray) == 0 {
		return "Error: no files found", nil
	}

	var sb strings.Builder
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
		}
	}

	result := sb.String()
	if result == "" {
		return "Error: diff not found for the requested paths", nil
	}
	return result, nil
}
