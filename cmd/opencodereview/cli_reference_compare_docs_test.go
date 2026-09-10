// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIReferenceDocumentsSessionCompare pins that every locale of the CLI
// reference documents `ocr session compare`. The four files are hand-synced
// (see PR #920), so the usual failure is a new command landing in `en` only.
// ponytail: substring checks, not a markdown parse - the whole point is to
// catch a missing file, and a parser would not catch it any better.
func TestCLIReferenceDocumentsSessionCompare(t *testing.T) {
	for _, locale := range []string{"en", "zh", "ja", "ru"} {
		t.Run(locale, func(t *testing.T) {
			path := filepath.Join("..", "..", "pages", "src", "content", "docs", locale, "cli-reference.md")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			for _, want := range []string{
				"`ocr session compare <before> <after>`", // command-summary table row
				"### `ocr session compare`",              // reference section
				"`ocr session diff <before> <after>`",    // alias
				"not_reviewed",                           // the JSON bucket that is easy to forget
			} {
				if !strings.Contains(string(body), want) {
					t.Errorf("%s: missing %q", path, want)
				}
			}
		})
	}
}
