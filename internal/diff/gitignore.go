// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

type GitignorePatterns struct {
	lines      []string
	ignoreCase bool
}

func LoadGitignoreAtRef(ctx context.Context, repoDir, ref string, runner *gitcmd.Runner) (GitignorePatterns, error) {
	if runner == nil {
		runner = gitcmd.New(1)
	}
	entries, err := runner.Output(ctx, repoDir, "ls-tree", "-z", "--full-tree", "--end-of-options", ref)
	if err != nil {
		return GitignorePatterns{}, fmt.Errorf("locate root .gitignore at %s: %w", ref, err)
	}
	for entry := range strings.SplitSeq(string(entries), "\x00") {
		metadata, name, _ := strings.Cut(entry, "\t")
		if name != ".gitignore" {
			continue
		}
		mode, oid, _ := strings.Cut(metadata, " blob ")
		if mode != "100644" && mode != "100755" {
			break
		}
		data, err := runner.Output(ctx, repoDir, "cat-file", "blob", oid)
		if err != nil {
			return GitignorePatterns{}, fmt.Errorf("read root .gitignore at %s: %w", ref, err)
		}
		lines := parseGitignorePatterns(data)
		if len(lines) == 0 {
			break
		}
		ignoreCase, err := runner.ConfigBool(ctx, repoDir, "core.ignoreCase")
		if err != nil {
			return GitignorePatterns{}, fmt.Errorf("read core.ignoreCase: %w", err)
		}
		return GitignorePatterns{lines: lines, ignoreCase: ignoreCase}, nil
	}
	return GitignorePatterns{}, nil
}

// ExcludedDirs is the list of directory prefixes that scanners and diff
// providers should always skip. Exposed so internal/scan and other consumers
// can reuse the same blocklist.
func ExcludedDirs() []string {
	out := make([]string, len(providerDirIgnoreDirs))
	copy(out, providerDirIgnoreDirs)
	return out
}

// ProviderDirPrefix returns the blocklist prefix that excludes relPath,
// including the trailing slash (for example "vendor/"), or "" if none match.
func ProviderDirPrefix(relPath string) string {
	for _, prefix := range providerDirIgnoreDirs {
		dirPart := strings.TrimSuffix(prefix, "/")
		if relPath == dirPart || strings.HasPrefix(relPath, prefix) {
			return prefix
		}
	}
	return ""
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

// IsDirectoryExcluded reports whether a walker can prune this directory.
func IsDirectoryExcluded(relPath string, patterns []string) bool {
	return isProviderDirExcluded(relPath) || (GitignorePatterns{lines: patterns}).matches(relPath, true)
}

// MatchGitignorePattern reports whether relPath matches a single
// gitignore-style pattern, using the simplified semantics that diff.Provider
// already implements (basename match, prefix match, directory-only suffix).
// Useful when callers want to test a single pattern in isolation.
func MatchGitignorePattern(relPath, pat string) bool {
	return GitignorePatterns{lines: []string{pat}}.Matches(relPath)
}
