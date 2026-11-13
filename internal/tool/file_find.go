// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"time"
)

const (
	fileFindMaxCount = 100
	fileFindTimeout  = 10 * time.Second
)

// FileFindProvider finds files by name or pattern in the repository at the selected version.
type FileFindProvider struct {
	FileReader *FileReader
	Access     *CodeAccess
}

func NewFileFind(fr *FileReader) *FileFindProvider { return &FileFindProvider{FileReader: fr} }

func (p *FileFindProvider) Tool() Tool { return FileFind }

func (p *FileFindProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return p.Access.execute(ctx, args, p.FileReader, func(reader *FileReader) (string, error) {
		selected := *p
		selected.FileReader = reader
		return selected.execute(ctx, args)
	})
}

func (p *FileFindProvider) execute(ctx context.Context, args map[string]any) (string, error) {
	queryName, _ := args["query_name"].(string)
	if strings.TrimSpace(queryName) == "" {
		return "// The file was not found", nil
	}

	caseSensitive, _ := args["case_sensitive"].(bool)
	// Normalize Windows-style backslash separators so subpath queries
	// (e.g. `pkg\util.go`) resolve identically on every platform.
	query := strings.ReplaceAll(queryName, "\\", "/")

	files, err := p.listFiles(ctx)
	if err != nil {
		return "", err
	}

	// Precompute the query comparison form once; the file list can be
	// large, so avoid recomputing strings.ToLower(query) on every iter.
	var queryCmp string
	if caseSensitive {
		queryCmp = query
	} else {
		queryCmp = strings.ToLower(query)
	}

	// 1. First pass: match against base filename (maintains precision for pure filename queries).
	var matched []string
	for _, f := range files {
		base := f
		if idx := strings.LastIndex(f, "/"); idx != -1 {
			base = f[idx+1:]
		}
		baseCmp := base
		if !caseSensitive {
			baseCmp = strings.ToLower(base)
		}
		if strings.Contains(baseCmp, queryCmp) {
			matched = append(matched, f)
		}
		if len(matched) >= fileFindMaxCount {
			break
		}
	}

	// 2. Fallback pass: if basename match returned no results, match against the full relative path
	// (enables directory-scoped queries such as `pkg/util` or `internal/diff`).
	if len(matched) == 0 {
		for _, f := range files {
			pathCmp := f
			if !caseSensitive {
				pathCmp = strings.ToLower(f)
			}
			if strings.Contains(pathCmp, queryCmp) {
				matched = append(matched, f)
			}
			if len(matched) >= fileFindMaxCount {
				break
			}
		}
	}

	if len(matched) == 0 {
		return "// The file was not found", nil
	}
	return strings.Join(matched, "\n"), nil
}

func (p *FileFindProvider) listFiles(parent context.Context) ([]string, error) {
	if p.FileReader.Mode == ModeEmpty {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(parent, fileFindTimeout)
	defer cancel()
	source, content, err := p.FileReader.fileSource()
	if err != nil {
		return nil, err
	}
	paths, err := source.ListFiles(ctx, content)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, path := range paths {
		if !shouldSkipFile(path) {
			files = append(files, path)
		}
	}
	return files, nil
}

// shouldSkipFile returns true if a repository path should be skipped.
// Keeps only widely useful files (those with recognizable extensions).
func shouldSkipFile(path string) bool {
	// Keep extensionless build/config files like Makefile, Dockerfile, LICENSE
	base := path
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		base = path[idx+1:]
	}
	hasExt := strings.Contains(base, ".")
	if !hasExt {
		// Allow well-known extensionless files
		switch base {
		case "Makefile", "Dockerfile", "LICENSE", "Vagrantfile", "Containerfile", "BUILD", "WORKSPACE":
			return false
		}
		return true // skip other extensionless files
	}
	return false
}
