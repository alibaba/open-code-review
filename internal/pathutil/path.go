// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// CanonicalPath returns an absolute path with symlinks resolved.
func CanonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// LexicallyWithinBase reports whether the cleaned path is base or a descendant.
func LexicallyWithinBase(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	// Rel folds case on Windows even when the filesystem distinguishes those paths.
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		filepath.Join(base, rel) == filepath.Clean(target)
}

// WithinBase reports whether target is base itself or contained under base.
func WithinBase(base, target string) bool {
	return LexicallyWithinBase(base, target) || sameFileWithinBase(base, target)
}

func sameFileWithinBase(base, target string) bool {
	if !filepath.IsAbs(base) || !filepath.IsAbs(target) {
		return false
	}
	baseInfo, err := os.Stat(base)
	if err != nil {
		return false
	}
	for cur := target; ; {
		info, err := os.Stat(cur)
		if err == nil && os.SameFile(baseInfo, info) {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}
