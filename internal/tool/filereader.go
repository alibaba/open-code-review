// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/pathutil"
)

// ReviewMode represents the active review mode.
type ReviewMode int

const (
	// ModeWorkspace reads files from the current working tree.
	ModeWorkspace ReviewMode = iota
	// ModeRange reads files as they exist at a specific git ref (--to value).
	ModeRange
	// ModeCommit reads files as they exist at a specific commit hash.
	ModeCommit
	ModeEmpty
)

// ParseReviewMode returns the correct ReviewMode based on provided flag values.
func ParseReviewMode(from, to, commit string) ReviewMode {
	if commit != "" {
		return ModeCommit
	}
	if from != "" && to != "" {
		return ModeRange
	}
	return ModeWorkspace
}

// RefValue returns the git ref that should be used for reading file contents
// in range or commit mode. Returns ("", false) for workspace mode.
func (m ReviewMode) RefValue(toRef, commit string) (string, bool) {
	switch m {
	case ModeRange:
		return toRef, true
	case ModeCommit:
		return commit, true
	default:
		return "", false
	}
}

// FileReader resolves file contents according to the active review mode.
type FileReader struct {
	RepoDir string
	Mode    ReviewMode
	// Ref is the git ref to use for ModeRange (--to) or ModeCommit (--commit).
	// Empty for ModeWorkspace.
	Ref    string
	Runner *gitcmd.Runner
}

// Read returns the full content of a file path (relative to RepoDir),
// resolved according to the active review mode.
// - Workspace: reads directly from the filesystem.
// - Range / Commit: uses `git show <Ref>:<path>` to read at the given ref.
func (fr *FileReader) Read(ctx context.Context, path string) (string, error) {
	switch fr.Mode {
	case ModeEmpty:
		return "", fmt.Errorf("file %q does not exist in the empty tree", path)
	case ModeWorkspace:
		_, content, err := fr.readFromDisk(path)
		return content, err
	case ModeRange, ModeCommit:
		_, content, err := fr.readFromGitShow(ctx, path)
		return content, err
	default:
		_, content, err := fr.readFromDisk(path)
		return content, err
	}
}

func (fr *FileReader) readFromDisk(rawPath string) (string, string, error) {
	fullPath, resolvedRel, err := fr.resolveWorkspacePath(rawPath)
	if err != nil {
		return "", "", err
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("read file %q: %w", rawPath, err)
	}
	return resolvedRel, string(content), nil
}

type ambiguousPathError struct {
	requested  string
	candidates []string
	ref        string
}

func (e *ambiguousPathError) Error() string {
	if e.ref != "" {
		return fmt.Sprintf("file path %q is ambiguous at ref %s; matching files: %s", e.requested, e.ref, strings.Join(e.candidates, ", "))
	}
	return fmt.Sprintf("file path %q is ambiguous; matching files: %s", e.requested, strings.Join(e.candidates, ", "))
}

// cleanRepoPath cleans a repository-relative path without rewriting
// backslashes so literal-backslash filenames on POSIX remain distinct.
func cleanRepoPath(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return strings.TrimPrefix(p, "/")
}

// cleanNativeRepoPath cleans a repository-relative path according to the host
// OS path semantics. On Windows backslashes are directory separators; on POSIX
// backslashes are preserved as literal filename characters.
func cleanNativeRepoPath(p string) string {
	if runtime.GOOS == "windows" {
		return normalizePath(p)
	}
	return cleanRepoPath(p)
}

// normalizePath standardizes relative file paths across platforms by converting
// Windows backslashes to forward slashes, cleaning redundant segments, and stripping
// leading slashes or "./".
func normalizePath(p string) string {
	return cleanRepoPath(strings.ReplaceAll(p, "\\", "/"))
}

