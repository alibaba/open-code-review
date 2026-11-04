// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package workspace resolves OCR result roots and validates local file navigation.
package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ResultRoot follows the CLI contract: review paths are relative to the Git
// root; scan paths are relative to --repo, or the process cwd when omitted.
func ResultRoot(ctx context.Context, cwd string, args []string) (string, error) {
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
