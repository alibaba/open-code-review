// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// fetchTarget is the remote branch that review --fetch refreshes.
type fetchTarget struct {
	remote string
	branch string
}

// trackingRef is where the fetched branch lands, spelled in full because the
// short form can be shadowed by a local branch of the same name.
func (t fetchTarget) trackingRef() string {
	return "refs/remotes/" + t.remote + "/" + t.branch
}

// parseFetchTarget reads --from as a branch on a configured remote. An explicit
// --remote is authoritative: --from is then a branch on it, minus an optional
// "<remote>/" prefix naming that same remote, so every branch stays reachable
// even when its namespace shares a name with another remote. Without --remote,
// a leading "<remote>/" selects that remote, matched against the longest
// configured name because remote names may contain '/'; anything else is a
// branch on origin.
func parseFetchTarget(from, remote string, remotes []string) (fetchTarget, error) {
	if strings.HasPrefix(from, "-") {
		return fetchTarget{}, fmt.Errorf("--from value %q is not a valid git ref: refs must not start with '-'", from)
	}
	if strings.HasPrefix(from, "refs/") {
		return fetchTarget{}, fmt.Errorf("--fetch needs --from to be a short branch name such as main or origin/main, got %q", from)
	}
	var target fetchTarget
	if remote != "" {
		target = fetchTarget{remote: remote, branch: strings.TrimPrefix(from, remote+"/")}
	} else {
		target = fetchTarget{remote: "origin", branch: from}
		prefix := ""
		for _, r := range remotes {
			if len(r) > len(prefix) && strings.HasPrefix(from, r+"/") {
				prefix = r
			}
		}
		if prefix != "" {
			target = fetchTarget{remote: prefix, branch: strings.TrimPrefix(from, prefix+"/")}
		}
	}
	if !slices.Contains(remotes, target.remote) {
		return fetchTarget{}, fmt.Errorf("--fetch: %q is not a configured git remote", target.remote)
	}
	if target.branch == "HEAD" {
		return fetchTarget{}, notABranchError(from)
	}
	return target, nil
}

func notABranchError(from string) error {
	return fmt.Errorf("--fetch needs --from to name a branch, got %q", from)
}

// checkFetchGitVersion is a variable so tests can simulate an older Git.
var checkFetchGitVersion = gitcmd.CheckGitVersion

// resolveFetchTarget is the local half of --fetch: it validates --from and
// --remote and picks the branch to fetch without contacting the remote, so a
// target that can never be fetched is reported as early as an invalid ref is
// without --fetch. It returns nil without --fetch.
func resolveFetchTarget(ctx context.Context, cc *commonContext, opts reviewOptions) (*fetchTarget, error) {
	if !opts.fetch {
		return nil, nil
	}
	// OCR only warns about an old Git at startup, but the fetch below needs
	// --porcelain, so fail clearly here instead of with git's usage text.
	if err := checkFetchGitVersion(); err != nil {
		return nil, fmt.Errorf("--fetch: %w", err)
	}
	remotes, err := cc.GitRunner.Output(ctx, cc.RepoDir, "remote")
	if err != nil {
		return nil, fmt.Errorf("--fetch: list git remotes: %w", err)
	}
	target, err := parseFetchTarget(opts.from, opts.remote, strings.Fields(string(remotes)))
	if err != nil {
		return nil, err
	}
	if _, err := cc.GitRunner.Output(ctx, cc.RepoDir, "check-ref-format", target.trackingRef()); err != nil {
		return nil, notABranchError(opts.from)
	}
	if err := refuseSymbolicDestination(ctx, cc, target.trackingRef()); err != nil {
		return nil, err
	}
	return &target, nil
}

// refuseSymbolicDestination rejects a destination that is a symbolic ref:
// fetching into it writes through to its target, which may be a local branch.
func refuseSymbolicDestination(ctx context.Context, cc *commonContext, dest string) error {
	if _, err := cc.GitRunner.Output(ctx, cc.RepoDir, "symbolic-ref", "--quiet", dest); err == nil {
		return fmt.Errorf("--fetch: %s is a symbolic ref; refusing to fetch into it", dest)
	}
	return nil
}

