// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeTempFile writes content to a temporary file and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "background.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestLoadBackgroundFileNotFound(t *testing.T) {
	_, err := loadBackgroundFile(filepath.Join(t.TempDir(), "does-not-exist.md"))
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestResolveBackgroundFilePath(t *testing.T) {
	repo := filepath.FromSlash("/path/to/repo")

	t.Run("relative anchored at repo", func(t *testing.T) {
		got := resolveBackgroundFilePath(repo, filepath.FromSlash("./docs/context.md"))
		want := filepath.Join(repo, "docs", "context.md")
		if got != want {
			t.Errorf("resolveBackgroundFilePath = %q, want %q", got, want)
		}
	})

	t.Run("absolute unchanged", func(t *testing.T) {
		// FromSlash is not enough on its own: it only swaps separators, and
		// `\etc\context.md` is rooted but not absolute on Windows, where
		// filepath.IsAbs wants a volume. Without the drive letter this case
		// exercised the relative branch instead of the one it names.
		abs := filepath.FromSlash("/etc/context.md")
		if runtime.GOOS == "windows" {
			abs = `C:\etc\context.md`
		}
		if got := resolveBackgroundFilePath(repo, abs); got != abs {
			t.Errorf("resolveBackgroundFilePath = %q, want %q (absolute must be untouched)", got, abs)
		}
	})

	t.Run("empty unchanged", func(t *testing.T) {
		if got := resolveBackgroundFilePath(repo, ""); got != "" {
			t.Errorf("resolveBackgroundFilePath = %q, want empty", got)
		}
	})

	t.Run("empty repoDir falls back to the relative path", func(t *testing.T) {
		// When --repo is omitted and repo detection fails, repoDir is empty.
		// Ensure it falls back to the CWD.
		rel := filepath.FromSlash("./docs/context.md")
		got := resolveBackgroundFilePath("", rel)
		want := filepath.FromSlash("docs/context.md")
		if got != want {
			t.Errorf("resolveBackgroundFilePath = %q, want %q", got, want)
		}
	})
}

// TestLoadBackgroundFileRelativeToRepo verifies the end-to-end path: a relative
// --background-file argument resolves against the repo directory, not the
// process CWD, and is read successfully from there.
func TestLoadBackgroundFileRelativeToRepo(t *testing.T) {
	repo := t.TempDir()
	docs := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docs, "context.md"), []byte("Repo-relative context."), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	resolved := resolveBackgroundFilePath(repo, filepath.FromSlash("./docs/context.md"))
	got, err := loadBackgroundFile(resolved)
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}
	if !strings.Contains(got, "Repo-relative context.") {
		t.Errorf("expected repo-relative file to be read, got %q", got)
	}
}

func TestLoadBackgroundFileEmpty(t *testing.T) {
	cases := map[string]string{
		"zero bytes":      "",
		"whitespace only": "   \n\t \n  ",
		"invisible only":  "\u200B\u200E\u00AD\uFEFF",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadBackgroundFile(writeTempFile(t, content))
			if err == nil {
				t.Fatal("expected an error for empty-after-sanitisation content, got nil")
			}
			if !strings.Contains(err.Error(), "empty") {
				t.Errorf("error = %q, want it to mention 'empty'", err)
			}
		})
	}
}

