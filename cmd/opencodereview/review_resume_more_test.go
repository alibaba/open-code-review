// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// writeRangeResumeSession persists a range-mode session with the given completed
// file checkpoints and returns its ID. HOME must already point at a temp dir.
func writeRangeResumeSession(t *testing.T, repoDir string, files ...string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	})
	for _, f := range files {
		sh.RecordReviewItemDone(f, "", f, "fp-"+f, nil)
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// TestLoadReviewResumeState_WithSession drives the fixture-backed branches of
// loadReviewResumeState: a successful resume, a review-mode mismatch, and a
// session that completed no items.
func TestLoadReviewResumeState_WithSession(t *testing.T) {
	t.Run("success returns state with completed items", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir, "a.go", "b.go")

		state, err := loadReviewResumeState(repoDir, reviewOptions{resume: id, from: "main", to: "feature"})
		if err != nil {
			t.Fatalf("loadReviewResumeState: %v", err)
		}
		if state == nil || state.CompletedCount() != 2 {
			t.Fatalf("got %v, want state with 2 completed items", state)
		}
	})

	t.Run("review mode mismatch errors", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir, "a.go")

		// Session was range-mode; request commit-mode resume.
		_, err := loadReviewResumeState(repoDir, reviewOptions{resume: id, commit: "HEAD"})
		if err == nil {
			t.Fatal("expected error for mode mismatch")
		}
	})

	// A parent run where every item failed used to be blocked here, which made
	// the one case resume exists for unrecoverable. It is now admitted: the
	// manifest is verifiable, so the whole selected set is simply re-dispatched.
	t.Run("no completed items is admitted", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir) // no items recorded

		state, err := loadReviewResumeState(repoDir, reviewOptions{resume: id, from: "main", to: "feature"})
		if err != nil {
			t.Fatalf("loadReviewResumeState: %v", err)
		}
		if state == nil || state.CompletedCount() != 0 {
			t.Fatalf("got %v, want an admitted state with 0 completed items", state)
		}
	})
}

// gitIn runs git in dir with a fixed identity so commits are reproducible.
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

// commitFile writes content to name and commits it.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", message)
}

// revParse resolves a ref to its full SHA, so a test can spell the same commit a
// different way.
func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// initResumeRepo adds a second commit to the shared single-commit fixture, so
// HEAD~1..HEAD is a real range with a reviewable Go file in it.
func initResumeRepo(t *testing.T) string {
	t.Helper()
	dir := initTestGitRepo(t)
	commitFile(t, dir, "main.go", "package main\n\nfunc main() {}\n", "add main")
	return dir
}

func resumeTestArgs(repoDir string, opts reviewOptions) agent.Args {
	return agent.Args{
		RepoDir:    repoDir,
		From:       opts.from,
		To:         opts.to,
		ReviewMode: reviewModeFromOptions(opts),
		Template:   template.Template{MaxTokens: 4000},
		Provider:   "anthropic",
		Model:      "claude",
	}
}

