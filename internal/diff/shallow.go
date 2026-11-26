// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// On-demand fetch support for shallow clones (#634).
//
// A CI checkout like GitLab's `GIT_DEPTH: "1"` single-branch clone has neither the
// target-branch ref (`ocr review --from origin/main` fails rev-parse) nor enough
// history for `merge-base(from, to)` to find a common ancestor. When that happens,
// the missing refs and history are fetched from origin on demand instead of failing:
//
//   - Refs are fetched with destination-bearing refspecs (`+<src>:<dst>`). These are
//     the only form that extends the clone: a bare-SHA or bare-branch want lands in
//     FETCH_HEAD and is a silent no-op for a shallow boundary, while a destination
//     refspec makes the server compute a shallow walk that deepens `.git/shallow`
//     past the want.
//   - History depth is bounded: one depth-1 fetch for missing refs (EnsureShallowRefs,
//     used by CLI ref validation) plus a doubling deepen loop D in {2, 4, ..., cap}
//     for the merge-base (computeMergeBase). No unconditional --unshallow.
//   - Everything is conditional: the repo must be shallow, a ref or merge-base must
//     actually be missing, and a raw `git remote get-url origin` must return a URL
//     (RemoteIdentity is useless as this probe — it canonicalizes file:// remotes
//     to ""). Non-shallow repositories gain zero git invocations on success paths.
//   - Every fetch is a single invocation, argv-only, with all options before
//     --end-of-options: after that marker git parses `--depth=<D>` as a refspec
//     pathname and dies with rc=128 ("strange pathname blocked"). Refs therefore
//     reach git as discrete argv entries, never through a shell.

// shallowDeepenMaxDepth caps the doubling deepen loop. It is a package variable so
// tests can lower the bound and exercise the exhausted path deterministically.
var shallowDeepenMaxDepth = 1024

// shallowFetchMu serializes fetch/deepen attempts within this process. Concurrent
// OCR runs over one clone only ever add refs and deepen boundaries, but serializing
// keeps in-process Providers from issuing divergent fetch sequences.
var shallowFetchMu sync.Mutex

// ocrFetchPrefix is the destination namespace for SHA-derived wants. It sits under
// refs/remotes so remote-prune semantics stay untouched (prune only manages
// refs/remotes/origin/*), without impersonating a real remote-tracking branch.
const ocrFetchPrefix = "refs/remotes/ocr-fetch/"

// shaLikeRe matches full hexadecimal object names (git's abbreviated range is 4..40;
// a user-supplied review ref of 7..40 hex chars is far more likely a commit id than
// a branch, and treating it as one is what makes `--to ${CI_COMMIT_SHA}` fetchable).
var shaLikeRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// fetchWant is a derived fetch request for one review ref.
type fetchWant struct {
	// refspec is the destination-bearing refspec to hand `git fetch`.
	refspec string
	// alt is an additional local resolution spelling to try after the fetch:
	// the ocr-fetch destination for SHA wants ("" when the want's destination
	// is already covered by the `origin/<name>` fallback).
	alt string
}

// shortSHA abbreviates an object name for the ocr-fetch destination ref.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// deriveFetchWant turns a review ref spelling into a destination-bearing fetch
// want. It is pure; repo-dependent spellings (HEAD) must be mapped by
// Provider.deriveWant first.
//
//   - SHA-looking refs: `+<sha>:refs/remotes/ocr-fetch/<sha[:12]>`. The short
//     destination keeps ref names readable while staying unique for any two
//     wants that differ within the first 12 chars (a collision only merges two
//     fetch destinations of the same commit).
//   - `origin/<name>`: `+<name>:refs/remotes/origin/<name>`, recreating the
//     tracking ref a full clone would have.
//   - bare `<name>` (branch or tag): `+<name>:refs/remotes/origin/<name>`.
//     Tags additionally auto-create refs/tags/<name> as a side effect.
//
// HEAD is refused here on purpose: `+HEAD:...` fetches the remote's default
// branch, which is the wrong side of the comparison whenever HEAD tracks
// something else. Callers resolve HEAD to its branch name (or SHA when
// detached) before deriving a want. Refs holding ":" cannot form a refspec,
// and refs holding glob metacharacters ("*", "?", "[") would make git match
// against the remote instead of fetching a literal name; the fetch rejects
// them and behavior degrades to today's error.
func deriveFetchWant(ref string) (fetchWant, bool) {
	if ref == "" || ref == "HEAD" {
		return fetchWant{}, false
	}
	if strings.ContainsAny(ref, ":*?[") || strings.HasPrefix(ref, "-") {
		// Not expressible as a safe refspec: ':' breaks refspec parsing,
		// '*'/'?'/'[' trigger glob matching on the remote (a want of '*'
		// would fetch every matching ref into the clone), and '-' is
		// option-like. validateReviewRefs rejects leading dashes long before
		// this, but the library path must not assemble a broken or dangerous
		// refspec either. None of these characters can appear in a valid
		// refname (git-check-ref-format), so no literal ref is lost.
		return fetchWant{}, false
	}
	if shaLikeRe.MatchString(ref) {
		short := shortSHA(ref)
		dst := ocrFetchPrefix + short
		return fetchWant{refspec: "+" + ref + ":" + dst, alt: dst}, true
	}
	if name, ok := strings.CutPrefix(ref, "origin/"); ok {
		if name == "" {
			return fetchWant{}, false
		}
		return fetchWant{refspec: "+" + name + ":refs/remotes/origin/" + name}, true
	}
	return fetchWant{refspec: "+" + ref + ":refs/remotes/origin/" + ref}, true
}

