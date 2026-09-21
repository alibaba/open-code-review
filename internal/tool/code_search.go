// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	gitGrepMaxCount = 100
	gitGrepTimeout  = 10 * time.Second
)

// CodeSearchProvider performs text search across the repository using git grep.
type CodeSearchProvider struct {
	FileReader *FileReader
}

func NewCodeSearch(fr *FileReader) *CodeSearchProvider { return &CodeSearchProvider{FileReader: fr} }

func (p *CodeSearchProvider) Tool() Tool { return CodeSearch }

func (p *CodeSearchProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	searchText, _ := args["search_text"].(string)
	caseSensitive, _ := args["case_sensitive"].(bool)
	usePerlRegexp, _ := args["use_perl_regexp"].(bool)

	filePatternsIface, _ := args["file_patterns"].([]any)
	var patterns []string
	for _, item := range filePatternsIface {
		if s, ok := item.(string); ok && s != "" {
			if hasTraversalPathComponent(s) {
				return "Error: file_patterns must not contain ..", nil
			}
			// Treat backslashes as separators so Windows pathspecs match.
			patterns = append(patterns, strings.ReplaceAll(s, "\\", "/"))
		}
	}

	if strings.TrimSpace(searchText) == "" {
		return "Error: search_text is blank", nil
	}

	result, err := p.gitGrep(ctx, searchText, caseSensitive, usePerlRegexp, patterns)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (p *CodeSearchProvider) buildGrepArgs(searchText string, caseSensitive bool, usePerlRegexp bool, noIndex bool, pathspec []string) []string {
	// core.quotepath=false reports non-ASCII paths literally instead of as
	// quoted octal escapes, which file_read cannot open.
	cmdArgs := []string{"--no-pager", "-c", "core.quotepath=false", "grep"}

	if noIndex {
		// Non-git directory: search the working tree directly while still
		// honoring .gitignore and skipping .git (via --exclude-standard).
		cmdArgs = append(cmdArgs, "--no-index", "--exclude-standard")
	} else if p.FileReader.Ref == "" {
		cmdArgs = append(cmdArgs, "--untracked")
	}

	if !caseSensitive {
		cmdArgs = append(cmdArgs, "-i")
	}
	if usePerlRegexp {
		cmdArgs = append(cmdArgs, "-P")
	} else {
		cmdArgs = append(cmdArgs, "-F")
	}

	// Binary-match notices are plain text even with -z and would corrupt the
	// NUL-delimited records below. Only request line matches from text files.
	cmdArgs = append(cmdArgs, "-I", "-n", "-z", "--no-color")
	// git grep limits matches per file. Fetch one extra to distinguish an exact
	// limit from truncated results, then enforce the global limit below.
	cmdArgs = append(cmdArgs, "--max-count", fmt.Sprintf("%d", gitGrepMaxCount+1))

	cmdArgs = append(cmdArgs, "-e", searchText)

	if ref := p.FileReader.Ref; ref != "" {
		if strings.HasPrefix(ref, "-") {
			// Defense-in-depth: reject option-like refs here even though
			// validateReviewRefs already verifies the ref upstream.
			// NOTE: git grep < 2.45 does not support --end-of-options before
			// the revision, so this is the one git invocation where we can't
			// rely on that separator.
			return nil
		}
		cmdArgs = append(cmdArgs, ref)
	}

	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, pathspec...)

	return cmdArgs
}

