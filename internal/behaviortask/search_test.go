// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/tool"
)

type searchBoundaryProvider func(context.Context, map[string]any) (string, error)

func (provider searchBoundaryProvider) Tool() tool.Tool { return tool.CodeSearch }

func (provider searchBoundaryProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return provider(ctx, args)
}

func TestSearchProviderBoundaryFiltering(test *testing.T) {
	input := fixture(test)
	input.Diffs = nil
	for _, mode := range []string{"range", "commit", "workspace"} {
		for _, side := range []string{"old", "new"} {
			test.Run(mode+"/"+side, func(test *testing.T) {
				current := input
				current.Mode = mode
				if mode == "workspace" {
					current.Resolution.ResolvedHead = ""
				}
				code, err := prepareCodeTools(test.Context(), current)
				if err != nil {
					test.Fatal(err)
				}
				otherSidePath := "renamed.go"
				if side == "new" {
					otherSidePath = "consumer.go"
				}
				calls := 0
				provider := searchBoundaryProvider(func(ctx context.Context, args map[string]any) (string, error) {
					calls++
					if ctx != test.Context() || args["search_text"] != "needle" || args["side"] != side || args["case_sensitive"] != true {
						test.Fatalf("provider request changed: %+v", args)
					}
					patterns, ok := args["file_patterns"].([]any)
					if !ok || len(patterns) == 0 {
						test.Fatalf("missing literal search scope: %+v", args)
					}
					for _, raw := range patterns {
						pattern, ok := raw.(string)
						if !ok || !strings.HasPrefix(pattern, ":(top,literal)") || !code.fileAllowed(side, strings.TrimPrefix(pattern, ":(top,literal)")) {
							test.Fatalf("unauthorized provider pattern: %v", raw)
						}
					}
					content := strings.Join([]string{
						"1|ORPHAN_PRIVATE_SENTINEL",
						"Warning: simulated search diagnostic",
						"File: excluded.go", "Match lines: 1", "2|EXCLUDED_PRIVATE_SENTINEL",
						"File: main.go", "Match lines: 2", "7|AUTHORIZED_FIRST", "8|AUTHORIZED_SECOND",
						"File: " + otherSidePath, "Match lines: 1", "3|OTHER_SIDE_PRIVATE_SENTINEL",
						"File: missing.go", "Match lines: 1", "4|MISSING_PRIVATE_SENTINEL",
						"File: .env", "Match lines: 1", "5|SECRET_PRIVATE_SENTINEL",
						"UNNUMBERED_PRIVATE_SENTINEL",
						"Error: simulated search failure",
						"File: main.go", "Match lines: 1", "9|AUTHORIZED_LAST",
					}, "\n")
					if side == "old" {
						content = "SIDE: old\nREVISION: " + code.comparison.Old + "\n" + content
					}
					return content, nil
				})
				args := map[string]any{"search_text": "needle", "side": side, "case_sensitive": true, "file_patterns": []any{"*.go"}}
				content, err := code.search(test.Context(), provider, args, side)
				if err != nil {
					test.Fatal(err)
				}
				want := "Warning: simulated search diagnostic\nFile: main.go\nMatch lines: 2\n7|AUTHORIZED_FIRST\n8|AUTHORIZED_SECOND\n\n" +
					"Error: simulated search failure\nFile: main.go\nMatch lines: 1\n9|AUTHORIZED_LAST\n\n"
				if side == "old" {
					want = "SIDE: old\nREVISION: " + code.comparison.Old + "\n" + want
				}
				if content != want || calls != 1 {
					test.Fatalf("filtered result: calls=%d\ngot: %q\nwant: %q", calls, content, want)
				}
				if !slices.Equal(args["file_patterns"].([]any), []any{"*.go"}) {
					test.Fatalf("caller patterns mutated: %+v", args)
				}
			})
		}
	}
}

