package diff

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/open-code-review/open-code-review/internal/pathutil"
)

func readWorkspaceFileForDiff(repoDir, relPath string) ([]byte, error) {
	repoRoot, err := pathutil.CanonicalPath(repoDir)
	if err != nil {
		return nil, fmt.Errorf("resolve repository path %q: %w", repoDir, err)
	}
	if filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("file path %q must be relative, not absolute", relPath)
	}

	fullPath := filepath.Join(repoRoot, relPath)
	if !pathutil.WithinBase(repoRoot, fullPath) {
		return nil, fmt.Errorf("file path %q is outside repository", relPath)
	}

	parent, err := filepath.EvalSymlinks(filepath.Dir(fullPath))
	if err != nil {
		return nil, fmt.Errorf("resolve parent path for %q: %w", relPath, err)
	}
	if !pathutil.WithinBase(repoRoot, parent) {
		return nil, fmt.Errorf("file path %q is outside repository", relPath)
	}

	info, err := os.Lstat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("stat file %q: %w", relPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("file path %q is a directory", relPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read symlink %q: %w", relPath, err)
		}
		return []byte(target), nil
	}

	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return nil, fmt.Errorf("resolve file %q: %w", relPath, err)
	}
	if !pathutil.WithinBase(repoRoot, resolvedPath) {
		return nil, fmt.Errorf("file path %q is outside repository", relPath)
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", relPath, err)
	}
	return content, nil
}
