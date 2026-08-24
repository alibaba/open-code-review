// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/github"
	"github.com/alibaba/open-code-review/internal/model"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type lineRange struct {
	start int
	end   int
}

type diffInventory struct {
	left  map[string][]lineRange
	right map[string][]lineRange
}

type locationClass uint8

const (
	locationInvalid locationClass = iota
	locationUnverified
	locationLeftOnly
	locationRightOnly
	locationSideUnknown
)

func buildDiffInventory(files []github.ChangedFile) diffInventory {
	inventory := diffInventory{
		left:  make(map[string][]lineRange),
		right: make(map[string][]lineRange),
	}
	for _, file := range files {
		left, right, complete := parseDiffRanges(file.Patch)
		if !complete {
			continue
		}
		inventory.left[file.Filename] = left
		inventory.right[file.Filename] = right
	}
	return inventory
}

func parseDiffRanges(patch string) ([]lineRange, []lineRange, bool) {
	if patch == "" {
		return nil, nil, false
	}
	type hunk struct {
		oldStart, oldExpected, oldObserved int
		newStart, newExpected, newObserved int
	}
	var left, right []lineRange
	var current *hunk
	sawHunk := false
	complete := true
	flush := func() {
		if current == nil {
			return
		}
		if current.oldObserved != current.oldExpected || current.newObserved != current.newExpected {
			complete = false
		}
		if current.oldExpected > 0 {
			left = append(left, lineRange{start: current.oldStart, end: current.oldStart + current.oldExpected - 1})
		}
		if current.newExpected > 0 {
			right = append(right, lineRange{start: current.newStart, end: current.newStart + current.newExpected - 1})
		}
	}

	for _, line := range strings.Split(patch, "\n") {
		if match := hunkHeaderPattern.FindStringSubmatch(line); match != nil {
			flush()
			current = &hunk{
				oldStart:    parseHunkNumber(match[1]),
				oldExpected: parseHunkCount(match[2]),
				newStart:    parseHunkNumber(match[3]),
				newExpected: parseHunkCount(match[4]),
			}
			sawHunk = true
			continue
		}
		if current == nil {
			if line != "" {
				complete = false
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "\\") {
			continue
		}
		switch line[0] {
		case ' ':
			current.oldObserved++
			current.newObserved++
		case '-':
			current.oldObserved++
		case '+':
			current.newObserved++
		default:
			complete = false
		}
	}
	flush()
	if !sawHunk || !complete {
		return nil, nil, false
	}
	return left, right, true
}

func parseHunkNumber(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

func parseHunkCount(raw string) int {
	if raw == "" {
		return 1
	}
	return parseHunkNumber(raw)
}

func classifyLocation(finding Finding, inventory diffInventory) locationClass {
	comment := finding.Comment
	start, end, ok := commentLocation(comment)
	if !ok {
		return locationInvalid
	}
	switch finding.Side {
	case SideNew:
		if containsRange(inventory.right[comment.Path], start, end) {
			return locationRightOnly
		}
		return locationUnverified
	case SideOld:
		return locationLeftOnly
	default:
		return locationSideUnknown
	}
}

func commentLocation(comment model.LlmComment) (int, int, bool) {
	end := comment.EndLine
	if end <= 0 {
		end = comment.StartLine
	}
	if end <= 0 {
		return 0, 0, false
	}
	start := end
	if comment.StartLine > 0 {
		start = comment.StartLine
	}
	if start > end {
		return 0, 0, false
	}
	return start, end, true
}

func containsRange(ranges []lineRange, start, end int) bool {
	for _, candidate := range ranges {
		if start >= candidate.start && end <= candidate.end {
			return true
		}
	}
	return false
}
