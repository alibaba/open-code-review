// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchMissingBlobCannotBecomeSuccessfulEvidence(test *testing.T) {
	input := fixture(test)
	writeSearchBoundaryFile(test, input.RepoDir, "a-binary.dat", "\x00diagnostic-needle\n")
	writeSearchBoundaryFile(test, input.RepoDir, "z-missing.go", "diagnostic-needle\n")
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Search diagnostics fixture")
	input.Resolution.ResolvedBase = git(test, input.RepoDir, "rev-parse", "HEAD")
	git(test, input.RepoDir, "commit", "--allow-empty", "-qm", "Search diagnostics comparison")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	input.Diffs = nil
	runner := newTestRunner(test, input, nil, nil)
	missing := git(test, input.RepoDir, "rev-parse", "HEAD:z-missing.go")
	if err := os.Remove(filepath.Join(input.RepoDir, ".git", "objects", missing[:2], missing[2:])); err != nil {
		test.Fatal(err)
	}
	for _, side := range []string{"new", "old"} {
		test.Run(side, func(test *testing.T) {
			outcome, err := runner.dispatch(test.Context(), "code_search", marshal(test, map[string]any{
				"search_text": "diagnostic-needle", "side": side,
				"file_patterns": []any{"a-binary.dat", "z-missing.go"},
			}))
			if err != nil {
				test.Fatal(err)
			}
			var observation Observation
			if err := json.Unmarshal([]byte(outcome.Content), &observation); err != nil {
				test.Fatal(err)
			}
			if observation.ErrorClass != "tool_error" || !strings.HasPrefix(stripSide(observation.ReturnedContent), "Warning:") ||
				!strings.Contains(observation.ReturnedContent, "unable to read") || len(observation.Locations) != 0 {
				test.Fatalf("incomplete search was usable evidence: %+v", observation)
			}
			if err := runner.validateAnalysis(Analysis{Markdown: "Search result [[" + observation.ID + "]]."}); err == nil {
				test.Fatal("accepted a citation to an incomplete search")
			}
		})
	}
}
