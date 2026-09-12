// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// Shallow-clone integration tests for on-demand fetch (#634).
//
// Every fixture is a throwaway repository under t.TempDir() wired to a
// file:// origin: the tests are hermetic (no network, fixed trees, no timing
// dependence) and never touch any repository outside their temp dirs.

// upstreamFixture is the "origin" side: a main branch, a feature branch cut
// from mergeBase, and a tag pointing at mergeBase.
type upstreamFixture struct {
	dir        string
	mainBranch string // "main" or "master", git-version dependent
	mainTip    string
	mergeBase  string
	featureTip string
}

// newUpstreamFixture builds main c1..c5 with feature cut at c3 plus feat1 and
// feat2, and tag v1 at c3. The merge-base of the two branch tips is c3 (two
// commits back on each side).
func newUpstreamFixture(t *testing.T) *upstreamFixture {
	t.Helper()
	if skipNonUnixShallowFixture(t) {
		return nil
	}
	repo := initBareRepo(t)
	writeCommit(t, repo, "f.txt", "c1\n", "c1")
	writeCommit(t, repo, "f.txt", "c1\nc2\n", "c2")
	writeCommit(t, repo, "f.txt", "c1\nc2\nc3\n", "c3")
	mergeBase := gitOut(t, repo, "rev-parse", "HEAD")
	mainBranch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	runGitTest(t, repo, "checkout", "-q", "-b", "feature")
	runGitTest(t, repo, "tag", "v1")
	writeCommit(t, repo, "feat.txt", "feat1\n", "feat1")
	writeCommit(t, repo, "feat.txt", "feat1\nfeat2\n", "feat2")
	featureTip := gitOut(t, repo, "rev-parse", "HEAD")
	runGitTest(t, repo, "checkout", "-q", mainBranch)
	writeCommit(t, repo, "f.txt", "c1\nc2\nc3\nc4\n", "c4")
	writeCommit(t, repo, "f.txt", "c1\nc2\nc3\nc4\nc5\n", "c5")
	mainTip := gitOut(t, repo, "rev-parse", "HEAD")
	return &upstreamFixture{
		dir:        repo,
		mainBranch: mainBranch,
		mainTip:    mainTip,
		mergeBase:  mergeBase,
		featureTip: featureTip,
	}
}

// newDeepUpstreamFixture builds main c1..c12 with feature cut at c4 plus three
// commits, so the merge-base is eight commits back on the main side. This is
// the shape that forces the deepen loop through several doubling rounds.
func newDeepUpstreamFixture(t *testing.T) *upstreamFixture {
	t.Helper()
	if skipNonUnixShallowFixture(t) {
		return nil
	}
	repo := initBareRepo(t)
	writeCommit(t, repo, "f.txt", "c1\n", "c1")
	for i := 2; i <= 4; i++ {
		writeCommit(t, repo, "f.txt", fmt.Sprintf("c1..c%d\n", i), fmt.Sprintf("c%d", i))
	}
	mergeBase := gitOut(t, repo, "rev-parse", "HEAD")
	mainBranch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	runGitTest(t, repo, "checkout", "-q", "-b", "feature")
	for i := 1; i <= 3; i++ {
		writeCommit(t, repo, "feat.txt", fmt.Sprintf("feat%d\n", i), fmt.Sprintf("feat%d", i))
	}
	featureTip := gitOut(t, repo, "rev-parse", "HEAD")
	runGitTest(t, repo, "checkout", "-q", mainBranch)
	for i := 5; i <= 12; i++ {
		writeCommit(t, repo, "f.txt", fmt.Sprintf("c1..c%d\n", i), fmt.Sprintf("c%d", i))
	}
	mainTip := gitOut(t, repo, "rev-parse", "HEAD")
	return &upstreamFixture{
		dir:        repo,
		mainBranch: mainBranch,
		mainTip:    mainTip,
		mergeBase:  mergeBase,
		featureTip: featureTip,
	}
}

// newShallowClone clones the fixture the way GitLab CI does with GIT_DEPTH=1:
// single-branch, depth 1, feature checked out, and no main-side history or
// tracking ref at all.
func newShallowClone(t *testing.T, up *upstreamFixture) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	runGitTest(t, t.TempDir(), "clone", "-q", "--depth", "1", "--single-branch", "--branch", "feature", "file://"+up.dir, clone)
	return clone
}

