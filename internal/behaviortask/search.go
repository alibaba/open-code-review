// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/tool"
)

const scopedPathBytes = 16 * 1024
const searchMatchLimit = 100

func literalPathBatches(groups [][]string) ([][]string, error) {
	var batches [][]string
	var batch []string
	batchBytes := 0
	for _, group := range groups {
		var patterns []string
		groupBytes := 0
		for _, filePath := range group {
			pattern := ":(top,literal)" + filePath
			patterns = append(patterns, pattern)
			groupBytes += len(pattern) + 1
		}
		if groupBytes > scopedPathBytes {
			return nil, fmt.Errorf("analysis path group exceeds the safe Git argument limit")
		}
		if batchBytes+groupBytes > scopedPathBytes {
			batches = append(batches, batch)
			batch, batchBytes = nil, 0
		}
		batch = append(batch, patterns...)
		batchBytes += groupBytes
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches, nil
}

func (code *codeTools) searchPaths(ctx context.Context, args map[string]any, side string) ([][]string, error) {
	command := []string{"ls-files", "-z", "--cached"}
	if code.comparison.Mode == "workspace" && side == "new" {
		command = append(command, "--others", "--exclude-standard")
	} else {
		revision := code.comparison.Head
		if side == "old" {
			if code.comparison.BaseIsEmpty {
				return nil, nil
			}
			revision = code.comparison.Old
		}
		command = append(command, "--with-tree="+revision)
	}
	command = append(command, "--")
	patterns, _ := args["file_patterns"].([]any)
	patternBytes := 0
	for _, raw := range patterns {
		pattern, _ := raw.(string)
		pattern = strings.ReplaceAll(pattern, "\\", "/")
		for _, component := range strings.Split(pattern, "/") {
			if component == ".." {
				return nil, fmt.Errorf("file_patterns must not contain ..")
			}
		}
		if pattern != "" {
			patternBytes += len(pattern) + 1
			command = append(command, pattern)
		}
	}
	if patternBytes > scopedPathBytes {
		return nil, fmt.Errorf("file_patterns exceed the safe Git argument limit; narrow the search")
	}
	data, err := gitcmd.New(1).Output(ctx, code.comparison.Repository, command...)
	if err != nil {
		return nil, fmt.Errorf("list search scope: %w", err)
	}
	seen := map[string]bool{}
	var paths []string
	for _, filePath := range strings.Split(string(data), "\x00") {
		if !seen[filePath] && code.fileAllowed(side, filePath) {
			seen[filePath] = true
			paths = append(paths, filePath)
		}
	}
	sort.Strings(paths)
	groups := make([][]string, 0, len(paths))
	for _, filePath := range paths {
		groups = append(groups, []string{filePath})
	}
	return literalPathBatches(groups)
}

func (code *codeTools) search(ctx context.Context, provider tool.Provider, args map[string]any, side string) (string, error) {
	if side != "old" && side != "new" {
		return "", fmt.Errorf("side must be new or old")
	}
	searchText, _ := args["search_text"].(string)
	if strings.TrimSpace(searchText) == "" {
		return "Error: search_text is blank", nil
	}
	batches, err := code.searchPaths(ctx, args, side)
	if err != nil {
		return "", err
	}
	var body strings.Builder
	count := 0
	truncated := false
	for _, batch := range batches {
		providerArgs := maps.Clone(args)
		patterns := make([]any, len(batch))
		for index, pattern := range batch {
			patterns[index] = pattern
		}
		providerArgs["file_patterns"] = patterns
		content, err := provider.Execute(ctx, providerArgs)
		if err != nil {
			return "", err
		}
		filePath := ""
		var matches []string
		flush := func() {
			if len(matches) > 0 {
				fmt.Fprintf(&body, "File: %s\nMatch lines: %d\n%s\n\n", filePath, len(matches), strings.Join(matches, "\n"))
				matches = nil
			}
		}
		for _, line := range strings.Split(stripSide(content), "\n") {
			switch {
			case strings.HasPrefix(line, "File: "):
				flush()
				filePath = strings.TrimPrefix(line, "File: ")
			case numberedLine.MatchString(line) && code.fileAllowed(side, filePath):
				if count < searchMatchLimit {
					matches = append(matches, line)
					count++
				} else {
					truncated = true
				}
			case strings.HasPrefix(line, "Note: Showing the first "):
				truncated = true
			case strings.HasPrefix(line, "Error:") || strings.HasPrefix(line, "Warning:"):
				flush()
				fmt.Fprintln(&body, line)
			}
		}
		flush()
		if truncated {
			break
		}
	}
	result := body.String()
	if result == "" {
		result = "No matches found"
	}
	if truncated {
		result = fmt.Sprintf("Note: Showing the first %d matches. Narrow file_patterns to see the rest.\n", searchMatchLimit) + result
	}
	if side == "old" {
		prefix := "SIDE: old\nREVISION: " + code.comparison.Old + "\n"
		if code.comparison.BaseIsEmpty {
			prefix = "SIDE: old\nSOURCE: empty-tree\n"
		}
		result = prefix + result
	}
	return result, nil
}