// fetchReviewBase is the network half of --fetch: it fetches the branch
// resolveFetchTarget chose, then freezes the range endpoints so the rest of the
// review reads the commits resolved here even if the refs move again. It
// rewrites opts.from to the remote-tracking branch, which is what the session
// records as the requested base. It returns nil when target is nil.
func fetchReviewBase(ctx context.Context, cc *commonContext, opts *reviewOptions, target *fetchTarget) (*diff.InputResolution, error) {
	if target == nil {
		return nil, nil
	}
	dest := target.trackingRef()
	// Checked again here because other checks may have run since
	// resolveFetchTarget, and this one guards local branches.
	if err := refuseSymbolicDestination(ctx, cc, dest); err != nil {
		return nil, err
	}

	q := newQuietHandle(opts.outputFormat, opts.audience)
	defer q.Restore()
	w := stdout.Writer()
	fmt.Fprintf(w, "[ocr] Fetching %s from %s\n", target.branch, target.remote)
	// --refmap= keeps configured fetch refspecs from writing refs beyond dest,
	// and an empty fetch.bundleURI keeps bundle downloads out of refs/bundles/;
	// the other switches keep tags, submodules, FETCH_HEAD and background
	// maintenance out of it. --porcelain --verbose reports what dest now holds;
	// --porcelain needs Git 2.41, the minimum OCR supports.
	out, stderr, err := cc.GitRunner.RunSplit(ctx, cc.RepoDir, "-c", "fetch.bundleURI=", "fetch",
		"--porcelain", "--verbose", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head",
		"--no-auto-maintenance", "--refmap=", "--end-of-options", target.remote, "+refs/heads/"+target.branch+":"+dest)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fetchFailure(*target, stderr, err)
	}
	// Git exits 0 when it refuses an update, e.g. one that would need new
	// shallow roots; dest is then stale and must not be reviewed.
	tip, ok := fetchedTip(out, dest)
	if !ok {
		msg := fmt.Sprintf("fetch %s from %s: git did not update %s", target.branch, target.remote, dest)
		if diag := sanitizeTerminal(strings.TrimSpace(stderr)); diag != "" {
			msg += ": " + diag
		}
		return nil, errors.New(msg)
	}
	base := target.remote + "/" + target.branch
	resolution, err := agent.ResolveInput(ctx, agent.Args{
		RepoDir:   cc.RepoDir,
		From:      tip,
		To:        opts.to,
		GitRunner: cc.GitRunner,
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, resolveRangeError(ctx, cc, base, opts.to, err)
	}
	opts.from = base
	fmt.Fprintf(w, "[ocr] Resolved base: %s -> %s\n", base, shortSHA(tip))
	fmt.Fprintf(w, "[ocr] Resolved target: %s -> %s\n", opts.to, shortSHA(resolution.ResolvedHead))
	fmt.Fprintf(w, "[ocr] Merge base: %s\n", shortSHA(resolution.ResolvedBase))
	return resolution, nil
}

// fetchedTip returns the commit that `git fetch --porcelain --verbose` reports
// for dest. Lines read "<flag> <old> <new> <ref>"; a refused update is flagged
// '!' or left out entirely.
func fetchedTip(porcelain, dest string) (string, bool) {
	for _, line := range strings.Split(porcelain, "\n") {
		if len(line) < 2 || !strings.ContainsRune(" +*=", rune(line[0])) {
			continue
		}
		if f := strings.Fields(line[2:]); len(f) == 3 && f[2] == dest {
			return f[1], true
		}
	}
	return "", false
}

func fetchFailure(t fetchTarget, stderr string, err error) error {
	diag := sanitizeTerminal(strings.TrimSpace(stderr))
	if diag == "" {
		return fmt.Errorf("fetch %s from %s: %w", t.branch, t.remote, err)
	}
	if strings.Contains(diag, "couldn't find remote ref") {
		diag += " (--fetch refreshes a branch; to review a tag or commit, drop --fetch)"
	}
	return fmt.Errorf("fetch %s from %s: %w: %s", t.branch, t.remote, err, diag)
}

func resolveRangeError(ctx context.Context, cc *commonContext, base, to string, err error) error {
	err = fmt.Errorf("--fetch: resolve %s..%s: %w", base, to, err)
	if out, _ := cc.GitRunner.Output(ctx, cc.RepoDir, "rev-parse", "--is-shallow-repository"); strings.TrimSpace(string(out)) == "true" {
		err = fmt.Errorf("%w; this clone is shallow, so deepen it (git fetch --deepen=<n> or --unshallow) and retry", err)
	}
	return err
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
