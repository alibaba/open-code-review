// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func seedSession(t *testing.T, repoDir, sessionID string) string {
	t.Helper()
	dir, err := SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func seedAttributedSession(t *testing.T, repoDir, sessionID string) string {
	t.Helper()
	path := seedSession(t, repoDir, sessionID)
	rec := `{"type":"session_start","cwd":"` + strings.ReplaceAll(repoDir, `\`, `\\`) +
		`","sessionId":"` + sessionID + `","timestamp":"2025-06-01T10:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestDeleteSession_RemovesOnlyTheNamedSession(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()

	target := seedAttributedSession(t, repoDir, "20250601-100000-aaaaaa")
	keep := seedAttributedSession(t, repoDir, "20250601-110000-bbbbbb")

	if err := DeleteSession(repoDir, "20250601-100000-aaaaaa"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("target session still present: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("unrelated session was removed: %v", err)
	}
}

func TestDeleteSession_MissingSessionIsReportedNotSilentlyIgnored(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	seedSession(t, repoDir, "20250601-100000-aaaaaa")

	err := DeleteSession(repoDir, "20250601-999999-zzzzzz")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestDeleteSession_RefusesIdsThatEscapeTheSessionsDirectory(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()

	dir, err := SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	outside := filepath.Join(filepath.Dir(dir), "important.jsonl")
	if err := os.WriteFile(outside, []byte("keep me\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for _, id := range []string{
		"../important",
		"../../important",
		"..",
		".",
		"sub/../../important",
		`..\important`,
		"a/b",
	} {
		t.Run(id, func(t *testing.T) {
			err := DeleteSession(repoDir, id)
			if err == nil {
				t.Fatalf("DeleteSession(%q) returned no error", id)
			}
			if errors.Is(err, ErrSessionNotFound) {
				t.Errorf("DeleteSession(%q) treated a traversal as a missing session; it must be refused outright", id)
			}
		})
	}

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("a file outside the sessions directory was removed: %v", err)
	}
}

func TestDeleteSession_RefusesAnEmptyID(t *testing.T) {
	setTestHome(t, t.TempDir())
	for _, id := range []string{"", "   "} {
		if err := DeleteSession(t.TempDir(), id); err == nil {
			t.Errorf("DeleteSession(%q) returned no error", id)
		}
	}
}

func TestDeleteSession_RefusesASessionRecordedForAnotherRepo(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	base := t.TempDir()
	repoA := filepath.Join(base, "a-b", "c")
	repoB := filepath.Join(base, "a", "b-c")
	for _, d := range []string{repoA, repoB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	dirA, err := SessionsDir(repoA)
	if err != nil {
		t.Fatalf("SessionsDir(A): %v", err)
	}
	dirB, err := SessionsDir(repoB)
	if err != nil {
		t.Fatalf("SessionsDir(B): %v", err)
	}
	if dirA != dirB {
		t.Skipf("paths no longer collide (%s vs %s); the encoding may have been fixed", dirA, dirB)
	}

	if err := os.MkdirAll(dirB, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dirB, "20250601-100000-bbbbbb.jsonl")
	rec := `{"type":"session_start","cwd":` + strconv.Quote(repoB) + `,"sessionId":"20250601-100000-bbbbbb"}` + "\n"
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err = DeleteSession(repoA, "20250601-100000-bbbbbb")
	if !errors.Is(err, ErrSessionOtherRepo) {
		t.Fatalf("want ErrSessionOtherRepo, got %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("another repository's session was deleted: %v", statErr)
	}

	if err := DeleteSession(repoB, "20250601-100000-bbbbbb"); err != nil {
		t.Fatalf("DeleteSession from the owning repo: %v", err)
	}
}

func TestDeleteSession_UnattributableSessionIsRefusedAgainstARepository(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	path := seedSession(t, repoDir, "20250601-100000-nocwd")

	err := DeleteSession(repoDir, "20250601-100000-nocwd")
	if !errors.Is(err, ErrSessionUnverifiable) {
		t.Fatalf("DeleteSession error = %v, want ErrSessionUnverifiable", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("session was deleted anyway: %v", statErr)
	}
}

func TestDeleteSessionAt_UnattributableSessionIsStillDeletable(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	path := seedSession(t, repoDir, "20250601-100000-nocwd")

	found, err := FindSessionsByID("20250601-100000-nocwd")
	if err != nil {
		t.Fatalf("FindSessionsByID: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d sessions, want 1", len(found))
	}
	if err := DeleteSessionAt(found[0]); err != nil {
		t.Fatalf("DeleteSessionAt: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("session still present: %v", statErr)
	}
	_ = repoDir
}

func TestSameRepoPath_OneDirectorySpelledTwoWays(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "a-b")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	t.Run("trailing separator and dot segments", func(t *testing.T) {
		if !sameRepoPath(repo, repo+string(os.PathSeparator)) {
			t.Error("a trailing separator must not read as a different repository")
		}
		if !sameRepoPath(repo, filepath.Join(repo, ".")) {
			t.Error("a dot segment must not read as a different repository")
		}
	})

	t.Run("reached through a symlink", func(t *testing.T) {
		link := filepath.Join(base, "link-to-a-b")
		if err := os.Symlink(repo, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if !sameRepoPath(repo, link) {
			t.Error("a symlink to the same directory must compare equal")
		}
	})

	t.Run("case variation", func(t *testing.T) {
		upper := filepath.Join(base, "A-B")
		if _, err := os.Stat(upper); err != nil {
			t.Skip("case-sensitive filesystem: A-B and a-b are different directories here")
		}
		if !sameRepoPath(repo, upper) {
			t.Error("on a case-insensitive filesystem the two spellings are one directory")
		}
	})

	t.Run("genuinely different directories stay distinct", func(t *testing.T) {
		other := filepath.Join(base, "c-d")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if sameRepoPath(repo, other) {
			t.Error("two different directories must not compare equal")
		}
	})

	t.Run("a vanished directory falls back to exact spelling", func(t *testing.T) {
		gone := filepath.Join(base, "removed")
		if !sameRepoPath(gone, gone) {
			t.Error("an identical path must match even when it no longer exists")
		}
		if sameRepoPath(gone, filepath.Join(base, "removed-other")) {
			t.Error("without a filesystem to ask, differing paths must not match")
		}
	})
}

func TestDeleteSessionAt_RefusesPathOutsideSessionsRoot(t *testing.T) {
	setTestHome(t, t.TempDir())
	outside := filepath.Join(t.TempDir(), "victim.jsonl")
	if err := os.WriteFile(outside, []byte("not a session\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := DeleteSessionAt(Location{SessionID: "victim", Path: outside, EncodedDir: "anything"})
	if !errors.Is(err, ErrSessionOutsideRoot) {
		t.Fatalf("DeleteSessionAt error = %v, want ErrSessionOutsideRoot", err)
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Errorf("a file outside the sessions root was deleted: %v", statErr)
	}
}

func TestDeleteSessionAt_RefusesATraversingEncodedDir(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	outside := filepath.Join(home, "important.jsonl")
	if err := os.WriteFile(outside, []byte("keep me\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for _, encoded := range []string{"..", ".", "../..", "a/b", `a\b`, ""} {
		loc := Location{
			SessionID:  "important",
			EncodedDir: encoded,
			Path:       filepath.Join(home, ".opencodereview", sessionSubDir, encoded, "important.jsonl"),
		}
		if err := DeleteSessionAt(loc); !errors.Is(err, ErrSessionOutsideRoot) {
			t.Errorf("EncodedDir %q: error = %v, want ErrSessionOutsideRoot", encoded, err)
		}
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Errorf("a file outside the sessions root was deleted: %v", statErr)
	}
}

func TestDeleteSessionAt_RefusesAPathThatDisagreesWithItsParts(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	path := seedSession(t, repoDir, "20250601-100000-aaaaaa")

	found, err := FindSessionsByID("20250601-100000-aaaaaa")
	if err != nil || len(found) != 1 {
		t.Fatalf("FindSessionsByID: %v, %d results", err, len(found))
	}
	forged := found[0]
	forged.EncodedDir = forged.EncodedDir + "-other"

	if err := DeleteSessionAt(forged); !errors.Is(err, ErrSessionOutsideRoot) {
		t.Errorf("error = %v, want ErrSessionOutsideRoot", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the session was deleted on a Location that does not describe it: %v", statErr)
	}
}

func TestDeleteSessionAt_DeletesALocationFromTheSearch(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	path := seedSession(t, repoDir, "20250601-100000-aaaaaa")

	found, err := FindSessionsByID("20250601-100000-aaaaaa")
	if err != nil || len(found) != 1 {
		t.Fatalf("FindSessionsByID: %v, %d results", err, len(found))
	}
	if err := DeleteSessionAt(found[0]); err != nil {
		t.Fatalf("DeleteSessionAt: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("session still present: %v", statErr)
	}
}

func TestFindSessionsByID_ReportsAStatFailureRatherThanSkippingIt(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root traverses a 0000 directory regardless of its mode")
	}
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := t.TempDir()
	path := seedSession(t, repoDir, "20250601-100000-aaaaaa")

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.Stat(path); err == nil || os.IsNotExist(err) {
		t.Skip("this environment does not enforce directory modes")
	}

	found, err := FindSessionsByID("20250601-100000-aaaaaa")
	if err == nil {
		t.Fatalf("a stat failure must be reported, got %d results and no error", len(found))
	}
	if os.IsNotExist(err) {
		t.Errorf("a permission failure was reported as a missing file: %v", err)
	}
}
