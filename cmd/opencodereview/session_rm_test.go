// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/alibaba/open-code-review/internal/session"
)

func seedRmSession(t *testing.T, _ /*home*/, repoDir, sessionID string) string {
	t.Helper()
	dir, err := session.SessionsDir(repoDir)
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

func TestSessionRm_DeclinedConfirmationKeepsTheSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSessionRecorded(t, repoDir, "20250601-100000-aaaaaa", "main")

	sessionRmRepoDir = repoDir
	sessionRmYes = false
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	out := &bytes.Buffer{}
	sessionRmCmd.SetOut(out)
	sessionRmCmd.SetIn(strings.NewReader("n\n"))

	if err := runSessionRm(sessionRmCmd, "20250601-100000-aaaaaa"); err != nil {
		t.Fatalf("runSessionRm: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("session was deleted despite a declined confirmation: %v", err)
	}
	if !strings.Contains(out.String(), "Cancelled") {
		t.Errorf("expected the command to say it cancelled, got %q", out.String())
	}
}

func TestSessionRm_NonInteractiveStdinDoesNotDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSessionRecorded(t, repoDir, "20250601-100000-aaaaaa", "main")

	sessionRmRepoDir = repoDir
	sessionRmYes = false
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	sessionRmCmd.SetOut(&bytes.Buffer{})
	sessionRmCmd.SetIn(strings.NewReader(""))

	if err := runSessionRm(sessionRmCmd, "20250601-100000-aaaaaa"); err != nil {
		t.Fatalf("runSessionRm: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("session was deleted on an empty stdin: %v", err)
	}
}

func TestSessionRm_YesFlagDeletesWithoutPrompting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSessionRecorded(t, repoDir, "20250601-100000-aaaaaa", "main")

	sessionRmRepoDir = repoDir
	sessionRmYes = true
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	out := &bytes.Buffer{}
	sessionRmCmd.SetOut(out)
	sessionRmCmd.SetIn(strings.NewReader(""))

	if err := runSessionRm(sessionRmCmd, "20250601-100000-aaaaaa"); err != nil {
		t.Fatalf("runSessionRm: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("session still present after --yes: %v", err)
	}
	if !strings.Contains(out.String(), "Deleted session") {
		t.Errorf("expected a confirmation line, got %q", out.String())
	}
}

func TestSessionRm_UnknownIDExplainsHowToList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()

	sessionRmRepoDir = repoDir
	sessionRmYes = true
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	sessionRmCmd.SetOut(&bytes.Buffer{})
	sessionRmCmd.SetIn(strings.NewReader(""))

	err := runSessionRm(sessionRmCmd, "20250601-999999-zzzzzz")
	if err == nil {
		t.Fatal("expected an error for an unknown session id")
	}
	if !strings.Contains(err.Error(), "ocr session list") {
		t.Errorf("error should point at 'ocr session list', got %q", err.Error())
	}
}

func TestSessionRm_RejectsTraversalIDsBeforeTouchingTheFilesystem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()

	sessionRmRepoDir = repoDir
	sessionRmYes = true
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	out := &bytes.Buffer{}
	sessionRmCmd.SetOut(out)
	sessionRmCmd.SetIn(strings.NewReader(""))

	for _, id := range []string{"../escape", "../../escape", "a/b", "..", "."} {
		t.Run(id, func(t *testing.T) {
			err := runSessionRm(sessionRmCmd, id)
			if err == nil {
				t.Fatalf("runSessionRm(%q) returned no error", id)
			}
			if strings.Contains(err.Error(), "ocr session list") {
				t.Errorf("runSessionRm(%q) reported a missing session; an invalid id must be refused as invalid", id)
			}
		})
	}
}

