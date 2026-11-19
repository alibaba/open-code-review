// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
	workspace := input.Mode == "workspace"
	baseArgs := []string{"--no-optional-locks", "-c", "core.quotepath=false"}
	if input.Resolution.BaseIsEmpty && !workspace {
		baseArgs = append(baseArgs, "show", "--format=", "--root", "--diff-merges=first-parent")
	} else {
		baseArgs = append(baseArgs, "diff")
	}
	baseArgs = append(baseArgs, "--no-ext-diff", "--no-textconv", "--no-color")
	endpoints := []string{"--end-of-options"}
	if !input.Resolution.BaseIsEmpty {
		endpoints = append(endpoints, input.Resolution.ResolvedBase)
	}
	if !workspace {
		endpoints = append(endpoints, input.Resolution.ResolvedHead)
	} else if input.Resolution.BaseIsEmpty {
		empty, err := runner.Output(ctx, input.RepoDir, "hash-object", "-t", "tree", "--stdin")
		if err != nil {
			return input, err
		}
		endpoints = append(endpoints, strings.TrimSpace(string(empty)))
	}
	endpoints = append(endpoints, "--")
	readInventory := func(renameMode string) ([]model.Diff, error) {
		args := append(slices.Clone(baseArgs), "--name-status", "-z", renameMode)
		names, err := runner.Output(ctx, input.RepoDir, append(args, endpoints...)...)
		if err != nil {
			return nil, fmt.Errorf("read analysis change inventory: %w", err)
		}
		return parseInventory(string(names))
	}
	renameMode := "--find-renames=100%"
	if workspace {
		renameMode = "--no-renames"
	}
	changes, err := readInventory(renameMode)
	if err != nil {
		return input, err
	}
	var untracked map[string]bool
	if workspace {
		changes, untracked, err = workspaceInventory(ctx, input, code, changes, endpoints)
	} else if code.allowSimilarityCheck(changes) {
		changes, err = readInventory("--find-renames")
	}
	if err != nil {
		return input, err
	}
	excluded := make([]bool, len(changes))
	var pathGroups [][]string
	var renameGroups [][]string
	for index, changed := range changes {
		if (!changed.IsNew && !code.allowed["old"][changed.OldPath]) || (!changed.IsDeleted && !code.allowed["new"][changed.NewPath]) {
			excluded[index] = true
			continue
		}
		if untracked[changed.NewPath] {
			continue
		}
		if changed.IsRenamed {
			renameGroups = append(renameGroups, []string{changed.OldPath, changed.NewPath})
		} else {
			pathGroups = append(pathGroups, []string{changed.OldPath})
		}
	}
	batches, err := literalPathBatches(pathGroups)
	if err != nil {
		return input, err
	}
	byPath := map[string]model.Diff{}
	descendants := changeChildren(changes)
	readPatch := func(batch []string, renamed bool) error {
		patchArgs := append(slices.Clone(baseArgs), "--src-prefix=a/", "--dst-prefix=b/", "--unified=3", "--no-renames")
		if renamed {
			patchArgs = append(patchArgs, "--find-renames", "-l0")
		}
		patchArgs = append(patchArgs, endpoints...)
		batch = exactPatchPaths(batch, descendants)
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
		batches, err := literalPathBatches([][]string{paths})
		if err != nil {
			return input, err
		}
		if err := readPatch(batches[0], true); err != nil {
			return input, err
		}
	}
	input.Diffs = nil
	for index, changed := range changes {
		if excluded[index] {
			input.Diffs = append(input.Diffs, changed)
			continue
		}
		if untracked[changed.NewPath] {
			actual, present, err := workspaceUntrackedPatch(ctx, input, changed, runner)
			if err != nil {
				return input, err
			}
			if present {
				input.Diffs = append(input.Diffs, actual)
			}
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
	return input, nil
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

func changeChildren(changes []model.Diff) map[string]map[string]bool {
	children := map[string]map[string]bool{}
	for _, change := range changes {
		for _, filePath := range []string{change.OldPath, change.NewPath} {
			for {
				index := strings.LastIndexByte(filePath, '/')
				if index < 0 {
					break
				}
				parent := filePath[:index]
				if children[parent] == nil {
					children[parent] = map[string]bool{}
				}
				if children[parent][filePath] {
					break
				}
				children[parent][filePath] = true
				filePath = parent
			}
		}
	}
	return children
}

func exactPatchPaths(batch []string, children map[string]map[string]bool) []string {
	selected := map[string]bool{}
	ancestors := map[string]bool{}
	for _, spec := range batch {
		filePath := strings.TrimPrefix(spec, ":(top,literal)")
		selected[filePath] = true
		for {
			index := strings.LastIndexByte(filePath, '/')
			if index < 0 {
				break
			}
			filePath = filePath[:index]
			ancestors[filePath] = true
		}
	}
	var restrict func(string)
	restrict = func(filePath string) {
		if len(children[filePath]) == 0 {
			return
		}
		if !ancestors[filePath] {
			batch = append(batch, ":(top,exclude,literal)"+filePath+"/")
			return
		}
		for child := range children[filePath] {
			if !selected[child] && !ancestors[child] {
				batch = append(batch, ":(top,exclude,literal)"+child)
			} else if !selected[child] {
				restrict(child)
			}
		}
	}
	for filePath := range selected {
		restrict(filePath)
	}
	return batch
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
