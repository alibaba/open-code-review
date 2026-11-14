// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"fmt"
	"regexp"
	"strings"
)

const MaxReportBytes = 256 * 1024

var observationCitation = regexp.MustCompile(`\[\[(obs-[0-9]+)\]\]`)

func citedObservations(markdown string) []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, match := range observationCitation.FindAllStringSubmatch(markdown, -1) {
		id := match[1]
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}

func (runner *Runner) validateAnalysis(analysis Analysis) error {
	if strings.TrimSpace(analysis.Markdown) == "" || len(analysis.Markdown) > MaxReportBytes {
		return fmt.Errorf("markdown must be nonempty and at most %d bytes", MaxReportBytes)
	}
	switch analysis.DeclaredCompletion {
	case "", "unknown", "complete", "partial":
	default:
		return fmt.Errorf("declared_completion must be complete, partial, or unknown")
	}
	if strings.Contains(observationCitation.ReplaceAllString(analysis.Markdown, ""), "[[obs-") {
		return fmt.Errorf("malformed observation citation; use [[obs-000001]] with an actual observation ID")
	}
	for _, id := range citedObservations(analysis.Markdown) {
		observation, exists := runner.store.get(id)
		if !exists || observation.ErrorClass != "" {
			return fmt.Errorf("cited observation %q is missing or failed", id)
		}
	}
	return nil
}
