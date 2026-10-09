// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Location is a validated local file and optional one-based line number.
type Location struct {
	Path string
	Line *int
}

// OpenLocalFile opens an existing regular file contained within root and returns
// its canonical path. The caller must close the file on success.
func OpenLocalFile(root, path string) (string, *os.File, error) {
	if !filepath.IsAbs(root) || path == "" || strings.ContainsAny(path, "\x00\r\n") {
		return "", nil, fmt.Errorf("invalid local file path")
	}
	if !filepath.IsAbs(path) {
		if strings.ContainsAny(path, `\:`) {
			return "", nil, fmt.Errorf("invalid local file path")
		}
		for _, part := range strings.Split(path, "/") {
			if part == ".." {
				return "", nil, fmt.Errorf("invalid local file path")
			}
		}
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", nil, err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, err
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("local file is outside root")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("path is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	info, err = f.Stat()
	if err != nil {
		f.Close()
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return "", nil, fmt.Errorf("path is not a regular file")
	}
	return path, f, nil
}

// ResolveLocation only adds navigation when the existing file and the full
// reported line range are valid. Callers must retain finding text on failure.
func ResolveLocation(root, path string, startLine, endLine int) *Location {
	canonical, f, err := OpenLocalFile(root, path)
	if err != nil {
		return nil
	}
	defer f.Close()
	location := &Location{Path: canonical}
	if startLine == 0 && endLine == 0 {
		return location
	}
	if startLine < 1 || endLine < startLine || !hasLine(f, endLine) {
		return nil
	}
	line := startLine
	location.Line = &line
	return location
}

// hasLine reads bounded fragments even for an arbitrarily long source line.
// A trailing newline terminates its line; it does not create another one.
func hasLine(r io.Reader, wanted int) bool {
	reader := bufio.NewReader(r)
	line := 1
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > 0 && line == wanted {
			return true
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return false
		}
		line++
	}
}
