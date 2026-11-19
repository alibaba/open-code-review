// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

func (code *codeTools) loadWorkspaceScope(ctx context.Context, input Input, runner *gitcmd.Runner) error {
	data, err := runner.Output(ctx, input.RepoDir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return fmt.Errorf("list workspace files: %w", err)
	}
	filter := &rules.FileFilter{Exclude: input.ExcludePatterns}
	for _, filePath := range strings.Split(string(data), "\x00") {
		if filePath == "" {
			continue
		}
		if safePath(filePath) && diff.ProviderDirPrefix(filePath) == "" && !filter.IsUserExcluded(filePath) && regularWorkspaceFile(input.RepoDir, filePath) {
			code.allowed["new"][filePath] = true
		}
	}
	return nil
}

func regularWorkspaceFile(repoDir, filePath string) bool {
	if !safePath(filePath) {
		return false
	}
	current := repoDir
	components := strings.Split(filePath, "/")
	for index, component := range components {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if index == len(components)-1 {
			return info.Mode().IsRegular()
		}
		if !info.IsDir() {
			return false
		}
	}
	return false
}

func (code *codeTools) fileAllowed(side, filePath string) bool {
	if !code.allowed[side][filePath] {
		return false
	}
	return code.comparison.Mode != "workspace" || side != "new" || regularWorkspaceFile(code.comparison.Repository, filePath)
}

func workspaceInventory(ctx context.Context, input Input, code *codeTools, tracked []model.Diff, endpoints []string) ([]model.Diff, map[string]bool, error) {
	staged, err := code.runner.Output(ctx, input.RepoDir, append([]string{"diff", "--cached", "--name-status", "-z", "--no-ext-diff", "--no-textconv", "--find-renames=100%"}, endpoints...)...)
	if err != nil {
		return nil, nil, err
	}
	renames, err := parseInventory(string(staged))
	if err != nil {
		return nil, nil, err
	}
	byPath := map[string]model.Diff{}
	for _, changed := range tracked {
		byPath[changed.NewPath] = changed
	}
	for _, rename := range renames {
		if rename.IsRenamed && byPath[rename.NewPath].IsNew && byPath[rename.OldPath].IsDeleted && (!code.allowed["old"][rename.OldPath] || !code.allowed["new"][rename.NewPath]) {
			byPath[rename.NewPath] = rename
			delete(byPath, rename.OldPath)
		}
	}
	var candidates [][]string
	var additions, deletions bool
	for _, changed := range tracked {
		if current, ok := byPath[changed.NewPath]; !ok || current.IsRenamed {
			continue
		}
		if changed.IsNew && code.fileAllowed("new", changed.NewPath) {
			additions = true
		} else if changed.IsDeleted && code.allowed["old"][changed.OldPath] {
			deletions = true
		} else {
			continue
		}
		candidates = append(candidates, []string{changed.NewPath})
	}
	if additions && deletions {
		batches, err := literalPathBatches(candidates)
		if err != nil {
			return nil, nil, err
		}
		children := changeChildren(tracked)
		for _, batch := range batches {
			args := append([]string{"diff", "--no-ext-diff", "--no-textconv", "--name-status", "-z", "--find-renames"}, endpoints...)
			data, err := code.runner.Output(ctx, input.RepoDir, append(args, exactPatchPaths(batch, children)...)...)
			if err != nil {
				return nil, nil, err
			}
			pairs, err := parseInventory(string(data))
			if err != nil {
				return nil, nil, err
			}
			for _, pair := range pairs {
				if pair.IsRenamed {
					byPath[pair.NewPath] = pair
					delete(byPath, pair.OldPath)
				}
			}
		}
	}
	raw, err := code.runner.Output(ctx, input.RepoDir, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, nil, err
	}
	untracked := map[string]bool{}
	for _, filePath := range strings.Split(string(raw), "\x00") {
		if filePath == "" {
			continue
		}
		untracked[filePath] = true
		prior, exists := byPath[filePath]
		if exists && !prior.IsDeleted {
			return nil, nil, fmt.Errorf("ambiguous workspace change for %q", filePath)
		}
		byPath[filePath] = model.Diff{OldPath: filePath, NewPath: filePath, IsNew: !exists}
	}
	changes := make([]model.Diff, 0, len(byPath))
	for _, filePath := range slices.Sorted(maps.Keys(byPath)) {
		changes = append(changes, byPath[filePath])
	}
	return changes, untracked, nil
}

