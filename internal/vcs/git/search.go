// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/vcs"
)

func buildGrepArgs(ref vcs.ContentRef, query vcs.SearchQuery, noIndex bool) ([]string, error) {
	if strings.HasPrefix(ref.Revision, "-") {
		return nil, fmt.Errorf("invalid revision %q: ref must not start with '-'", ref.Revision)
	}
	for _, pattern := range query.Patterns {
		path := strings.ReplaceAll(pattern, "\\", "/")
		if strings.HasPrefix(path, ":(") {
			if end := strings.IndexByte(path, ')'); end >= 0 {
				path = path[end+1:]
			}
		}
		for _, component := range strings.Split(path, "/") {
			if component == ".." {
				return nil, fmt.Errorf("invalid search pattern %q: parent traversal", pattern)
			}
		}
		if _, err := repositoryPath(path); err != nil {
			return nil, fmt.Errorf("invalid search pattern %q: %w", pattern, err)
		}
	}
	args := []string{"--no-pager", "grep", "-I", "-z", "-n", "--no-color"}
	if noIndex {
		args = append(args, "--no-index", "--exclude-standard")
	} else if ref.IsWorkspace() {
		args = append(args, "--untracked")
	}
	if !query.CaseSensitive {
		args = append(args, "-i")
	}
	if query.PerlRegexp {
		args = append(args, "-P")
	} else {
		args = append(args, "-F")
	}
	if query.Limit > 0 && query.Limit < int(^uint(0)>>1) {
		args = append(args, "--max-count", strconv.Itoa(query.Limit+1))
	}
	args = append(args, "-e", query.Text)
	if !ref.IsWorkspace() {
		args = append(args, ref.Revision)
	}
	args = append(args, "--")
	return append(args, query.Patterns...), nil
}

// Search returns structured matches; Git syntax and NUL record decoding stay
// inside the adapter. The result limit applies across files, not to each file.
func (r *Repository) Search(parent context.Context, ref vcs.ContentRef, query vcs.SearchQuery) (vcs.SearchResult, error) {
	args, err := buildGrepArgs(ref, query, false)
	if err != nil {
		return vcs.SearchResult{}, err
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	out, stderr, err := r.commandRunner().RunSplit(ctx, r.RepoDir, args...)
	if err != nil && ctx.Err() != nil {
		return vcs.SearchResult{}, ctx.Err()
	}
	if err != nil && ref.IsWorkspace() && r.isNotGitRepository(ctx, err) {
		args, _ = buildGrepArgs(ref, query, true)
		out, stderr, err = r.commandRunner().RunSplit(ctx, r.RepoDir, args...)
	}
	if err != nil {
		if ctx.Err() != nil {
			return vcs.SearchResult{}, ctx.Err()
		}
		var exitErr *exec.ExitError
		exitCode := -1
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if out == "" {
			if exitCode == 1 && stderr == "" {
				return vcs.SearchResult{}, nil
			}
			return vcs.SearchResult{}, fmt.Errorf("git grep failed: %w: %s", err, trimGitUsage(stderr, exitCode))
		}
	}
	result, parseErr := parseGrepOutput(out, ref, query.Limit)
	if parseErr != nil {
		return vcs.SearchResult{}, parseErr
	}
	if err != nil && stderr != "" {
		result.Warning = strings.TrimSpace(stderr)
	}
	return result, nil
}

func parseGrepOutput(output string, ref vcs.ContentRef, limit int) (vcs.SearchResult, error) {
	var result vcs.SearchResult
	matchedFiles := make(map[string]bool)
	for output != "" {
		pathEnd := strings.IndexByte(output, 0)
		if pathEnd < 0 {
			return vcs.SearchResult{}, fmt.Errorf("invalid git grep filename record")
		}
		path := output[:pathEnd]
		output = output[pathEnd+1:]
		lineEnd := strings.IndexByte(output, 0)
		if lineEnd < 0 {
			return vcs.SearchResult{}, fmt.Errorf("invalid git grep line record")
		}
		line, err := strconv.Atoi(output[:lineEnd])
		if err != nil || line < 1 {
			return vcs.SearchResult{}, fmt.Errorf("invalid git grep line number %q", output[:lineEnd])
		}
		output = output[lineEnd+1:]
		textEnd := strings.IndexByte(output, '\n')
		if textEnd < 0 {
			textEnd = len(output)
		}
		content := output[:textEnd]
		if textEnd < len(output) {
			output = output[textEnd+1:]
		} else {
			output = ""
		}
		if !ref.IsWorkspace() {
			path = strings.TrimPrefix(path, ref.Revision+":")
		}
		matchedFiles[path] = true
		if limit > 0 && len(result.Matches) >= limit {
			result.Truncated = true
			continue
		}
		result.Matches = append(result.Matches, vcs.Match{Path: path, Line: line, Text: content})
	}
	result.MatchedFiles = len(matchedFiles)
	return result, nil
}

func trimGitUsage(stderr string, exitCode int) string {
	stderr = strings.TrimSpace(stderr)
	if exitCode == 129 {
		if idx := strings.IndexByte(stderr, '\n'); idx >= 0 {
			stderr = stderr[:idx]
		}
	}
	return strings.TrimSpace(stderr)
}