func (fr *FileReader) resolveWorkspacePath(rawPath string) (string, string, error) {
	cleanRel := cleanNativeRepoPath(rawPath)
	if cleanRel == "" {
		return "", "", fmt.Errorf("file path %q is empty or invalid", rawPath)
	}
	repoRoot, err := pathutil.CanonicalPath(fr.RepoDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve repository path %q: %w", fr.RepoDir, err)
	}

	primaryFull := filepath.Join(repoRoot, filepath.FromSlash(cleanRel))
	if !pathutil.WithinBase(repoRoot, primaryFull) {
		return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
	}

	resolvedPrimary, err := filepath.EvalSymlinks(primaryFull)
	if err == nil {
		if !pathutil.WithinBase(repoRoot, resolvedPrimary) {
			return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
		}
		return resolvedPrimary, cleanRel, nil
	}
	if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("resolve file %q: %w", rawPath, err)
	}

	// Only fall back to Windows-style separator normalization when the query
	// contains backslashes on POSIX (where winNorm differs from cleanRel).
	winNorm := normalizePath(rawPath)
	if winNorm == cleanRel {
		return primaryFull, cleanRel, nil
	}
	if winNorm == "" {
		return "", "", fmt.Errorf("file path %q is empty or invalid", rawPath)
	}

	winFull := filepath.Join(repoRoot, filepath.FromSlash(winNorm))
	if !pathutil.WithinBase(repoRoot, winFull) {
		return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
	}

	matches := findWorkspaceNormalizedMatches(repoRoot, winNorm)
	switch len(matches) {
	case 0:
		resolvedWin, winErr := filepath.EvalSymlinks(winFull)
		if winErr == nil {
			if !pathutil.WithinBase(repoRoot, resolvedWin) {
				return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
			}
			return resolvedWin, winNorm, nil
		}
		if !os.IsNotExist(winErr) {
			return "", "", fmt.Errorf("resolve file %q: %w", rawPath, winErr)
		}
		return primaryFull, cleanRel, nil
	case 1:
		matchRel := matches[0]
		matchFull := filepath.Join(repoRoot, filepath.FromSlash(matchRel))
		if !pathutil.WithinBase(repoRoot, matchFull) {
			return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
		}
		resolvedMatch, matchErr := filepath.EvalSymlinks(matchFull)
		if matchErr == nil {
			if !pathutil.WithinBase(repoRoot, resolvedMatch) {
				return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
			}
			return resolvedMatch, matchRel, nil
		}
		if os.IsNotExist(matchErr) {
			return matchFull, matchRel, nil
		}
		return "", "", fmt.Errorf("resolve file %q: %w", rawPath, matchErr)
	default:
		sort.Strings(matches)
		return "", "", &ambiguousPathError{
			requested:  rawPath,
			candidates: matches,
		}
	}
}

func findWorkspaceNormalizedMatches(repoRoot, winNorm string) []string {
	var matches []string
	visited := map[string]struct{}{repoRoot: {}}

	var walkDir func(curDiskDir, relDir string)
	walkDir = func(curDiskDir, relDir string) {
		entries, err := os.ReadDir(curDiskDir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if relDir == "" && name == ".git" {
				continue
			}
			entryRel := name
			if relDir != "" {
				entryRel = relDir + "/" + name
			}
			normRel := normalizePath(entryRel)
			canDescend := (normRel != "" && strings.HasPrefix(winNorm, normRel+"/")) || (normRel == "" && strings.Trim(name, "\\") == "")
			entryDiskPath := filepath.Join(curDiskDir, name)

			if entry.IsDir() {
				if canDescend {
					resolved, symErr := filepath.EvalSymlinks(entryDiskPath)
					if symErr == nil && pathutil.WithinBase(repoRoot, resolved) {
						if _, seen := visited[resolved]; !seen {
							visited[resolved] = struct{}{}
							walkDir(entryDiskPath, entryRel)
						}
					}
				}
				continue
			}

			if entry.Type()&os.ModeSymlink != 0 {
				resolved, symErr := filepath.EvalSymlinks(entryDiskPath)
				if symErr == nil {
					if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
						if pathutil.WithinBase(repoRoot, resolved) && canDescend {
							if _, seen := visited[resolved]; !seen {
								visited[resolved] = struct{}{}
								walkDir(resolved, entryRel)
							}
						}
						continue
					}
				}
			}

			if normRel == winNorm {
				matches = append(matches, entryRel)
			}
		}
	}

	walkDir(repoRoot, "")
	return matches
}