// writeVerifiableParent persists a review session whose manifest records identity
// with the given provider and model, as a completed parent run would.
func writeVerifiableParent(t *testing.T, repoDir string, id session.RunIdentity, provider, model string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", model, session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "HEAD~1",
		DiffTo:     "HEAD",
		Operation:  session.OperationReview,
	})
	b := sh.Manifest()
	b.SetRepository(session.ManifestRepository{IdentitySHA256: id.RepositorySHA256})
	b.SetInput(session.ManifestInput{
		Mode: id.Mode, ResolvedBase: id.ResolvedBase, ResolvedHead: id.ResolvedHead,
		SourceArtifactSHA256: id.SourceArtifactSHA256,
	})
	b.SetExecution(session.ManifestExecution{
		Provider: provider, Model: model, RuleConfigSHA256: id.RuleConfigSHA256,
		ReviewPromptSHA256: id.ReviewPromptSHA256,
	})
	if err := b.RegisterSelected(session.CoverageItem{ItemID: "item-1", Path: "main.go"}); err != nil {
		t.Fatalf("register selected: %v", err)
	}
	if err := b.MarkCompleted("item-1"); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	m, err := b.Finalize(time.Second)
	if err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	sh.SetFinalManifest(&m)
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// writeWorkspaceResumeSession persists a workspace-mode review session with the
// given completed file checkpoints and returns its ID. HOME must already point at
// a temp dir.
func writeWorkspaceResumeSession(t *testing.T, repoDir string, files ...string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeWorkspace,
	})
	for _, f := range files {
		sh.RecordReviewItemDone(f, "", f, "fp-"+f, nil)
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// writeVerifiableWorkspaceParent persists a workspace-mode review session whose
// manifest records the given identity, as a completed parent run would.
func writeVerifiableWorkspaceParent(t *testing.T, repoDir string, id session.RunIdentity, provider, model string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", model, session.SessionOptions{
		ReviewMode: session.ReviewModeWorkspace,
		Operation:  session.OperationReview,
	})
	b := sh.Manifest()
	b.SetRepository(session.ManifestRepository{IdentitySHA256: id.RepositorySHA256})
	b.SetInput(session.ManifestInput{
		Mode: id.Mode, ResolvedBase: id.ResolvedBase, ResolvedHead: id.ResolvedHead,
		SourceArtifactSHA256: id.SourceArtifactSHA256,
	})
	b.SetExecution(session.ManifestExecution{
		Provider: provider, Model: model, RuleConfigSHA256: id.RuleConfigSHA256,
		ReviewPromptSHA256: id.ReviewPromptSHA256,
	})
	if err := b.RegisterSelected(session.CoverageItem{ItemID: "item-1", Path: "main.go"}); err != nil {
		t.Fatalf("register selected: %v", err)
	}
	if err := b.MarkCompleted("item-1"); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	m, err := b.Finalize(time.Second)
	if err != nil {
		t.Fatalf("finalize manifest: %v", err)
	}
	sh.SetFinalManifest(&m)
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

