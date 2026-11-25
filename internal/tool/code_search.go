// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	gitGrepMaxCount = 100
	gitGrepTimeout  = 10 * time.Second

	CodeSearchBinaryMatch = "Binary file matches; content not shown"
)

// CodeSearchProvider performs text search across the repository using git grep.
type CodeSearchProvider struct {
	FileReader *FileReader
	Access     *CodeAccess
}

func NewCodeSearch(fr *FileReader) *CodeSearchProvider { return &CodeSearchProvider{FileReader: fr} }

func (p *CodeSearchProvider) Tool() Tool { return CodeSearch }

func (p *CodeSearchProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return p.Access.execute(ctx, args, p.FileReader, func(reader *FileReader) (string, error) {
		selected := *p
		selected.FileReader = reader
		return selected.execute(ctx, args)
	})
}

func (p *CodeSearchProvider) execute(ctx context.Context, args map[string]any) (string, error) {
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

func (p *CodeSearchProvider) buildGrepArgs(searchText string, caseSensitive bool, usePerlRegexp bool, noIndex bool, pathspec []string, options ...string) []string {
	cmdArgs := []string{"--no-pager", "grep", "-z", "-n", "--no-column", "--no-color"}
	cmdArgs = append(cmdArgs, options...)

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
	return slices.Contains(strings.Split(norm, "/"), "..")
}

func (p *CodeSearchProvider) runGitGrep(ctx context.Context, cmdArgs []string) (string, string, error) {
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
	if p.FileReader.Mode == ModeEmpty {
		return "No matches found", nil
	}
	searchCtx, cancel := context.WithTimeout(ctx, gitGrepTimeout)
	defer cancel()

	noIndex := false
	cmdArgs := p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, noIndex, pathspec)
	if cmdArgs == nil {
		return "Error: ref must not start with '-'", nil
	}

	outStr, errStr, err := p.runGitGrep(searchCtx, cmdArgs)

	// Non-git directory: `git grep` exits 128 with "not a git repository".
	// `ocr scan` supports plain directories, so retry in --no-index mode, which
	// searches the working tree directly while still honoring .gitignore.
	// Ref-based search needs a real repo, so it is not retried.
	if err != nil && p.FileReader.Ref == "" && isNotGitRepoError(err, errStr) {
		noIndex = true
		cmdArgs = p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, noIndex, pathspec)
		outStr, errStr, err = p.runGitGrep(searchCtx, cmdArgs)
	}

	if err != nil {
		if outStr == "" && errStr == "" && gitGrepExitCode(err) == 1 {
			return "No matches found", nil
		}
		if outStr == "" || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", gitGrepError(errStr, err, usePerlRegexp)
		}
	}

	type match struct {
		lineNum int
		content string
	}
	fileMatches := make(map[string][]match)
	var fileOrder []string

	refPrefix := ""
	if ref := p.FileReader.Ref; ref != "" {
		refPrefix = ref + ":"
	}

	matchCount := 0
	truncated := false
	matchedFiles := make(map[string]bool)
	countMatches := func() (string, error) {
		cmdArgs := p.buildGrepArgs(searchText, caseSensitive, usePerlRegexp, noIndex, pathspec, "--count")
		counts, stderr, err := p.runGitGrep(searchCtx, cmdArgs)
		if err != nil {
			return "", gitGrepError(stderr, err, usePerlRegexp)
		}
		return counts, nil
	}
	pathWarning, parseErr := scanGrepMatches(outStr, refPrefix, countMatches, func(path string, line int, content string) {
		seen := matchedFiles[path]
		matchedFiles[path] = true
		if matchCount >= gitGrepMaxCount {
			// Keep scanning to finish counting matched files, but render no more.
			truncated = true
			return
		}
		if !seen {
			fileOrder = append(fileOrder, path)
		}
		if line > 0 {
			fileMatches[path] = append(fileMatches[path], match{lineNum: line, content: content})
		}
		matchCount++
	})
	if parseErr != nil {
		return "", parseErr
	}
	if ctx.Err() != nil {
		return "", gitGrepError("", ctx.Err(), usePerlRegexp)
	}

	var sb strings.Builder
	if truncated {
		sb.WriteString(fmt.Sprintf("Note: Showing the first %d matches across %d matching files. Some files are partially shown or omitted entirely. Narrow file_patterns to see the rest.\n", gitGrepMaxCount, len(matchedFiles)))
	}

	for _, path := range fileOrder {
		matches := fileMatches[path]
		// Keep File headers on one line and distinguish escaped paths from literal ones.
		if strings.ContainsAny(path, "\\\r\n") {
			path = strconv.Quote(path)
		}
		if len(matches) == 0 {
			fmt.Fprintf(&sb, "File: %s\n%s\n\n", path, CodeSearchBinaryMatch)
			continue
		}
		sb.WriteString(fmt.Sprintf("File: %s\nMatch lines: %d\n", path, len(matches)))
		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("%d|%s\n", m.lineNum, m.content))
		}
		sb.WriteString("\n")
	}
	if err != nil && errStr != "" {
		sb.WriteString(fmt.Sprintf("Warning: %s\n", strings.TrimSpace(errStr)))
	}
	if pathWarning != nil {
		message := strings.NewReplacer("\r", " ", "\n", " ").Replace(pathWarning.Error())
		fmt.Fprintf(&sb, "Warning: Some git grep matches could not be verified: %s\n", message)
	}

	return sb.String(), nil
}

