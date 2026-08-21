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
	"unicode/utf8"
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
			patterns = append(patterns, s)
		}
	}

	if strings.TrimSpace(searchText) == "" {
		return "Error: search_text is blank", nil
	}

	result, err := p.gitGrep(ctx, searchText, caseSensitive, usePerlRegexp, patterns)
	if err != nil {
		return "", fmt.Errorf("code_search failed: %w", err)
	}
	return result, nil
}

func (p *CodeSearchProvider) buildGrepArgs(searchText string, caseSensitive bool, usePerlRegexp bool, noIndex bool, pathspec []string) []string {
	cmdArgs := []string{"--no-pager", "grep"}

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
	cmdArgs = append(cmdArgs, "--max-count", fmt.Sprintf("%d", gitGrepMaxCount))

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
	for _, part := range strings.Split(pathspec, "/") {
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
			return "code_search timed out. Try narrowing file_patterns to a more specific path.", nil
		}
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		if outStr == "" {
			if errStr == "" {
				return "No matches found", nil
			}
			return fmt.Sprintf("Error: %s", strings.TrimSpace(errStr)), nil
		}
	}

	lines := strings.Split(strings.TrimRight(outStr, "\n"), "\n")
	truncated := len(lines) >= gitGrepMaxCount

	fileMatches := make(map[string][]grepMatch)
	var fileOrder []string
	seen := make(map[string]bool)

	hasRef := p.FileReader.Ref != ""
	splitN := 3
	offset := 0
	if hasRef {
		splitN = 4
		offset = 1
	}

	var sb strings.Builder
	if truncated {
		sb.WriteString(fmt.Sprintf("Note: The results have been truncated. Only showing first %d results.\n", gitGrepMaxCount))
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", splitN)
		if len(parts) < splitN {
			continue
		}
		fname := parts[offset]
		m := grepMatch{}
		ln, parseErr := strconv.Atoi(parts[offset+1])
		if parseErr != nil {
			continue
		}
		m.lineNum = ln
		m.content = parts[offset+2]
		if !seen[fname] {
			seen[fname] = true
			fileOrder = append(fileOrder, fname)
		}
		fileMatches[fname] = append(fileMatches[fname], m)
	}

	for _, path := range fileOrder {
		matches := fileMatches[path]
		p.decodeMatches(ctx, path, matches)
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

// grepMatch is one `git grep` hit: a line number and the raw line body git
// printed for it.
type grepMatch struct {
	lineNum int
	content string
}

// decodeMatches replaces match bodies with decoded file lines when git grep
// emitted non-UTF-8 bytes for this file. It mutates matches in place.
//
// Detection deliberately does NOT run on the matched lines: a couple of grep
// hits is exactly the small-evidence case that misdetects — a 142-byte payload
// carrying one short CJK comment scores 51 where its whole file scores 100. So
// this reuses FileReader.ReadLines, which detects on whole-file evidence at its
// own seam, and simply reads the answer back out.
//
// Any failure leaves the raw bytes in place: a search result is never worth an
// error, and raw bytes are what ships today.
//
// The file can change between `git grep` and this re-read, in which case a
// line number would name different text. That is guarded by bailing out for
// the whole file on the only detectable symptom — the re-read is shorter than
// the highest matched line number. A same-length concurrent edit is not
// detectable without a lock, and a search result is not worth a lock; if it
// ever matters, compare the file's mtime and size across the two reads.
func (p *CodeSearchProvider) decodeMatches(ctx context.Context, path string, matches []grepMatch) {
	if p.FileReader == nil {
		return
	}
	// The common case: everything git printed is already valid UTF-8. One
	// utf8.ValidString per match line and no extra file read at all.
	needsDecode := false
	highest := 0
	for _, m := range matches {
		if !utf8.ValidString(m.content) {
			needsDecode = true
		}
		if m.lineNum > highest {
			highest = m.lineNum
		}
	}
	if !needsDecode || highest <= 0 {
		return
	}

	lines, _, err := p.FileReader.ReadLines(ctx, path, 1, highest)
	if err != nil || len(lines) < highest {
		return
	}
	for i := range matches {
		// git grep line numbers are 1-based, but this input is parsed from a
		// subprocess: never index on it without checking.
		if n := matches[i].lineNum; n >= 1 && n <= len(lines) {
			matches[i].content = lines[n-1]
		}
	}
}

func isNotGitRepoError(err error, stderr string) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 &&
		(strings.Contains(stderr, "not a git repository") || strings.Contains(stderr, ".git")) {
		return true
	}
	return false
}