// isShallowRepo reports whether the repository is a shallow clone. It is the
// gate for every on-demand fetch, so it must stay a single constant-time probe.
func (p *Provider) isShallowRepo(ctx context.Context) bool {
	out, _, err := p.runGitSplit(ctx, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// originURL returns the raw configured URL of the origin remote, or "" when
// there is none. Unlike RemoteIdentity it does not canonicalize local remotes
// away: a file:// origin is a perfectly fetchable origin.
func (p *Provider) originURL(ctx context.Context) string {
	out, _, err := p.runGitSplit(ctx, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// deriveWant maps a review ref to a fetch want, resolving HEAD repo-aware first:
// on a branch, HEAD becomes that branch's name; detached, it becomes HEAD's own
// SHA (always present locally, since it is checked out).
func (p *Provider) deriveWant(ctx context.Context, ref string) (fetchWant, bool) {
	if ref != "HEAD" {
		return deriveFetchWant(ref)
	}
	if out, _, err := p.runGitSplit(ctx, "symbolic-ref", "--short", "HEAD"); err == nil {
		if branch := strings.TrimSpace(out); branch != "" {
			return deriveFetchWant(branch)
		}
	}
	if out, _, err := p.runGitSplit(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err == nil {
		if sha := firstLine(out); sha != "" {
			return deriveFetchWant(sha)
		}
	}
	return fetchWant{}, false
}

// resolveRefWithFallback resolves ref to an immutable commit SHA, trying the raw
// spelling first, then `origin/<ref>` (a bare branch name fetched into
// refs/remotes/origin/<name> never resolves without the prefix), then the
// want's ocr-fetch destination (the only local spelling a SHA-derived want can
// resolve through).
func (p *Provider) resolveRefWithFallback(ctx context.Context, ref string, want fetchWant) string {
	if sha := p.resolveCommit(ctx, ref); sha != "" {
		return sha
	}
	if sha := p.resolveCommit(ctx, "origin/"+ref); sha != "" {
		return sha
	}
	if want.alt != "" {
		if sha := p.resolveCommit(ctx, want.alt); sha != "" {
			return sha
		}
	}
	return ""
}

// fetchShallowDepth runs one bounded fetch from origin at the given depth.
// Every option precedes --end-of-options (see the file comment); the refspecs
// follow as positional arguments. Git's stderr is returned alongside the error
// so callers can surface the actual failure instead of a bare exit status.
func (p *Provider) fetchShallowDepth(ctx context.Context, depth int, wants []fetchWant) (string, error) {
	args := make([]string, 0, 5+len(wants))
	args = append(args, "fetch", "--quiet", fmt.Sprintf("--depth=%d", depth), "--end-of-options", "origin")
	for _, w := range wants {
		args = append(args, w.refspec)
	}
	_, stderr, err := p.runGitSplit(ctx, args...)
	return stderr, err
}

// EnsureShallowRefs gives shallow clones a chance to resolve refs that failed
// raw `rev-parse` by fetching them from origin once at depth 1 (#634). It
// subsumes the fetch and the fallback retry: it returns a map from each
// requested ref spelling to the commit SHA it resolved to after the fetch, so
// the caller can substitute SHAs for spellings that only resolve through the
// fallback (downstream `git diff`/`git show` call sites are not fallback-aware).
//
// Refs absent from the map simply did not resolve. A nil map with a nil error
// means "nothing to do": the repository is not shallow, or has no origin URL —
// in both cases the caller must keep today's behavior untouched. A non-nil
// error is a fetch failure; the caller reports it as context and then fails
// with today's ref-resolution error. The fetch is attempted once, never
// retried.
func EnsureShallowRefs(ctx context.Context, runner *gitcmd.Runner, repoDir string, refs []string) (map[string]string, error) {
	p := &Provider{repoDir: repoDir, runner: runner}
	if !p.isShallowRepo(ctx) || p.originURL(ctx) == "" {
		return nil, nil
	}

	shallowFetchMu.Lock()
	defer shallowFetchMu.Unlock()

	var wants []fetchWant
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		want, ok := p.deriveWant(ctx, ref)
		if !ok || seen[want.refspec] {
			continue
		}
		seen[want.refspec] = true
		wants = append(wants, want)
	}
	if len(wants) == 0 {
		return nil, nil
	}
	if stderr, err := p.fetchShallowDepth(ctx, 1, wants); err != nil {
		return nil, gitFailure("git fetch", stderr, err)
	}

	resolved := make(map[string]string, len(refs))
	for _, ref := range refs {
		want, ok := p.deriveWant(ctx, ref)
		if !ok {
			continue
		}
		if sha := p.resolveRefWithFallback(ctx, ref, want); sha != "" {
			resolved[ref] = sha
		}
	}
	return resolved, nil
}

// deepenForMergeBase tries to recover a merge-base that raw `git merge-base`
// could not find in a shallow clone, by deepening both sides from origin with
// a doubling depth budget. from/to are the comparison's ref spellings; the
// loop operates on their resolved endpoint SHAs so bare branch names never
// reach git un-substituted on this path.
//
// It returns the merge-base SHA ("" when still not found) plus a diagnostic
// string holding the last fetch failure ("" when every round succeeded). The
// Provider records the deepest round attempted so GetDiff can append a
// shallow-clone hint only on paths where deepening actually ran.
func (p *Provider) deepenForMergeBase(ctx context.Context, from, to string) string {
	if !p.isShallowRepo(ctx) || p.originURL(ctx) == "" {
		return ""
	}
	wantFrom, okFrom := p.deriveWant(ctx, from)
	wantTo, okTo := p.deriveWant(ctx, to)
	if !okFrom || !okTo {
		return ""
	}
	shaFrom := p.resolveRefWithFallback(ctx, from, wantFrom)
	shaTo := p.resolveRefWithFallback(ctx, to, wantTo)
	// Without both endpoints as objects there is nothing to deepen toward; git
	// already failed on the raw spellings, so the caller keeps today's error.
	if shaFrom == "" || shaTo == "" || shaFrom == shaTo {
		return ""
	}

	shallowFetchMu.Lock()
	defer shallowFetchMu.Unlock()

	for depth := 2; depth <= shallowDeepenMaxDepth; depth *= 2 {
		p.deepenTried = depth
		stderr, err := p.fetchShallowDepth(ctx, depth, []fetchWant{wantFrom, wantTo})
		if err != nil {
			// One failing want fails the whole fetch (git is atomic about it),
			// and a depth budget cannot fix an unreachable or refusing origin:
			// stop, surface the failure through the hint, keep today's error.
			p.deepenDiag = fetchDiag(stderr, err)
			return ""
		}
		if out, mbErr := p.runGit(ctx, "merge-base", "--end-of-options", shaFrom, shaTo); mbErr == nil {
			if base := strings.TrimSpace(out); base != "" {
				return base
			}
		}
	}
	return ""
}

// fetchDiag reduces a fetch failure to a compact, single-line diagnostic for
// embedding in the shallow-clone hint.
func fetchDiag(stderr string, err error) string {
	diag := strings.TrimSpace(stderr)
	if diag == "" {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	// Keep the first line: fetch failures prefix progress noise with the fatal.
	if line, _, ok := strings.Cut(diag, "\n"); ok && line != "" {
		diag = strings.TrimSpace(line)
	}
	if len(diag) > 200 {
		diag = diag[:200]
		// Cutting by bytes can split the trailing rune. Git speaks the user's
		// locale, so the hint can carry non-ASCII text (same concern as
		// gitFailure's cut, #972); drop the partial sequence rather than
		// embed invalid UTF-8 in the error.
		for len(diag) > 0 {
			if _, size := utf8.DecodeLastRuneInString(diag); size > 1 || diag[len(diag)-1] < utf8.RuneSelf {
				break
			}
			diag = diag[:len(diag)-1]
		}
	}
	return diag
}