// skipNonUnixShallowFixture gates the file://-clone fixtures. The return value
// lets callers bail out after the skip fired.
func skipNonUnixShallowFixture(t *testing.T) bool {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file:// shallow-clone fixture relies on a Unix-style path URL")
		return true
	}
	return false
}

// ensureRefsForTest mirrors the cmd layer's validateReviewRefs recovery core:
// one depth-1 fetch for the missing refs, then fallback resolution. It fails
// the test when the fetch itself fails, matching the happy-path callers.
func ensureRefsForTest(t *testing.T, repoDir string, refs ...string) map[string]string {
	t.Helper()
	resolved, err := EnsureShallowRefs(context.Background(), gitcmd.New(2), repoDir, refs)
	if err != nil {
		t.Fatalf("EnsureShallowRefs(%v): %v", refs, err)
	}
	return resolved
}

// TestShallowRangeReviewCanonicalGitLabCase is the #634 reproducer: a
// GIT_DEPTH=1 single-branch clone reviewing `--from origin/<target> --to
// <CI_COMMIT_SHA>`. It is driven through the real sequence (depth-1 ensure
// fetch, SHA substitution, then the doubling deepen loop inside
// computeMergeBase) because a single-shot fetch cannot exercise the loop's
// round-by-round convergence.
func TestShallowRangeReviewCanonicalGitLabCase(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	// Validation pass: origin/<target> fails raw resolution in the clone.
	from := "origin/" + up.mainBranch
	resolved := ensureRefsForTest(t, clone, from)
	sha, ok := resolved[from]
	if !ok {
		t.Fatalf("origin/%s did not resolve after the ensure fetch (map: %v)", up.mainBranch, resolved)
	}
	if sha != up.mainTip {
		t.Errorf("resolved %s = %s, want the main tip %s", from, sha, up.mainTip)
	}

	// The cmd layer keeps this spelling post-recovery (it resolves raw once
	// the fetch created the tracking ref); --to <CI sha> resolved raw all
	// along. The provider sees exactly what production would construct.
	provider := NewProvider(clone, from, up.featureTip, gitcmd.New(2))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff in shallow clone: %v", err)
	}
	if got := provider.MergeBase(context.Background()); got != up.mergeBase {
		t.Errorf("merge-base = %s, want %s (c3)", got, up.mergeBase)
	}
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1 (feat.txt): %+v", len(diffs), diffs)
	}
	if diffs[0].NewPath != "feat.txt" || diffs[0].Insertions != 2 {
		t.Errorf("diff = %q (+%d), want feat.txt with 2 insertions", diffs[0].NewPath, diffs[0].Insertions)
	}
}

// TestShallowRangeReviewBareFromName covers `--from main`: the bare spelling
// neither resolves nor fetches without the origin/ mapping, and post-fetch it
// only resolves through the origin/<name> fallback.
func TestShallowRangeReviewBareFromName(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	resolved := ensureRefsForTest(t, clone, up.mainBranch)
	sha, ok := resolved[up.mainBranch]
	if !ok || sha != up.mainTip {
		t.Fatalf("bare %s resolved to %v (want %s) after fetch", up.mainBranch, resolved, up.mainTip)
	}
	// The bare spelling itself must still refuse to resolve: substitution is
	// what makes the downstream raw `git diff <base> <sha>` work.
	if out, err := gitOutErr(t, clone, "rev-parse", "--verify", "--quiet", up.mainBranch+"^{commit}"); err == nil {
		t.Errorf("bare %s unexpectedly resolves as %s; test no longer pins the fallback", up.mainBranch, out)
	}

	provider := NewProvider(clone, sha, up.featureTip, gitcmd.New(2))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff with substituted from-SHA: %v", err)
	}
	if len(diffs) != 1 || diffs[0].NewPath != "feat.txt" {
		t.Fatalf("got %+v, want the feat.txt diff", diffs)
	}
}

