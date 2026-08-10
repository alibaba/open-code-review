// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellRCFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := shellRCFiles(); len(got) != 0 {
		t.Errorf("shellRCFiles() with no rc files = %v, want empty", got)
	}

	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("# empty\n"), 0o644); err != nil {
		t.Fatalf("write .zshrc: %v", err)
	}
	got := shellRCFiles()
	if len(got) != 1 || got[0] != zshrc {
		t.Errorf("shellRCFiles() = %v, want [%s]", got, zshrc)
	}
}

func TestTryShellRC(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No rc files: not found, no error.
	if _, ok, err := tryShellRC(""); ok || err != nil {
		t.Fatalf("tryShellRC() with no files = ok:%v err:%v, want false,nil", ok, err)
	}

	// A complete rc file yields a resolved endpoint.
	rc := "export ANTHROPIC_BASE_URL=\"https://example.test\"\n" +
		"export ANTHROPIC_AUTH_TOKEN='tok-123'\n" +
		"export ANTHROPIC_MODEL=claude-x\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0o644); err != nil {
		t.Fatalf("write .zshrc: %v", err)
	}

	ep, ok, err := tryShellRC("")
	if err != nil || !ok {
		t.Fatalf("tryShellRC() = ok:%v err:%v, want true,nil", ok, err)
	}
	if ep.Token != "tok-123" || ep.Model != "claude-x" {
		t.Errorf("resolved endpoint = %+v, want token tok-123 model claude-x", ep)
	}

	// modelOverride takes precedence.
	ep, ok, err = tryShellRC("override-model")
	if err != nil || !ok {
		t.Fatalf("tryShellRC(override) = ok:%v err:%v", ok, err)
	}
	if ep.Model != "override-model" {
		t.Errorf("model override not applied: %q", ep.Model)
	}
}
