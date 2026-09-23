// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"path/filepath"
	"testing"
)

func TestLLMTestCommand_UsesDefaultConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	original := runLLMTestPath
	t.Cleanup(func() { runLLMTestPath = original })
	var gotPath string
	runLLMTestPath = func(configPath string) error {
		gotPath = configPath
		return nil
	}

	if err := llmTestCmd.RunE(llmTestCmd, nil); err != nil {
		t.Fatalf("llm test: %v", err)
	}
	if want := filepath.Join(home, ".opencodereview", "config.json"); gotPath != want {
		t.Fatalf("config path = %q, want %q", gotPath, want)
	}
}
