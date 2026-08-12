// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testBaseSHA = "0123456789abcdef0123456789abcdef01234567"

func TestParseBaseStateExternalRef(t *testing.T) {
	state, err := parseBaseState("base_sha: " + testBaseSHA + "\nsource: github\nref: https://github.com/org/repo/issues/123\n")
	if err != nil {
		t.Fatalf("parseBaseState: %v", err)
	}
	if state.BaseSHA != testBaseSHA || state.Source != "github" || state.Ref == "" || state.Summary != "" {
		t.Fatalf("state = %+v", state)
	}
}

func TestParseBaseStateUserSummary(t *testing.T) {
	state, err := parseBaseState("base_sha: " + testBaseSHA + "\nsource: user\nsummary: implement rate limiting\n")
	if err != nil {
		t.Fatalf("parseBaseState: %v", err)
	}
	if state.Source != "user" || state.Summary != "implement rate limiting" {
		t.Fatalf("state = %+v", state)
	}
}

func TestParseBaseStateRejectsInvalidForms(t *testing.T) {
	cases := map[string]string{
		"missing baseline":  "source: github\nref: issue\n",
		"short sha":         "base_sha: abc\nsource: github\nref: issue\n",
		"unknown field":     "base_sha: " + testBaseSHA + "\nsource: github\nref: issue\nextra: value\n",
		"duplicate field":   "base_sha: " + testBaseSHA + "\nsource: github\nsource: gitea\nref: issue\n",
		"both source forms": "base_sha: " + testBaseSHA + "\nsource: github\nref: issue\nsummary: text\n",
		"user ref":          "base_sha: " + testBaseSHA + "\nsource: user\nref: issue\n",
		"blank line":        "base_sha: " + testBaseSHA + "\n\nsource: github\nref: issue\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseBaseState(raw); err == nil {
				t.Fatal("expected invalid base state")
			}
		})
	}
}

func TestLoadBaseStateRequiresScratchFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadBaseState(dir); err == nil || !strings.Contains(err.Error(), baseStateRelativePath) {
		t.Fatalf("loadBaseState error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, ".scratch"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baseStatePath(dir), []byte("base_sha: "+testBaseSHA+"\nsource: user\nsummary: task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBaseState(dir); err != nil {
		t.Fatalf("loadBaseState: %v", err)
	}
}
