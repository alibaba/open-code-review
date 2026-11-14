// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
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

func prepareWorkspaceInput(ctx context.Context, input Input, code *codeTools) (Input, error) {
	runner := gitcmd.New(1)
	provider := diff.NewWorkspaceProviderWithBase(input.RepoDir, input.Resolution, runner)
	changes, err := provider.GetDiffSet(ctx)
	if err != nil {
		return input, err
	}
	type workspaceChange struct {
		diff     model.Diff
		excluded bool
	}
	groups := map[string][]workspaceChange{}
	var paths []string
	changes.ForEachInOrder(func(changed model.Diff, providerExcluded bool) {
		if changed.IsDeleted {
			changed.NewPath = changed.OldPath
		}
		if len(groups[changed.NewPath]) == 0 {
			paths = append(paths, changed.NewPath)
		}
		groups[changed.NewPath] = append(groups[changed.NewPath], workspaceChange{changed, providerExcluded})
	})
	input.Diffs = []model.Diff{}
	for _, filePath := range paths {
		group := groups[filePath]
		changed, excluded := group[0].diff, group[0].excluded
		if len(group) > 1 {
			if len(group) != 2 || !(group[0].diff.IsDeleted && group[1].diff.IsNew || group[1].diff.IsDeleted && group[0].diff.IsNew) {
				return input, fmt.Errorf("ambiguous workspace changes for %q", filePath)
			}
			deleted, added := group[0].diff, group[1].diff
			if added.IsDeleted {
				deleted, added = added, deleted
			}
			changed = model.Diff{OldPath: filePath, NewPath: filePath, IsBinary: deleted.IsBinary || added.IsBinary}
			excluded = excluded || group[1].excluded
			if !excluded && !changed.IsBinary && code.allowed["old"][filePath] && code.fileAllowed("new", filePath) {
				var present bool
				changed, present, err = workspaceNetDiff(ctx, input, deleted, runner)
				if err != nil {
					return input, err
				}
				if !present {
					continue
				}
			}
		}
		if excluded || changed.IsBinary || (!changed.IsNew && !code.allowed["old"][changed.OldPath]) || (!changed.IsDeleted && !code.fileAllowed("new", changed.NewPath)) {
			changed.Diff = ""
			changed.NewFileContent = ""
		}
		input.Diffs = append(input.Diffs, changed)
	}
	return input, nil
}

func workspaceNetDiff(ctx context.Context, input Input, deleted model.Diff, runner *gitcmd.Runner) (model.Diff, bool, error) {
	filePath := deleted.OldPath
	info, err := os.Lstat(filepath.Join(input.RepoDir, filePath))
	if err != nil {
		return model.Diff{}, false, err
	}
	if !info.Mode().IsRegular() {
		return model.Diff{}, false, fmt.Errorf("net workspace diff requires a regular file: %q", filePath)
	}
	patch, err := runner.Output(ctx, input.RepoDir, "-c", "core.quotepath=false", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", "--unified=3", "--end-of-options", input.Resolution.ResolvedBase+":"+filePath, "--", filePath)
	if err != nil {
		return model.Diff{}, false, fmt.Errorf("read net workspace diff for %q: %w", filePath, err)
	}
	mode := func(patch, prefix string) string {
		for _, line := range strings.Split(patch, "\n") {
			if value, ok := strings.CutPrefix(line, prefix); ok {
				return value
			}
		}
		return ""
	}
	oldMode, newMode := mode(deleted.Diff, "deleted file mode "), "100644"
	if info.Mode()&0111 != 0 {
		newMode = "100755"
	}
	var body strings.Builder
	_, remainder, _ := strings.Cut(string(patch), "\n")
	for _, line := range strings.Split(remainder, "\n") {
		if line != "" && !strings.HasPrefix(line, "old mode ") && !strings.HasPrefix(line, "new mode ") {
			body.WriteString(line + "\n")
		}
	}
	if body.Len() == 0 && oldMode == newMode {
		return model.Diff{}, false, nil
	}
	header := fmt.Sprintf("diff --git %q %q\n", "a/"+filePath, "b/"+filePath)
	if oldMode != newMode && oldMode != "" && newMode != "" {
		header += fmt.Sprintf("old mode %s\nnew mode %s\n", oldMode, newMode)
	}
	parsed, err := diff.ParseDiffText(ctx, header+body.String(), input.RepoDir, "", runner)
	if err != nil {
		return model.Diff{}, false, err
	}
	if len(parsed) != 1 {
		return model.Diff{}, false, fmt.Errorf("invalid net workspace diff for %q", filePath)
	}
	return parsed[0], true, nil
}