func workspaceUntrackedPatch(ctx context.Context, input Input, changed model.Diff, runner *gitcmd.Runner) (model.Diff, bool, error) {
	filePath := changed.NewPath
	file, err := tool.OpenRegularWorkspaceFile(input.RepoDir, filePath)
	if err != nil {
		return changed, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return changed, false, err
	}
	const limit = 64 << 20
	if info.Size() > limit {
		changed.IsBinary = true
		return changed, true, nil
	}
	mode := "100644"
	if info.Mode()&0111 != 0 {
		mode = "100755"
	}
	header := fmt.Sprintf("diff --git %q %q\n", "a/"+filePath, "b/"+filePath)
	if changed.IsNew {
		data, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil {
			return changed, false, err
		}
		if len(data) > limit || bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			changed.IsBinary = true
			return changed, true, nil
		}
		var patch strings.Builder
		patch.WriteString(header + "new file mode " + mode + "\n")
		if len(data) > 0 {
			text := bytes.TrimSuffix(data, []byte("\n"))
			changed.Insertions = int64(bytes.Count(text, []byte("\n"))) + 1
			fmt.Fprintf(&patch, "--- /dev/null\n+++ %q\n@@ -0,0 +1,%d @@\n", "b/"+filePath, changed.Insertions)
			for line := range bytes.SplitSeq(text, []byte("\n")) {
				patch.WriteByte('+')
				patch.Write(line)
				patch.WriteByte('\n')
			}
			if data[len(data)-1] != '\n' {
				patch.WriteString("\\ No newline at end of file\n")
			}
		}
		changed.Diff = strings.TrimSuffix(patch.String(), "\n")
		return changed, true, nil
	}
	oldEntry, err := runner.Output(ctx, input.RepoDir, "ls-tree", "-z", input.Resolution.ResolvedBase, "--", ":(literal)"+filePath)
	if err != nil {
		return changed, false, err
	}
	oldMode, _, ok := strings.Cut(string(oldEntry), " ")
	if !ok {
		return changed, false, fmt.Errorf("missing base mode for %q", filePath)
	}
	gitDir, err := runner.Output(ctx, input.RepoDir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return changed, false, err
	}
	snapshot, err := os.MkdirTemp("", "ocr-net-diff-*")
	if err != nil {
		return changed, false, err
	}
	defer os.RemoveAll(snapshot)
	target := filepath.Join(snapshot, filepath.FromSlash(filePath))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return changed, false, err
	}
	captured, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return changed, false, err
	}
	written, copyErr := io.Copy(captured, io.LimitReader(file, limit+1))
	closeErr := captured.Close()
	if copyErr != nil || closeErr != nil {
		return changed, false, errors.Join(copyErr, closeErr)
	}
	if written > limit {
		changed.IsBinary = true
		return changed, true, nil
	}
	patch, err := runner.Output(ctx, snapshot, "--git-dir="+strings.TrimSuffix(string(gitDir), "\n"), "--work-tree=.", "-c", "core.quotepath=false", "diff", "--no-textconv", "--no-ext-diff", "--no-color", "--end-of-options", input.Resolution.ResolvedBase+":"+filePath, "--", filePath)
	if err != nil {
		return changed, false, err
	}
	_, rest, _ := strings.Cut(string(patch), "\n")
	var body strings.Builder
	for _, line := range strings.Split(rest, "\n") {
		if line != "" && !strings.HasPrefix(line, "index ") && !strings.HasPrefix(line, "old mode ") && !strings.HasPrefix(line, "new mode ") {
			body.WriteString(line + "\n")
		}
	}
	if oldMode == mode && body.Len() == 0 {
		return changed, false, nil
	}
	if oldMode != mode {
		header += fmt.Sprintf("old mode %s\nnew mode %s\n", oldMode, mode)
	}
	parsed := diff.ParsePatch(header + body.String())
	if len(parsed) != 1 {
		return changed, false, fmt.Errorf("invalid net workspace diff for %q", filePath)
	}
	if parsed[0].IsBinary {
		parsed[0].Diff = ""
	}
	return parsed[0], true, nil
}
