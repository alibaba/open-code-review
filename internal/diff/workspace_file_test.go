package diff

import (
	"os"
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

func TestReadWorkspaceFileForDiffReadsRegularFile(t *testing.T) {
	repo := t.TempDir()
	want := "hello world\n"
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "sub", "file.txt"), []byte(want), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := readWorkspaceFileForDiff(repo, "sub/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestReadWorkspaceFileForDiffRejectsPathTraversal(t *testing.T) {
	repo := t.TempDir()

	_, err := readWorkspaceFileForDiff(repo, "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
	if !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("error = %q, want outside-repository message", err)
	}
}

func TestReadWorkspaceFileForDiffRejectsDirectory(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	_, err := readWorkspaceFileForDiff(repo, "subdir")
	if err == nil {
		t.Fatal("expected directory to be rejected")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("error = %q, want is-a-directory message", err)
	}
}

func TestReadWorkspaceFileForDiffRejectsParentSymlinkEscape(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()

	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "escape")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	_, err := readWorkspaceFileForDiff(repo, "escape/secret.txt")
	if err == nil {
		t.Fatal("expected parent-symlink escape to be rejected")
	}
	if !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("error = %q, want outside-repository message", err)
	}
}

func TestReadWorkspaceFileForDiffReturnsSymlinkTarget(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()

	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("TOP_SECRET\n"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(secretPath, filepath.Join(repo, "link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	got, err := readWorkspaceFileForDiff(repo, "link")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) == "TOP_SECRET\n" {
		t.Fatal("symlink target file content was read; expected target path instead")
	}
	if string(got) != secretPath {
		t.Fatalf("content = %q, want symlink target path %q", got, secretPath)
	}
}

func TestReadWorkspaceFileForDiffReadsInternalSymlink(t *testing.T) {
	repo := t.TempDir()
	realContent := "internal content\n"
	if err := os.WriteFile(filepath.Join(repo, "real.txt"), []byte(realContent), 0o644); err != nil {
		t.Fatalf("write real.txt: %v", err)
	}

	target := filepath.Join(repo, "real.txt")
	if err := os.Symlink(target, filepath.Join(repo, "internal_link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	got, err := readWorkspaceFileForDiff(repo, "internal_link")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != target {
		t.Fatalf("content = %q, want symlink target path %q", got, target)
	}
}

func TestReadWorkspaceFileForDiffRejectsNonexistentFile(t *testing.T) {
	repo := t.TempDir()

	_, err := readWorkspaceFileForDiff(repo, "does_not_exist.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "stat file") {
		t.Fatalf("error = %q, want stat-file message", err)
	}
}
