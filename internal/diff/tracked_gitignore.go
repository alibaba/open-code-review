// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

// filterGitignoredDiffs preserves the historical repository .gitignore
// filtering for parsed diffs while leaving OCR's built-in noisy-directory
// prefixes to the agent selection layer. That separation lets preview account
// for tracked vendor/node_modules/etc. changes without re-admitting tracked
// files that the repository explicitly ignores.
func (p *Provider) filterGitignoredDiffs(diffs []model.Diff) []model.Diff {
	patterns := p.loadGitignorePatterns()
	if len(patterns) == 0 {
		return diffs
	}

	result := make([]model.Diff, 0, len(diffs))
	for _, d := range diffs {
		path := d.NewPath
		if path == "/dev/null" {
			path = d.OldPath
		}
		if !isGitignoreExcluded(path, patterns) {
			result = append(result, d)
		}
	}
	return result
}

// isGitignoreExcluded resolves repository .gitignore patterns in file order,
// with the last matching pattern winning. Built-in noisy-directory prefixes are
// intentionally not considered here; callers that need those should use
// isPathExcluded/IsDefaultExcludedDirPath.
func isGitignoreExcluded(relPath string, gitignorePatterns []string) bool {
	excluded := false
	for _, pat := range gitignorePatterns {
		body, negated := strings.CutPrefix(pat, "!")
		if body == "" {
			continue
		}

		// Git uses a negated directory-only pattern such as !*/ to keep walking
		// into directories, not to re-admit every file below them.
		if negated && strings.HasSuffix(body, "/") {
			continue
		}

		if matchGitignoreBody(relPath, body) {
			excluded = !negated
		}
	}
	return excluded
}
