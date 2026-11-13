// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
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

func writeResumeSessionWithOpts(t *testing.T, repoDir, branch string, opts session.SessionOptions, files ...string) string {
	t.Helper()
	sh := session.New(repoDir, branch, "fake", opts)
	for _, f := range files {
		sh.RecordReviewItemDone(f, "", f, "fp-"+f, nil)
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// writeRangeResumeSession persists a range-mode session with the given completed
// file checkpoints and returns its ID. HOME must already point at a temp dir.
func writeRangeResumeSession(t *testing.T, repoDir string, files ...string) string {
	t.Helper()
	return writeResumeSessionWithOpts(t, repoDir, "feature", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	}, files...)
}

// TestLoadReviewResumeState_WithSession drives the fixture-backed branches of
// loadReviewResumeState: a successful resume, a review-mode mismatch, and a
// session that completed no items.
func TestLoadReviewResumeState_WithSession(t *testing.T) {
	t.Run("success returns state with completed items", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeRangeResumeSession(t, repoDir, "a.go", "b.go")

		state, err := loadReviewResumeState(repoDir, &reviewOptions{resume: id, from: "main", to: "feature"})
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
		_, err := loadReviewResumeState(repoDir, &reviewOptions{resume: id, commit: "HEAD"})
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

		state, err := loadReviewResumeState(repoDir, &reviewOptions{resume: id, from: "main", to: "feature"})
		if err != nil {
			t.Fatalf("loadReviewResumeState: %v", err)
		}
		if state == nil || state.CompletedCount() != 0 {
			t.Fatalf("got %v, want an admitted state with 0 completed items", state)
		}
	})
}

func TestLoadReviewResumeState_InferenceAndConflicts(t *testing.T) {
	setTestHome(t, t.TempDir())
	repoDir := t.TempDir()
	rangeID := writeRangeResumeSession(t, repoDir, "a.go")
	commitID := writeResumeSessionWithOpts(t, repoDir, "main", session.SessionOptions{
		ReviewMode: session.ReviewModeCommit,
		DiffCommit: "abc1234",
	}, "c.go")
	wsID := writeResumeSessionWithOpts(t, repoDir, "main", session.SessionOptions{
		ReviewMode: session.ReviewModeWorkspace,
	}, "w.go")

	inferCases := []struct {
		name       string
		opts       reviewOptions
		wantMode   string
		wantFrom   string
		wantTo     string
		wantCommit string
	}{
		{name: "auto-infers range parameters when not provided", opts: reviewOptions{resume: rangeID}, wantMode: session.ReviewModeRange, wantFrom: "main", wantTo: "feature"},
		{name: "auto-infers commit parameters when not provided", opts: reviewOptions{resume: commitID}, wantMode: session.ReviewModeCommit, wantCommit: "abc1234"},
		{name: "auto-infers workspace mode when not provided", opts: reviewOptions{resume: wsID}, wantMode: session.ReviewModeWorkspace},
	}
	for _, tc := range inferCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			state, err := loadReviewResumeState(repoDir, &opts)
			if err != nil {
				t.Fatalf("loadReviewResumeState: %v", err)
			}
			if state == nil || state.ReviewMode != tc.wantMode {
				t.Fatalf("got %v, want mode %q", state, tc.wantMode)
			}
			if opts.from != tc.wantFrom || opts.to != tc.wantTo || opts.commit != tc.wantCommit {
				t.Errorf("got from=%q to=%q commit=%q, want %q/%q/%q", opts.from, opts.to, opts.commit, tc.wantFrom, tc.wantTo, tc.wantCommit)
			}
		})
	}

	t.Run("preserves explicit runtime parameter overrides", func(t *testing.T) {
		opts := reviewOptions{
			resume:          wsID,
			model:           "custom-model",
			provider:        "custom-provider",
			concurrency:     4,
			effort:          "high",
			maxTokensBudget: 50000,
		}
		if _, err := loadReviewResumeState(repoDir, &opts); err != nil {
			t.Fatalf("loadReviewResumeState: %v", err)
		}
		if opts.model != "custom-model" || opts.provider != "custom-provider" || opts.concurrency != 4 || opts.effort != "high" || opts.maxTokensBudget != 50000 {
			t.Errorf("runtime parameters were mutated: %+v", opts)
		}
	})

	conflictCases := []struct {
		name string
		opts reviewOptions
	}{
		{name: "rejects --commit on workspace session", opts: reviewOptions{resume: wsID, commit: "HEAD"}},
		{name: "rejects --from/--to on workspace session", opts: reviewOptions{resume: wsID, from: "main", to: "dev"}},
		{name: "rejects --commit on range session", opts: reviewOptions{resume: rangeID, commit: "HEAD"}},
		{name: "rejects --from/--to on commit session", opts: reviewOptions{resume: commitID, from: "main", to: "feature"}},
	}
	for _, tc := range conflictCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			if _, err := loadReviewResumeState(repoDir, &opts); err == nil || !strings.Contains(err.Error(), "parameter conflict") {
				t.Fatalf("expected parameter conflict error, got: %v", err)
			}
		})
	}
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

