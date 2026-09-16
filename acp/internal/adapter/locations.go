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
	if !filepath.IsAbs(root) {
		root = filepath.Join(cwd, root)
	}
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
		root = strings.TrimSuffix(string(out), "\n")
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

// findingLocation only adds navigation when the existing file and the full
// reported line range are valid. Callers must retain finding text on failure.
func findingLocation(root string, comment contract.Comment) *acp.ToolCallLocation {
	if !filepath.IsAbs(root) || comment.Path == "" || strings.ContainsAny(comment.Path, "\x00\r\n\\") || strings.Contains(comment.Path, ":") {
		return nil
	}
	for _, part := range strings.Split(comment.Path, "/") {
		if part == ".." {
			return nil
		}
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil
	}
	path := comment.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
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
