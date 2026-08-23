// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/alibaba/open-code-review/internal/model"
)

const reviewRunIDBytes = 12 // 96 bits, rendered as 24 hexadecimal characters.

type postingIdentity struct {
	path           string
	startLine      int
	endLine        int
	category       string
	severity       string
	content        string
	suggestionCode string
	existingCode   string
}

// commentPostingIdentity is the single projection used for canonical ordering
// and retry-marker hashing. Keep it synchronized with fields that affect
// GitHub-rendered output. Thinking is intentionally excluded because it is
// never posted.
func commentPostingIdentity(comment model.LlmComment) postingIdentity {
	return postingIdentity{
		path:           comment.Path,
		startLine:      comment.StartLine,
		endLine:        comment.EndLine,
		category:       comment.Category,
		severity:       comment.Severity,
		content:        comment.Content,
		suggestionCode: comment.SuggestionCode,
		existingCode:   comment.ExistingCode,
	}
}

func comparePostingIdentity(left, right postingIdentity) int {
	if result := cmp.Compare(left.path, right.path); result != 0 {
		return result
	}
	if result := cmp.Compare(left.startLine, right.startLine); result != 0 {
		return result
	}
	if result := cmp.Compare(left.endLine, right.endLine); result != 0 {
		return result
	}
	for _, fields := range [][2]string{
		{left.category, right.category},
		{left.severity, right.severity},
		{left.content, right.content},
		{left.suggestionCode, right.suggestionCode},
		{left.existingCode, right.existingCode},
	} {
		if result := cmp.Compare(fields[0], fields[1]); result != 0 {
			return result
		}
	}
	return 0
}

func canonicalComments(comments []model.LlmComment) []model.LlmComment {
	ordered := slices.Clone(comments)
	slices.SortStableFunc(ordered, func(left, right model.LlmComment) int {
		return comparePostingIdentity(commentPostingIdentity(left), commentPostingIdentity(right))
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
		identity := commentPostingIdentity(comment)
		writeField(identity.path)
		writeField(fmt.Sprint(identity.startLine))
		writeField(fmt.Sprint(identity.endLine))
		writeField(identity.category)
		writeField(identity.severity)
		writeField(identity.content)
		writeField(identity.suggestionCode)
		writeField(identity.existingCode)
	}
	return hex.EncodeToString(hash.Sum(nil)[:reviewRunIDBytes])
}

func postingMarker(runID, kind string, batch int) string {
	return fmt.Sprintf("<!-- ocr-review-%s-%s-%d -->", runID, kind, batch)
}