func countSessionFiles(t *testing.T, repoDir string) int {
	t.Helper()
	dir, err := session.SessionsDir(repoDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read sessions dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n
}

// TestValidateResumeIdentity drives the command-layer check against a parent that
// really exists on disk, with the identity resolved from real git input rather
// than a fixture digest. Its most important assertion is the last one: a rejected
// resume must not leave a session behind, which is only true while the check runs
// before agent.New.
func TestValidateResumeIdentity(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := initResumeRepo(t)
	// A real range: HEAD~1..HEAD contains the commit that added main.go, so the
	// resolved identity is derived from actual reviewable content rather than from
	// the digest of an empty selected set.
	opts := reviewOptions{from: "HEAD~1", to: "HEAD"}
	args := resumeTestArgs(repoDir, opts)

	sealed, err := agent.ResolveIdentity(context.Background(), args)
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}

	parentID := writeVerifiableParent(t, repoDir, sealed.Identity, "anthropic", "claude")
	state, err := loadReviewResumeState(repoDir, reviewOptions{resume: parentID, from: opts.from, to: opts.to})
	if err != nil {
		t.Fatalf("loadReviewResumeState: %v", err)
	}
	if state.Manifest == nil {
		t.Fatal("parent manifest did not survive the round trip through session_end")
	}
	args.Resume = state

	t.Run("same provider and model is accepted", func(t *testing.T) {
		if _, err := validateResumeIdentity(context.Background(), args, opts); err != nil {
			t.Errorf("want accepted, got: %v", err)
		}
	})

	t.Run("nil state is a non-resume run", func(t *testing.T) {
		fresh := args
		fresh.Resume = nil
		if _, err := validateResumeIdentity(context.Background(), fresh, opts); err != nil {
			t.Errorf("a run with no --resume must not be checked, got: %v", err)
		}
	})

	t.Run("differently spelled but equivalent refs are accepted", func(t *testing.T) {
		// The false-rejection half of the contract. The parent was reviewed as
		// HEAD~1..HEAD; naming the very same two commits by full SHA is the same
		// input, and a check that compared ref text would kill this resume.
		equivalent := args
		equivalent.From = revParse(t, repoDir, "HEAD~1")
		equivalent.To = revParse(t, repoDir, "HEAD")
		if equivalent.From == args.From || equivalent.To == args.To {
			t.Fatal("fixture must use a different spelling than the parent did")
		}
		if _, err := validateResumeIdentity(context.Background(), equivalent, opts); err != nil {
			t.Errorf("ref spelling must not decide admission, got: %v", err)
		}
	})

	t.Run("implicit provider change is rejected and persists nothing", func(t *testing.T) {
		before := countSessionFiles(t, repoDir)

		changed := args
		changed.Provider, changed.Model = "openai", "gpt-5"
		_, err := validateResumeIdentity(context.Background(), changed, opts)
		if err == nil {
			t.Fatal("want rejection for a provider change with no --provider flag")
		}
		if !strings.Contains(err.Error(), "provider changed") {
			t.Errorf("error must name the provider as the cause, got: %v", err)
		}
		if after := countSessionFiles(t, repoDir); after != before {
			t.Errorf("rejection created %d session file(s); it must persist nothing", after-before)
		}
	})

	t.Run("explicit provider change is accepted", func(t *testing.T) {
		// opts.provider being non-empty is exactly what "--provider was passed"
		// means: the flag defaults to empty and nothing else assigns it.
		explicit := opts
		explicit.provider = "openai"
		changed := args
		changed.Provider, changed.Model = "openai", "gpt-5"
		if _, err := validateResumeIdentity(context.Background(), changed, explicit); err != nil {
			t.Errorf("want accepted with --provider, got: %v", err)
		}
	})

	t.Run("explicit model change requires matching prompts", func(t *testing.T) {
		explicit := opts
		explicit.model = "claude-next"
		changed := args
		changed.Model = explicit.model
		if _, err := validateResumeIdentity(context.Background(), changed, explicit); err != nil {
			t.Fatalf("want accepted with --model, got: %v", err)
		}

		changed.Template.MainTask.Messages = []template.ChatMessage{{Role: "system", Content: "Review the public API."}}
		_, err := validateResumeIdentity(context.Background(), changed, explicit)
		if err == nil || !strings.Contains(err.Error(), "review prompt identity changed") || !strings.Contains(err.Error(), "omit --resume") {
			t.Errorf("want prompt rejection with fresh-review hint despite --model, got: %v", err)
		}
	})

	t.Run("legacy manifest without prompt identity is rejected", func(t *testing.T) {
		legacyIdentity := sealed.Identity
		legacyIdentity.ReviewPromptSHA256 = ""
		legacy := opts
		legacy.resume = writeVerifiableParent(t, repoDir, legacyIdentity, args.Provider, args.Model)
		shown := captureStdout(t, func() {
			if err := runSessionShowCompat([]string{"--repo", repoDir, "--json", legacy.resume}); err != nil {
				t.Errorf("session show must accept a legacy manifest: %v", err)
			}
		})
		if !strings.Contains(shown, legacy.resume) {
			t.Errorf("session show did not display legacy session %q: %s", legacy.resume, shown)
		}
		legacyState, err := loadReviewResumeState(repoDir, legacy)
		if err != nil {
			t.Fatalf("legacy session must still load: %v", err)
		}
		legacyArgs := args
		legacyArgs.Resume = legacyState
		_, err = validateResumeIdentity(context.Background(), legacyArgs, legacy)
		if err == nil || !strings.Contains(err.Error(), "no review prompt identity") || !strings.Contains(err.Error(), "omit --resume") {
			t.Errorf("want missing prompt identity error with fresh-review hint, got: %v", err)
		}
	})

	// Last, because it moves the repo's HEAD for good: every case above needs the
	// input to still match.
	t.Run("changed input is rejected and persists nothing", func(t *testing.T) {
		before := countSessionFiles(t, repoDir)

		// This is the motivating case. The user re-runs the identical command —
		// `--from HEAD~1 --to HEAD` is unchanged — but a new commit means those refs
		// now name a different diff, so the checkpoints describe work on content
		// that is no longer under review.
		commitFile(t, repoDir, "main.go",
			"package main\n\nfunc main() { println(\"changed\") }\n", "change main")

		_, err := validateResumeIdentity(context.Background(), args, opts)
		if err == nil {
			t.Fatal("want rejection after the same refs resolved to a different diff")
		}
		if !strings.Contains(err.Error(), "reviewed input changed") {
			t.Errorf("error must name the input as the cause, got: %v", err)
		}
		if after := countSessionFiles(t, repoDir); after != before {
			t.Errorf("rejection created %d session file(s); it must persist nothing", after-before)
		}
	})
}

