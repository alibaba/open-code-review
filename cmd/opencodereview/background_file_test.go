package main

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestMergeBackgroundSanitizesInline(t *testing.T) {
	t.Run("inline only", func(t *testing.T) {
		// Control char, zero-width space and surrounding whitespace must be removed.
		got := mergeBackground("  \x00Inline\u200B context  ", "")
		if got != "Inline context" {
			t.Errorf("mergeBackground = %q, want %q", got, "Inline context")
		}
	})

	t.Run("inline combined with file", func(t *testing.T) {
		wrapped := backgroundOpenTag + "\nfrom file\n" + backgroundCloseTag
		got := mergeBackground("\x07dirty\uFEFF inline\n\n\n\nend", wrapped)
		if strings.ContainsRune(got, '\x07') || strings.ContainsRune(got, '\uFEFF') {
			t.Errorf("inline portion was not sanitised: %q", got)
		}
		// Excess blank lines in the inline portion are collapsed to one.
		if strings.Contains(got, "\n\n\n") {
			t.Errorf("inline newlines were not collapsed: %q", got)
		}
		// The file portion is preserved intact.
		if !strings.Contains(got, wrapped) {
			t.Errorf("file portion was altered: %q", got)
		}
	})
}

func TestMergeBackground(t *testing.T) {
	wrapped := backgroundOpenTag + "\nfrom file\n" + backgroundCloseTag

	t.Run("both present are combined", func(t *testing.T) {
		got := mergeBackground("inline context", wrapped)
		want := "inline context\n\n" + wrapped
		if got != want {
			t.Errorf("mergeBackground = %q, want %q", got, want)
		}
		// Both inputs must survive in the result.
		if !strings.Contains(got, "inline context") || !strings.Contains(got, "from file") {
			t.Errorf("merged background dropped one of the inputs: %q", got)
		}
	})

	t.Run("inline only", func(t *testing.T) {
		if got := mergeBackground("inline only", ""); got != "inline only" {
			t.Errorf("mergeBackground = %q, want %q", got, "inline only")
		}
	})

	t.Run("file only", func(t *testing.T) {
		if got := mergeBackground("", wrapped); got != wrapped {
			t.Errorf("mergeBackground = %q, want %q", got, wrapped)
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
// runReview when --background-file is supplied but --background is not: the
// inline background is first auto-filled from the commit message, then the
// background file is appended. Both must end up in the final background.
func TestBackgroundFromCommitThenFile(t *testing.T) {
	const commitMsg = "Implement rate limiting on login"
	repo, hash := initRepoWithCommit(t, commitMsg)

	// Mirror runReview: --background empty + --commit set -> use commit message.
	background := ""
	msg, err := getCommitMessage(repo, hash)
	if err != nil {
		t.Fatalf("getCommitMessage: %v", err)
	}
	if msg != commitMsg {
		t.Fatalf("commit message = %q, want %q", msg, commitMsg)
	}
	if background == "" {
		background = msg
	}

	// Then --background-file is loaded and merged in.
	fileBg, err := loadBackgroundFile(writeTempFile(t, "Extra context from a file."))
	if err != nil {
		t.Fatalf("loadBackgroundFile: %v", err)
	}
	background = mergeBackground(background, fileBg)

	// The commit message must come first, followed by the wrapped file content.
	if !strings.HasPrefix(background, commitMsg+"\n\n") {
		t.Errorf("expected commit message to lead the background, got %q", background)
	}
	if !strings.Contains(background, "Extra context from a file.") {
		t.Errorf("expected file content to be appended, got %q", background)
	}
	if !strings.Contains(background, backgroundOpenTag) || !strings.Contains(background, backgroundCloseTag) {
		t.Errorf("expected file content to keep its delimiters, got %q", background)
	}
}