func TestSessionRm_CorruptSessionIsDeletableAndDescribedHonestly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()

	dir, err := session.SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "20250601-100000-corrupt.jsonl")
	if err := os.WriteFile(path, []byte("this is not json\n{\"also\": not json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sessionRmRepoDir = ""
	sessionRmYes = false
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	out := &bytes.Buffer{}
	sessionRmCmd.SetOut(out)
	sessionRmCmd.SetIn(strings.NewReader("y\n"))

	if err := runSessionRm(sessionRmCmd, "20250601-100000-corrupt"); err != nil {
		t.Fatalf("runSessionRm: %v", err)
	}
	if !strings.Contains(out.String(), "could not be read") {
		t.Errorf("a corrupt session should be described as unreadable, got %q", out.String())
	}
	if strings.Contains(out.String(), "started: 0001-01-01") {
		t.Errorf("zero-value metadata was printed as if real: %q", out.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("corrupt session was not deleted: %v", err)
	}
}

func TestSessionRm_OtherRepoRefusedBeforePrompting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	base := t.TempDir()
	repoA := filepath.Join(base, "a-b", "c")
	repoB := filepath.Join(base, "a", "b-c")
	for _, d := range []string{repoA, repoB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	dirA, _ := session.SessionsDir(repoA)
	dirB, _ := session.SessionsDir(repoB)
	if dirA != dirB {
		t.Skip("paths no longer collide; the encoding may have been fixed")
	}
	if err := os.MkdirAll(dirB, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dirB, "20250601-100000-bbbbbb.jsonl")
	rec := `{"type":"session_start","cwd":"` + strings.ReplaceAll(repoB, `\`, `\\`) + `","sessionId":"20250601-100000-bbbbbb","timestamp":"2025-06-01T10:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sessionRmRepoDir = repoA
	sessionRmYes = false
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })

	out := &bytes.Buffer{}
	sessionRmCmd.SetOut(out)
	sessionRmCmd.SetIn(strings.NewReader("y\n"))

	err := runSessionRm(sessionRmCmd, "20250601-100000-bbbbbb")
	if err == nil {
		t.Fatal("deleting another repository's session must fail")
	}
	if !strings.Contains(err.Error(), "different repository") {
		t.Errorf("error should name the cause, got %q", err.Error())
	}
	if strings.Contains(out.String(), "Delete session") {
		t.Errorf("the user was prompted before the refusal: %q", out.String())
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the other repository's session was deleted: %v", statErr)
	}
}

func makeUnreadable(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skipf("this platform does not enforce the file mode: %s is still readable after chmod 000", path)
	}
}

func TestSessionRm_UnreadableSessionIsReportedNotTreatedAsCorrupt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSession(t, home, repoDir, "20250601-100000-locked")
	makeUnreadable(t, path)

	sessionRmRepoDir = repoDir
	sessionRmYes = true
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })
	sessionRmCmd.SetOut(&bytes.Buffer{})
	sessionRmCmd.SetIn(strings.NewReader(""))

	err := runSessionRm(sessionRmCmd, "20250601-100000-locked")
	if err == nil {
		t.Fatal("an unreadable session must be reported, not deleted")
	}
	if !strings.Contains(err.Error(), "cannot read session") {
		t.Errorf("error should say the file could not be read, got %q", err.Error())
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("an unreadable session was deleted anyway: %v", statErr)
	}
}

func TestSessionRm_PromptMatchesTheListFormatting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()

	dir, err := session.SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "20250601-100000-fmt.jsonl")
	rec := `{"type":"session_start","cwd":"` + strings.ReplaceAll(repoDir, `\`, `\\`) +
		`","sessionId":"20250601-100000-fmt","timestamp":"2025-06-01T10:00:00Z","gitBranch":"feature-x"}` + "\n"
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sessionRmRepoDir = repoDir
	sessionRmYes = false
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })
	out := &bytes.Buffer{}
	sessionRmCmd.SetOut(out)
	sessionRmCmd.SetIn(strings.NewReader("n\n"))

	if err := runSessionRm(sessionRmCmd, "20250601-100000-fmt"); err != nil {
		t.Fatalf("runSessionRm: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "T10:00:00Z") {
		t.Errorf("prompt used RFC3339; 'ocr session list' shows a local timestamp: %q", got)
	}
	for _, want := range []string{"repo:", "branch:", "started:", "files:", "comments:"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q: %q", want, got)
		}
	}
}

