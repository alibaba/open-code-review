// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
)

// PatchProvider reads unified diff files from an external directory.
type PatchProvider struct {
	repoDir string
	diffDir string
	ref     string
	runner  *gitcmd.Runner
}

func NewPatchProvider(repoDir, diffDir, ref string, runner *gitcmd.Runner) *PatchProvider {
	return &PatchProvider{repoDir: repoDir, diffDir: diffDir, ref: ref, runner: runner}
}

func (p *PatchProvider) ResolveInput(context.Context) InputResolution {
	return InputResolution{ResolvedHead: p.ref}
}

func (p *PatchProvider) RemoteIdentity(context.Context) string { return "" }

func patchPaths(diffDir string) ([]string, error) {
	info, err := os.Stat(diffDir)
	if err != nil {
		return nil, fmt.Errorf("stat diff directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("diff path %q is not a directory", diffDir)
	}

	var paths []string
	err = filepath.WalkDir(diffDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".patch" || ext == ".diff" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk diff directory: %w", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("diff directory %q contains no .patch or .diff files", diffDir)
	}
	return paths, nil
}

// ValidatePatchDirectory verifies that path is a readable directory containing
// at least one supported patch file.
func ValidatePatchDirectory(path string) error {
	_, err := patchPaths(path)
	return err
}

// stripBinaryPatchSections removes binary file sections that do not carry blob
// data. External PR patches commonly contain only "Binary files ... differ",
// which git apply cannot materialize when the new blob is not in the object
// database. Text sections remain applicable and the parser still reports the
// omitted files as binary changes.
func stripBinaryPatchSections(data []byte) ([]byte, bool) {
	lines := strings.Split(string(data), "\n")
	var output strings.Builder
	var section strings.Builder
	sectionIsBinary := false
	hadSection := false
	skipped := false
	flush := func() {
		if section.Len() == 0 {
			return
		}
		if sectionIsBinary {
			skipped = true
		} else {
			output.WriteString(section.String())
		}
		section.Reset()
		sectionIsBinary = false
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			hadSection = true
		}
		if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
			sectionIsBinary = true
		}
		section.WriteString(line)
		section.WriteByte('\n')
	}
	flush()
	if !hadSection {
		return data, false
	}
	return []byte(strings.TrimSuffix(output.String(), "\n")), skipped
}

// MaterializePatchCommit applies the patch directory to base in a temporary
// index and creates an unreferenced commit for use as an immutable post-image.
// It does not modify the working tree, the selected branch, or any repository ref.
func MaterializePatchCommit(ctx context.Context, repoDir, diffDir, base string, runner *gitcmd.Runner) (string, error) {
	paths, err := patchPaths(diffDir)
	if err != nil {
		return "", err
	}
	index, err := os.CreateTemp("", "ocr-patch-index-*")
	if err != nil {
		return "", fmt.Errorf("create temporary patch index: %w", err)
	}
	indexPath := index.Name()
	if err := index.Close(); err != nil {
		os.Remove(indexPath)
		return "", fmt.Errorf("close temporary patch index: %w", err)
	}
	if err := os.Remove(indexPath); err != nil {
		return "", fmt.Errorf("prepare temporary patch index: %w", err)
	}
	defer os.Remove(indexPath)
	env := []string{"GIT_INDEX_FILE=" + indexPath}

	run := func(input []byte, args ...string) ([]byte, error) {
		if runner != nil {
			return runner.OutputWithInputEnv(ctx, repoDir, input, env, args...)
		}
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repoDir
		cmd.Env = gitcmd.MergeEnv(os.Environ(), env)
		cmd.Stdin = bytes.NewReader(input)
		return cmd.CombinedOutput()
	}
	if out, err := run(nil, "read-tree", base); err != nil {
		return "", fmt.Errorf("initialize patch snapshot from %s: %w: %s", base, err, strings.TrimSpace(string(out)))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read patch %q: %w", path, err)
		}
		applyData, skippedBinary := stripBinaryPatchSections(data)
		if skippedBinary {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: skipping binary patch sections in %s; external patch has no binary blob data\n", path)
		}
		if len(bytes.TrimSpace(applyData)) == 0 {
			continue
		}
		if out, err := run(applyData, "apply", "--cached", "--whitespace=nowarn"); err != nil {
			return "", fmt.Errorf("apply patch %q to %s: %w: %s", path, base, err, strings.TrimSpace(string(out)))
		}
	}
	treeOut, err := run(nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("write patch snapshot tree: %w: %s", err, strings.TrimSpace(string(treeOut)))
	}
	tree := strings.TrimSpace(string(treeOut))
	commitOut, err := run([]byte("OpenCodeReview patch post-image\n"),
		"-c", "user.name=OpenCodeReview", "-c", "user.email=ocr@localhost",
		"commit-tree", tree, "-p", base)
	if err != nil {
		return "", fmt.Errorf("create patch snapshot commit: %w: %s", err, strings.TrimSpace(string(commitOut)))
	}
	return strings.TrimSpace(string(commitOut)), nil
}

// GetDiff reads .patch and .diff files in lexical order and parses their
// contents using the same unified-diff parser as git-backed review modes.
func (p *PatchProvider) GetDiff(ctx context.Context) ([]model.Diff, error) {
	paths, err := patchPaths(p.diffDir)
	if err != nil {
		return nil, err
	}

	var combined strings.Builder
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read patch %q: %w", path, readErr)
		}
		combined.Write(data)
		combined.WriteString("\n")
	}
	parsed, err := ParseDiffText(ctx, combined.String(), p.repoDir, p.ref, p.runner)
	if err != nil {
		return nil, fmt.Errorf("parse patches: %w", err)
	}
	if len(parsed) == 0 && strings.TrimSpace(combined.String()) != "" {
		return nil, fmt.Errorf("parse patches: patch files contain content but no file diffs")
	}
	return parsed, nil
}
