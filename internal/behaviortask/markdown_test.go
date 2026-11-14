// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestMarkdownValidation(test *testing.T) {
	input := fixture(test)
	runner := newTestRunner(test, input, nil, nil)
	observePair(test, runner)
	runner.store.items = append(runner.store.items, Observation{ID: "obs-000003", ErrorClass: "unavailable"})
	for _, fixture := range []struct {
		name     string
		analysis Analysis
		valid    bool
	}{
		{"free form", Analysis{Markdown: "Plain prose without mandatory headings or citations."}, true},
		{"long report remains valid", Analysis{Markdown: strings.Repeat("Necessary detail with a trigger, consequence and boundary.\n", 100)}, true},
		{"citations", Analysis{Markdown: "Before [[obs-000001]]; after [[obs-000002]]. Repeated [[obs-000001]]."}, true},
		{"self partial", Analysis{Markdown: "Questions remain.", DeclaredCompletion: "partial"}, true},
		{"self unknown", Analysis{Markdown: "Investigation report.", DeclaredCompletion: "unknown"}, true},
		{"empty", Analysis{}, false},
		{"whitespace", Analysis{Markdown: " \n\t"}, false},
		{"too large", Analysis{Markdown: strings.Repeat("x", MaxReportBytes+1)}, false},
		{"utf8 byte limit", Analysis{Markdown: strings.Repeat("\u2192", MaxReportBytes/3+1)}, false},
		{"unknown ID", Analysis{Markdown: "[[obs-999999]]"}, false},
		{"failed ID", Analysis{Markdown: "[[obs-000003]]"}, false},
		{"malformed ID", Analysis{Markdown: "[[obs-missing]]"}, false},
		{"unclosed marker", Analysis{Markdown: "[[obs-000001]"}, false},
		{"invalid completeness", Analysis{Markdown: "Report", DeclaredCompletion: "verified"}, false},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			if err := runner.validateAnalysis(fixture.analysis); (err == nil) != fixture.valid {
				test.Fatalf("validation: %v", err)
			}
		})
	}
	if ids := citedObservations("[[obs-000002]], [[obs-000001]], [[obs-000002]]"); !reflect.DeepEqual(ids, []string{"obs-000002", "obs-000001"}) {
		test.Fatalf("citations: %v", ids)
	}
}

func TestMarkdownOnlySubmissionAndArtifacts(test *testing.T) {
	input := fixture(test)
	markdown := "### Unusual order\n\nQuotes: \"fixture\"; Unicode: \u6d4b\u8bd5.\n\n```go\nreturn \"value\"\n```\n\n| Item | State |\n| --- | --- |\n| Policy | Open |\n"
	runner := newTestRunner(test, input, func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		return response("submit_analysis", marshal(test, map[string]string{"markdown": markdown})), nil
	}, nil)
	result, err := runner.Run(test.Context())
	if err != nil || !result.Completed || result.Analysis.Markdown != markdown || result.Analysis.DeclaredCompletion != "unknown" {
		test.Fatalf("result: %+v, %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(runner.deps.ArtifactDir, "report.md"))
	if err != nil || string(data) != markdown {
		test.Fatalf("Markdown changed: %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(runner.deps.ArtifactDir, "result.json"))
	if err != nil {
		test.Fatal(err)
	}
	var persisted Result
	if err := json.Unmarshal(data, &persisted); err != nil || persisted.Analysis.Markdown != markdown || persisted.SchemaVersion != "ocr.behavior-impact/v2" {
		test.Fatalf("persisted: %+v, %v", persisted, err)
	}
}

func TestMarkdownSchemaRejectsLegacyAndBadTypes(test *testing.T) {
	runner := newTestRunner(test, fixture(test), nil, nil)
	for _, raw := range []string{`{}`, `{"markdown":null}`, `{"markdown":[]}`, `{"markdown":"report","declared_completion":"verified"}`, `{"scenarios":[],"impacts":[]}`, `{"markdown":"report","scenarios":[]}`} {
		if _, err := runner.dispatch(test.Context(), "submit_analysis", raw); err == nil {
			test.Fatalf("accepted %s", raw)
		}
	}
	if _, err := runner.dispatch(test.Context(), "submit_analysis", `{"markdown":"No mandated schema or headings."}`); err != nil {
		test.Fatal(err)
	}
}

func TestMarkdownDeliveryFailureKeepsAcceptedReport(test *testing.T) {
	runner := newTestRunner(test, fixture(test), func(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
		return response("submit_analysis", `{"markdown":"Report","declared_completion":"complete"}`), nil
	}, nil)
	path := filepath.Join(runner.deps.ArtifactDir, "report.md")
	if err := os.WriteFile(path, []byte("existing artifact"), 0600); err != nil {
		test.Fatal(err)
	}
	result, err := runner.Run(test.Context())
	if err == nil || !result.Completed || result.Analysis.Markdown != "Report" || len(result.Errors) == 0 {
		test.Fatalf("delivery: %+v, %v", result, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing artifact" {
		test.Fatalf("artifact overwritten: %q %v", data, err)
	}
}