func TestSearchProviderBoundaryOutcomes(test *testing.T) {
	input := fixture(test)
	code, err := prepareCodeTools(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	providerErr := errors.New("provider unavailable")
	for _, scenario := range []struct {
		name          string
		side          string
		searchText    any
		patterns      []any
		baseIsEmpty   bool
		providerBody  string
		providerError error
		wantBody      string
		wantError     string
		wantCalls     int
	}{
		{name: "provider_error", side: "new", searchText: "needle", providerBody: "File: main.go\n1|discarded partial output", providerError: providerErr, wantError: providerErr.Error(), wantCalls: 1},
		{name: "empty_search", side: "new", searchText: "", wantBody: "Error: search_text is blank"},
		{name: "whitespace_search", side: "old", searchText: " \t\n", wantBody: "Error: search_text is blank"},
		{name: "missing_search", side: "new", wantBody: "Error: search_text is blank"},
		{name: "non_string_search", side: "new", searchText: 1, wantBody: "Error: search_text is blank"},
		{name: "invalid_side", side: "base", searchText: "needle", wantError: "side must be new or old"},
		{name: "invalid_side_before_blank_search", side: "", wantError: "side must be new or old"},
		{name: "no_matches", side: "new", searchText: "needle", providerBody: "No matches found", wantBody: "No matches found", wantCalls: 1},
		{name: "only_unauthorized_matches", side: "old", searchText: "needle", providerBody: "File: excluded.go\nMatch lines: 1\n1|PRIVATE_SENTINEL\n", wantBody: "No matches found", wantCalls: 1},
		{name: "empty_patterns_scope", side: "new", searchText: "needle", patterns: []any{"missing/"}, wantBody: "No matches found"},
		{name: "empty_old_tree", side: "old", searchText: "needle", baseIsEmpty: true, wantBody: "No matches found"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			current := *code
			current.comparison.BaseIsEmpty = scenario.baseIsEmpty
			if scenario.baseIsEmpty {
				current.comparison.Old = ""
			}
			calls := 0
			provider := searchBoundaryProvider(func(context.Context, map[string]any) (string, error) {
				calls++
				return scenario.providerBody, scenario.providerError
			})
			args := map[string]any{"search_text": scenario.searchText, "file_patterns": scenario.patterns, "side": scenario.side}
			content, err := current.search(test.Context(), provider, args, scenario.side)
			if scenario.wantError != "" {
				if err == nil || err.Error() != scenario.wantError || content != "" {
					test.Fatalf("error result: content=%q error=%v", content, err)
				}
				if scenario.providerError != nil && !errors.Is(err, scenario.providerError) {
					test.Fatalf("provider error identity lost: %v", err)
				}
			} else {
				want := scenario.wantBody
				if scenario.side == "old" && want == "No matches found" {
					if scenario.baseIsEmpty {
						want = "SIDE: old\nSOURCE: empty-tree\n" + want
					} else {
						want = "SIDE: old\nREVISION: " + current.comparison.Old + "\n" + want
					}
				}
				if err != nil || content != want {
					test.Fatalf("search result: got=%q want=%q error=%v", content, want, err)
				}
			}
			if calls != scenario.wantCalls {
				test.Fatalf("provider calls: got=%d want=%d", calls, scenario.wantCalls)
			}
		})
	}
}

func writeSearchBoundaryFile(test *testing.T, repository, name, content string) {
	test.Helper()
	filePath := filepath.Join(repository, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		test.Fatal(err)
	}
}

func assertSearchBoundaryPaths(test *testing.T, code *codeTools, side string, patterns []any, want []string) {
	test.Helper()
	batches, err := code.searchPaths(test.Context(), map[string]any{"file_patterns": patterns}, side)
	if err != nil {
		test.Fatal(err)
	}
	var paths []string
	for _, batch := range batches {
		for _, pattern := range batch {
			if !strings.HasPrefix(pattern, ":(top,literal)") {
				test.Fatalf("nonliteral provider pathspec: %q", pattern)
			}
			paths = append(paths, strings.TrimPrefix(pattern, ":(top,literal)"))
		}
	}
	if !slices.Equal(paths, want) {
		test.Fatalf("patterns=%v side=%s: got=%q want=%q", patterns, side, paths, want)
	}
}

