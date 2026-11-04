// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestDeriveFetchWant(t *testing.T) {
	full := "9a209cf5d9e256f40bde0313e25440252d1e9ead"
	cases := []struct {
		name       string
		ref        string
		wantOK     bool
		refspec    string
		wantResolv string
	}{
		{
			name:       "full sha",
			ref:        full,
			wantOK:     true,
			refspec:    "+" + full + ":" + ocrFetchPrefix + full[:12],
			wantResolv: ocrFetchPrefix + full[:12],
		},
		{
			name:       "short sha",
			ref:        full[:12],
			wantOK:     true,
			refspec:    "+" + full[:12] + ":" + ocrFetchPrefix + full[:12],
			wantResolv: ocrFetchPrefix + full[:12],
		},
		{
			name:       "minimal abbreviation",
			ref:        full[:7],
			wantOK:     true,
			refspec:    "+" + full[:7] + ":" + ocrFetchPrefix + full[:7],
			wantResolv: ocrFetchPrefix + full[:7],
		},
		{
			name:       "origin-prefixed name",
			ref:        "origin/main",
			wantOK:     true,
			refspec:    "+main:refs/remotes/origin/main",
			wantResolv: "",
		},
		{
			name:       "origin-prefixed slashed name",
			ref:        "origin/feature/x",
			wantOK:     true,
			refspec:    "+feature/x:refs/remotes/origin/feature/x",
			wantResolv: "",
		},
		{
			name:       "bare branch name",
			ref:        "main",
			wantOK:     true,
			refspec:    "+main:refs/remotes/origin/main",
			wantResolv: "",
		},
		{
			name:       "bare tag name",
			ref:        "v1.2.3",
			wantOK:     true,
			refspec:    "+v1.2.3:refs/remotes/origin/v1.2.3",
			wantResolv: "",
		},
		// HEAD is refused: `+HEAD:...` would fetch the remote's default
		// branch, the wrong side of the comparison. Callers map HEAD to its
		// branch (or SHA) before deriving a want.
		{name: "head refused", ref: "HEAD"},
		{name: "empty refused", ref: ""},
		{name: "origin slash alone refused", ref: "origin/"},
		// A colon cannot appear in a refspec source; a leading dash is
		// injection-shaped (validateReviewRefs rejects it far earlier).
		{name: "colon refused", ref: "main:refs/x"},
		{name: "leading dash refused", ref: "-O./pwn.sh"},
		// Glob metacharacters make git match against the remote instead of
		// fetching a literal name: a want of '*' would fetch every matching
		// ref. None of these can appear in a valid refname, so no literal
		// ref is lost by refusing them.
		{name: "star refused", ref: "*"},
		{name: "star in name refused", ref: "refs/heads/*"},
		{name: "question mark refused", ref: "mai?"},
		{name: "bracket refused", ref: "mai[n]"},
		{name: "glob after origin prefix refused", ref: "origin/refs/heads/*"},
		// Uppercase hex is not a SHA spelling in git; it stays a (bogus) ref
		// name and the fetch degrades to today's error.
		{
			name:    "uppercase hex is a name",
			ref:     "ABCDEF1234",
			wantOK:  true,
			refspec: "+ABCDEF1234:refs/remotes/origin/ABCDEF1234",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := deriveFetchWant(tc.ref)
			if ok != tc.wantOK {
				t.Fatalf("deriveFetchWant(%q) ok = %v, want %v", tc.ref, ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got.refspec != tc.refspec {
				t.Errorf("refspec = %q, want %q", got.refspec, tc.refspec)
			}
			if got.alt != tc.wantResolv {
				t.Errorf("alt = %q, want %q", got.alt, tc.wantResolv)
			}
		})
	}
}

// TestDeriveWantMapsHeadToBranch pins the repo-aware HEAD mapping: on a branch
// the want addresses that branch, never `+HEAD:...`.
func TestDeriveWantMapsHeadToBranch(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	branch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	p := NewWorkspaceProvider(repo, nil)
	want, ok := p.deriveWant(context.Background(), "HEAD")
	if !ok {
		t.Fatal("deriveWant(HEAD) refused on a checked-out branch")
	}
	if want.refspec != "+"+branch+":refs/remotes/origin/"+branch {
		t.Errorf("refspec = %q, want the current branch %q", want.refspec, branch)
	}
}

// TestDeriveWantMapsDetachedHeadToSHA pins the detached-HEAD mapping: HEAD's
// own SHA is always present locally, so the SHA path can deepen its history.
func TestDeriveWantMapsDetachedHeadToSHA(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	runGitTest(t, repo, "checkout", "-q", "--detach", "HEAD")

	p := NewWorkspaceProvider(repo, nil)
	want, ok := p.deriveWant(context.Background(), "HEAD")
	if !ok {
		t.Fatal("deriveWant(HEAD) refused on a detached HEAD")
	}
	if want.refspec != "+"+head+":"+ocrFetchPrefix+head[:12] {
		t.Errorf("refspec = %q, want the HEAD SHA %q", want.refspec, head)
	}
}

