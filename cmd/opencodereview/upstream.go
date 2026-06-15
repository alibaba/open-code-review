package main

import (
	"fmt"
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
