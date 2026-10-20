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

// WithinBase reports whether target is base itself or contained under base.
func WithinBase(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		rel = ".."
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return true
	}

	return sameFileWithinBase(base, target)
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

// HomeEnvVar overrides where OCR keeps its config, rules and sessions.
const HomeEnvVar = "OCR_HOME"

// OCRHome returns the OCR home directory.
//
// It is $OCR_HOME when that is set, so a user can keep OCR out of $HOME, and
// ~/.opencodereview otherwise. The default is unchanged, so existing installs
// keep reading the config and sessions they already have.
func OCRHome() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(HomeEnvVar)); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".opencodereview"), nil
}
