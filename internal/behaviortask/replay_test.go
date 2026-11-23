// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/llm"
)

func TestPreparedReplaySmoke(test *testing.T) {
	root := os.Getenv("OCR_BEHAVIOR_REPLAY")
	if root == "" {
		test.Skip("set OCR_BEHAVIOR_REPLAY to a verified handoff replay directory")
	}
	home := test.TempDir()
	test.Setenv("HOME", home)
	test.Setenv("USERPROFILE", home)
	for _, caseID := range []string{"pr-1894", "pr-1882", "pr-1531"} {
		test.Run(caseID, func(test *testing.T) {
			var captured struct {
				CaseID     string `json:"case_id"`
				Background string `json:"background"`
				Comparison struct {
					Old  string `json:"old_sha"`
					Head string `json:"head_sha"`
				} `json:"comparison"`
				Views struct {
					New string `json:"new"`
				} `json:"views"`
			}
			data, err := os.ReadFile(filepath.Join(root, caseID, "input.json"))
			if err != nil {
				test.Fatal(err)
			}
			if err := json.Unmarshal(data, &captured); err != nil {
				test.Fatal(err)
			}
			if captured.CaseID != caseID {
				test.Fatal("case identity mismatch")
			}
			patch, err := os.ReadFile(filepath.Join(root, caseID, "comparison.diff"))
			if err != nil {
				test.Fatal(err)
			}
			changes, err := diff.ParseDiffText(context.Background(), string(patch), captured.Views.New, captured.Comparison.Head, nil)
			if err != nil {
				test.Fatal(err)
			}
			input := Input{RepoDir: captured.Views.New, Mode: "range", Resolution: diff.InputResolution{ResolvedBase: captured.Comparison.Old, ResolvedHead: captured.Comparison.Head}, Diffs: changes, Contexts: []Context{{ID: "background", Kind: "background", Source: "input.json", Content: captured.Background}}}
			var runner *Runner
			requests := 0
			client := fakeClient(func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
				requests++
				if requests == 1 {
					paths := []string{}
					for filePath := range runner.code.diffs {
						paths = append(paths, filePath)
					}
					sort.Strings(paths)
					return response("file_read_diff", marshal(test, map[string]any{"path_array": paths})), nil
				}
				analysis := Analysis{Markdown: "Engineering replay of [[obs-000001]] only; no model quality evaluation or repository code execution.", DeclaredCompletion: "partial"}
				return response("submit_analysis", marshal(test, analysis)), nil
			})
			runner = newTestRunner(test, input, client, func(deps *Deps) { deps.Model = "fixture-client-not-a-quality-evaluation" })
			result, err := runner.Run(context.Background())
			if err != nil || !result.Completed || result.Analysis.DeclaredCompletion != "partial" || len(result.ObservationIDs) != 1 || len(result.Errors) != 0 {
				test.Fatalf("replay %s: %+v, %v", caseID, result, err)
			}
			if len(runner.store.items[0].Locations) == 0 {
				test.Fatal("no recorded diff evidence")
			}
			test.Logf("engineering replay %s old=%s head=%s changes=%d observed_ranges=%d; quality NOT evaluated", caseID, result.Comparison.Old, result.Comparison.Head, len(result.Scope), len(runner.store.items[0].Locations))
		})
	}
}
