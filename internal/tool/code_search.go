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
		cmdArgs = append(cmdArgs, "--end-of-options")
		cmdArgs = append(cmdArgs, ref)
	}

	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, pathspec...)

	return cmdArgs
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

	outStr, errStr, err := p.runGitGrep(ctx, cmdArgs)

	// Non-git directory: `git grep` exits 128 with "not a git repository".
	// `ocr scan` supports plain directories, so retry in --no-index mode, which
	// searches the working tree directly while still honoring .gitignore.
	// Ref-based search needs a real repo, so it is not retried.
	if err != nil && p.FileReader.Ref == "" && strings.Contains(errStr, "not a git repository") {
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
		m := match{}
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