func resumeTestContext(repoDir string) *commonContext {
	return &commonContext{
		RepoDir:   repoDir,
		Template:  &template.Template{MaxTokens: 4000},
		IsGitRepo: true,
	}
}

// writeVerifiableParent persists a review session whose manifest records identity
// with the given provider and model, as a completed parent run would.
func writeVerifiableParent(t *testing.T, repoDir string, id session.RunIdentity, provider, model string) string {
	t.Helper()
	return writeVerifiableParentWithOpts(t, repoDir, "feature", session.SessionOptions{
		ReviewMode: session.ReviewModeRange,
		DiffFrom:   "HEAD~1",
		DiffTo:     "HEAD",
		Operation:  session.OperationReview,
	}, id, nil, provider, model)
}

func writeVerifiableParentWithOpts(t *testing.T, repoDir, branch string, sessOpts session.SessionOptions, id session.RunIdentity, fps []string, provider, model string) string {
	t.Helper()
	sh := session.New(repoDir, branch, model, sessOpts)
	b := sh.Manifest()
	b.SetRepository(session.ManifestRepository{IdentitySHA256: id.RepositorySHA256})
	b.SetInput(session.ManifestInput{
		Mode: id.Mode, ResolvedBase: id.ResolvedBase, ResolvedHead: id.ResolvedHead,
		SourceArtifactSHA256: id.SourceArtifactSHA256,
	})
	b.SetExecution(session.ManifestExecution{
		Provider: provider, Model: model, RuleConfigSHA256: id.RuleConfigSHA256,
	})
	items := fps
	recordDone := len(fps) > 0
	if !recordDone {
		items = []string{""}
	}
	for i, fp := range items {
		itemID := fmt.Sprintf("item-%d", i)
		if err := b.RegisterSelected(session.CoverageItem{ItemID: itemID, Fingerprint: fp, Path: "main.go"}); err != nil {
			t.Fatalf("register selected: %v", err)
		}
		if err := b.MarkCompleted(itemID); err != nil {
			t.Fatalf("mark completed: %v", err)
		}
		if recordDone {
			sh.RecordReviewItemDone("main.go", "", "main.go", fp, nil)
		}
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
	t.Setenv("HOME", t.TempDir())
	repoDir := initResumeRepo(t)
	cc := resumeTestContext(repoDir)
	// A real range: HEAD~1..HEAD contains the commit that added main.go, so the
	// resolved identity is derived from actual reviewable content rather than from
	// the digest of an empty selected set.
	opts := reviewOptions{from: "HEAD~1", to: "HEAD"}

	sealed, err := agent.ResolveIdentity(context.Background(), agent.Args{
		RepoDir:    repoDir,
		From:       opts.from,
		To:         opts.to,
		ReviewMode: reviewModeFromOptions(opts),
		Template:   *cc.Template,
	})
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}

	parentID := writeVerifiableParent(t, repoDir, sealed.Identity, "anthropic", "claude")
	state, err := loadReviewResumeState(repoDir, &reviewOptions{resume: parentID, from: opts.from, to: opts.to})
	if err != nil {
		t.Fatalf("loadReviewResumeState: %v", err)
	}
	if state.Manifest == nil {
		t.Fatal("parent manifest did not survive the round trip through session_end")
	}

	t.Run("same provider and model is accepted", func(t *testing.T) {
		rt := &llmRuntime{Provider: "anthropic", Model: "claude"}
		if _, err := validateResumeIdentity(context.Background(), cc, opts, rt, state); err != nil {
			t.Errorf("want accepted, got: %v", err)
		}
	})

	t.Run("nil state is a non-resume run", func(t *testing.T) {
		rt := &llmRuntime{Provider: "anthropic", Model: "claude"}
		if _, err := validateResumeIdentity(context.Background(), cc, opts, rt, nil); err != nil {
			t.Errorf("a run with no --resume must not be checked, got: %v", err)
		}
	})

	t.Run("differently spelled but equivalent refs are accepted", func(t *testing.T) {
		// The false-rejection half of the contract. The parent was reviewed as
		// HEAD~1..HEAD; naming the very same two commits by full SHA is the same
		// input, and a check that compared ref text would kill this resume.
		equivalent := opts
		equivalent.from = revParse(t, repoDir, "HEAD~1")
		equivalent.to = revParse(t, repoDir, "HEAD")
		if equivalent.from == opts.from || equivalent.to == opts.to {
			t.Fatal("fixture must use a different spelling than the parent did")
		}
		rt := &llmRuntime{Provider: "anthropic", Model: "claude"}
		if _, err := validateResumeIdentity(context.Background(), cc, equivalent, rt, state); err != nil {
			t.Errorf("ref spelling must not decide admission, got: %v", err)
		}
	})

	t.Run("implicit provider change is rejected and persists nothing", func(t *testing.T) {
		before := countSessionFiles(t, repoDir)

		rt := &llmRuntime{Provider: "openai", Model: "gpt-5"}
		_, err := validateResumeIdentity(context.Background(), cc, opts, rt, state)
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
		rt := &llmRuntime{Provider: "openai", Model: "gpt-5"}
		if _, err := validateResumeIdentity(context.Background(), cc, explicit, rt, state); err != nil {
			t.Errorf("want accepted with --provider, got: %v", err)
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

		rt := &llmRuntime{Provider: "anthropic", Model: "claude"}
		_, err := validateResumeIdentity(context.Background(), cc, opts, rt, state)
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

func TestValidateResumeIdentity_Workspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repoDir := initResumeRepo(t)
	gitIn(t, repoDir, "branch", "-M", "main")
	// Add a third commit without touching main.go so HEAD~1 rollback keeps the
	// uncommitted modification to main.go intact.
	commitFile(t, repoDir, "step.go", "package main\n\nfunc step() {}\n", "add step")
	cc := resumeTestContext(repoDir)

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() { println(\"ws\") }\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	opts := reviewOptions{}
	sealed, err := agent.ResolveIdentity(context.Background(), agent.Args{
		RepoDir:    repoDir,
		ReviewMode: session.ReviewModeWorkspace,
		Template:   *cc.Template,
	})
	if err != nil {
		t.Fatalf("ResolveIdentity: %v", err)
	}

	parentID := writeVerifiableParentWithOpts(t, repoDir, "main", session.SessionOptions{
		ReviewMode: session.ReviewModeWorkspace,
		Operation:  session.OperationReview,
	}, sealed.Identity, sealed.ItemFingerprints, "anthropic", "claude")
	state, err := loadReviewResumeState(repoDir, &reviewOptions{resume: parentID})
	if err != nil {
		t.Fatalf("loadReviewResumeState: %v", err)
	}

	commitSingleFile := func(t *testing.T, name, content, msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		gitIn(t, repoDir, "add", name)
		gitIn(t, repoDir, "commit", "-m", msg)
	}

	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr string
	}{
		{
			name: "matching branch and workspace diff is accepted",
		},
		{
			name: "different branch with same commit is accepted",
			setup: func(t *testing.T) {
				gitIn(t, repoDir, "checkout", "-b", "other-branch")
				t.Cleanup(func() { gitIn(t, repoDir, "checkout", "main") })
			},
		},
		{
			name: "descendant commit is accepted",
			setup: func(t *testing.T) {
				commitSingleFile(t, "extra.go", "package main\n\nfunc extra() {}\n", "extra commit")
			},
		},
		{
			name: "historical rollback commit is rejected",
			setup: func(t *testing.T) {
				gitIn(t, repoDir, "checkout", "main~2")
				t.Cleanup(func() { gitIn(t, repoDir, "checkout", "main") })
			},
			wantErr: "is not a descendant of session base commit",
		},
		{
			name: "non-descendant commit is rejected",
			setup: func(t *testing.T) {
				gitIn(t, repoDir, "checkout", "--orphan", "diverged-branch")
				t.Cleanup(func() { gitIn(t, repoDir, "checkout", "main") })
				commitSingleFile(t, "div.go", "package main\n", "diverged commit")
			},
			wantErr: "is not a descendant of session base commit",
		},
		{
			name: "zero matching items is rejected",
			setup: func(t *testing.T) {
				if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() { println(\"entirely different\") }\n"), 0o644); err != nil {
					t.Fatalf("write main.go: %v", err)
				}
			},
			wantErr: "no completed review items",
		},
	}

	rt := &llmRuntime{Provider: "anthropic", Model: "claude"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}
			_, err := validateResumeIdentity(context.Background(), cc, opts, rt, state)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("want accepted, got error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("want error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}