func (fr *FileReader) readFromGitShow(parentCtx context.Context, rawPath string) (string, string, error) {
	cleanRel := cleanNativeRepoPath(rawPath)
	if cleanRel == "" {
		return "", "", fmt.Errorf("file path %q is empty or invalid", rawPath)
	}
	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") {
		return "", "", fmt.Errorf("file path %q is outside repository", rawPath)
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	out, err := fr.gitShowBytes(ctx, cleanRel)
	if err == nil {
		return cleanRel, string(out), nil
	}

	target, resolved, fallbackErr := fr.resolveGitFallbackPath(ctx, rawPath, cleanRel)
	if fallbackErr != nil {
		return "", "", fallbackErr
	}
	if !resolved {
		return "", "", fmt.Errorf("git show %s:%s: %w", fr.Ref, cleanRel, err)
	}

	out, err = fr.gitShowBytes(ctx, target)
	if err != nil {
		return "", "", fmt.Errorf("git show %s:%s: %w", fr.Ref, target, err)
	}
	return target, string(out), nil
}

func (fr *FileReader) gitShowBytes(ctx context.Context, repoRel string) ([]byte, error) {
	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", fr.Ref + ":" + repoRel}
	if fr.Runner != nil {
		return fr.Runner.Output(ctx, fr.RepoDir, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = fr.RepoDir
	return cmd.Output()
}

func (fr *FileReader) listGitTreePaths(ctx context.Context) ([]string, error) {
	args := []string{"-c", "core.quotepath=false", "ls-tree", "-r", "-z", "--name-only", "--end-of-options", fr.Ref}
	var out []byte
	var err error
	if fr.Runner != nil {
		out, err = fr.Runner.Output(ctx, fr.RepoDir, args...)
	} else {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = fr.RepoDir
		out, err = cmd.Output()
	}
	if err != nil {
		return nil, err
	}
	records := bytes.Split(out, []byte{0})
	paths := make([]string, 0, len(records))
	for _, rec := range records {
		if len(rec) > 0 {
			paths = append(paths, string(rec))
		}
	}
	return paths, nil
}

func (fr *FileReader) resolveGitFallbackPath(ctx context.Context, rawPath, cleanRel string) (string, bool, error) {
	if ctx.Err() != nil {
		return "", false, nil
	}
	winNorm := normalizePath(rawPath)
	if winNorm == cleanRel {
		return "", false, nil
	}
	if winNorm == "" {
		return "", false, fmt.Errorf("file path %q is empty or invalid", rawPath)
	}
	if winNorm == ".." || strings.HasPrefix(winNorm, "../") {
		return "", false, fmt.Errorf("file path %q is outside repository", rawPath)
	}

	treePaths, err := fr.listGitTreePaths(ctx)
	if err != nil {
		return "", false, nil
	}

	var matches []string
	for _, entry := range treePaths {
		if normalizePath(entry) == winNorm {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return "", false, nil
	case 1:
		return matches[0], true, nil
	default:
		sort.Strings(matches)
		return "", false, &ambiguousPathError{
			requested:  rawPath,
			candidates: matches,
			ref:        fr.Ref,
		}
	}
}

// ReadLines returns a window of lines from the file plus the total line count.
// startLine is 1-based; maxLines is the maximum number of lines to collect.
func (fr *FileReader) ReadLines(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	_, lines, total, err := fr.readLinesResolved(ctx, path, startLine, maxLines)
	return lines, total, err
}

func (fr *FileReader) readLinesResolved(ctx context.Context, path string, startLine, maxLines int) (string, []string, int, error) {
	switch fr.Mode {
	case ModeEmpty:
		return "", nil, 0, fmt.Errorf("file %q does not exist in the empty tree", path)
	case ModeWorkspace:
		return fr.readLinesFromDisk(path, startLine, maxLines)
	case ModeRange, ModeCommit:
		innerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return fr.readLinesFromGitShow(innerCtx, path, startLine, maxLines)
	default:
		return fr.readLinesFromDisk(path, startLine, maxLines)
	}
}

// scanLines reads from r line by line, collecting at most maxLines lines
// starting from startLine (1-based), while counting the total number of lines.
// The behavior matches strings.Split(content, "\n") for trailing-newline files.
func scanLines(r io.Reader, startLine, maxLines int) ([]string, int, error) {
	br := bufio.NewReader(r)
	var collected []string
	lineNum := 0
	lastHadNewline := false

	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			lineNum++
			lastHadNewline = line[len(line)-1] == '\n'
			trimmed := strings.TrimSuffix(line, "\n")
			trimmed = strings.TrimSuffix(trimmed, "\r")
			if lineNum >= startLine && len(collected) < maxLines {
				collected = append(collected, trimmed)
			}
		}
		if err != nil {
			if err != io.EOF {
				return nil, 0, err
			}
			break
		}
	}

	if lastHadNewline {
		lineNum++
		if lineNum >= startLine && len(collected) < maxLines {
			collected = append(collected, "")
		}
	}

	return collected, lineNum, nil
}

func (fr *FileReader) readLinesFromDisk(rawPath string, startLine, maxLines int) (string, []string, int, error) {
	fullPath, resolvedRel, err := fr.resolveWorkspacePath(rawPath)
	if err != nil {
		return "", nil, 0, err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return "", nil, 0, fmt.Errorf("read file %q: %w", rawPath, err)
	}
	defer f.Close()

	lines, total, err := scanLines(f, startLine, maxLines)
	if err != nil {
		return "", nil, 0, err
	}
	return resolvedRel, lines, total, nil
}

func (fr *FileReader) readLinesFromGitShow(ctx context.Context, rawPath string, startLine, maxLines int) (string, []string, int, error) {
	cleanRel := cleanNativeRepoPath(rawPath)
	if cleanRel == "" {
		return "", nil, 0, fmt.Errorf("file path %q is empty or invalid", rawPath)
	}
	if cleanRel == ".." || strings.HasPrefix(cleanRel, "../") {
		return "", nil, 0, fmt.Errorf("file path %q is outside repository", rawPath)
	}

	collected, totalLines, err := fr.gitShowLines(ctx, cleanRel, startLine, maxLines)
	if err == nil {
		return cleanRel, collected, totalLines, nil
	}

	target, resolved, fallbackErr := fr.resolveGitFallbackPath(ctx, rawPath, cleanRel)
	if fallbackErr != nil {
		return "", nil, 0, fallbackErr
	}
	if !resolved {
		return "", nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, cleanRel, err)
	}

	collected, totalLines, err = fr.gitShowLines(ctx, target, startLine, maxLines)
	if err != nil {
		return "", nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, target, err)
	}
	return target, collected, totalLines, nil
}

func (fr *FileReader) gitShowLines(ctx context.Context, repoRel string, startLine, maxLines int) ([]string, int, error) {
	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", fr.Ref + ":" + repoRel}

	var collected []string
	var totalLines int

	if fr.Runner != nil {
		err := fr.Runner.Stream(ctx, fr.RepoDir, func(stdout io.Reader) error {
			var scanErr error
			collected, totalLines, scanErr = scanLines(stdout, startLine, maxLines)
			return scanErr
		}, args...)
		if err != nil {
			return nil, 0, err
		}
		return collected, totalLines, nil
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = fr.RepoDir
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}

	collected, totalLines, scanErr := scanLines(stdoutPipe, startLine, maxLines)
	if scanErr != nil {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()

	if scanErr != nil {
		return nil, 0, scanErr
	}
	if waitErr != nil {
		return nil, 0, waitErr
	}
	return collected, totalLines, nil
}
