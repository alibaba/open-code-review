// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/pathutil"
	"github.com/alibaba/open-code-review/internal/vcs"
)

var defaultRunner = gitcmd.New(16)

func (r *Repository) commandRunner() *gitcmd.Runner {
	if r.Runner != nil {
		return r.Runner
	}
	return defaultRunner
}

func repositoryPath(filePath string) (string, error) {
	filePath = strings.ReplaceAll(filePath, "\\", "/")
	if filePath == "" || strings.HasPrefix(filePath, "/") || filepath.IsAbs(filePath) || filepath.VolumeName(filePath) != "" {
		return "", fmt.Errorf("file path %q is outside repository", filePath)
	}
	filePath = path.Clean(filePath)
	for _, component := range strings.Split(filePath, "/") {
		if component == ".." {
			return "", fmt.Errorf("file path %q is outside repository", filePath)
		}
	}
	return filePath, nil
}

func (r *Repository) workspacePath(path string) (string, error) {
	root, err := pathutil.CanonicalPath(r.RepoDir)
	if err != nil {
		return "", fmt.Errorf("resolve repository path %q: %w", r.RepoDir, err)
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if !pathutil.WithinBase(root, full) {
		return "", fmt.Errorf("file path %q is outside repository", path)
	}
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", fmt.Errorf("read file %q: %w", path, err)
	}
	if !pathutil.WithinBase(root, resolved) {
		return "", fmt.Errorf("file path %q is outside repository", path)
	}
	return resolved, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// ReadFile owns the process and stream, including callbacks that stop after
// inspecting a prefix. Successful callbacks are followed by a drain so the
// subprocess can finish while still holding the shared Runner's slot.
func (r *Repository) ReadFile(parent context.Context, ref vcs.ContentRef, path string, consume func(io.Reader) error) error {
	if err := parent.Err(); err != nil {
		return err
	}
	path, err := repositoryPath(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	read := func(reader io.Reader) error {
		stream := contextReader{ctx: ctx, reader: reader}
		if err := consume(stream); err != nil {
			return err
		}
		_, err := io.Copy(io.Discard, stream)
		return err
	}
	if ref.IsWorkspace() {
		full, err := r.workspacePath(path)
		if err != nil {
			return err
		}
		file, err := os.Open(full)
		if err != nil {
			return fmt.Errorf("read file %q: %w", path, err)
		}
		defer file.Close()
		if err := consume(contextReader{ctx: ctx, reader: file}); err != nil {
			return err
		}
		return ctx.Err()
	}
	err = r.commandRunner().Stream(ctx, r.RepoDir, read, "show", "--end-of-options", ref.Revision+":"+path)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("git show %s:%s: %w", ref.Revision, path, err)
	}
	return nil
}

// ListFiles returns paths without tool-specific extension filters. NUL output
// preserves names containing spaces, colons, tabs, or newlines.
func (r *Repository) ListFiles(parent context.Context, ref vcs.ContentRef) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	if !ref.IsWorkspace() {
		args = []string{"ls-tree", "-r", "-z", "--name-only", "--end-of-options", ref.Revision}
	}
	output, stderr, err := r.commandRunner().RunSplit(ctx, r.RepoDir, args...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if ref.IsWorkspace() && (r.isNotGitRepository(ctx, err) || errors.Is(err, exec.ErrNotFound)) {
			return r.walkFiles(ctx)
		}
		return nil, fmt.Errorf("list repository files: %w: %s", err, strings.TrimSpace(stderr))
	}
	var files []string
	seen := make(map[string]bool)
	for _, path := range strings.Split(output, "\x00") {
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	return files, nil
}

func (r *Repository) isNotGitRepository(ctx context.Context, err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128 {
		return false
	}
	// Exit status avoids localized diagnostics; a corrupt index still belongs
	// to a repository, so its listing failure must be returned to the caller.
	_, probeErr := r.commandRunner().Output(ctx, r.RepoDir, "rev-parse", "--git-dir")
	return errors.As(probeErr, &exitErr) && exitErr.ExitCode() == 128
}

func (r *Repository) walkFiles(ctx context.Context) ([]string, error) {
	root, err := filepath.Abs(r.RepoDir)
	if err != nil {
		return nil, err
	}
	patterns := diff.LoadGitignorePatterns(root)
	var files []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if path != root {
				fmt.Fprintf(os.Stderr, "[ocr] WARNING: walk error at %s: %v\n", path, err)
				return nil
			}
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if diff.IsPathExcluded(root, rel, patterns) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}