func seedRmSessionRecorded(t *testing.T, repoDir, sessionID, branch string) string {
	t.Helper()
	path := seedRmSession(t, "", repoDir, sessionID)
	rec := `{"type":"session_start","cwd":"` + strings.ReplaceAll(repoDir, `\`, `\\`) +
		`","sessionId":"` + sessionID + `","timestamp":"2025-06-01T10:00:00Z","gitBranch":"` + branch + `"}` + "\n"
	if err := os.WriteFile(path, []byte(rec), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func runSessionRmForTest(t *testing.T, repoFlag, sessionID, stdin string, yes bool) (string, error) {
	t.Helper()
	sessionRmRepoDir = repoFlag
	sessionRmYes = yes
	t.Cleanup(func() { sessionRmRepoDir, sessionRmYes = "", false })
	var out bytes.Buffer
	sessionRmCmd.SetOut(&out)
	sessionRmCmd.SetIn(strings.NewReader(stdin))
	err := runSessionRm(sessionRmCmd, sessionID)
	return out.String(), err
}

func TestSessionRm_FindsTheSessionWithoutBeingToldTheRepository(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSessionRecorded(t, repoDir, "20250601-100000-alone", "main")

	out, err := runSessionRmForTest(t, "", "20250601-100000-alone", "y\n", false)
	if err != nil {
		t.Fatalf("runSessionRm: %v", err)
	}
	if !strings.Contains(out, repoDir) {
		t.Errorf("the prompt must name the repository the session was found in, got %q", out)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("session was not deleted: %v", statErr)
	}
}

func TestSessionRm_ListsCandidatesRatherThanGuessing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoA := t.TempDir()
	repoB := t.TempDir()
	pathA := seedRmSessionRecorded(t, repoA, "20250601-100000-shared", "main")
	pathB := seedRmSessionRecorded(t, repoB, "20250601-100000-shared", "feature")

	_, err := runSessionRmForTest(t, "", "20250601-100000-shared", "y\n", true)
	if err == nil {
		t.Fatal("an id saved for two repositories must not be deleted on a guess")
	}
	for _, want := range []string{repoA, repoB, "--repo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q so the caller can disambiguate, got %q", want, err.Error())
		}
	}
	for _, path := range []string{pathA, pathB} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("an ambiguous id deleted %s anyway: %v", path, statErr)
		}
	}
}

func TestSessionRm_RepoFlagNarrowsAnAmbiguousID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoA := t.TempDir()
	repoB := t.TempDir()
	pathA := seedRmSessionRecorded(t, repoA, "20250601-100000-shared", "main")
	pathB := seedRmSessionRecorded(t, repoB, "20250601-100000-shared", "feature")

	if _, err := runSessionRmForTest(t, repoB, "20250601-100000-shared", "", true); err != nil {
		t.Fatalf("runSessionRm --repo: %v", err)
	}
	if _, statErr := os.Stat(pathB); !os.IsNotExist(statErr) {
		t.Errorf("the named repository's session was not deleted: %v", statErr)
	}
	if _, statErr := os.Stat(pathA); statErr != nil {
		t.Errorf("the other repository's session was deleted too: %v", statErr)
	}
}

func TestSessionRm_UnknownIDAnywhereSaysSoWithoutPrompting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	seedRmSessionRecorded(t, t.TempDir(), "20250601-100000-present", "main")

	out, err := runSessionRmForTest(t, "", "20250601-100000-missing", "y\n", false)
	if err == nil {
		t.Fatal("an id that is saved nowhere must be an error")
	}
	if !strings.Contains(err.Error(), "ocr session list") {
		t.Errorf("error should point at the listing command, got %q", err.Error())
	}
	if strings.Contains(out, "Delete session") {
		t.Errorf("the caller must not be asked to confirm deleting something that is not there, got %q", out)
	}
}

