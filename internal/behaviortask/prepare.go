// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
)

func PrepareInput(ctx context.Context, input Input) (Input, error) {
	input.Diffs = nil
	code, err := prepareCodeTools(ctx, input)
	if err != nil {
		return input, err
	}
	runner := code.runner
	baseArgs := []string{"--no-optional-locks", "-c", "core.quotepath=false", "--attr-source=" + input.Resolution.ResolvedHead}
	if input.Resolution.BaseIsEmpty {
		baseArgs = append(baseArgs, "show", "--format=", "--root", "--diff-merges=first-parent")
	} else {
		baseArgs = append(baseArgs, "diff")
	}
	baseArgs = append(baseArgs, "--no-ext-diff", "--no-textconv", "--no-color")
	endpoints := []string{"--end-of-options"}
	if !input.Resolution.BaseIsEmpty {
		endpoints = append(endpoints, input.Resolution.ResolvedBase)
	}
	endpoints = append(endpoints, input.Resolution.ResolvedHead, "--")
	readInventory := func(renameMode string) ([]model.Diff, error) {
		args := append(slices.Clone(baseArgs), "--name-status", "-z", renameMode)
		names, err := runner.Output(ctx, input.RepoDir, append(args, endpoints...)...)
		if err != nil {
			return nil, fmt.Errorf("read analysis change inventory: %w", err)
		}
		return parseInventory(string(names))
	}
	changes, err := readInventory("--find-renames=100%")
	if err != nil {
		return input, err
	}
	if code.allowSimilarityCheck(changes) {
		changes, err = readInventory("--find-renames")
		if err != nil {
			return input, err
		}
	}
	if len(changes) > 0 {
		patterns, err := diff.LoadGitignoreAtRef(ctx, input.RepoDir, input.Resolution.ResolvedHead, runner)
		if err != nil {
			return input, err
		}
		changes = slices.DeleteFunc(changes, func(change model.Diff) bool { return patterns.Matches(change.NewPath) })
	}
	excluded := make([]bool, len(changes))
	var pathGroups [][]string
	var renameGroups [][]string
	for index, changed := range changes {
		if (!changed.IsNew && !code.allowed["old"][changed.OldPath]) || (!changed.IsDeleted && !code.allowed["new"][changed.NewPath]) {
			excluded[index] = true
			continue
		}
		if changed.IsRenamed {
			renameGroups = append(renameGroups, []string{changed.OldPath, changed.NewPath})
		} else {
			pathGroups = append(pathGroups, []string{changed.OldPath})
		}
	}
	batches, err := pathBatches(pathGroups, exactFilePathspec)
	if err != nil {
		return input, err
	}
	byPath := map[string]model.Diff{}
	readPatch := func(batch []string, renamed bool) error {
		patchArgs := append(slices.Clone(baseArgs), "--src-prefix=a/", "--dst-prefix=b/", "--unified=3", "--no-renames")
		if renamed {
			patchArgs = append(patchArgs, "--find-renames", "-l0")
		}
		patchArgs = append(patchArgs, endpoints...)
		patch, err := runner.Output(ctx, input.RepoDir, append(slices.Clone(patchArgs), batch...)...)
		if err != nil {
			return fmt.Errorf("read scoped analysis diff: %w", err)
		}
		parsed := diff.ParsePatch(string(patch))
		for _, changed := range parsed {
			if changed.IsDeleted {
				changed.NewPath = changed.OldPath
			}
			byPath[changed.OldPath+"\x00"+changed.NewPath] = changed
		}
		return nil
	}
	for _, batch := range batches {
		if err := readPatch(batch, false); err != nil {
			return input, err
		}
	}
	for _, paths := range renameGroups {
		batches, err := pathBatches([][]string{paths}, exactFilePathspec)
		if err != nil {
			return input, err
		}
		if err := readPatch(batches[0], true); err != nil {
			return input, err
		}
	}
	for index, changed := range changes {
		if err := ctx.Err(); err != nil {
			return input, err
		}
		if excluded[index] {
			input.Diffs = append(input.Diffs, changed)
			continue
		}
		actual, exists := byPath[changed.OldPath+"\x00"+changed.NewPath]
		if !exists {
			return input, fmt.Errorf("analysis diff does not match inventory for %q", changed.NewPath)
		}
		if actual.IsBinary {
			actual.Diff = ""
		}
		input.Diffs = append(input.Diffs, actual)
	}
	return input, ctx.Err()
}

func (code *codeTools) allowSimilarityCheck(changes []model.Diff) bool {
	var additions, deletions bool
	for _, changed := range changes {
		switch {
		case changed.IsNew:
			if !code.allowed["new"][changed.NewPath] {
				return false
			}
			additions = true
		case changed.IsDeleted:
			if !code.allowed["old"][changed.OldPath] {
				return false
			}
			deletions = true
		}
	}
	return additions && deletions
}

func exactFilePathspec(name string) string {
	_, lastSize := utf8.DecodeLastRuneInString(name)
	var spec strings.Builder
	spec.WriteString(":(top,glob)")
	for index, char := range name {
		if index == len(name)-lastSize || strings.ContainsRune(`\*?[`, char) {
			spec.WriteByte('\\')
		}
		spec.WriteRune(char)
	}
	return spec.String()
}

func parseInventory(raw string) ([]model.Diff, error) {
	entries := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	changes := []model.Diff{}
	if raw == "" {
		return changes, nil
	}
	for index := 0; index < len(entries); {
		status := entries[index]
		index++
		if status == "" || index >= len(entries) {
			return nil, fmt.Errorf("invalid analysis change inventory")
		}
		filePath := entries[index]
		index++
		changed := model.Diff{OldPath: filePath, NewPath: filePath}
		switch status[0] {
		case 'A':
			changed.IsNew = true
		case 'D':
			changed.IsDeleted = true
		case 'R':
			if index >= len(entries) {
				return nil, fmt.Errorf("missing rename destination")
			}
			changed.NewPath = entries[index]
			index++
			changed.IsRenamed = true
		case 'M', 'T':
		default:
			return nil, fmt.Errorf("unsupported analysis change status %q", status)
		}
		changes = append(changes, changed)
	}
	return changes, nil
}