// TestShallowRangeReviewToHead covers `--to HEAD`: HEAD resolves raw (it is
// the checked-out commit), and the deepen loop maps it to the current branch
// for its feature-side want instead of fetching the remote's default branch.
func TestShallowRangeReviewToHead(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	from := "origin/" + up.mainBranch
	resolved := ensureRefsForTest(t, clone, from)

	provider := NewProvider(clone, resolved[from], "HEAD", gitcmd.New(2))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff with --to HEAD: %v", err)
	}
	if got := provider.MergeBase(context.Background()); got != up.mergeBase {
		t.Errorf("merge-base = %s, want %s", got, up.mergeBase)
	}
	if len(diffs) != 1 || diffs[0].NewPath != "feat.txt" {
		t.Fatalf("got %+v, want the feat.txt diff", diffs)
	}
}

// TestShallowRangeReviewFromCheckedOutToBareTargetBranch covers
// `--from <checked-out branch> --to <bare target-branch name>` (validation
// N3): the to-side bare name resolves only via the fallback, and the resolved
// SHA must reach GetDiff's raw `git diff <base> <to>` — without the
// substitution this spelling dies with "unknown revision".
func TestShallowRangeReviewFromCheckedOutToBareTargetBranch(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	// "feature" resolves raw (it is checked out); only the to-side name needs
	// the recovery, exactly like validateReviewRefs' pending set.
	resolved := ensureRefsForTest(t, clone, up.mainBranch)
	provider := NewProvider(clone, "feature", resolved[up.mainBranch], gitcmd.New(2))
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff with bare to-side name substituted: %v", err)
	}
	if len(diffs) != 1 || diffs[0].NewPath != "f.txt" {
		t.Fatalf("got %+v, want the f.txt diff of the main-side commits", diffs)
	}
	if diffs[0].Insertions != 2 {
		t.Errorf("insertions = %d, want 2 (c4 and c5 lines)", diffs[0].Insertions)
	}
}

// TestShallowRangeReviewTagRef pins the tag path: `+v1:refs/remotes/origin/v1`
// also auto-creates refs/tags/v1 (git's tag auto-follow), `v1^{commit}`
// resolves, and a range from the tag completes after deepening.
func TestShallowRangeReviewTagRef(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	resolved := ensureRefsForTest(t, clone, "v1")
	if resolved["v1"] != up.mergeBase {
		t.Fatalf("v1 resolved to %v, want the tagged commit %s", resolved["v1"], up.mergeBase)
	}
	if got := gitOut(t, clone, "rev-parse", "--verify", "refs/tags/v1^{commit}"); got != up.mergeBase {
		t.Errorf("refs/tags/v1 = %s, want auto-follow to pin %s", got, up.mergeBase)
	}

	provider := NewProvider(clone, resolved["v1"], up.featureTip, gitcmd.New(2))
	if _, err := provider.GetDiff(context.Background()); err != nil {
		t.Fatalf("GetDiff from tag ref in shallow clone: %v", err)
	}
	if got := provider.MergeBase(context.Background()); got != up.mergeBase {
		t.Errorf("merge-base = %s, want the tagged commit %s", got, up.mergeBase)
	}
}

// TestShallowRangeReviewDeepHistory forces the doubling loop through several
// rounds: the merge-base is eight commits back on the main side of a
// twelve-commit history, which a depth-1 clone cannot reach.
func TestShallowRangeReviewDeepHistory(t *testing.T) {
	up := newDeepUpstreamFixture(t)
	clone := newShallowClone(t, up)

	from := "origin/" + up.mainBranch
	resolved := ensureRefsForTest(t, clone, from)
	provider := NewProvider(clone, resolved[from], up.featureTip, gitcmd.New(2))
	if _, err := provider.GetDiff(context.Background()); err != nil {
		t.Fatalf("GetDiff with deep history: %v", err)
	}
	if got := provider.MergeBase(context.Background()); got != up.mergeBase {
		t.Errorf("merge-base = %s, want %s", got, up.mergeBase)
	}
}