func scanGrepMatches(output, refPrefix string, countMatches func() (string, error), emit func(string, int, string)) (warning error, err error) {
	binaryPrefix := "Binary file " + refPrefix
	binaryBoundary := " matches\n" + binaryPrefix
	ignoreMatch := func(string, int, string) {}
	var lastName string
	for output != "" {
		name, record, hasSeparator := strings.Cut(output, "\x00")
		if names, binary := strings.CutPrefix(name, binaryPrefix); binary && hasSeparator && refPrefix != "" {
			// The binary path may contain another diagnostic, and either path may
			// contain the apparent transition to a ref-prefixed text record.
			boundary := " matches\n" + refPrefix
			path, textPath, found := strings.Cut(names, boundary)
			if found && path != "" && textPath != "" && !strings.Contains(textPath, boundary) {
				singleBinary := !strings.Contains(path, binaryBoundary)
				if singleBinary {
					emit(path, 0, "")
				}
				if singleBinary || warning != nil {
					name = refPrefix + textPath
				}
			}
		}
		if strings.HasPrefix(name, binaryPrefix) && (!hasSeparator || strings.Contains(name, " matches\n")) {
			if !hasSeparator {
				path, ok := strings.CutSuffix(name[len(binaryPrefix):], " matches\n")
				if ok && path != "" && !strings.Contains(path, binaryBoundary) {
					emit(path, 0, "")
					break
				}
			}
			// A text filename can look exactly like a binary diagnostic followed
			// by another filename. Git's per-file counts delimit the text groups.
			if warning == nil {
				counts, pathErr := countMatches()
				if pathErr == nil {
					// The working tree may have changed between the two searches.
					pathErr = scanGrepMatchesByCount(output, counts, lastName, refPrefix, ignoreMatch)
					if pathErr == nil {
						return nil, scanGrepMatchesByCount(output, counts, lastName, refPrefix, emit)
					}
				}
				if errors.Is(pathErr, context.Canceled) {
					return nil, pathErr
				}
				warning = pathErr
				continue
			}
			if !hasSeparator {
				break
			}
			// The next NUL belongs to a text match, but its path may overlap
			// the binary diagnostics. Skip that record and resume after it.
			output, err = scanGrepTextLine("", record, ignoreMatch)
			if err != nil {
				return errors.Join(warning, err), nil
			}
			continue
		}
		path, hasRef := strings.CutPrefix(name, refPrefix)
		if !hasSeparator || !hasRef || path == "" {
			return errors.Join(warning, fmt.Errorf("invalid path in git grep output")), nil
		}
		output, err = scanGrepTextLine(path, record, emit)
		if err != nil {
			return errors.Join(warning, err), nil
		}
		lastName = name
	}
	return warning, nil
}

func scanGrepMatchesByCount(output, counts, lastName, refPrefix string, emit func(string, int, string)) error {
	for counts != "" {
		name, rest, hasName := strings.Cut(counts, "\x00")
		number, remaining, hasCount := strings.Cut(rest, "\n")
		count, err := strconv.Atoi(number)
		if !hasName || !hasCount || err != nil || count <= 0 {
			return fmt.Errorf("invalid git grep match count")
		}
		counts = remaining
		if lastName != "" {
			if name == lastName {
				lastName = ""
			}
			continue
		}
		path, ok := strings.CutPrefix(name, refPrefix)
		if !ok || path == "" {
			return fmt.Errorf("invalid path in git grep counts")
		}
		// Git counts matching lines inside a binary file but prints one diagnostic.
		if rest, binary := strings.CutPrefix(output, "Binary file "+name+" matches\n"); binary {
			emit(path, 0, "")
			output = rest
			continue
		}
		prefix := name + "\x00"
		for range count {
			record, ok := strings.CutPrefix(output, prefix)
			if !ok {
				return fmt.Errorf("git grep results differ from the per-file counts")
			}
			output, err = scanGrepTextLine(path, record, emit)
			if err != nil {
				return err
			}
		}
	}
	if lastName != "" || output != "" {
		return fmt.Errorf("git grep results differ from the per-file counts")
	}
	return nil
}

func scanGrepTextLine(path, record string, emit func(string, int, string)) (string, error) {
	number, payload, hasSeparator := strings.Cut(record, "\x00")
	line, err := strconv.Atoi(number)
	if !hasSeparator || err != nil || line <= 0 {
		return "", fmt.Errorf("invalid line number in git grep output")
	}
	content, rest, terminated := strings.Cut(payload, "\n")
	if !terminated {
		return "", fmt.Errorf("incomplete text record in git grep output")
	}
	emit(path, line, content)
	return rest, nil
}

func gitGrepExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func gitGrepError(stderr string, err error, usePerlRegexp bool) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("git grep timed out; try narrowing file_patterns to a more specific path: %w", err)
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	exitCode := gitGrepExitCode(err)
	message := trimGitUsage(stderr, exitCode)
	if message == "" {
		return fmt.Errorf("git grep failed: %w", err)
	}
	if usePerlRegexp && exitCode == 128 && isPerlRegexpCompileError(message) {
		message += `; fix the regular expression (e.g. use \( for a literal opening parenthesis), or set use_perl_regexp=false to search for the entire text literally`
	}
	return fmt.Errorf("git grep failed: %w: %s", err, message)
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
	return gitGrepExitCode(err) == 128 &&
		(strings.Contains(stderr, "not a git repository") || strings.Contains(stderr, ".git"))
}

func isPerlRegexpCompileError(stderr string) bool {
	for line := range strings.SplitSeq(stderr, "\n") {
		line = strings.Replace(line, "\uFF1A", ":", 1)
		_, diagnostic, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		diagnostic = strings.TrimSpace(diagnostic)
		if strings.HasPrefix(diagnostic, "-e option, '") ||
			strings.HasPrefix(diagnostic, "pcre2_compile failed") {
			return true
		}
	}
	return false
}
