// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRulesListsEveryDefaultExcludePattern(t *testing.T) {
	patterns := readPatternFile(t, "default_exclude_patterns.json")
	// The same page also documents the secret-path list, which is a separate
	// file, so those entries are expected and must not be reported as strays.
	known := make(map[string]bool)
	for _, p := range append(append([]string{}, patterns...), readPatternFile(t, "default_secret_patterns.json")...) {
		known[p] = true
	}

	for _, locale := range []string{"en", "zh", "ja", "ru", "ko"} {
		t.Run(locale, func(t *testing.T) {
			path := filepath.Join("..", "..", "pages", "src", "content", "docs", locale, "review-rules.md")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			listed := make(map[string]bool)
			for _, line := range strings.Split(string(body), "\n") {
				if entry, ok := strings.CutPrefix(strings.TrimSpace(line), "- `"); ok {
					if entry, ok := strings.CutSuffix(entry, "`"); ok {
						listed[entry] = true
					}
				}
			}
			for _, p := range patterns {
				if !listed[p] {
					t.Errorf("%s does not list %q", path, p)
				}
			}
			for entry := range listed {
				if known[entry] {
					continue
				}
				if strings.HasPrefix(entry, "**/") || strings.HasPrefix(entry, "lib/**/") {
					t.Errorf("%s lists %q, which is not in default_exclude_patterns.json", path, entry)
				}
			}
		})
	}
}

func readPatternFile(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "allowlist", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return out
}
