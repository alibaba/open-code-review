package main

import (
	"fmt"
	"os"
	"strings"
)

// isUpstreamURL reports whether spec looks like a git URL or local path rather
// than the name of a configured remote.
func isUpstreamURL(spec string) bool {
	if strings.Contains(spec, "://") {
		return true
	}
	if strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "./") ||
		strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "~") {
		return true
	}
	// scp-like syntax: user@host:path (a ':' that precedes any '/').
	if i := strings.Index(spec, ":"); i > 0 {
		slash := strings.Index(spec, "/")
		if (slash == -1 || i < slash) && strings.Contains(spec[:i], "@") {
			return true
		}
	}
	return false
}

// upstreamRemoteExists reports whether name is a configured git remote in repoDir.
func upstreamRemoteExists(repoDir, name string) bool {
	out, err := runGitCmd(repoDir, "remote")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

// parseSymrefDefaultBranch extracts the default branch name from the output of
// `git ls-remote --symref <target> HEAD`, e.g. a line "ref: refs/heads/main\tHEAD".
func parseSymrefDefaultBranch(lsRemoteOut string) (string, error) {
	for _, line := range strings.Split(lsRemoteOut, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ref:") {
			continue
		}
		fields := strings.Fields(line) // ["ref:", "refs/heads/main", "HEAD"]
		if len(fields) >= 2 && strings.HasPrefix(fields[1], "refs/heads/") {
			return strings.TrimPrefix(fields[1], "refs/heads/"), nil
		}
	}
	return "", fmt.Errorf("no default branch (HEAD symref) found in ls-remote output")
}

// discoverDefaultBranch queries target (a remote name or URL) for its default
// branch via `git ls-remote --symref`.
func discoverDefaultBranch(repoDir, target string) (string, error) {
	out, err := runGitCmd(repoDir, "ls-remote", "--symref", "--end-of-options", target, "HEAD")
	if err != nil {
		return "", fmt.Errorf("git ls-remote %q failed: %s", target, strings.TrimSpace(string(out)))
	}
	branch, perr := parseSymrefDefaultBranch(string(out))
	if perr != nil {
		return "", fmt.Errorf("determine default branch for %q: %w", target, perr)
	}
	return branch, nil
}

// defaultTo returns to, or "HEAD" when to is empty.
func defaultTo(to string) string {
	if to == "" {
		return "HEAD"
	}
	return to
}

// resolveUpstream turns an --upstream spec (remote name or URL) into a concrete
// range to review: it fetches the upstream branch as needed and returns a usable
// `from` ref plus the `to` ref (defaulting to HEAD). The result feeds the normal
// ModeRange path: merge-base(from,to)..to.
func resolveUpstream(repoDir string, opts reviewOptions) (from, to string, err error) {
	target := opts.upstream
	isURL := isUpstreamURL(target)

	if !isURL && !upstreamRemoteExists(repoDir, target) {
		return "", "", fmt.Errorf("--upstream %q: no git remote named %q; pass a git URL or run \"git remote add %s <url>\"", target, target, target)
	}
	if isURL && opts.noFetch {
		return "", "", fmt.Errorf("--no-fetch cannot be used with a URL upstream (%q); a URL must be fetched", target)
	}

	// Determine the upstream branch.
	branch := opts.upstreamBranch
	if branch == "" {
		if opts.noFetch {
			// Local-only discovery via the remote's HEAD symref, e.g.
			// "refs/remotes/origin/HEAD" -> "refs/remotes/origin/main".
			out, e := runGitCmd(repoDir, "symbolic-ref", "--end-of-options", "refs/remotes/"+target+"/HEAD")
			ref := strings.TrimSpace(string(out))
			prefix := "refs/remotes/" + target + "/"
			if e != nil || !strings.HasPrefix(ref, prefix) {
				return "", "", fmt.Errorf("--no-fetch: cannot determine default branch for %q locally; pass --upstream-branch", target)
			}
			branch = strings.TrimPrefix(ref, prefix)
		} else {
			branch, err = discoverDefaultBranch(repoDir, target)
			if err != nil {
				return "", "", err
			}
		}
	}

	// Fetch unless asked not to.
	if !opts.noFetch {
		fmt.Fprintf(os.Stderr, "[ocr] fetching %s from %s...\n", branch, target)
		if out, e := runGitCmd(repoDir, "fetch", "--end-of-options", target, branch); e != nil {
			// For a configured remote, fall back to a local tracking ref if present.
			if !isURL {
				if _, verr := runGitCmd(repoDir, "rev-parse", "--verify", "--end-of-options", target+"/"+branch+"^{commit}"); verr == nil {
					fmt.Fprintf(os.Stderr, "[ocr] fetch failed (%s); using local %s/%s\n", strings.TrimSpace(string(out)), target, branch)
					return target + "/" + branch, defaultTo(opts.to), nil
				}
			}
			return "", "", fmt.Errorf("git fetch %s %s failed: %s", target, branch, strings.TrimSpace(string(out)))
		}
	}

	if isURL {
		out, e := runGitCmd(repoDir, "rev-parse", "--verify", "--end-of-options", "FETCH_HEAD^{commit}")
		if e != nil {
			return "", "", fmt.Errorf("resolve FETCH_HEAD after fetching %q: %s", target, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), defaultTo(opts.to), nil
	}
	return target + "/" + branch, defaultTo(opts.to), nil
}
