package diff

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWorkspaceFileForDiffRejectsAbsolutePath(t *testing.T) {
	repo := t.TempDir()
	absPath := filepath.Join(repo, "file.txt")

	_, err := readWorkspaceFileForDiff(repo, absPath)
	if err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
	if !strings.Contains(err.Error(), "must be relative") {
		t.Fatalf("error = %q, want relative-path message", err)
	}
}