// TestValidateResumeIdentityWorkspace pins the workspace half of resume admission:
// the worktree is the input, so it must be unchanged, and there is no commit
// endpoint to pin, which is why the base commit is compared instead.
func TestValidateResumeIdentityWorkspace(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := initResumeRepo(t)
	// An uncommitted edit is what a workspace run reviews; HEAD alone has no diff
	// to offer.
	wip := "package main\n\nfunc main() { println(\"wip\") }\n"
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(wip), 0o644); err != nil {
		t.Fatalf("write workspace change: %v", err)
	}
	opts := reviewOptions{}
	args := resumeTestArgs(repoDir, opts)

	sealed, err := agent.ResolveIdentity(context.Background(), args)
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}
	if sealed.Identity.Mode != session.InputModeWorkspace {
		t.Fatalf("identity mode = %q, want workspace", sealed.Identity.Mode)
	}

	parentID := writeVerifiableWorkspaceParent(t, repoDir, sealed.Identity, "anthropic", "claude")
	state, err := loadReviewResumeState(repoDir, reviewOptions{resume: parentID})
	if err != nil {
		t.Fatalf("loadReviewResumeState: %v", err)
	}
	if state.ReviewMode != session.ReviewModeWorkspace {
		t.Fatalf("state mode = %q, want workspace", state.ReviewMode)
	}
	args.Resume = state

	t.Run("unchanged worktree is accepted", func(t *testing.T) {
		if _, err := validateResumeIdentity(context.Background(), args, opts); err != nil {
			t.Errorf("want accepted, got: %v", err)
		}
	})

	t.Run("changed worktree is rejected and persists nothing", func(t *testing.T) {
		before := countSessionFiles(t, repoDir)
		if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(wip+"// more\n"), 0o644); err != nil {
			t.Fatalf("write workspace change: %v", err)
		}
		_, err := validateResumeIdentity(context.Background(), args, opts)
		if err == nil {
			t.Fatal("want rejection after the worktree changed")
		}
		if !strings.Contains(err.Error(), "workspace changed") {
			t.Errorf("error must name the workspace as the cause, got: %v", err)
		}
		if after := countSessionFiles(t, repoDir); after != before {
			t.Errorf("rejection created %d session file(s); it must persist nothing", after-before)
		}
	})
}

// TestFileReadRef pins the other half of sealing the input. The diff is pinned to
// the commit admission resolved, so file_read has to read at that same commit:
// leaving it on the ref the user typed would let the model read one version of a
// file while reviewing the diff of another.
func TestFileReadRef(t *testing.T) {
	sealed := &diff.InputResolution{ResolvedBase: "aaa", ResolvedHead: "bbb"}
	cases := []struct {
		name   string
		mode   tool.ReviewMode
		opts   reviewOptions
		sealed *diff.InputResolution
		want   string
	}{
		{"range without a seal keeps the typed ref", tool.ModeRange,
			reviewOptions{from: "main", to: "feature"}, nil, "feature"},
		{"range with a seal reads at the sealed head", tool.ModeRange,
			reviewOptions{from: "main", to: "feature"}, sealed, "bbb"},
		{"commit with a seal reads at the sealed head", tool.ModeCommit,
			reviewOptions{commit: "HEAD"}, sealed, "bbb"},
		// Workspace content is the working tree, which is exactly what its diff
		// describes, so there is no ref to pin and none to carry over.
		{"workspace has no ref even with a seal", tool.ModeWorkspace,
			reviewOptions{}, sealed, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fileReadRef(tc.mode, tc.opts, tc.sealed); got != tc.want {
				t.Errorf("fileReadRef = %q, want %q", got, tc.want)
			}
		})
	}
}
