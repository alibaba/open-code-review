// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	acp "github.com/coder/acp-go-sdk"
)

// resultRoot follows the CLI contract: review paths are relative to the Git
// root; scan paths are relative to --repo, or the process cwd when omitted.
func resultRoot(ctx context.Context, cwd string, args []string) (string, error) {
	if !filepath.IsAbs(cwd) || len(args) == 0 {
		return "", fmt.Errorf("invalid result directory or operation")
	}
	root := cwd
	switch args[0] {
	case "review":
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(probe, "git", "-C", root, "rev-parse", "--show-toplevel")
		cmd.WaitDelay = time.Second
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("resolve review root: %w", err)
		}
		root = filepath.Clean(strings.TrimSpace(string(out)))
	case "scan":
	default:
		return "", fmt.Errorf("unsupported result operation")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("result root is not a directory")
	}
	return canonical, nil
}

// openLocalFile opens an existing regular file contained within root and returns
// its canonical path. The caller must close the file on success.
func openLocalFile(root, path string) (string, *os.File, error) {
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

// findingLocation only adds navigation when the existing file and the full
// reported line range are valid. Callers must retain finding text on failure.
func findingLocation(root string, comment contract.Comment) *acp.ToolCallLocation {
	path, f, err := openLocalFile(root, comment.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	location := &acp.ToolCallLocation{Path: path}
	if comment.StartLine == 0 && comment.EndLine == 0 {
		return location
	}
	if comment.StartLine < 1 || comment.EndLine < comment.StartLine || !hasLine(f, comment.EndLine) {
		return nil
	}
	line := comment.StartLine
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
