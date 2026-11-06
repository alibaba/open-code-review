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

	// -z terminates the pathname and the line number with NUL rather than ':'
	// and a newline, so a pathname holding either one survives verbatim instead
	// of being misparsed or quoted. See parseGrepOutput.
	cmdArgs = append(cmdArgs, "-n", "-z", "--no-color")
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

	fileMatches := make(map[string][]grepMatch)
	var fileOrder []string
	seen := make(map[string]bool)

	matchCount := 0
	truncated := false
	matchedFiles := make(map[string]bool)
	for _, rec := range parseGrepOutput(outStr, p.FileReader.Ref) {
		// Count every file with a text match, including those beyond the render
		// budget, so the truncation note can report the true matched-file count.
		matchedFiles[rec.path] = true
		if matchCount >= gitGrepMaxCount {
			// Keep scanning to finish counting matched files, but render no more.
			truncated = true
			continue
		}
		if !seen[rec.path] {
			seen[rec.path] = true
			fileOrder = append(fileOrder, rec.path)
		}
		fileMatches[rec.path] = append(fileMatches[rec.path], rec)
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

// grepMatch is one match as `git grep -z -n` reports it.
type grepMatch struct {
	path    string
	lineNum int
	content string
}

// parseGrepOutput parses `git grep -z -n` output, whose records are
//
//	<path>\0<line number>\0<line content>\n
//
// with the searched revision and a ':' in front of the path when a revision was
// given.
//
// The separators have to be NUL. git's default format separates the path from
// the line number with ':', so a pathname that holds a ':' is indistinguishable
// from the field separator: "src/a:b.go:3:code" splits into "src/a", "b.go" and
// the rest, the line-number field is no longer numeric, and the match was
// dropped without a trace. git also quotes a pathname holding '"', a backslash
// or a newline (and a newline additionally splits one record into two lines), so
// the path it printed could not be opened by file_read. -z prints the pathname
// and the line number verbatim instead.
func parseGrepOutput(outStr, ref string) []grepMatch {
	var matches []grepMatch
	rest := outStr
	for {
		sep := strings.IndexByte(rest, 0)
		if sep < 0 {
			// A trailing "Binary file <path> matches" line has no separators.
			return matches
		}
		path := rest[:sep]
		rest = rest[sep+1:]

		// -z leaves the "Binary file <path> matches" line git prints for a
		// binary match unchanged, so it lands in front of the next record's
		// path. It carries no match to render.
		for {
			nl := strings.IndexByte(path, '\n')
			if nl < 0 || !isBinaryMatchLine(path[:nl]) {
				break
			}
			path = path[nl+1:]
		}

		sep = strings.IndexByte(rest, 0)
		if sep < 0 {
			return matches
		}
		lineField := rest[:sep]
		rest = rest[sep+1:]

		// The content runs to the end of its line, which starts the next
		// record. Consume it before validating this one, so a record this
		// parser cannot read never desynchronizes the rest of the output.
		content := rest
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			content, rest = rest[:nl], rest[nl+1:]
		} else {
			rest = ""
		}

		if ref != "" {
			trimmed, ok := strings.CutPrefix(path, ref+":")
			if !ok {
				continue
			}
			path = trimmed
		}
		lineNum, parseErr := strconv.Atoi(lineField)
		if parseErr != nil || path == "" {
			// Skip records whose line-number field is not numeric.
			continue
		}
		matches = append(matches, grepMatch{path: path, lineNum: lineNum, content: content})
		if rest == "" {
			return matches
		}
	}
}

func isBinaryMatchLine(line string) bool {
	return strings.HasPrefix(line, "Binary file ") && strings.HasSuffix(line, " matches")
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
