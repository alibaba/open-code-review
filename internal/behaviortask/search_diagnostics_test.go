// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/tool"
)

func TestSearchMissingBlobPreservesOnlyObservedMatches(test *testing.T) {
	input := fixture(test)
	writeSearchBoundaryFile(test, input.RepoDir, "a-binary.dat", "\x00diagnostic-needle\n")
	writeSearchBoundaryFile(test, input.RepoDir, "a-text.go", "diagnostic-needle\n")
	writeSearchBoundaryFile(test, input.RepoDir, "z-missing.go", "diagnostic-needle\nmissing file only\n")
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
		for _, mixed := range []bool{false, true} {
			name := side + "/binary"
			patterns := []any{"a-binary.dat", "z-missing.go"}
			if mixed {
				name = side + "/text-and-binary"
				patterns = append(patterns, "a-text.go")
			}
			test.Run(name, func(test *testing.T) {
				outcome, err := runner.dispatch(test.Context(), "code_search", marshal(test, map[string]any{
					"search_text": "diagnostic-needle", "side": side,
					"file_patterns": patterns,
				}))
				if err != nil {
					test.Fatal(err)
				}
				var observation Observation
				if err := json.Unmarshal([]byte(outcome.Content), &observation); err != nil {
					test.Fatal(err)
				}
				if !observation.Truncated || !strings.Contains(observation.ReturnedContent, tool.CodeSearchIncompletePrefix) ||
					!strings.Contains(observation.ReturnedContent, "unable to read") || !strings.Contains(observation.Continuation, "do not infer absence") {
					test.Fatalf("missing incomplete coverage: %+v", observation)
				}
				citationErr := runner.validateAnalysis(Analysis{Markdown: "Search result [[" + observation.ID + "]]."})
				if mixed {
					if observation.ErrorClass != "" || citationErr != nil || len(observation.Locations) != 1 ||
						observation.Locations[0].Path != "a-text.go" || observation.Locations[0].StartLine != 1 ||
						observation.Locations[0].EndLine != 1 {
						test.Fatalf("lost valid match evidence: %+v, citation error: %v", observation, citationErr)
					}
				} else if observation.ErrorClass != "tool_error" || len(observation.Locations) != 0 || citationErr == nil {
					test.Fatalf("search without text matches became usable evidence: %+v", observation)
				}
			})
		}
	}
}

func TestSearchBenignWarningKeepsCompleteMatchEvidence(test *testing.T) {
	input := fixture(test)
	runner := newTestRunner(test, input, nil, nil)
	registry := tool.NewRegistry()
	registry.Register(searchBoundaryProvider(func(context.Context, map[string]any) (string, error) {
		return "File: main.go\nMatch lines: 1\n1|new value\n\nWarning: warning: no threads support, ignoring grep.threads in builtin/grep.c\n", nil
	}))
	registry.Freeze()
	runner.code.registry = registry
	outcome, err := runner.dispatch(test.Context(), "code_search", marshal(test, map[string]any{
		"search_text": "new value", "side": "new", "file_patterns": []any{"main.go"},
	}))
	if err != nil {
		test.Fatal(err)
	}
	var observation Observation
	if err := json.Unmarshal([]byte(outcome.Content), &observation); err != nil {
		test.Fatal(err)
	}
	if observation.ErrorClass != "" || observation.Truncated || len(observation.Locations) != 1 ||
		observation.Locations[0].Path != "main.go" || !strings.Contains(observation.ReturnedContent, "no threads support") {
		test.Fatalf("benign warning invalidated match evidence: %+v", observation)
	}
	if err := runner.validateAnalysis(Analysis{Markdown: "Search result [[" + observation.ID + "]]."}); err != nil {
		test.Fatal(err)
	}
}
