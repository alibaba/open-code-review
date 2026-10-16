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
	if result == noMatchesResult && !usePerlRegexp {
		if alts := literalAlternatives(searchText); alts != nil {
			return p.searchAlternatives(ctx, searchText, alts, caseSensitive, patterns)
		}
	}
	return result, nil
}

const noMatchesResult = "No matches found"

// literalAlternatives splits a literal search_text on '|' when that yields at
// least two non-blank alternatives. Models routinely write "a|b" expecting
// alternation while use_perl_regexp is off; the literal search then finds
// nothing and the model concludes the code does not exist.
func literalAlternatives(searchText string) []string {
	if !strings.Contains(searchText, "|") {
		return nil
	}
	var alts []string
	for _, part := range strings.Split(searchText, "|") {
		if t := strings.TrimSpace(part); t != "" {
			alts = append(alts, t)
		}
	}
	if len(alts) < 2 {
		return nil
	}
	return alts
}

// searchAlternatives re-runs a literal search that found nothing as a search
// for any of its '|'-separated parts, and says so in the result, so the model
// learns both the answer and the tool's literal semantics.
func (p *CodeSearchProvider) searchAlternatives(ctx context.Context, searchText string, alts []string, caseSensitive bool, pathspec []string) (string, error) {
	quoted := make([]string, len(alts))
	for i, a := range alts {
		quoted[i] = strconv.Quote(a)
	}
	list := strings.Join(quoted, ", ")
	result, err := p.gitGrepPatterns(ctx, alts, caseSensitive, false, pathspec)
	if err != nil {
		if ctx.Err() != nil {
			return "", err
		}
		// The literal search already answered; a failed retry must not turn
		// that answer into an error about a query the model never wrote, but
		// the model must not read it as "none of the alternatives exist".
		return fmt.Sprintf("No matches found. search_text %q is literal text unless use_perl_regexp is true, so '|' is not alternation. Note: the retry as alternatives %s did not complete (%s); set use_perl_regexp: true to search for any of them.", searchText, list, retryFailureClass(err)), nil
	}
	if result == noMatchesResult {
		return fmt.Sprintf("No matches found. search_text %q is literal text unless use_perl_regexp is true, so '|' is not alternation; none of the alternatives %s match either.", searchText, list), nil
	}
	return fmt.Sprintf("Note: no line contains the literal text %q (search_text is literal unless use_perl_regexp is true, so '|' is not alternation). Showing lines that contain any of %s instead.\n", searchText, list) + result, nil
}

func retryFailureClass(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	return "failed"
}

func (p *CodeSearchProvider) buildGrepArgs(searchText string, caseSensitive bool, usePerlRegexp bool, noIndex bool, pathspec []string) []string {
	return p.buildGrepArgsPatterns([]string{searchText}, caseSensitive, usePerlRegexp, noIndex, pathspec)
}

// buildGrepArgsPatterns builds a git grep invocation matching any of patterns.
func (p *CodeSearchProvider) buildGrepArgsPatterns(patterns []string, caseSensitive bool, usePerlRegexp bool, noIndex bool, pathspec []string) []string {
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

	cmdArgs = append(cmdArgs, "-n", "--no-color")
	// git grep limits matches per file. Fetch one extra to distinguish an exact
	// limit from truncated results, then enforce the global limit below.
	cmdArgs = append(cmdArgs, "--max-count", fmt.Sprintf("%d", gitGrepMaxCount+1))

	for _, pattern := range patterns {
		cmdArgs = append(cmdArgs, "-e", pattern)
	}

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
	return p.gitGrepPatterns(ctx, []string{searchText}, caseSensitive, usePerlRegexp, pathspec)
}

func (p *CodeSearchProvider) gitGrepPatterns(ctx context.Context, patterns []string, caseSensitive bool, usePerlRegexp bool, pathspec []string) (string, error) {
	cmdArgs := p.buildGrepArgsPatterns(patterns, caseSensitive, usePerlRegexp, false, pathspec)
	if cmdArgs == nil {
		return "Error: ref must not start with '-'", nil
	}

	outStr, errStr, err := p.runGitGrep(ctx, cmdArgs)

	// Non-git directory: `git grep` exits 128 with "not a git repository".
	// `ocr scan` supports plain directories, so retry in --no-index mode, which
	// searches the working tree directly while still honoring .gitignore.
	// Ref-based search needs a real repo, so it is not retried.
	if err != nil && p.FileReader.Ref == "" && isNotGitRepoError(err, errStr) {
		cmdArgs = p.buildGrepArgsPatterns(patterns, caseSensitive, usePerlRegexp, true, pathspec)
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
				return noMatchesResult, nil
			}
			trimmedErr := trimGitUsage(errStr, exitCode)
			if trimmedErr == "" {
				return "", fmt.Errorf("git grep failed: %w", err)
			}
			return "", fmt.Errorf("git grep failed: %w: %s", err, trimmedErr)
		}
	}

	lines := strings.Split(strings.TrimRight(outStr, "\n"), "\n")

	type match struct {
		lineNum int
		content string
	}
	fileMatches := make(map[string][]match)
	var fileOrder []string
	seen := make(map[string]bool)

	hasRef := p.FileReader.Ref != ""
	splitN := 3
	offset := 0
	if hasRef {
		splitN = 4
		offset = 1
	}

	matchCount := 0
	truncated := false
	matchedFiles := make(map[string]bool)
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", splitN)
		if len(parts) < splitN {
			continue
		}
		fname := parts[offset]
		ln, parseErr := strconv.Atoi(parts[offset+1])
		if parseErr != nil {
			// Skip lines whose line-number field is not numeric.
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
		m := match{lineNum: ln, content: parts[offset+2]}
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