// TestResolveRefWithFallbackChain pins the resolution order the recovery
// relies on: raw spelling first, then origin/<name> (bare names never resolve
// without the prefix after a fetch), then the want's ocr-fetch destination
// (the only local spelling of a SHA-derived want). The alt leg is shadowed in
// the integration paths — once a fetch lands the object, the raw spelling
// resolves by object-id lookup — so only a direct call with a fabricated want
// can pin it. All resolution is real git against update-ref-created refs.
func TestResolveRefWithFallbackChain(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	tip := gitOut(t, repo, "rev-parse", "HEAD")
	branch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	// What a fetch leaves behind: a remote-tracking ref for the bare name and
	// an ocr-fetch destination for a SHA-derived want.
	runGitTest(t, repo, "update-ref", "refs/remotes/origin/"+branch, tip)
	const alt = "refs/remotes/ocr-fetch/deadbeefcafe"
	runGitTest(t, repo, "update-ref", alt, tip)
	p := NewWorkspaceProvider(repo, nil)

	// Raw spelling wins even when an alt is available.
	if got := p.resolveRefWithFallback(context.Background(), "HEAD", fetchWant{alt: alt}); got != tip {
		t.Errorf("raw leg: got %q, want %q", got, tip)
	}
	// Bare branch name: only origin/<name> resolves.
	if got := p.resolveRefWithFallback(context.Background(), branch, fetchWant{}); got != tip {
		t.Errorf("origin/ leg: bare %q got %q, want %q via the tracking ref", branch, got, tip)
	}
	// SHA spelling that is not an object: only the want's alt resolves.
	shaWant := fetchWant{refspec: "+0000deadbeef:" + alt, alt: alt}
	if got := p.resolveRefWithFallback(context.Background(), "0000deadbeef", shaWant); got != tip {
		t.Errorf("alt leg: got %q, want %q via %s", got, tip, alt)
	}
	// Nothing resolves: "" (callers keep today's error).
	if got := p.resolveRefWithFallback(context.Background(), "no-such-ref", fetchWant{}); got != "" {
		t.Errorf("unresolvable ref returned %q, want empty", got)
	}
}

// TestEnsureShallowRefsNonRepoIsNoop pins the gate against directories that
// are not git repositories at all: no error, no fetch, nothing resolved.
func TestEnsureShallowRefsNonRepoIsNoop(t *testing.T) {
	resolved, err := EnsureShallowRefs(context.Background(), gitcmd.New(2), t.TempDir(), []string{"main"})
	if err != nil || resolved != nil {
		t.Fatalf("EnsureShallowRefs on a non-repo = (%v, %v), want (nil, nil)", resolved, err)
	}
}

func TestIsShallowRepo(t *testing.T) {
	if skipNonUnixShallowFixture(t) {
		return
	}
	up := newUpstreamFixture(t)
	clone := newShallowClone(t, up)

	shallow := NewWorkspaceProvider(clone, nil)
	if !shallow.isShallowRepo(context.Background()) {
		t.Error("depth-1 clone reported as not shallow")
	}

	full := NewWorkspaceProvider(up.dir, nil)
	if full.isShallowRepo(context.Background()) {
		t.Error("upstream full clone reported as shallow")
	}
}

// TestFetchDiag covers the reduction of a fetch failure to a compact hint.
func TestFetchDiag(t *testing.T) {
	if got := fetchDiag("fatal: remote error: upload-pack: not our ref 1234\n", errFakeFetch()); !strings.Contains(got, "not our ref") {
		t.Errorf("diag %q lost git's message", got)
	}
	if got := fetchDiag("", errFakeFetch()); got == "" {
		t.Error("empty stderr must fall back to the error text")
	}
	if got := fetchDiag("", nil); got != "" {
		t.Errorf("nil error with empty stderr must yield empty diag, got %q", got)
	}
	long := "fatal: " + strings.Repeat("x", 400)
	if got := fetchDiag(long, errFakeFetch()); len(got) > 202 {
		t.Errorf("diag is %d bytes, truncation is not bounding it", len(got))
	}
	// A byte cut must not split the trailing rune: git speaks the user's
	// locale (#972 precedent in gitFailure), so the hint has to stay valid
	// UTF-8 even when truncated mid-multibyte.
	mb := "fatal: " + strings.Repeat("あ", 100) // allow-non-english: multibyte truncation fixture (#972 rune-boundary concern)
	if got := fetchDiag(mb, nil); !utf8.ValidString(got) {
		t.Errorf("truncated diag is not valid UTF-8: %q", got)
	}
}
