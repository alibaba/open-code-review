// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSearchDeniedPathsNeverReturned(test *testing.T) {
	input := fixture(test)
	for _, name := range []string{`private\credentials`, "main.go:1:private", "private\ncredentials"} {
		if err := os.WriteFile(filepath.Join(input.RepoDir, name), []byte("scope needle PRIVATE_SENTINEL\n"), 0600); err != nil {
			test.Skipf("unsupported fixture filename: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(input.RepoDir, "public.go"), []byte("scope needle PUBLIC_SENTINEL\n"), 0600); err != nil {
		test.Fatal(err)
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Scope fixture")
	base := git(test, input.RepoDir, "rev-parse", "HEAD")
	git(test, input.RepoDir, "commit", "--allow-empty", "-qm", "Comparison head")
	head := git(test, input.RepoDir, "rev-parse", "HEAD")
	input.Diffs = nil
	for _, mode := range []string{"range", "commit", "workspace"} {
		for _, side := range []string{"old", "new"} {
			test.Run(mode+"/"+side, func(test *testing.T) {
				current := input
				current.Mode = mode
				current.Resolution.ResolvedBase = base
				current.Resolution.ResolvedHead = head
				if mode == "workspace" {
					current.Resolution.ResolvedHead = ""
				}
				runner := newTestRunner(test, current, nil, nil)
				result, err := runner.dispatch(test.Context(), "code_search", fmt.Sprintf(`{"search_text":"scope needle","side":%q}`, side))
				if err != nil {
					test.Fatal(err)
				}
				persisted, err := os.ReadFile(runner.store.path)
				if err != nil {
					test.Fatal(err)
				}
				for _, content := range []string{result.Content, string(persisted)} {
					if strings.Contains(content, "PRIVATE_SENTINEL") || !strings.Contains(content, "PUBLIC_SENTINEL") {
						test.Fatalf("incorrect authorized search result: %s", content)
					}
				}
				var observation Observation
				if err := json.Unmarshal([]byte(result.Content), &observation); err != nil {
					test.Fatal(err)
				}
				if len(observation.Locations) != 1 || observation.Locations[0].Path != "public.go" || observation.Side != side {
					test.Fatalf("incorrect authorized locations: %+v", observation)
				}
			})
		}
	}
}

func TestLargeDeniedInventory(test *testing.T) {
	input := fixture(test)
	blob := git(test, input.RepoDir, "rev-parse", "HEAD:main.go")
	var entries strings.Builder
	for index := 0; index < 12000; index++ {
		fmt.Fprintf(&entries, "100644 %s\tvendor/%s%05d.go\n", blob, strings.Repeat("dependency-", 10), index)
	}
	command := exec.Command("git", "update-index", "--index-info")
	command.Dir = input.RepoDir
	command.Stdin = strings.NewReader(entries.String())
	if output, err := command.CombinedOutput(); err != nil {
		test.Fatalf("index fixture: %s: %v", output, err)
	}
	git(test, input.RepoDir, "commit", "-qm", "Dependency inventory")
	input.Resolution.ResolvedBase = git(test, input.RepoDir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(input.RepoDir, "main.go"), []byte("one source change\n"), 0600); err != nil {
		test.Fatal(err)
	}
	git(test, input.RepoDir, "add", "main.go")
	git(test, input.RepoDir, "commit", "-qm", "Single source change")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != 1 || !strings.Contains(prepared.Diffs[0].Diff, "+one source change") {
		test.Fatalf("scoped preparation: %+v %v", prepared.Diffs, err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	for _, patterns := range [][]any{nil, {"main.go"}, {"vendor/"}} {
		observation, err := code.execute(test.Context(), "code_search", map[string]any{"search_text": "change", "file_patterns": patterns})
		if err != nil || strings.Contains(observation.ReturnedContent, "vendor/") {
			test.Fatalf("scoped search: %+v %v", observation, err)
		}
		if len(patterns) == 0 || patterns[0] == "main.go" {
			if !strings.Contains(observation.ReturnedContent, "one source change") {
				test.Fatalf("source match missing: %+v", observation)
			}
		}
	}
}

func TestLiteralPathBatches(test *testing.T) {
	groups := [][]string{{"first.go"}, {"old.go", "new.go"}, {strings.Repeat("x", scopedPathBytes-40)}}
	batches, err := literalPathBatches(groups)
	if err != nil || len(batches) != 2 || !slices.Equal(batches[0], []string{":(top,literal)first.go", ":(top,literal)old.go", ":(top,literal)new.go"}) {
		test.Fatalf("path batches: %+v %v", batches, err)
	}
	for _, batch := range batches {
		if len(strings.Join(batch, "\x00"))+1 > scopedPathBytes {
			test.Fatal("oversized path batch")
		}
	}
	if _, err := literalPathBatches([][]string{{strings.Repeat("x", scopedPathBytes)}}); err == nil {
		test.Fatal("oversized group accepted")
	}
	if batches, err := literalPathBatches(nil); err != nil || len(batches) != 0 {
		test.Fatalf("empty scope broadened: %+v %v", batches, err)
	}
}

func TestScopedSearchBatchesAndPatterns(test *testing.T) {
	input := fixture(test)
	paths := make([]string, 480)
	for index := range paths {
		paths[index] = fmt.Sprintf("source-%03d-%s.go", index, strings.Repeat("path", 5))
		content := "unmatched\n"
		if index%4 == 0 {
			content = "batch needle\n"
		}
		if err := os.WriteFile(filepath.Join(input.RepoDir, paths[index]), []byte(content), 0600); err != nil {
			test.Fatal(err)
		}
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Batched source fixture")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != len(paths)+3 {
		test.Fatalf("batched diff preparation: %d %v", len(prepared.Diffs), err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	for _, fixture := range []struct {
		patterns  []any
		count     int
		truncated bool
	}{
		{nil, 100, true},
		{[]any{"source-00*"}, 3, false},
		{[]any{"source-[0123]*"}, 100, false},
		{[]any{"source-00*", ":(exclude)" + paths[0]}, 2, false},
		{[]any{paths[len(paths)-4]}, 1, false},
		{[]any{"missing/"}, 0, false},
	} {
		observation, err := code.execute(test.Context(), "code_search", map[string]any{"search_text": "batch needle", "file_patterns": fixture.patterns})
		if err != nil || len(observation.Locations) != fixture.count || observation.Truncated != fixture.truncated {
			test.Fatalf("patterns %v: matches=%d truncated=%t error=%v", fixture.patterns, len(observation.Locations), observation.Truncated, err)
		}
	}
	for _, patterns := range [][]any{{"../escape"}, {strings.Repeat("x", scopedPathBytes+1)}, {":(invalid-magic)"}} {
		if _, err := code.execute(test.Context(), "code_search", map[string]any{"search_text": "needle", "file_patterns": patterns}); err == nil {
			test.Fatalf("invalid patterns accepted: %v", patterns)
		}
	}
}
