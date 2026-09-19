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

	// gitGrepPatternOrigin is how `git grep` names a pattern given with -e when
	// it reports that the pattern did not compile. builtin/grep.c registers that
	// origin, and grep.c renders the failure as
	// "<origin>, '<pattern>': <reason>" through a literal, untranslated format
	// string -- so the marker holds in any locale and has been spelled this way
	// from git 2.20 through 2.50. Only a bad pattern produces it: ref and
	// pathspec problems are diagnosed before patterns are compiled.
	gitGrepPatternOrigin = "-e option, '"

	// maxPCRECompileReason bounds the reason taken from a compile failure.
	// PCRE2's own messages are short, so a longer tail means the message had an
	// unexpected shape and is better left out of the tool result.
	maxPCRECompileReason = 100
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
	noIndex := false
	cmdArgs := p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, noIndex, pathspec)
	if cmdArgs == nil {
		return "Error: ref must not start with '-'", nil
	}

	outStr, errStr, err := p.runGitGrep(ctx, cmdArgs)

	// Non-git directory: `git grep` exits 128 with "not a git repository".
	// `ocr scan` supports plain directories, so retry in --no-index mode, which
	// searches the working tree directly while still honoring .gitignore.
	// Ref-based search needs a real repo, so it is not retried.
	if err != nil && p.FileReader.Ref == "" && isNotGitRepoError(err, errStr) {
		noIndex = true
		cmdArgs = p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, noIndex, pathspec)
		outStr, errStr, err = p.runGitGrep(ctx, cmdArgs)
	}

	// A pattern PCRE cannot compile stops the search before it reads a single
	// file, costing the model a whole round trip over a term it usually meant
	// literally (#1340). Retry the same search as a fixed string and say so in
	// the result. Only a usable literal answer replaces the failure: if the
	// retry fails on its own -- a timeout, say -- the original diagnosis below
	// is reported unchanged, so this never swaps one error for another.
	literal := false
	patternStderr := ""
	if err != nil && usePerlRegexp && isPCRECompileError(errStr) {
		patternStderr = errStr
		literalArgs := p.buildGrepArgs(searchText, caseSensitive, false, noIndex, pathspec)
		literalOut, literalErrStr, literalErr := p.runGitGrep(ctx, literalArgs)
		if literalErr == nil || isGitGrepNoMatch(literalErr, literalOut, literalErrStr) {
			outStr, errStr, err = literalOut, literalErrStr, literalErr
			literal = true
		}
	}

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("git grep timed out; try narrowing file_patterns to a more specific path: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			return "", err
		}
		if isGitGrepNoMatch(err, outStr, errStr) {
			return patternFallbackNote(literal, patternStderr) + "No matches found", nil
		}
		if outStr == "" {
			var exitErr *exec.ExitError
			exitCode := -1
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
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
	if literal {
		sb.WriteString(patternFallbackNote(literal, patternStderr))
	}
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

// isPCRECompileError reports whether `git grep` rejected the -e pattern instead
// of searching for it. git diagnoses a bad pattern before it resolves a ref or
// a pathspec, so a failure carrying this marker is always about the pattern.
func isPCRECompileError(stderr string) bool {
	return strings.Contains(stderr, gitGrepPatternOrigin)
}

// pcreCompileReason returns PCRE's own reason for a rejected pattern
// ("missing closing parenthesis", "range out of order in character class"), or
// "" when the message has some other shape. The reason is what tells the model
// how to repair its pattern.
func pcreCompileReason(stderr string) string {
	// git quotes the pattern between the origin marker and the reason, and the
	// pattern may itself contain that separator, so split on the last one.
	idx := strings.LastIndex(stderr, "': ")
	if idx < 0 {
		return ""
	}
	reason := stderr[idx+len("': "):]
	if nl := strings.IndexByte(reason, '\n'); nl >= 0 {
		reason = reason[:nl]
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > maxPCRECompileReason {
		return ""
	}
	return reason
}

// patternFallbackNote explains, above the matches it qualifies, that a pattern
// was searched literally after PCRE rejected it. It returns "" when no fallback
// happened, so a caller can prefix it unconditionally.
func patternFallbackNote(fallback bool, stderr string) string {
	if !fallback {
		return ""
	}
	const note = "Note: the pattern is not valid PCRE syntax%s, so it was searched as a literal string instead; the matches below are literal, not regex, matches.\n"
	reason := pcreCompileReason(stderr)
	if reason == "" {
		return fmt.Sprintf(note, "")
	}
	return fmt.Sprintf(note, " ("+reason+")")
}

// isGitGrepNoMatch reports whether a failed `git grep` run found nothing, which
// git reports as exit status 1 with no output on either stream. That is an
// answer to hand back, not an error to propagate.
func isGitGrepNoMatch(err error, stdout, stderr string) bool {
	if err == nil || stdout != "" || stderr != "" {
		return false
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
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