// TestShallowRangeReviewBoundExhaustedHintsShallow pins the bounded-failure
// contract: with the depth cap lowered by the test hook, exhausting the loop
// keeps today's error text as the prefix and appends the shallow-clone hint.
func TestShallowRangeReviewBoundExhaustedHintsShallow(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	orig := shallowDeepenMaxDepth
	shallowDeepenMaxDepth = 2 // the 2-back merge-base needs D=4: one doomed round
	t.Cleanup(func() { shallowDeepenMaxDepth = orig })

	from := "origin/" + up.mainBranch
	resolved := ensureRefsForTest(t, clone, from)
	provider := NewProvider(clone, resolved[from], up.featureTip, gitcmd.New(2))
	_, err := provider.GetDiff(context.Background())
	if err == nil {
		t.Fatal("expected GetDiff to fail with the deepen bound exhausted")
	}
	wantPrefix := fmt.Sprintf("cannot find merge-base between %s and %s", resolved[from], up.featureTip)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("error %q does not keep today's message as the prefix", err)
	}
	if !strings.Contains(err.Error(), "shallow clone") {
		t.Errorf("error %q does not mention the shallow-clone situation", err)
	}
	if !strings.Contains(err.Error(), "depth 2") {
		t.Errorf("error %q does not report the attempted depth bound", err)
	}
}

// TestShallowEnsureRejectedUnknownSHA pins the server-gated SHA-want edge: a
// SHA the origin does not have makes the whole fetch fail atomically, the
// error carries git's own diagnosis, and no refs are resolved.
func TestShallowEnsureRejectedUnknownSHA(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	const unknown = "1234567890123456789012345678901234567890"
	resolved, err := EnsureShallowRefs(context.Background(), gitcmd.New(2), clone, []string{unknown})
	if err == nil {
		t.Fatal("expected the fetch of an unknown SHA to fail")
	}
	if resolved != nil {
		t.Errorf("resolved map = %v, want nil on fetch failure", resolved)
	}
	if !strings.Contains(err.Error(), "git fetch failed") {
		t.Errorf("error %q lost the operation name", err)
	}
	// Anchor on the object id, not git's prose: the SHA is locale-proof and
	// proves git's message survived.
	if !strings.Contains(err.Error(), unknown) {
		t.Errorf("error %q carries no message from git", err)
	}
}

// TestShallowNoRemotePreservesTodayErrors pins the no-origin path: no fetch
// is ever attempted (argv recorded through a delegating git shim) and the
// provider keeps today's exact merge-base error, without the shallow hint.
func TestShallowNoRemotePreservesTodayErrors(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)
	runGitTest(t, clone, "remote", "remove", "origin")

	// Validation layer: nothing to do, and nothing attempted.
	resolved, err := EnsureShallowRefs(context.Background(), gitcmd.New(2), clone, []string{up.mainBranch})
	if err != nil || resolved != nil {
		t.Fatalf("EnsureShallowRefs without origin = (%v, %v), want (nil, nil)", resolved, err)
	}

	log := installRecordingGitShim(t)
	provider := NewProvider(clone, up.mainBranch, "HEAD", gitcmd.New(2))
	_, err = provider.GetDiff(context.Background())
	if err == nil {
		t.Fatal("expected GetDiff to fail without the origin history")
	}
	want := fmt.Sprintf("cannot find merge-base between %s and HEAD", up.mainBranch)
	if err.Error() != want {
		t.Errorf("error = %q, want today's exact %q", err, want)
	}
	if fetches := grepShimLog(t, log, "fetch"); len(fetches) > 0 {
		t.Errorf("git fetch attempted without an origin remote: %v", fetches)
	}
}

