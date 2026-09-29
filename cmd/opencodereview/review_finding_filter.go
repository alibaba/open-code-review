// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

var reviewSeverities = []string{"low", "medium", "high", "critical"}
var reviewCategories = []string{"bug", "security", "performance", "maintainability", "test", "style", "documentation", "other"}

func validateReviewFindingFilters(opts *reviewOptions) error {
	opts.minSeverity = strings.ToLower(strings.TrimSpace(opts.minSeverity))
	if opts.minSeverity != "" && !slices.Contains(reviewSeverities, opts.minSeverity) {
		return fmt.Errorf("invalid --min-severity value %q: must be critical, high, medium, or low", opts.minSeverity)
	}
	categories := splitPaths(strings.ToLower(opts.excludeCategories))
	for _, category := range categories {
		if !slices.Contains(reviewCategories, category) {
			return fmt.Errorf("invalid --exclude-categories value %q: must be one of %s", category, strings.Join(reviewCategories, ", "))
		}
	}
	opts.excludeCategories = strings.Join(categories, ",")
	return nil
}

// filterReviewComments only filters the reported findings. Session checkpoints
// retain the original comments so resumed reviews can use a different policy.
func filterReviewComments(comments []model.LlmComment, opts reviewOptions) []model.LlmComment {
	if opts.minSeverity == "" && opts.excludeCategories == "" {
		return comments
	}
	minRank := slices.Index(reviewSeverities, opts.minSeverity)
	excluded := parseFilterSet(opts.excludeCategories)
	filtered := make([]model.LlmComment, 0, len(comments))
	for _, comment := range comments {
		severity := strings.ToLower(strings.TrimSpace(comment.Severity))
		category := strings.ToLower(strings.TrimSpace(comment.Category))
		rank := slices.Index(reviewSeverities, severity)
		// Fail open for either unknown field, even if the other field matches
		// an exclusion. Older models and sessions may omit this metadata.
		if rank < 0 || !slices.Contains(reviewCategories, category) {
			filtered = append(filtered, comment)
			continue
		}
		if rank < minRank || excluded[category] {
			continue
		}
		filtered = append(filtered, comment)
	}
	return filtered
}
