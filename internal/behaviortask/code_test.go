// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestCodeScopeAndObservationLocations(test *testing.T) {
	input := fixture(test)
	input.Diffs = append(input.Diffs, model.Diff{OldPath: ".env", NewPath: ".env", Diff: "SECRET"}, model.Diff{OldPath: "excluded.go", NewPath: "excluded.go", Diff: "EXCLUDED"})
	runner := newTestRunner(test, input, nil, nil)
	for _, testCase := range []struct {
		name, args, want, absent string
		locations                int
	}{
		{"file_read", `{"file_path":"deleted.go","side":"old"}`, "deleted workflow", "", 2},
		{"file_read", `{"file_path":"deleted.go"}`, "unavailable", "deleted workflow", 0},
		{"file_read", `{"file_path":"consumer.go","side":"old"}`, "consumer", "", 3},
		{"file_read", `{"file_path":"consumer.go"}`, "unavailable", "consumer\n", 0},
		{"file_read", `{"file_path":"renamed.go"}`, "consumer", "", 3},
		{"file_read", `{"file_path":".env"}`, "unavailable", "SECRET", 0},
		{"file_read", `{"file_path":"excluded.go"}`, "unavailable", "EXCLUDED", 0},
		{"file_read", `{"file_path":"../main.go"}`, "unavailable", "old value", 0},
		{"code_search", `{"search_text":"old value","side":"old"}`, "old value", "SECRET", 2},
		{"code_search", `{"search_text":"old value","file_patterns":[":(exclude)main.go"]}`, "old value", "EXCLUDED", 1},
		{"code_search", `{"search_text":"SECRET","file_patterns":[".env"]}`, "No matches", "SECRET old", 0},
		{"code_search", `{"search_text":"not-matched-anywhere"}`, "No matches", "", 0},
		{"file_find", `{"query_name":"env"}`, "No authorized files", ".env", 0},
		{"file_find", `{"query_name":"consumer","side":"old"}`, "consumer.go", "", 0},
		{"file_read_diff", `{"path_array":["deleted.go"]}`, "deleted workflow", "SECRET", 1},
		{"file_read_diff", `{"path_array":["main.go"]}`, "new value", "SECRET", 4},
		{"file_read_diff", `{"path_array":[".env"]}`, "diff not found", "SECRET", 0},
	} {
		test.Run(testCase.name+testCase.args, func(test *testing.T) {
			outcome, err := runner.dispatch(context.Background(), testCase.name, testCase.args)
			if err != nil {
				test.Fatal(err)
			}
			var observation Observation
			if err := json.Unmarshal([]byte(outcome.Content), &observation); err != nil {
				test.Fatal(err)
			}
			lineCount := 0
			for _, location := range observation.Locations {
				lineCount += location.EndLine - location.StartLine + 1
			}
			if !strings.Contains(observation.ReturnedContent, testCase.want) || (testCase.absent != "" && strings.Contains(observation.ReturnedContent, testCase.absent)) || lineCount != testCase.locations {
				test.Fatalf("observation: %+v", observation)
			}
			if observation.ContentHash != contentHash(observation.ReturnedContent) || observation.ComparisonID != runner.code.comparison.ID {
				test.Fatal("invalid observation identity")
			}
		})
	}
	for _, args := range []string{`{"file_path":"main.go","side":"HEAD"}`, `{"file_path":"main.go","ref":"HEAD"}`, `{"file_path":"main.go","start_line":1.5}`} {
		if _, err := runner.dispatch(context.Background(), "file_read", args); err == nil {
			test.Fatalf("accepted %s", args)
		}
	}
	if _, err := runner.dispatch(context.Background(), "file_read_diff", `{"path_array":["main.go"],"side":"old"}`); err == nil {
		test.Fatal("diff accepted side")
	}
	if runner.code.scope[len(runner.code.scope)-1].ExcludedReason == "" {
		test.Fatal("exclusion missing from scope")
	}
	for _, filePath := range []string{".env", ".env.production", "nested/.ssh/config", "secret.pem", "key.key", ".netrc", "id_ed25519", "..", "../x", "a/../b", "/etc/passwd", "x\ny", "a:b", "a\\b", ".git/config"} {
		if safePath(filePath) {
			test.Errorf("unsafe path accepted: %q", filePath)
		}
	}
}

func TestTruncationAndObservationRanges(test *testing.T) {
	input := fixture(test)
	if err := os.WriteFile(filepath.Join(input.RepoDir, "large.go"), []byte(strings.Repeat("needle\n", 600)), 0600); err != nil {
		test.Fatal(err)
	}
	for index := 0; index < 101; index++ {
		if err := os.WriteFile(filepath.Join(input.RepoDir, fmt.Sprintf("listed-%03d.go", index)), []byte("text"), 0600); err != nil {
			test.Fatal(err)
		}
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Large fixture")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	runner := newTestRunner(test, input, nil, nil)
	for _, call := range []struct{ name, args string }{
		{"file_read", `{"file_path":"large.go"}`},
		{"code_search", `{"search_text":"needle","file_patterns":["large.go"]}`},
		{"file_find", `{"query_name":"listed-"}`},
	} {
		outcome, err := runner.dispatch(context.Background(), call.name, call.args)
		if err != nil {
			test.Fatal(err)
		}
		var observation Observation
		if err := json.Unmarshal([]byte(outcome.Content), &observation); err != nil {
			test.Fatal(err)
		}
		if !observation.Truncated || observation.Continuation == "" {
			test.Fatalf("missing truncation: %+v", observation)
		}
		if call.name == "file_read" {
			if len(observation.Locations) != 1 || observation.Locations[0].EndLine != 500 {
				test.Fatal("wrong returned range")
			}

		}
	}

}

func TestFrozenRevisionDoesNotReadWorkspace(test *testing.T) {
	input := fixture(test)
	if err := os.WriteFile(filepath.Join(input.RepoDir, "main.go"), []byte("WORKSPACE SECRET"), 0600); err != nil {
		test.Fatal(err)
	}
	runner := newTestRunner(test, input, nil, nil)
	outcome, err := runner.dispatch(context.Background(), "file_read", `{"file_path":"main.go"}`)
	if err != nil || strings.Contains(outcome.Content, "WORKSPACE") || !strings.Contains(outcome.Content, "new value") {
		test.Fatalf("workspace fallback: %s, %v", outcome.Content, err)
	}
	input.Resolution.ResolvedBase = strings.Repeat("f", 40)
	if _, err := prepareCodeTools(context.Background(), input); err == nil {
		test.Fatal("missing old revision accepted")
	}
	input.Resolution = diff.InputResolution{ResolvedHead: "HEAD"}
	if _, err := prepareCodeTools(context.Background(), input); err == nil {
		test.Fatal("moving ref accepted")
	}
}
