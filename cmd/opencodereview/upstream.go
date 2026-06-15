package main

import (
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
