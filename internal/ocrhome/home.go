// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package ocrhome resolves the user-level storage directory shared by OCR commands.
package ocrhome

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns OCR_HOME when non-empty, otherwise ~/.opencodereview.
// Overrides must be absolute.
// It does not create the directory or migrate existing data.
func Dir() (string, error) {
	if dir := os.Getenv("OCR_HOME"); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("OCR_HOME must be an absolute path: %q", dir)
		}
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".opencodereview"), nil
}
