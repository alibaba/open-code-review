package diff

// ExcludedDirs is the list of directory prefixes that scanners and diff
// providers should always skip. Exposed so internal/scan and other consumers
// can reuse the same blocklist.
func ExcludedDirs() []string {
	out := make([]string, len(providerDirIgnoreDirs))
	copy(out, providerDirIgnoreDirs)
	return out
}

// LoadGitignorePatterns reads and parses .gitignore patterns from the given
// repository root. Returns nil if the file is missing or unreadable.
func LoadGitignorePatterns(repoDir string) []string {
	stub := &Provider{repoDir: repoDir}
	return stub.loadGitignorePatterns()
}

// IsPathExcluded returns true when relPath matches any of the supplied
// gitignore-style patterns or any default excluded directory prefix
// (see ExcludedDirs).
func IsPathExcluded(repoDir, relPath string, patterns []string) bool {
	stub := &Provider{repoDir: repoDir}
	return stub.isPathExcluded(relPath, patterns)
}

// MatchGitignorePattern reports whether relPath matches a single
// gitignore-style pattern, using the simplified semantics that diff.Provider
// already implements (basename match, prefix match, directory-only suffix).
// Useful when callers want to test a single pattern in isolation.
func MatchGitignorePattern(relPath, pat string) bool {
	return matchGitignorePattern(relPath, pat)
}