// TestNonShallowRepoNeverFetches pins requirement 2: a full clone pays zero
// new git invocations on the success path, and only the constant-time shallow
// probe on the merge-base failure path — never a fetch.
func TestNonShallowRepoNeverFetches(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	mainBranch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	runGitTest(t, repo, "checkout", "-q", "-b", "feature")
	writeCommit(t, repo, "b.txt", "side\n", "side change")

	log := installRecordingGitShim(t)

	// Success path: no shallow probe at all.
	if _, err := NewProvider(repo, mainBranch, "feature", gitcmd.New(2)).GetDiff(context.Background()); err != nil {
		t.Fatalf("GetDiff in full clone: %v", err)
	}
	for _, line := range readShimLog(t, log) {
		if strings.Contains(line, "is-shallow-repository") {
			t.Errorf("shallow probe run on a success path: %q", line)
		}
		if strings.HasPrefix(line, "fetch ") {
			t.Errorf("git fetch run on a success path: %q", line)
		}
	}

	// Failure path (unresolvable endpoint): today's exact error, one shallow
	// probe allowed, still no fetch.
	_, err := NewProvider(repo, mainBranch, "nonexistent-ref-xyz", gitcmd.New(2)).GetDiff(context.Background())
	if err == nil {
		t.Fatal("expected GetDiff to fail for an unresolvable ref")
	}
	want := fmt.Sprintf("cannot find merge-base between %s and nonexistent-ref-xyz", mainBranch)
	if err.Error() != want {
		t.Errorf("error = %q, want today's exact %q", err, want)
	}
	if fetches := grepShimLog(t, log, "fetch"); len(fetches) > 0 {
		t.Errorf("git fetch attempted in a non-shallow repository: %v", fetches)
	}
	probes := grepShimLog(t, log, "is-shallow-repository")
	if len(probes) > 1 {
		t.Errorf("shallow probe run %d times on the failure path, want exactly one", len(probes))
	}
}

// TestShallowUnreachableOriginSurfacesGitFailure pins the offline path: refs
// resolve (the ensure fetch ran while origin was still reachable), the
// merge-base is missing, and every deepen round fails against the now
// unreachable origin — today's error stays the prefix, the shallow hint and
// git's own diagnosis ride along.
func TestShallowUnreachableOriginSurfacesGitFailure(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	resolved := ensureRefsForTest(t, clone, "origin/"+up.mainBranch)
	runGitTest(t, clone, "remote", "set-url", "origin", "file:///ocr-634-nonexistent-origin")

	provider := NewProvider(clone, resolved["origin/"+up.mainBranch], up.featureTip, gitcmd.New(2))
	_, err := provider.GetDiff(context.Background())
	if err == nil {
		t.Fatal("expected GetDiff to fail against an unreachable origin")
	}
	wantPrefix := "cannot find merge-base between " + resolved["origin/"+up.mainBranch] + " and " + up.featureTip
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("error %q does not keep today's message as the prefix", err)
	}
	if !strings.Contains(err.Error(), "shallow clone") {
		t.Errorf("error %q does not mention the shallow-clone situation", err)
	}
	if !strings.Contains(err.Error(), "depth 2 failed") {
		t.Errorf("error %q does not surface the fetch failure with the bound", err)
	}
}

// TestShallowFetchArgvOrderPinned guards the load-bearing argv order: every
// fetch option must precede --end-of-options, which must precede origin and
// the destination-bearing refspecs. The reversed order (any option after
// --end-of-options) makes git parse the option as a refspec pathname and the
// whole feature dies with rc=128 — pinned empirically in PLAN_VALIDATION N1.
func TestShallowFetchArgvOrderPinned(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	// Recovery first (real git), then record every fetch the deepen loop
	// issues while GetDiff recovers the merge-base.
	ensureRefsForTest(t, clone, "origin/"+up.mainBranch)
	log := installRecordingGitShim(t)
	provider := NewProvider(clone, "origin/"+up.mainBranch, up.featureTip, gitcmd.New(2))
	if _, err := provider.GetDiff(context.Background()); err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	fetches := grepShimLog(t, log, "fetch ")
	if len(fetches) == 0 {
		t.Fatal("no deepen fetches recorded")
	}
	re := regexp.MustCompile(`^fetch --quiet --depth=[0-9]+ --end-of-options origin \+[^ ]+:[^ ]+( \+[^ ]+:[^ ]+)*$`)
	for _, line := range fetches {
		if !re.MatchString(line) {
			t.Errorf("fetch argv %q does not match the pinned order (options, --end-of-options, origin, destination-bearing refspecs)", line)
		}
	}
}

