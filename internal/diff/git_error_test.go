// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestGitFailure(t *testing.T) {
	baseErr := errors.New("exit status 129")

	t.Run("includes git's own message", func(t *testing.T) {
		err := gitFailure("git show", "error: unknown option `diff-merges=first-parent'\n", baseErr)
		got := err.Error()
		if !strings.Contains(got, "unknown option") {
			t.Errorf("error %q drops git's message", got)
		}
		if !strings.Contains(got, "git show failed") {
			t.Errorf("error %q lost the operation name", got)
		}
		if !strings.Contains(got, "exit status 129") {
			t.Errorf("error %q lost the exit status", got)
		}
	})

	t.Run("wrapped error stays unwrappable", func(t *testing.T) {
		// Callers up the stack match on the underlying error, so quoting git's
		// output must not cost them errors.Is.
		err := gitFailure("git show", "fatal: bad object", baseErr)
		if !errors.Is(err, baseErr) {
			t.Error("gitFailure broke the error chain")
		}
	})

	t.Run("empty output leaves no dangling separator", func(t *testing.T) {
		err := gitFailure("git show", "   \n\t ", baseErr)
		if got := err.Error(); got != "git show failed: exit status 129" {
			t.Errorf("error = %q, want the bare form with no trailing colon", got)
		}
	})

	t.Run("long output keeps the tail", func(t *testing.T) {
		// runGit returns stdout and stderr combined, so a command that failed
		// partway through carries real diff ahead of git's diagnosis. The
		// diagnosis is last, which is the half worth keeping.
		noise := strings.Repeat("+padding line\n", 400)
		err := gitFailure("git diff", noise+"fatal: the real problem", baseErr)
		got := err.Error()
		if !strings.Contains(got, "fatal: the real problem") {
			t.Error("truncation dropped the tail, which is where git's diagnosis is")
		}
		if !strings.Contains(got, "...") {
			t.Error("truncated output is not marked as truncated")
		}
		if len(got) > gitDiagLimit+200 {
			t.Errorf("error is %d bytes; truncation is not bounding it", len(got))
		}
	})

	t.Run("short output is not truncated", func(t *testing.T) {
		if got := gitFailure("git show", "fatal: bad object", baseErr).Error(); strings.Contains(got, "...") {
			t.Errorf("error %q was truncated despite fitting the limit", got)
		}
	})

	// Git speaks the user's locale. #972 came from a Japanese-language Windows
	// install, so truncating by bytes can land in the middle of a rune and turn
	// a confusing error into an unreadable one.
	t.Run("multibyte output survives truncation", func(t *testing.T) {
		msg := strings.Repeat("致命的なエラーが発生しました。", 400) // allow-non-english: multibyte truncation fixture, mirrors the locale in #972
		err := gitFailure("git show", msg, baseErr)
		got := err.Error()
		if !utf8.ValidString(got) {
			t.Error("truncation produced invalid UTF-8")
		}
		if strings.Contains(got, "�") {
			t.Error("truncation left a replacement character mid-rune")
		}
		if !strings.HasSuffix(got, "。") { // allow-non-english: asserts the fixture's trailing rune is intact
			t.Errorf("error %q does not end on the original tail", got[len(got)-40:])
		}
	})
}

// TestGetDiff_CommitFailureSurfacesGitMessage is the regression test for #972:
// `git show` failing used to surface as a bare exit status, so neither the
// reporter nor a maintainer could tell an unsupported option from a bad
// revision without re-running the command by hand.
func TestGetDiff_CommitFailureSurfacesGitMessage(t *testing.T) {
	repo := initRepoWithChange(t)
	runner := gitcmd.New(2)

	provider := NewCommitProvider(repo, "0b335be72cb7e342c115ff3ffadbe741a4715377", runner)

	_, err := provider.GetDiff(context.Background())
	if err == nil {
		t.Fatal("expected GetDiff to fail for a commit that does not exist")
	}

	got := err.Error()
	if !strings.Contains(got, "git show failed") {
		t.Errorf("error %q lost the operation name", got)
	}
	// Git words this differently across versions ("bad object", "unknown
	// revision", "ambiguous argument"), so assert that it said something at
	// all rather than pinning one phrasing.
	if !strings.Contains(got, "fatal:") && !strings.Contains(got, "error:") {
		t.Errorf("error %q carries no message from git; only the exit status survived", got)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Error("underlying *exec.ExitError is no longer reachable")
	}
}
