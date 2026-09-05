// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitOutNoFail(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestPatchProviderReadsPatchDirectoryInOrder(t *testing.T) {
	repo := t.TempDir()
	patchDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "diff --git a/a.go b/a.go\nindex 123..456 100644\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,2 @@\n package a\n+func A() {}\n"
	if err := os.WriteFile(filepath.Join(patchDir, "002.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := NewPatchProvider(repo, patchDir, "", nil)
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].NewPath != "a.go" || diffs[0].Insertions != 1 {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}
}

func TestPatchProviderRejectsEmptyDirectory(t *testing.T) {
	provider := NewPatchProvider(t.TempDir(), t.TempDir(), "", nil)
	if _, err := provider.GetDiff(context.Background()); err == nil {
		t.Fatal("expected empty patch directory error")
	}
}

func TestValidatePatchDirectory(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		if err := ValidatePatchDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("expected missing patch directory error")
		}
	})
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "change.patch")
		if err := os.WriteFile(path, []byte("patch"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidatePatchDirectory(path); err == nil {
			t.Fatal("expected non-directory patch path error")
		}
	})
	t.Run("no patches", func(t *testing.T) {
		if err := ValidatePatchDirectory(t.TempDir()); err == nil {
			t.Fatal("expected directory without patch files error")
		}
	})
	t.Run("valid", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "change.diff"), []byte("patch"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidatePatchDirectory(dir); err != nil {
			t.Fatalf("ValidatePatchDirectory: %v", err)
		}
	})
}

func TestPatchProviderRejectsNonEmptyUnparseablePatch(t *testing.T) {
	patchDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(patchDir, "invalid.patch"), []byte("not a unified diff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := NewPatchProvider(t.TempDir(), patchDir, "", nil)
	if _, err := provider.GetDiff(context.Background()); err == nil {
		t.Fatal("expected non-empty unparseable patch error")
	}
}

func TestPatchProviderReadsPostImageFromRef(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.go", "package a\n\nfunc A() {}\n", "post-image")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patchDir := t.TempDir()
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,3 @@\n package a\n+\n+func A() {}\n"
	if err := os.WriteFile(filepath.Join(patchDir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := NewPatchProvider(repo, patchDir, head, nil)
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if got, want := diffs[0].NewFileContent, "package a\n\nfunc A() {}\n"; got != want {
		t.Fatalf("NewFileContent = %q, want committed post-image %q", got, want)
	}
	if got := provider.ResolveInput(context.Background()).ResolvedHead; got != head {
		t.Fatalf("ResolvedHead = %q, want %q", got, head)
	}
}

func TestPatchProviderWithoutRefReadsWorkspacePostImage(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.go", "package old\n", "base")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "added.go"), []byte("package added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patchDir := t.TempDir()
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-package old\n+package new\n"
	if err := os.WriteFile(filepath.Join(patchDir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := NewPatchProvider(repo, patchDir, "", nil)
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if got := diffs[0].NewFileContent; got != "package new\n" {
		t.Fatalf("NewFileContent = %q, want workspace post-image", got)
	}
}

func TestMaterializePatchCommit(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.go", "package a\n", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("dirty workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patchDir := t.TempDir()
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,3 @@\n package a\n+\n+func A() {}\ndiff --git a/new.go b/new.go\nnew file mode 100644\n--- /dev/null\n+++ b/new.go\n@@ -0,0 +1 @@\n+package a\n"
	if err := os.WriteFile(filepath.Join(patchDir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	postImage, err := MaterializePatchCommit(context.Background(), repo, patchDir, base, nil)
	if err != nil {
		t.Fatalf("MaterializePatchCommit: %v", err)
	}
	if got := gitOut(t, repo, "rev-parse", "HEAD"); got != base {
		t.Fatalf("HEAD moved to %q, want base %q", got, base)
	}
	if got := gitOut(t, repo, "show", postImage+":a.go"); got != "package a\n\nfunc A() {}" {
		t.Fatalf("post-image a.go = %q", got)
	}
	if got := gitOut(t, repo, "show", postImage+":new.go"); got != "package a" {
		t.Fatalf("post-image new.go = %q", got)
	}
	content, err := os.ReadFile(filepath.Join(repo, "a.go"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "dirty workspace\n" {
		t.Fatalf("working tree changed to %q", got)
	}
}

func TestMaterializePatchCommitRejectsPatchForWrongBase(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.go", "package a\n", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	patchDir := t.TempDir()
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-package wrong\n+package changed\n"
	if err := os.WriteFile(filepath.Join(patchDir, "bad.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := MaterializePatchCommit(context.Background(), repo, patchDir, base, nil); err == nil {
		t.Fatal("expected patch application to fail against the wrong base")
	}
}

func TestMaterializePatchCommitOverridesInheritedIndexFile(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.go", "package a\n", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "a.go")
	indexPath := filepath.Join(repo, ".git", "index")
	originalIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	originalIndexFile, hadIndexFile := os.LookupEnv("GIT_INDEX_FILE")
	if err := os.Setenv("GIT_INDEX_FILE", indexPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadIndexFile {
			_ = os.Setenv("GIT_INDEX_FILE", originalIndexFile)
		} else {
			_ = os.Unsetenv("GIT_INDEX_FILE")
		}
	})

	patchDir := t.TempDir()
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,2 @@\n package a\n+func A() {}\n"
	if err := os.WriteFile(filepath.Join(patchDir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializePatchCommit(context.Background(), repo, patchDir, base, nil); err != nil {
		t.Fatalf("MaterializePatchCommit: %v", err)
	}
	afterIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterIndex) != string(originalIndex) {
		t.Fatal("inherited repository index was modified")
	}
}

func TestMaterializePatchCommitSkipsExternalBinarySections(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.go", "package a\n", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	patchDir := t.TempDir()
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,2 @@\n package a\n+func A() {}\n\n" +
		"diff --git a/image.png b/image.png\nnew file mode 100644\nBinary files /dev/null and b/image.png differ\n"
	if err := os.WriteFile(filepath.Join(patchDir, "change.patch"), []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	postImage, err := MaterializePatchCommit(context.Background(), repo, patchDir, base, nil)
	if err != nil {
		t.Fatalf("MaterializePatchCommit: %v", err)
	}
	if got := gitOut(t, repo, "show", postImage+":a.go"); got != "package a\nfunc A() {}" {
		t.Fatalf("text post-image = %q", got)
	}
	if _, err := gitOutNoFail(repo, "show", postImage+":image.png"); err == nil {
		t.Fatal("external binary patch unexpectedly created a blob without binary data")
	}
}