// TestShallowGarbageRefKeepsTodayErrorNoDeepen pins the defensive guards of
// the deepen loop: in a shallow clone WITH an origin, an endpoint that cannot
// resolve at all (garbage ref) or a spelling that cannot form a want
// ("origin/") must keep today's exact error — no shallow hint, since the loop
// never started — and attempt no fetch.
func TestShallowGarbageRefKeepsTodayErrorNoDeepen(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	log := installRecordingGitShim(t)
	for _, tc := range []struct {
		name string
		to   string
	}{
		{name: "unresolvable endpoint", to: "no-such-ref-xyz"},
		{name: "non-derivable spelling", to: "origin/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(clone, up.featureTip, tc.to, gitcmd.New(2)).GetDiff(context.Background())
			if err == nil {
				t.Fatal("expected GetDiff to fail")
			}
			want := fmt.Sprintf("cannot find merge-base between %s and %s", up.featureTip, tc.to)
			if err.Error() != want {
				t.Errorf("error = %q, want today's exact %q (no shallow hint)", err, want)
			}
		})
	}
	if fetches := grepShimLog(t, log, "fetch "); len(fetches) > 0 {
		t.Errorf("git fetch attempted for unresolvable endpoints: %v", fetches)
	}
}

// TestShallowEnsureNonDerivableSpellingSkipsFetch pins the EnsureShallowRefs
// want-derivation guards: a spelling that cannot form a refspec ("origin/")
// fetches nothing and resolves nothing on its own, yet a derivable ref in the
// same list still fetches and resolves — and the depth-1 ensure fetch itself
// is destination-bearing with the pinned argv order (the argv-order test only
// records deepen fetches, since recovery runs before its shim is installed).
func TestShallowEnsureNonDerivableSpellingSkipsFetch(t *testing.T) {
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	// Nothing derivable: no fetch at all.
	log := installRecordingGitShim(t)
	resolved, err := EnsureShallowRefs(context.Background(), gitcmd.New(2), clone, []string{"origin/"})
	if err != nil || resolved != nil {
		t.Fatalf("EnsureShallowRefs([origin/]) = (%v, %v), want (nil, nil)", resolved, err)
	}
	if fetches := grepShimLog(t, log, "fetch "); len(fetches) > 0 {
		t.Fatalf("git fetch attempted with no derivable want: %v", fetches)
	}

	// Mixed list: the derivable ref fetches once (destination-bearing), the
	// non-derivable spelling is skipped rather than failing the batch.
	resolved, err = EnsureShallowRefs(context.Background(), gitcmd.New(2), clone, []string{up.mainBranch, "origin/"})
	if err != nil {
		t.Fatalf("EnsureShallowRefs mixed list: %v", err)
	}
	if resolved[up.mainBranch] != up.mainTip {
		t.Errorf("resolved[%s] = %v, want the main tip %s", up.mainBranch, resolved[up.mainBranch], up.mainTip)
	}
	if _, ok := resolved["origin/"]; ok {
		t.Errorf("non-derivable spelling resolved to %q", resolved["origin/"])
	}
	fetches := grepShimLog(t, log, "fetch ")
	wantArgv := fmt.Sprintf("fetch --quiet --depth=1 --end-of-options origin +%s:refs/remotes/origin/%s", up.mainBranch, up.mainBranch)
	if len(fetches) != 1 || fetches[0] != wantArgv {
		t.Errorf("ensure fetch argv = %v, want exactly [%s]", fetches, wantArgv)
	}
}

// ---- recording git shim ----

// installRecordingGitShim puts a `git` at the front of PATH that appends every
// invocation's arguments to a log file and then delegates to the real git, so
// tests can assert which commands were (never) attempted without changing
// git's behavior.
func installRecordingGitShim(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH shim relies on a shebang script")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("locate git: %v", err)
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "git-argv.log")
	shim := filepath.Join(dir, "git")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shQuote(log) + "\nexec " + shQuote(realGit) + " \"$@\"\n"
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatalf("write git shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// shQuote single-quotes s for safe embedding in the shim's shell script.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func readShimLog(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read shim log: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func grepShimLog(t *testing.T, log, substr string) []string {
	t.Helper()
	var hits []string
	for _, line := range readShimLog(t, log) {
		if strings.Contains(line, substr) {
			hits = append(hits, line)
		}
	}
	return hits
}

// gitOutErr runs git and returns stdout and the error, for assertions that
// pin a ref NOT resolving.
func gitOutErr(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// errFakeFetch stands in for the exec error gitFailure wraps.
func errFakeFetch() error { return errors.New("exit status 128") }