func TestLoadBackgroundFileControlCharRemoval(t *testing.T) {
	// Mix in NUL, bell, DEL, a C1 control char, zero-width space, BOM and an
	// LTR mark around legitimate text.
	content := "Hello\x00\x07world\x7f\u0085!\u200B\uFEFF\u200E"
	got, err := loadBackgroundFile(writeTempFile(t, content))
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}
	for _, bad := range []string{"\x00", "\x07", "\x7f", "\u0085", "\u200B", "\uFEFF", "\u200E"} {
		if strings.Contains(got, bad) {
			t.Errorf("result still contains control/invisible char %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "Helloworld!") {
		t.Errorf("expected cleaned text to contain %q, got %q", "Helloworld!", got)
	}
}

func TestSanitizeMarkdownPreservesNewlinesAndTabs(t *testing.T) {
	got := sanitizeMarkdown("line1\n\tindented\nline3")
	want := "line1\n\tindented\nline3"
	if got != want {
		t.Errorf("sanitizeMarkdown = %q, want %q", got, want)
	}
}

func TestSanitizeMarkdownCollapsesNewlines(t *testing.T) {
	got := sanitizeMarkdown("a\n\n\n\n\nb")
	want := "a\n\nb"
	if got != want {
		t.Errorf("sanitizeMarkdown = %q, want %q", got, want)
	}
}

func TestSanitizeMarkdownNormalizesCRLF(t *testing.T) {
	got := sanitizeMarkdown("a\r\nb\r\nc")
	want := "a\nb\nc"
	if got != want {
		t.Errorf("sanitizeMarkdown = %q, want %q", got, want)
	}
}

func TestSanitizeMarkdownTrims(t *testing.T) {
	got := sanitizeMarkdown("   \n  hello  \n   ")
	if got != "hello" {
		t.Errorf("sanitizeMarkdown = %q, want %q", got, "hello")
	}
}

func TestLoadBackgroundFileDelimiters(t *testing.T) {
	got, err := loadBackgroundFile(writeTempFile(t, "Some requirement context."))
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}
	if !strings.HasPrefix(got, backgroundOpenTag+"\n") {
		t.Errorf("result missing opening delimiter: %q", got)
	}
	if !strings.HasSuffix(got, "\n"+backgroundCloseTag) {
		t.Errorf("result missing closing delimiter: %q", got)
	}
	want := backgroundOpenTag + "\nSome requirement context.\n" + backgroundCloseTag
	if got != want {
		t.Errorf("result = %q, want %q", got, want)
	}
}

func TestLoadBackgroundFileRejectsReservedDelimiters(t *testing.T) {
	for _, tag := range []string{backgroundOpenTag, backgroundCloseTag} {
		t.Run(tag, func(t *testing.T) {
			content := "Some context " + tag + " and more text."
			_, err := loadBackgroundFile(writeTempFile(t, content))
			if err == nil {
				t.Fatalf("expected an error for content containing %q, got nil", tag)
			}
			if !strings.Contains(err.Error(), "reserved delimiters") {
				t.Errorf("error = %q, want it to mention 'reserved delimiters'", err)
			}
		})
	}
}

func TestSelectBackground(t *testing.T) {
	wrapped := backgroundOpenTag + "\nfrom file\n" + backgroundCloseTag

	t.Run("file takes precedence over inline", func(t *testing.T) {
		got := selectBackground("inline context", wrapped)
		if got != wrapped {
			t.Errorf("selectBackground = %q, want file content %q", got, wrapped)
		}
	})

	t.Run("inline only", func(t *testing.T) {
		if got := selectBackground("inline only", ""); got != "inline only" {
			t.Errorf("selectBackground = %q, want %q", got, "inline only")
		}
	})

	t.Run("file only", func(t *testing.T) {
		if got := selectBackground("", wrapped); got != wrapped {
			t.Errorf("selectBackground = %q, want %q", got, wrapped)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		if got := selectBackground("", ""); got != "" {
			t.Errorf("selectBackground = %q, want empty", got)
		}
	})
}

func TestLoadBackgroundFileSoftLimit(t *testing.T) {
	// Just above the soft limit but below the hard size limit: must succeed.
	content := strings.Repeat("a", backgroundSoftLimit+100)
	got, err := loadBackgroundFile(writeTempFile(t, content))
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}
	if !strings.Contains(got, content) {
		t.Error("expected content to be preserved past the soft limit")
	}
}

