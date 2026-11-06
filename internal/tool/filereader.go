// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	case ModeWorkspace:
		return fr.readFromDisk(path)
	case ModeRange, ModeCommit:
		return fr.readFromGitShow(ctx, path)
	default:
		return fr.readFromDisk(path)
	}
}

func (fr *FileReader) readFromDisk(path string) (string, error) {
	fullPath, err := fr.resolveWorkspacePath(path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	return string(content), nil
}

func (fr *FileReader) resolveWorkspacePath(path string) (string, error) {
	repoRoot, err := pathutil.CanonicalPath(fr.RepoDir)
	if err != nil {
		return "", fmt.Errorf("resolve repository path %q: %w", fr.RepoDir, err)
	}

	fullPath := filepath.Join(repoRoot, path)
	if !pathutil.WithinBase(repoRoot, fullPath) {
		return "", fmt.Errorf("file path %q is outside repository", path)
	}

	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// The path is not there. Resolve its parent first: a path that
			// leaves the repository through a symlinked directory must still
			// be refused as out-of-repository, and today the missing file
			// hides that escape behind a bare "does not exist". The pattern
			// is the one internal/diff/workspace_file.go already uses.
			parent, parentErr := filepath.EvalSymlinks(filepath.Dir(fullPath))
			if parentErr == nil && !pathutil.WithinBase(repoRoot, parent) {
				return "", fmt.Errorf("file path %q is outside repository", path)
			}
			return "", pathNotFoundInWorkspace(path, err)
		}
		return "", fmt.Errorf("resolve file %q: %w", path, err)
	}
	if !pathutil.WithinBase(repoRoot, resolvedPath) {
		return "", fmt.Errorf("file path %q is outside repository", path)
	}
	return resolvedPath, nil
}

// runGit runs a git command in the repository and returns its stdout.
func (fr *FileReader) runGit(ctx context.Context, args ...string) ([]byte, error) {
	if fr.Runner != nil {
		return fr.Runner.Output(ctx, fr.RepoDir, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = fr.RepoDir
	return cmd.Output()
}

// pathExistsAtRef reports whether path is present in the tree at the
// reader's ref.
//
// It returns (true, nil) when the path is present, (false, nil) when it is
// absent, and (false, err) when the answer could not be established — the ref
// does not resolve, git failed, or the context ended. A caller must treat the
// third case as unknown and fall back to the ordinary read rather than
// rejecting the path.
func (fr *FileReader) pathExistsAtRef(ctx context.Context, path string) (bool, error) {
	// `rev-parse --verify --quiet` prints the object id and exits 0 when the
	// path exists, and exits 1 with no output when it does not. It is the
	// same command shape internal/diff/git.go uses to resolve a ref.
	_, err := fr.runGit(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", fr.Ref+":"+path)
	if err == nil {
		return true, nil
	}
	// Exit 1 means "not there" or "the ref does not resolve": rev-parse
	// reports both the same way. Ask the ref itself before concluding that
	// the path is absent.
	if !isExitCode(err, 1) {
		return false, err
	}
	if _, refErr := fr.runGit(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", fr.Ref+"^{commit}"); refErr != nil {
		return false, refErr
	}
	return false, nil
}

// isExitCode reports whether err is a git invocation that exited with the
// given status.
func isExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}

// rejectAbsentPath returns the structural rejection when path is known to be
// absent at the reader's ref. When the answer cannot be established it
// returns nil, so that the ordinary read proceeds and its own error surfaces
// as it does today.
func (fr *FileReader) rejectAbsentPath(ctx context.Context, path string) error {
	if fr.Ref == "" {
		// There is nothing to resolve the path against, so there is nothing
		// to decide: let the ordinary read report what it finds.
		return nil
	}
	exists, err := fr.pathExistsAtRef(ctx, path)
	if err != nil {
		return nil
	}
	if exists {
		return nil
	}
	return pathNotFoundAtRef(path, fr.Ref)
}

func (fr *FileReader) readFromGitShow(parentCtx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	if err := fr.rejectAbsentPath(ctx, path); err != nil {
		return "", err
	}

	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", fr.Ref + ":" + path}
	if fr.Runner != nil {
		output, err := fr.Runner.Output(ctx, fr.RepoDir, args...)
		if err != nil {
			return "", fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
		}
		return string(output), nil
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = fr.RepoDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
	}
	return string(output), nil
}

// ReadLines returns a window of lines from the file plus the total line count.
// startLine is 1-based; maxLines is the maximum number of lines to collect.
func (fr *FileReader) ReadLines(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	switch fr.Mode {
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

func (fr *FileReader) readLinesFromDisk(path string, startLine, maxLines int) ([]string, int, error) {
	fullPath, err := fr.resolveWorkspacePath(path)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, fmt.Errorf("read file %q: %w", path, err)
	}
	defer f.Close()

	return scanLines(f, startLine, maxLines)
}

func (fr *FileReader) readLinesFromGitShow(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	if err := fr.rejectAbsentPath(ctx, path); err != nil {
		return nil, 0, err
	}

	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", fr.Ref + ":" + path}

	var collected []string
	var totalLines int

	if fr.Runner != nil {
		err := fr.Runner.Stream(ctx, fr.RepoDir, func(stdout io.Reader) error {
			var scanErr error
			collected, totalLines, scanErr = scanLines(stdout, startLine, maxLines)
			return scanErr
		}, args...)
		if err != nil {
			return nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
		}
		return collected, totalLines, nil
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = fr.RepoDir
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
	}

	collected, totalLines, scanErr := scanLines(stdoutPipe, startLine, maxLines)
	if scanErr != nil {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()

	if scanErr != nil {
		return nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, path, scanErr)
	}
	if waitErr != nil {
		return nil, 0, fmt.Errorf("git show %s:%s: %w", fr.Ref, path, waitErr)
	}
	return collected, totalLines, nil
}
