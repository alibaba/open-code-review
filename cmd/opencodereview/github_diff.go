// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/github"
)

var githubHunkHeaderPattern = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

type githubLineRange struct {
	start int
	end   int
}

type githubDiffInventory map[string][]githubLineRange

func buildGitHubDiffInventory(files []github.ChangedFile) githubDiffInventory {
	inventory := make(githubDiffInventory)
	for _, file := range files {
		ranges, complete := parseGitHubRightSideRanges(file.Patch)
		if complete {
			inventory[file.Filename] = ranges
		}
	}
	return inventory
}

func (inventory githubDiffInventory) contains(path string, startLine, endLine int) bool {
	if startLine <= 0 || endLine < startLine {
		return false
	}
	for _, lineRange := range inventory[path] {
		if startLine >= lineRange.start && endLine <= lineRange.end {
			return true
		}
	}
	return false
}

func parseGitHubRightSideRanges(patch string) ([]githubLineRange, bool) {
	if patch == "" {
		return nil, false
	}
	type hunkState struct {
		start    int
		end      int
		expected int
		observed int
	}

	var ranges []githubLineRange
	var current *hunkState
	complete := true
	sawHunk := false
	flush := func() {
		if current == nil {
			return
		}
		if current.observed != current.expected {
			complete = false
		}
		if current.end >= current.start {
			ranges = append(ranges, githubLineRange{start: current.start, end: current.end})
		}
	}

	for _, line := range strings.Split(patch, "\n") {
		if match := githubHunkHeaderPattern.FindStringSubmatch(line); match != nil {
			flush()
			start, _ := strconv.Atoi(match[1])
			expected := 1
			if match[2] != "" {
				expected, _ = strconv.Atoi(match[2])
			}
			current = &hunkState{start: start, end: start - 1, expected: expected}
			sawHunk = true
			continue
		}
		if current == nil || line == "" || strings.HasPrefix(line, "\\") || strings.HasPrefix(line, "-") {
			continue
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ") {
			current.end = current.start + current.observed
			current.observed++
		}
	}
	flush()
	return ranges, sawHunk && complete
}
