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
)

// Cmd-layer tests for the shallow-clone recovery in validateReviewRefs
// (#634). Fixtures are throwaway repositories under t.TempDir() with a
// file:// origin — hermetic, no network.

// runGitFixture runs git in dir, failing the test on error, and returns
// trimmed stdout.
func runGitFixture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git -C %s %v: %v\n%s", dir, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// shallowFixture is an upstream repo plus a GIT_DEPTH=1 single-branch clone.
type shallowFixture struct {
	upstreamDir string
	cloneDir    string
	mainBranch  string
	mainTip     string
	featureTip  string
}

func newShallowFixture(t *testing.T) *shallowFixture {
	t.Helper()
	up := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"config", "commit.gpgsign", "false"},
	} {
		runGitFixture(t, up, args...)
	}
	write := func(name, content, msg string) {
		t.Helper()
		if err := execWriteFile(filepath.Join(up, name), content); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		runGitFixture(t, up, "add", name)
		runGitFixture(t, up, "commit", "-q", "-m", msg)
	}
	write("f.txt", "c1\n", "c1")
	write("f.txt", "c1\nc2\n", "c2")
	write("f.txt", "c1\nc2\nc3\n", "c3")
	mainBranch := runGitFixture(t, up, "rev-parse", "--abbrev-ref", "HEAD")
	runGitFixture(t, up, "checkout", "-q", "-b", "feature")
	write("feat.txt", "feat1\n", "feat1")
	write("feat.txt", "feat1\nfeat2\n", "feat2")
	featureTip := runGitFixture(t, up, "rev-parse", "HEAD")
	runGitFixture(t, up, "checkout", "-q", mainBranch)
	write("f.txt", "c1\nc2\nc3\nc4\n", "c4")
	write("f.txt", "c1\nc2\nc3\nc4\nc5\n", "c5")
	mainTip := runGitFixture(t, up, "rev-parse", "HEAD")

	clone := filepath.Join(t.TempDir(), "clone")
	parent := t.TempDir()
	cmd := exec.Command("git", "-C", parent, "clone", "-q", "--depth", "1", "--single-branch", "--branch", "feature", "file://"+up, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shallow clone: %v\n%s", err, out)
	}
	return &shallowFixture{
		upstreamDir: up,
		cloneDir:    clone,
		mainBranch:  mainBranch,
		mainTip:     mainTip,
		featureTip:  featureTip,
	}
}

func execWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// TestValidateReviewRefsShallowRetrySubstitutesSHA covers the retry branch:
// `--from <bare target branch>` fails raw resolution in the GIT_DEPTH=1
// clone, gets fetched at depth 1, resolves via the origin/<name> fallback,
// and the resolved SHA replaces the flag value (the downstream diff/show
// call sites are not fallback-aware). `--to <CI sha>` resolved raw and keeps
// its spelling.
func TestValidateReviewRefsShallowRetrySubstitutesSHA(t *testing.T) {
	fx := newShallowFixture(t)
	opts := reviewOptions{from: fx.mainBranch, to: fx.featureTip}

	if err := validateReviewRefs(context.Background(), nil, fx.cloneDir, &opts); err != nil {
		t.Fatalf("validateReviewRefs in shallow clone: %v", err)
	}
	if opts.from != fx.mainTip {
		t.Errorf("--from substituted to %q, want the main tip %q", opts.from, fx.mainTip)
	}
	if opts.to != fx.featureTip {
		t.Errorf("--to = %q, want the raw spelling kept (%q)", opts.to, fx.featureTip)
	}
	// The N3 hand-off: the substituted value must resolve through a plain,
	// non-fallback rev-parse (what `git diff`/`git show` will do with it).
	if _, err := runGitCmd(fx.cloneDir, "rev-parse", "--verify", "--end-of-options", opts.from+"^{commit}"); err != nil {
		t.Errorf("substituted --from %q does not resolve raw: %v", opts.from, err)
	}
}