func TestSearchPathsBoundaryIndexConflicts(test *testing.T) {
	input := fixture(test)
	for _, name := range []string{"file-to-directory", "directory-to-file/child.go", "removed.go"} {
		writeSearchBoundaryFile(test, input.RepoDir, name, "historical needle\n")
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Historical search paths")
	input.Resolution.ResolvedBase = git(test, input.RepoDir, "rev-parse", "HEAD")
	git(test, input.RepoDir, "commit", "--allow-empty", "-qm", "Search comparison head")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	input.Diffs = nil
	git(test, input.RepoDir, "rm", "-q", "file-to-directory", "directory-to-file/child.go", "removed.go")
	writeSearchBoundaryFile(test, input.RepoDir, "file-to-directory/current.go", "current needle\n")
	writeSearchBoundaryFile(test, input.RepoDir, "directory-to-file", "current needle\n")
	writeSearchBoundaryFile(test, input.RepoDir, "index-only.go", "current needle\n")
	git(test, input.RepoDir, "add", ".")
	for _, mode := range []string{"range", "commit", "workspace"} {
		for _, side := range []string{"old", "new"} {
			test.Run(mode+"/"+side, func(test *testing.T) {
				current := input
				current.Mode = mode
				if mode == "workspace" {
					current.Resolution.ResolvedHead = ""
				}
				code, err := prepareCodeTools(test.Context(), current)
				if err != nil {
					test.Fatal(err)
				}
				live := mode == "workspace" && side == "new"
				for _, name := range []string{"file-to-directory", "directory-to-file/child.go", "removed.go"} {
					want := []string{name}
					if live {
						want = nil
						if name == "file-to-directory" {
							want = []string{"file-to-directory/current.go"}
						}
					}
					assertSearchBoundaryPaths(test, code, side, []any{name}, want)
				}
				wantDirectory := []string{"directory-to-file/child.go"}
				if live {
					wantDirectory = nil
				}
				assertSearchBoundaryPaths(test, code, side, []any{"directory-to-file/"}, wantDirectory)
				wantAll := []string{"directory-to-file/child.go", "file-to-directory", "main.go", "removed.go", "renamed.go"}
				if live {
					wantAll = []string{"directory-to-file", "file-to-directory/current.go", "index-only.go", "main.go", "renamed.go"}
				}
				assertSearchBoundaryPaths(test, code, side, nil, wantAll)
			})
		}
	}
}

func TestSearchPathsBoundaryWindowsAndLiteralPatterns(test *testing.T) {
	input := fixture(test)
	literalPath := "source/nested/unit[case].go"
	for name, content := range map[string]string{
		"source/nested/main.go":  "needle MAIN_SENTINEL\n",
		literalPath:              "needle LITERAL_SENTINEL\n",
		"source/nested/unitc.go": "needle DECOY_SENTINEL\n",
	} {
		writeSearchBoundaryFile(test, input.RepoDir, name, content)
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Literal search filenames")
	input.Resolution.ResolvedBase = git(test, input.RepoDir, "rev-parse", "HEAD")
	git(test, input.RepoDir, "commit", "--allow-empty", "-qm", "Literal search comparison head")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	input.Diffs = nil
	for _, mode := range []string{"range", "commit", "workspace"} {
		for _, side := range []string{"old", "new"} {
			test.Run(mode+"/"+side, func(test *testing.T) {
				current := input
				current.Mode = mode
				if mode == "workspace" {
					current.Resolution.ResolvedHead = ""
				}
				code, err := prepareCodeTools(test.Context(), current)
				if err != nil {
					test.Fatal(err)
				}
				assertSearchBoundaryPaths(test, code, side, []any{`source\nested\main.go`}, []string{"source/nested/main.go"})
				assertSearchBoundaryPaths(test, code, side, []any{`source\nested\`}, []string{"source/nested/main.go", literalPath, "source/nested/unitc.go"})
				for _, pattern := range []string{":(top,literal)" + literalPath, `:(top,literal)source\nested\unit[case].go`} {
					assertSearchBoundaryPaths(test, code, side, []any{pattern}, []string{literalPath})
					observation, err := code.execute(test.Context(), "code_search", map[string]any{"search_text": "needle", "side": side, "file_patterns": []any{pattern}})
					if err != nil || len(observation.Locations) != 1 || observation.Locations[0].Path != literalPath || !strings.Contains(observation.ReturnedContent, "LITERAL_SENTINEL") || strings.Contains(observation.ReturnedContent, "DECOY_SENTINEL") {
						test.Fatalf("literal filename search broadened: %+v error=%v", observation, err)
					}
				}
			})
		}
	}
}
