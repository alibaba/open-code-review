package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
	"github.com/open-code-review/open-code-review/internal/session"
)

func TestValidateReviewRefsRejectsOptionLikeCommit(t *testing.T) {
	err := validateReviewRefs(t.TempDir(), reviewOptions{commit: "-O./pwn.sh"})
	if err == nil {
		t.Fatal("expected option-like --commit ref to be rejected")
	}
	if !strings.Contains(err.Error(), "--commit") || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateReviewRefsRejectsOptionLikeRangeRef(t *testing.T) {
	err := validateReviewRefs(t.TempDir(), reviewOptions{to: "-O./pwn.sh"})
	if err == nil {
		t.Fatal("expected option-like --to ref to be rejected")
	}
	if !strings.Contains(err.Error(), "--to") || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsRejectsToWithoutFrom(t *testing.T) {
	_, err := parseReviewFlags([]string{"--to", "HEAD"})
	if err == nil {
		t.Fatal("expected --to without --from to fail")
	}
	if !strings.Contains(err.Error(), "--from is required when --to is specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsRejectsFromWithoutTo(t *testing.T) {
	_, err := parseReviewFlags([]string{"--from", "main"})
	if err == nil {
		t.Fatal("expected --from without --to to fail")
	}
	if !strings.Contains(err.Error(), "--to is required when --from is specified") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseReviewFlagsAllowsFromAndTo(t *testing.T) {
	opts, err := parseReviewFlags([]string{"--from", "main", "--to", "HEAD"})
	if err != nil {
		t.Fatalf("expected --from/--to to pass, got: %v", err)
	}
	if opts.from != "main" || opts.to != "HEAD" {
		t.Fatalf("unexpected opts: from=%q to=%q", opts.from, opts.to)
	}
}

// TestLoadDismissalFilterReturnsNilWhenAbsent verifies D2: when no dismissal
// store exists, loadDismissalFilter returns nil (one stat, no read) so the
// review is byte-identical to the stateless default.
func TestLoadDismissalFilterReturnsNilWhenAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	if f := loadDismissalFilter(repoDir); f != nil {
		t.Errorf("loadDismissalFilter with no store returned non-nil: %v", f)
	}
}

// TestLoadDismissalFilterReturnsFilterWhenStoreExists verifies that a present,
// valid store produces a non-nil filter that suppresses the recorded finding.
func TestLoadDismissalFilterReturnsFilterWhenStoreExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	target := model.LlmComment{Path: "a.go", StartLine: 1, EndLine: 2, Content: "bug"}
	store, err := session.LoadDismissals(repoDir)
	if err != nil {
		t.Fatalf("LoadDismissals: %v", err)
	}
	store.Record(session.DismissalEntry{Fingerprint: session.DismissalFingerprint(target)})
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	f := loadDismissalFilter(repoDir)
	if f == nil {
		t.Fatal("loadDismissalFilter returned nil despite a valid store existing")
	}
	out := f.Suppress([]model.LlmComment{target, {Path: "a.go", StartLine: 9, EndLine: 9, Content: "other"}})
	if len(out) != 1 || out[0].Content != "other" {
		t.Errorf("filter did not suppress the recorded finding: %+v", out)
	}
}

// TestLoadDismissalFilterCorruptReturnsNil verifies D6/AS5: a corrupt store
// makes loadDismissalFilter print a warning and return nil (proceed stateless),
// leaving the file untouched.
func TestLoadDismissalFilterCorruptReturnsNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := t.TempDir()
	path, err := session.DismissalFilePath(repoDir)
	if err != nil {
		t.Fatalf("DismissalFilePath: %v", err)
	}
	garbage := []byte("{ broken json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, garbage, 0600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	if f := loadDismissalFilter(repoDir); f != nil {
		t.Errorf("loadDismissalFilter with corrupt store returned non-nil: %v", f)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != string(garbage) {
		t.Errorf("corrupt store was modified by loadDismissalFilter")
	}
}
