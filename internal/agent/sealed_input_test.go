// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/session"
)

// sealRepo builds a two-commit repo on a branch, so `main..feature` is a real
// range whose head a later commit can move.
func sealRepo(t *testing.T) string {
	t.Helper()
	dir := initPreviewRepo(t)
	gitIn(t, dir, "branch", "-M", "main")
	gitIn(t, dir, "checkout", "-b", "feature")
	commitIn(t, dir, "main.go", "package main\n\nfunc main() {}\n", "add main")
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func commitIn(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", msg)
}

// TestSealedInputPinsRunToAdmittedCommits is the guarantee that lets the resume
// decision live entirely before the run exists.
//
// The command resolves the input once to compare identities, and the run resolves
// it again to review it. If the second resolve could see a different commit, a
// ref moving in between would admit one input and review another — and the
// mismatch would only surface once a child session and manifest were on disk.
// Handing the run the sealed endpoints removes the window: the second resolve
// reads the same immutable commits, so it can only ever agree.
//
// The control half matters as much as the pinned half: without the seal the same
// moved ref really does change the identity, so this test proves the seal is what
// holds it still rather than the fixture being inert.
func TestSealedInputPinsRunToAdmittedCommits(t *testing.T) {
	dir := sealRepo(t)
	args := Args{
		RepoDir:  dir,
		From:     "main",
		To:       "feature",
		Template: template.Template{MaxTokens: 4000},
	}

	sealed, err := ResolveIdentity(context.Background(), args)
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if sealed.Resolution.ResolvedBase == "" || sealed.Resolution.ResolvedHead == "" {
		t.Fatalf("a range resolve must yield both endpoints, got %+v", sealed.Resolution)
	}

	// The ref moves after admission: `feature` now names a commit the admitted
	// identity never covered.
	commitIn(t, dir, "late.go", "package main\n\nfunc late() {}\n", "move the ref")

	t.Run("the sealed run still reviews the admitted input", func(t *testing.T) {
		pinned := args
		pinned.SealedInput = &sealed.Resolution
		if got := runPathIdentity(t, pinned); got != sealed.Identity {
			t.Errorf("a sealed run must review exactly the admitted input:\n admitted = %+v\n ran      = %+v",
				sealed.Identity, got)
		}
	})

	t.Run("without the seal the same move changes the input", func(t *testing.T) {
		if got := runPathIdentity(t, args); got == sealed.Identity {
			t.Fatal("fixture proves nothing: the moved ref must change an unsealed identity")
		}
	})
}

// runPathIdentity replays what the run itself selects — the same diff load
// followed by the same two filter passes — and reads the identity off that
// selection. Going through the Agent rather than through ResolveIdentity is the
// point: it is the run's own load that the seal has to steer.
func runPathIdentity(t *testing.T, args Args) session.RunIdentity {
	t.Helper()
	a := &Agent{args: args}
	if err := a.loadDiffs(context.Background()); err != nil {
		t.Fatalf("loadDiffs: %v", err)
	}
	a.diffs = a.filterLargeDiffs(a.filterDiffs(a.diffs))
	return a.runIdentity()
}