// TestValidateReviewRefsShallowOriginSpelling covers the canonical GitLab
// spelling `--from origin/<target branch>`: it fails raw resolution (the
// single-branch clone has no such tracking ref), gets fetched, and then
// resolves under its own spelling — which every downstream raw `git diff`/
// `git show` accepts, so it is deliberately NOT substituted. Only spellings
// that resolve via the fallback (bare names, ocr-fetch SHAs) are replaced.
func TestValidateReviewRefsShallowOriginSpelling(t *testing.T) {
	fx := newShallowFixture(t)
	opts := reviewOptions{from: "origin/" + fx.mainBranch, to: fx.featureTip}

	if err := validateReviewRefs(context.Background(), nil, fx.cloneDir, &opts); err != nil {
		t.Fatalf("validateReviewRefs with origin/ spelling: %v", err)
	}
	if opts.from != "origin/"+fx.mainBranch {
		t.Errorf("--from = %q, want the origin/ spelling kept (it resolves raw post-fetch)", opts.from)
	}
	// And the kept spelling must resolve raw now, which is what makes it safe
	// downstream.
	if _, err := runGitCmd(fx.cloneDir, "rev-parse", "--verify", "--end-of-options", opts.from+"^{commit}"); err != nil {
		t.Errorf("--from %q does not resolve after recovery: %v", opts.from, err)
	}
}

// TestValidateReviewRefsShallowToBareBranch covers the mixed case: the
// origin/ spelling is kept while the bare to-side name — which resolves only
// through the fallback — is substituted with its SHA.
func TestValidateReviewRefsShallowToBareBranch(t *testing.T) {
	fx := newShallowFixture(t)
	opts := reviewOptions{from: "origin/" + fx.mainBranch, to: fx.mainBranch}

	if err := validateReviewRefs(context.Background(), nil, fx.cloneDir, &opts); err != nil {
		t.Fatalf("validateReviewRefs with bare --to: %v", err)
	}
	if opts.from != "origin/"+fx.mainBranch {
		t.Errorf("--from = %q, want the origin/ spelling kept", opts.from)
	}
	if opts.to != fx.mainTip {
		t.Errorf("--to = %q, want the substituted main tip %q", opts.to, fx.mainTip)
	}
}

// TestValidateReviewRefsShallowCommitMode covers `--commit <bare branch>`:
// commit mode resolves a single commit, and the same recovery applies.
func TestValidateReviewRefsShallowCommitMode(t *testing.T) {
	fx := newShallowFixture(t)
	opts := reviewOptions{commit: fx.mainBranch}

	if err := validateReviewRefs(context.Background(), nil, fx.cloneDir, &opts); err != nil {
		t.Fatalf("validateReviewRefs --commit in shallow clone: %v", err)
	}
	if opts.commit != fx.mainTip {
		t.Errorf("--commit substituted to %q, want %q", opts.commit, fx.mainTip)
	}
}

// TestValidateReviewRefsShallowNoRemoteKeepsError pins the no-origin path:
// today's exact error, no substitution, no recovery.
func TestValidateReviewRefsShallowNoRemoteKeepsError(t *testing.T) {
	fx := newShallowFixture(t)
	runGitFixture(t, fx.cloneDir, "remote", "remove", "origin")
	opts := reviewOptions{from: fx.mainBranch, to: fx.featureTip}

	err := validateReviewRefs(context.Background(), nil, fx.cloneDir, &opts)
	if err == nil {
		t.Fatal("expected today's error without an origin remote")
	}
	want := fmt.Sprintf("--from value %q is not a valid commit ref", fx.mainBranch)
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want prefix %q", err, want)
	}
	if opts.from != fx.mainBranch || opts.to != fx.featureTip {
		t.Errorf("options were modified without recovery: %+v", opts)
	}
}

// TestValidateReviewRefsShallowUnreachableOriginKeepsError pins the offline
// path: the recovery fetch fails, the failure is reported as context, and the
// verdict stays today's exact error.
func TestValidateReviewRefsShallowUnreachableOriginKeepsError(t *testing.T) {
	fx := newShallowFixture(t)
	runGitFixture(t, fx.cloneDir, "remote", "set-url", "origin", "file:///ocr-634-nonexistent-origin")
	opts := reviewOptions{from: fx.mainBranch, to: fx.featureTip}

	err := validateReviewRefs(context.Background(), nil, fx.cloneDir, &opts)
	if err == nil {
		t.Fatal("expected today's error with an unreachable origin")
	}
	want := fmt.Sprintf("--from value %q is not a valid commit ref", fx.mainBranch)
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want prefix %q", err, want)
	}
	if opts.from != fx.mainBranch {
		t.Errorf("--from was substituted despite the failed fetch: %q", opts.from)
	}
}
