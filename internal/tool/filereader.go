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
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/pathutil"
	"github.com/alibaba/open-code-review/internal/textenc"
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
	text, _, _ := textenc.DecodeSource(path, content)
	return text, nil
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
			return fullPath, nil
		}
		return "", fmt.Errorf("resolve file %q: %w", path, err)
	}
	if !pathutil.WithinBase(repoRoot, resolvedPath) {
		return "", fmt.Errorf("file path %q is outside repository", path)
	}
	return resolvedPath, nil
}

func (fr *FileReader) readFromGitShow(parentCtx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", fr.Ref + ":" + path}
	if fr.Runner != nil {
		output, err := fr.Runner.Output(ctx, fr.RepoDir, args...)
		if err != nil {
			return "", fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
		}
		text, _, _ := textenc.DecodeSource(path, output)
		return text, nil
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = fr.RepoDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git show %s:%s: %w", fr.Ref, path, err)
	}
	text, _, _ := textenc.DecodeSource(path, output)
	return text, nil
}

// decodeMaxBytes bounds how much of a file file_read will buffer in order to
// run charset detection. Detection needs whole-file evidence — a head window is
// measurably harmful, because a file whose first non-ASCII line sits past the
// window collapses from confidence 100 to 55 and turns into a false skip — so a
// file larger than this keeps the streaming path and its bytes stay raw.
//
// The number matches scan.DefaultMaxFileSizeBytes. file_read has no other byte
// cap (only fileReadMaxLines) and the model picks which file to read, so
// without this an unbounded allocation would be driven by model input.
//
// It is one constant with no config knob. Raise it if a real repo has >2 MiB
// legacy source files.
const decodeMaxBytes = 2 << 20

// decodeThenScan buffers up to decodeMaxBytes from r so charset detection sees
// whole-file evidence, decodes, and scans the result. A stream larger than the
// cap is handed on unread through io.MultiReader, keeping today's streaming
// behaviour with its bytes raw. Peak buffering is bounded at the cap either way.
//
// Decoding must happen before scanLines: once the bytes are split into lines
// there is no whole-file evidence left to detect on.
func decodeThenScan(path string, r io.Reader, startLine, maxLines int) ([]string, int, error) {
	data, err := io.ReadAll(io.LimitReader(r, decodeMaxBytes+1))
	if err != nil {
		return nil, 0, err
	}
	if len(data) > decodeMaxBytes {
		return scanLines(io.MultiReader(bytes.NewReader(data), r), startLine, maxLines)
	}
	text, _, _ := textenc.DecodeSource(path, data)
	return scanLines(strings.NewReader(text), startLine, maxLines)
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

	return decodeThenScan(path, f, startLine, maxLines)
}

func (fr *FileReader) readLinesFromGitShow(ctx context.Context, path string, startLine, maxLines int) ([]string, int, error) {
	args := []string{"-c", "core.quotepath=false", "show", "--end-of-options", fr.Ref + ":" + path}

	var collected []string
	var totalLines int

	if fr.Runner != nil {
		err := fr.Runner.Stream(ctx, fr.RepoDir, func(stdout io.Reader) error {
			var scanErr error
			collected, totalLines, scanErr = decodeThenScan(path, stdout, startLine, maxLines)
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

	collected, totalLines, scanErr := decodeThenScan(path, stdoutPipe, startLine, maxLines)
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