func TestSessionRm_TraversalIDIsRefusedBeforeSearchingEveryRepository(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	for _, id := range []string{"../escape", "sub/../../escape", `..\escape`, ".", ".."} {
		if _, err := session.FindSessionsByID(id); err == nil {
			t.Errorf("FindSessionsByID(%q) must be refused", id)
		}
		if _, err := runSessionRmForTest(t, "", id, "", true); err == nil {
			t.Errorf("ocr session rm %q must be refused", id)
		}
	}
}

func TestSessionRm_RepoFlagRefusesAnUnverifiableSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSession(t, home, repoDir, "20250601-100000-nocwd")

	out, err := runSessionRmForTest(t, repoDir, "20250601-100000-nocwd", "y\n", true)
	if err == nil {
		t.Fatal("a session that records no repository must not be deleted on the strength of --repo")
	}
	if !errors.Is(err, session.ErrSessionUnverifiable) {
		t.Errorf("error should be ErrSessionUnverifiable, got %v", err)
	}
	if !strings.Contains(err.Error(), "without --repo") {
		t.Errorf("error should say how to delete it instead, got %q", err.Error())
	}
	if strings.Contains(out, "Delete session") {
		t.Errorf("the caller must not be prompted before the refusal, got %q", out)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("the session was deleted anyway: %v", statErr)
	}
}

func TestSessionRm_UnverifiableSessionIsStillDeletableByIDAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repoDir := t.TempDir()
	path := seedRmSession(t, home, repoDir, "20250601-100000-nocwd")

	if _, err := runSessionRmForTest(t, "", "20250601-100000-nocwd", "", true); err != nil {
		t.Fatalf("runSessionRm without --repo: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("the session was not deleted: %v", statErr)
	}
}

func TestSessionRm_CompletionOffersSessionsFromEveryRepository(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	here := t.TempDir()
	elsewhere := t.TempDir()
	seedRmSessionRecorded(t, here, "20250601-100000-here", "main")
	seedRmSessionRecorded(t, elsewhere, "20250601-100000-away", "main")

	sessionRmRepoDir = ""
	t.Cleanup(func() { sessionRmRepoDir = "" })
	if err := sessionRmCmd.Flags().Set("repo", ""); err != nil {
		t.Fatalf("clear --repo: %v", err)
	}

	got, _ := completeSessionIDsAnywhere(sessionRmCmd, nil, "20250601")
	joined := strings.Join(got, "\n")
	for _, want := range []string{"20250601-100000-here", "20250601-100000-away"} {
		if !strings.Contains(joined, want) {
			t.Errorf("completion is missing %q; got %q", want, joined)
		}
	}
	if !strings.Contains(joined, elsewhere) {
		t.Errorf("completion should name the repository each id belongs to; got %q", joined)
	}
}

func TestSessionRm_CompletionNarrowsWithRepoFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	here := t.TempDir()
	elsewhere := t.TempDir()
	seedRmSessionRecorded(t, here, "20250601-100000-here", "main")
	seedRmSessionRecorded(t, elsewhere, "20250601-100000-away", "main")

	if err := sessionRmCmd.Flags().Set("repo", here); err != nil {
		t.Fatalf("set --repo: %v", err)
	}
	t.Cleanup(func() { _ = sessionRmCmd.Flags().Set("repo", ""); sessionRmRepoDir = "" })

	joined := strings.Join(firstOf(completeSessionIDsAnywhere(sessionRmCmd, nil, "20250601")), "\n")
	if !strings.Contains(joined, "20250601-100000-here") {
		t.Errorf("completion lost this repository's session; got %q", joined)
	}
	if strings.Contains(joined, "20250601-100000-away") {
		t.Errorf("--repo should scope the completion; got %q", joined)
	}
}

func firstOf(completions []string, _ cobra.ShellCompDirective) []string { return completions }
