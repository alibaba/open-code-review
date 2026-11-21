// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileRead_RejectsCredentialPath(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".env", "API_TOKEN=sk-live-123\n")
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "config"), ".env.production", "DB_PASSWORD=hunter2\n")

	p := NewFileRead(&FileReader{RepoDir: dir, Mode: ModeWorkspace})
	for _, path := range []string{".env", "./config/.env.production", "config\\.env.production"} {
		out, err := p.Execute(context.Background(), map[string]any{"file_path": path})
		if err != nil {
			t.Fatalf("file_read(%q) returned an error: %v", path, err)
		}
		if !strings.Contains(out, "excluded from review") {
			t.Errorf("file_read(%q) = %q, want a credential-path refusal", path, out)
		}
		for _, secret := range []string{"sk-live-123", "hunter2"} {
			if strings.Contains(out, secret) {
				t.Errorf("file_read(%q) leaked %q", path, secret)
			}
		}
	}
}

func TestCodeSearch_OmitsCredentialMatches(t *testing.T) {
	dir := setupTestRepo(t)
	writeTestFile(t, dir, ".env", "TOKEN=sk-live-123\nNAME=Hello\n")

	p := NewCodeSearch(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	out, err := p.gitGrep(context.Background(), "sk-live-123", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "sk-live-123") {
		t.Errorf("credential line reached the model: %s", out)
	}
	if !strings.Contains(out, "omitted") {
		t.Errorf("expected an omission note, got: %s", out)
	}

	out, err = p.gitGrep(context.Background(), "Hello", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "File: hello.go") {
		t.Errorf("expected the regular file match, got: %s", out)
	}
	if strings.Contains(out, "File: .env") {
		t.Errorf("credential file was rendered: %s", out)
	}

	out, err = p.Execute(context.Background(), map[string]any{"search_text": "sk-live-123"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "sk-live-123") {
		t.Errorf("Execute leaked the credential line: %s", out)
	}
}

func TestFileFind_OmitsCredentialPaths(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go", "package a\n")
	writeTestFile(t, dir, ".env", "TOKEN=sk-live-123\n")

	p := NewFileFind(&FileReader{RepoDir: dir, Mode: ModeWorkspace})

	out, err := p.Execute(context.Background(), map[string]any{"query_name": "env"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, ".env") {
		t.Errorf("file_find listed a credential path: %s", out)
	}

	out, err = p.Execute(context.Background(), map[string]any{"query_name": ".env"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("file_find should not resolve a credential path, got: %s", out)
	}
}