func TestLoadBackgroundFileOversized(t *testing.T) {
	// A file larger than maxBackgroundFileBytes must be rejected up front,
	// before its content is read into memory.
	content := strings.Repeat("a", maxBackgroundFileBytes+1)
	_, err := loadBackgroundFile(writeTempFile(t, content))
	if err == nil {
		t.Fatal("expected an error for an oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("error = %q, want it to mention the byte 'maximum'", err)
	}
}

func TestLoadBackgroundFileDirectory(t *testing.T) {
	if _, err := loadBackgroundFile(t.TempDir()); err == nil {
		t.Fatal("expected an error when the path is a directory, got nil")
	}
}

func TestLoadBackgroundFileHardLimit(t *testing.T) {
	content := strings.Repeat("a", backgroundHardLimit+1)
	_, err := loadBackgroundFile(writeTempFile(t, content))
	if err == nil {
		t.Fatal("expected an error when exceeding the hard size limit, got nil")
	}
	if !strings.Contains(err.Error(), "hard limit") {
		t.Errorf("error = %q, want it to mention 'hard limit'", err)
	}
}

func TestLoadBackgroundFileHardLimitExcludesWrapper(t *testing.T) {
	// The wrapper delimiters must NOT count toward the limit: cleaned content of
	// exactly the hard limit is accepted even though the wrapped string is longer.
	content := strings.Repeat("a", backgroundHardLimit)
	if _, err := loadBackgroundFile(writeTempFile(t, content)); err != nil {
		t.Fatalf("cleaned content at the hard limit must be accepted, got: %v", err)
	}
}

func TestLoadBackgroundFileMultiByteRuneCount(t *testing.T) {
	// Multi-byte runes must be counted as single characters, not bytes.
	// A precomposed accented letter is one rune but two bytes; a string of exactly
	// backgroundHardLimit runes (~2x the byte count) must still be accepted.
	content := strings.Repeat("\u00E9", backgroundHardLimit)
	got, err := loadBackgroundFile(writeTempFile(t, content))
	if err != nil {
		t.Fatalf("loadBackgroundFile rejected content within the rune limit: %v", err)
	}
	if !strings.Contains(got, content) {
		t.Error("expected multi-byte content to be preserved")
	}
}

// initRepoWithCommit creates a real git repository with a single commit whose
// message is `message`, and returns the repo directory and the commit hash.
func initRepoWithCommit(t *testing.T, message string) (string, string) {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) []byte {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
		return out
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run("add", ".")
	run("commit", "-q", "-m", message)
	hash := strings.TrimSpace(string(run("rev-parse", "HEAD")))
	return repo, hash
}

// TestBackgroundFromCommitThenFile reproduces the resolution order used by
// TestBackgroundFilePrecedenceOverCommit verifies that when --background-file
// is set, the commit-message fallback does not fire: file content wins.
func TestBackgroundFilePrecedenceOverCommit(t *testing.T) {
	const commitMsg = "Implement rate limiting on login"
	repo, hash := initRepoWithCommit(t, commitMsg)

	// Verify the commit message is retrievable.
	msg, err := getCommitMessage(repo, hash)
	if err != nil {
		t.Fatalf("getCommitMessage: %v", err)
	}
	if msg != commitMsg {
		t.Fatalf("commit message = %q, want %q", msg, commitMsg)
	}

	// Mirror the new ordering: --background-file is set, so selectBackground
	// is called and the commit-message fallback is skipped entirely.
	background := ""
	fileBg, err := loadBackgroundFile(writeTempFile(t, "Extra context from a file."))
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}
	background = selectBackground(background, fileBg)

	// Only the file content must be present; the commit message must NOT appear.
	if strings.Contains(background, commitMsg) {
		t.Errorf("commit message should not appear when --background-file is set, got %q", background)
	}
	if !strings.Contains(background, "Extra context from a file.") {
		t.Errorf("expected file content in background, got %q", background)
	}
	if !strings.Contains(background, backgroundOpenTag) || !strings.Contains(background, backgroundCloseTag) {
		t.Errorf("expected file content to keep its delimiters, got %q", background)
	}
}

// TestReviewAndDelegateAgreeOnBackground verifies that review and delegate
// produce identical backgrounds for the same --commit + --background-file input.
func TestReviewAndDelegateAgreeOnBackground(t *testing.T) {
	const commitMsg = "Add rate limiting"
	repo, hash := initRepoWithCommit(t, commitMsg)

	bgFile := writeTempFile(t, "File-based context for review.")
	fileBg, err := loadBackgroundFile(bgFile)
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}

	// Simulate the review command path.
	reviewBg := ""
	if fileBg != "" {
		reviewBg = selectBackground(reviewBg, fileBg)
	} else if hash != "" && reviewBg == "" {
		if msg, err := getCommitMessage(repo, hash); err == nil && msg != "" {
			reviewBg = msg
		}
	}

	// Simulate the delegate command path (must be identical logic).
	delegateBg := ""
	if fileBg != "" {
		delegateBg = selectBackground(delegateBg, fileBg)
	} else if hash != "" && delegateBg == "" {
		if msg, err := getCommitMessage(repo, hash); err == nil && msg != "" {
			delegateBg = msg
		}
	}

	if reviewBg != delegateBg {
		t.Errorf("review and delegate disagree:\n  review:   %q\n  delegate: %q", reviewBg, delegateBg)
	}
	if strings.Contains(reviewBg, commitMsg) {
		t.Errorf("commit message should not appear when --background-file is set, got %q", reviewBg)
	}
	if !strings.Contains(reviewBg, "File-based context for review.") {
		t.Errorf("expected file content in background, got %q", reviewBg)
	}
}