func hasTraversalPathComponent(pathspec string) bool {
	norm := strings.ReplaceAll(pathspec, "\\", "/")
	for _, part := range strings.Split(norm, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func (p *CodeSearchProvider) runGitGrep(parentCtx context.Context, cmdArgs []string) (string, string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, gitGrepTimeout)
	defer cancel()

	if p.FileReader.Runner != nil {
		stdout, stderr, err := p.FileReader.Runner.RunSplit(ctx, p.FileReader.RepoDir, cmdArgs...)
		if ctx.Err() != nil && err != nil {
			return "", "", ctx.Err()
		}
		return stdout, stderr, err
	}

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = p.FileReader.RepoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil && err != nil && cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == -1 {
		return "", "", ctx.Err()
	}
	return stdout.String(), stderr.String(), err
}

func (p *CodeSearchProvider) gitGrep(ctx context.Context, searchText string, caseSensitive bool, usePerlRegexp bool, pathspec []string) (string, error) {
	cmdArgs := p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, false, pathspec)
	if cmdArgs == nil {
		return "Error: ref must not start with '-'", nil
	}

	outStr, errStr, err := p.runGitGrep(ctx, cmdArgs)

	// Non-git directory: `git grep` exits 128 with "not a git repository".
	// `ocr scan` supports plain directories, so retry in --no-index mode, which
	// searches the working tree directly while still honoring .gitignore.
	// Ref-based search needs a real repo, so it is not retried.
	if err != nil && p.FileReader.Ref == "" && isNotGitRepoError(err, errStr) {
		cmdArgs = p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, true, pathspec)
		outStr, errStr, err = p.runGitGrep(ctx, cmdArgs)
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("git grep timed out; try narrowing file_patterns to a more specific path: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		if outStr == "" {
			var exitErr *exec.ExitError
			exitCode := -1
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
			if errStr == "" && exitCode == 1 {
				return "No matches found", nil
			}
			trimmedErr := trimGitUsage(errStr, exitCode)
			if trimmedErr == "" {
				return "", fmt.Errorf("git grep failed: %w", err)
			}
			return "", fmt.Errorf("git grep failed: %w: %s", err, trimmedErr)
		}
	}

	type match struct {
		lineNum int
		content string
	}
	fileMatches := make(map[string][]match)
	var fileOrder []string
	seen := make(map[string]bool)

	matchCount := 0
	truncated := false
	matchedFiles := make(map[string]bool)
	// With -n -z, Git emits path NUL line-number NUL content LF.
	// Consume the path before looking for LF: filenames may contain both
	// colons and newlines, and must not be parsed as display-formatted text.
	for remaining := outStr; remaining != ""; {
		fname, rest, ok := strings.Cut(remaining, "\x00")
		if !ok {
			break
		}
		lineNumber, rest, ok := strings.Cut(rest, "\x00")
		if !ok {
			break
		}
		content, next, _ := strings.Cut(rest, "\n")
		remaining = next
		if ref := p.FileReader.Ref; ref != "" {
			fname = strings.TrimPrefix(fname, ref+":")
		}
		ln, parseErr := strconv.Atoi(lineNumber)
		if parseErr != nil {
			continue
		}
		// Count every file with a text match, including those beyond the render
		// budget, so the truncation note can report the true matched-file count.
		matchedFiles[fname] = true
		if matchCount >= gitGrepMaxCount {
			// Keep scanning to finish counting matched files, but render no more.
			truncated = true
			continue
		}
		m := match{lineNum: ln, content: content}
		if !seen[fname] {
			seen[fname] = true
			fileOrder = append(fileOrder, fname)
		}
		fileMatches[fname] = append(fileMatches[fname], m)
		matchCount++
	}

	var sb strings.Builder
	if truncated {
		sb.WriteString(fmt.Sprintf("Note: Showing the first %d matches across %d matching files. Some files are partially shown or omitted entirely. Narrow file_patterns to see the rest.\n", gitGrepMaxCount, len(matchedFiles)))
	}

	for _, path := range fileOrder {
		matches := fileMatches[path]
		sb.WriteString(fmt.Sprintf("File: %s\nMatch lines: %d\n", path, len(matches)))
		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("%d|%s\n", m.lineNum, m.content))
		}
		sb.WriteString("\n")
	}

	if err != nil && errStr != "" {
		sb.WriteString(fmt.Sprintf("Warning: %s\n", strings.TrimSpace(errStr)))
	}

	return sb.String(), nil
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

func isNotGitRepoError(err error, stderr string) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 &&
		(strings.Contains(stderr, "not a git repository") || strings.Contains(stderr, ".git")) {
		return true
	}
	return false
}
