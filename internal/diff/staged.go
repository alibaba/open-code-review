// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

// StagedSnapshot identifies an immutable comparison between HEAD and the
// complete staged tree. BaseCommit is empty before a repository's first commit;
// BaseTree is then Git's empty tree. Tree is a tree object, never a commit.
type StagedSnapshot struct {
	BaseCommit string
	BaseTree   string
	Tree       string
}

// stagedRawDiffHasGitlink checks modes from git diff --raw -z --no-renames.
// Each record contains a header and one path, both NUL-terminated. Consuming
// each pair keeps arbitrary path bytes separate from the structural modes.
func stagedRawDiffHasGitlink(text string) (bool, error) {
	for text != "" {
		header, rest, ok := strings.Cut(text, "\x00")
		if !ok {
			return false, fmt.Errorf("cannot read staged raw diff header")
		}
		path, rest, ok := strings.Cut(rest, "\x00")
		if !ok || path == "" {
			return false, fmt.Errorf("cannot read staged raw diff path")
		}
		fields := strings.Fields(header)
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") {
			return false, fmt.Errorf("cannot read staged raw diff modes")
		}
		if fields[0] == ":160000" || fields[1] == "160000" {
			return true, nil
		}
		text = rest
	}
	return false, nil
}

// CaptureStagedSnapshot copies the active index before asking Git to write its
// tree. Only the private copy can be rewritten: the user's index, refs and work
// tree stay untouched. Tree objects may be added to the repository object store.
// Concurrent HEAD or index changes detected during capture require a retry.
func CaptureStagedSnapshot(ctx context.Context, repoDir string, runner *gitcmd.Runner) (*StagedSnapshot, error) {
	if runner == nil {
		runner = gitcmd.New(0)
	}
	run := func(env []string, args ...string) (string, error) {
		out, stderr, err := runner.RunSplitWithEnv(ctx, repoDir, env, args...)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", gitFailure("capture staged snapshot", stderr, err)
		}
		return out, nil
	}
	base, err := stagedBaseCommit(ctx, repoDir, runner)
	if err != nil {
		return nil, err
	}
	indexPath, err := run(nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil, err
	}
	indexPath = strings.TrimSpace(indexPath)
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repoDir, indexPath)
	}
	original, exists, err := readStagedIndex(indexPath)
	if err != nil {
		return nil, err
	}
	privateDir, err := os.MkdirTemp("", "ocr-staged-index-")
	if err != nil {
		return nil, fmt.Errorf("create private staged index: %w", err)
	}
	defer os.RemoveAll(privateDir)
	privateIndex := filepath.Join(privateDir, "index")
	env := []string{"GIT_INDEX_FILE=" + privateIndex, "GIT_OPTIONAL_LOCKS=0"}
	if exists {
		if err := os.WriteFile(privateIndex, original, 0o600); err != nil {
			return nil, fmt.Errorf("copy staged index: %w", err)
		}
	} else if _, err := run(env, "read-tree", "--empty"); err != nil {
		return nil, err
	}

	shared, err := run(env, "rev-parse", "--shared-index-path")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(shared) != "" {
		return nil, fmt.Errorf("staged review does not support a split index; disable split index before retrying")
	}
	entries, err := run(env, "ls-files", "--sparse", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	for entry := range strings.SplitSeq(entries, "\x00") {
		if entry == "" {
			continue
		}
		header, _, _ := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if len(fields) != 3 {
			return nil, fmt.Errorf("cannot read staged index entry")
		}
		if fields[2] != "0" {
			return nil, fmt.Errorf("staged review requires all index conflicts to be resolved")
		}
		if fields[0] == "040000" {
			return nil, fmt.Errorf("staged review does not support a sparse index; expand the index before retrying")
		}
	}

	var baseTree string
	if base == "" {
		// An empty stdin produces the correct empty tree for the repository's
		// object format, including SHA-256 repositories.
		baseTree, err = run(env, "hash-object", "-t", "tree", "-w", "--stdin")
	} else {
		baseTree, err = run(env, "rev-parse", "--verify", "--end-of-options", base+"^{tree}")
	}
	if err != nil {
		return nil, err
	}
	baseTree = strings.TrimSpace(baseTree)

	// Git exposes intent-to-add entries differently under these two flags,
	// including empty files. Comparing NUL-delimited path sets avoids parsing
	// ls-files --debug's human-readable flags or looking at working-tree bytes.
	visible, err := run(env, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-renames", "--ita-visible-in-index", "--name-only", "-z", "--end-of-options", baseTree, "--")
	if err != nil {
		return nil, err
	}
	invisible, err := run(env, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-renames", "--ita-invisible-in-index", "--name-only", "-z", "--end-of-options", baseTree, "--")
	if err != nil {
		return nil, err
	}
	if visible != invisible {
		return nil, fmt.Errorf("staged review does not support intent-to-add entries; stage their contents or remove them from the index")
	}
	tree, err := run(env, "-c", "core.splitIndex=false", "write-tree")
	if err != nil {
		return nil, err
	}

	current, currentExists, err := readStagedIndex(indexPath)
	if err != nil {
		return nil, err
	}
	currentBase, err := stagedBaseCommit(ctx, repoDir, runner)
	if err != nil {
		return nil, err
	}
	if currentExists != exists || !bytes.Equal(original, current) || currentBase != base {
		return nil, fmt.Errorf("HEAD or the index changed while capturing the staged snapshot; retry the review")
	}
	return &StagedSnapshot{BaseCommit: base, BaseTree: baseTree, Tree: strings.TrimSpace(tree)}, nil
}

func readStagedIndex(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read staged index: %w", err)
	}
	return data, true, nil
}

func stagedBaseCommit(ctx context.Context, repoDir string, runner *gitcmd.Runner) (string, error) {
	return resolveStagedBaseCommit(ctx, func(args ...string) (string, string, error) {
		return runner.RunSplit(ctx, repoDir, args...)
	})
}

func resolveStagedBaseCommit(ctx context.Context, run func(...string) (string, string, error)) (string, error) {
	out, stderr, err := run("rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	// Only a symbolic HEAD whose target ref does not exist is unborn. Do not
	// turn invalid repositories, corrupt refs or cancellation into an empty base.
	ref, symbolicStderr, symbolicErr := run("symbolic-ref", "--quiet", "HEAD")
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if symbolicErr != nil {
		var exitErr *exec.ExitError
		if errors.As(symbolicErr, &exitErr) && exitErr.ExitCode() == 1 {
			// A detached HEAD has no symbolic target; preserve the original
			// commit-resolution failure rather than the expected probe status.
			return "", gitFailure("resolve staged review HEAD", stderr, err)
		}
		return "", gitFailure("git symbolic-ref for staged HEAD", symbolicStderr, symbolicErr)
	}
	_, lookupStderr, lookupErr := run("show-ref", "--verify", "--quiet", strings.TrimSpace(ref))
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if lookupErr != nil {
		var exitErr *exec.ExitError
		if errors.As(lookupErr, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", gitFailure("git show-ref for staged HEAD", lookupStderr, lookupErr)
	}
	return "", gitFailure("resolve staged review HEAD", stderr, err)
}
