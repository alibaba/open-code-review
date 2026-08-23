// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/alibaba/open-code-review/internal/model"
)

func canonicalComments(comments []model.LlmComment) []model.LlmComment {
	ordered := slices.Clone(comments)
	slices.SortStableFunc(ordered, func(left, right model.LlmComment) int {
		if left.Path != right.Path {
			if left.Path < right.Path {
				return -1
			}
			return 1
		}
		if left.StartLine != right.StartLine {
			return left.StartLine - right.StartLine
		}
		if left.EndLine != right.EndLine {
			return left.EndLine - right.EndLine
		}
		fields := [][2]string{
			{left.Category, right.Category},
			{left.Severity, right.Severity},
			{left.Content, right.Content},
			{left.SuggestionCode, right.SuggestionCode},
			{left.ExistingCode, right.ExistingCode},
		}
		for _, field := range fields {
			if field[0] < field[1] {
				return -1
			}
			if field[0] > field[1] {
				return 1
			}
		}
		return 0
	})
	return ordered
}

func reviewRunID(owner, name string, prNumber int, baseBranch string, target Target, comments []model.LlmComment) string {
	hash := sha256.New()
	writeField := func(value string) {
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write([]byte(value))
	}
	writeField(owner)
	writeField(name)
	writeField(fmt.Sprint(prNumber))
	writeField(baseBranch)
	writeField(target.ResolvedBase)
	writeField(target.ResolvedHead)
	for _, comment := range canonicalComments(comments) {
		writeField(comment.Path)
		writeField(fmt.Sprint(comment.StartLine))
		writeField(fmt.Sprint(comment.EndLine))
		writeField(comment.Category)
		writeField(comment.Severity)
		writeField(comment.Content)
		writeField(comment.SuggestionCode)
		writeField(comment.ExistingCode)
	}
	return hex.EncodeToString(hash.Sum(nil)[:12])
}

func postingMarker(runID, kind string, batch int) string {
	return fmt.Sprintf("<!-- ocr-review-%s-%s-%d -->", runID, kind, batch)
}
