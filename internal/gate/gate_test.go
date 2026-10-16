// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gate

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// This fixture follows the public JSON contract, rather than building inputs
// through the evaluator's internal structs. Unknown metadata is intentional.
const completeResult = `{
  "status": "complete",
  "summary": {"files_reviewed": 1, "comments": 0},
  "comments": [],
  "warnings": [],
  "tool_calls": {"total": 2, "by_tool": {"file_read": 2}, "failure": 0, "failure_by_tool": {}, "failure_details": []},
  "manifest": {
    "schema_version": "ocr.run-manifest/v1",
    "operation": "review",
    "run_id": "run-1",
    "terminal_state": "complete",
    "input": {"mode": "range", "resolved_base": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "resolved_head": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
    "coverage": {
      "selected": [{"item_id": "item-1", "path": "main.go", "fingerprint": "fp-1"}],
      "completed": [{"item_id": "item-1", "path": "main.go", "fingerprint": "fp-1"}],
      "reused": [], "failed": [], "waived": []
    }
  }
}`

func fixture(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(completeResult), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func manifest(doc map[string]any) map[string]any { return doc["manifest"].(map[string]any) }
func coverage(doc map[string]any) map[string]any { return manifest(doc)["coverage"].(map[string]any) }
func calls(doc map[string]any) map[string]any    { return doc["tool_calls"].(map[string]any) }

func setState(doc map[string]any, state string) {
	doc["status"] = state
	manifest(doc)["terminal_state"] = state
}

func outcome(doc map[string]any, name string) {
	c := coverage(doc)
	c[name] = c["completed"]
	c["completed"] = []any{}
}

func failures(doc map[string]any, names ...string) {
	counts := map[string]any{}
	details := []any{}
	for _, name := range names {
		n, _ := counts[name].(int)
		counts[name] = n + 1
		details = append(details, map[string]any{"tool_name": name, "arguments": "private payload", "error": "private error"})
	}
	c := calls(doc)
	c["failure"] = len(names)
	c["failure_by_tool"] = counts
	c["failure_details"] = details
}

func evaluateFixture(t *testing.T, doc map[string]any, policy Policy) Result {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Evaluate(strings.NewReader(string(data)), policy)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion || got.Checks == nil {
		t.Fatalf("missing result contract: %+v", got)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "private") {
		t.Fatal("gate exposed raw tool failure details")
	}
	return got
}

func hasCode(result Result, code string) bool {
	for _, check := range result.Checks {
		if check.Code == code {
			return true
		}
	}
	return false
}

func TestEvaluateDecisions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(map[string]any)
		policy Policy
		status Status
		code   string
	}{
		{"complete empty", func(map[string]any) {}, Policy{}, Pass, "complete_coverage"},
		{"null comments from producer", func(d map[string]any) { d["comments"] = nil }, Policy{FailOnSeverity: "high"}, Pass, "below_threshold"},
		{"same input reuse", func(d map[string]any) { outcome(d, "reused") }, Policy{}, Pass, "complete_coverage"},
		{"waived is not reviewed", func(d map[string]any) { outcome(d, "waived") }, Policy{}, Inconclusive, "incomplete_coverage"},
		{"run failure with completed items", func(d map[string]any) {
			manifest(d)["run_failure"] = map[string]any{"classification": "internal"}
			setState(d, "failed")
		}, Policy{}, Inconclusive, "run_failed"},
		{"all failed", func(d map[string]any) { outcome(d, "failed"); setState(d, "failed") }, Policy{}, Inconclusive, "incomplete_coverage"},
		{"skipped", func(d map[string]any) {
			coverage(d)["selected"], coverage(d)["completed"] = []any{}, []any{}
			setState(d, "skipped")
		}, Policy{}, Inconclusive, "no_selected_items"},
		{"version matches", func(map[string]any) {}, Policy{ExpectedHead: strings.Repeat("b", 40), ExpectedBase: strings.Repeat("a", 40)}, Pass, "revision_matches"},
		{"wrong head", func(map[string]any) {}, Policy{ExpectedHead: strings.Repeat("c", 40)}, Inconclusive, "revision_mismatch"},
		{"wrong base", func(map[string]any) {}, Policy{ExpectedBase: strings.Repeat("c", 40)}, Inconclusive, "revision_mismatch"},
		{"missing reviewed head", func(d map[string]any) { delete(manifest(d)["input"].(map[string]any), "resolved_head") }, Policy{ExpectedHead: strings.Repeat("b", 40)}, Inconclusive, "revision_mismatch"},
		{"workspace cannot attest commit", func(d map[string]any) { manifest(d)["input"].(map[string]any)["mode"] = "workspace" }, Policy{ExpectedBase: strings.Repeat("a", 40)}, Inconclusive, "revision_mismatch"},
		{"exploration error alone", func(d map[string]any) { failures(d, "code_search", "file_read") }, Policy{}, Pass, "no_comment_delivery_failure"},
		{"comment submission error", func(d map[string]any) { failures(d, "code_comment") }, Policy{}, Inconclusive, "comment_delivery_unverified"},
		{"threshold met", func(d map[string]any) { d["comments"] = []any{map[string]any{"severity": " High "}} }, Policy{FailOnSeverity: "high"}, Fail, "severity_threshold"},
		{"above threshold", func(d map[string]any) { d["comments"] = []any{map[string]any{"severity": "critical"}} }, Policy{FailOnSeverity: "medium"}, Fail, "severity_threshold"},
		{"below threshold", func(d map[string]any) { d["comments"] = []any{map[string]any{"severity": "low"}} }, Policy{FailOnSeverity: "high"}, Pass, "below_threshold"},
		{"unknown severity", func(d map[string]any) { d["comments"] = []any{map[string]any{"severity": "info"}} }, Policy{FailOnSeverity: "low"}, Inconclusive, "unknown_severity"},
		{"missing severity", func(d map[string]any) { d["comments"] = []any{map[string]any{"content": "a finding"}} }, Policy{FailOnSeverity: "high"}, Inconclusive, "unknown_severity"},
		{"severity disabled", func(d map[string]any) { d["comments"] = []any{map[string]any{"severity": "critical"}} }, Policy{}, Pass, "complete_coverage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fixture(t)
			tc.edit(doc)
			before, _ := json.Marshal(doc)
			got := evaluateFixture(t, doc, tc.policy)
			after, _ := json.Marshal(doc)
			if got.Status != tc.status || !hasCode(got, tc.code) {
				t.Fatalf("got %+v, want %s with %s", got, tc.status, tc.code)
			}
			if got.RunID != "run-1" || string(before) != string(after) {
				t.Fatal("lost run identity or mutated input")
			}
		})
	}
}

func TestPartialFailuresRetainIndependentChecks(t *testing.T) {
	for _, class := range []string{"budget", "timeout", "provider"} {
		t.Run(class, func(t *testing.T) {
			doc := fixture(t)
			c := coverage(doc)
			item := map[string]any{"item_id": "item-2", "path": "other.go", "classification": class}
			c["selected"] = append(c["selected"].([]any), item)
			c["failed"] = []any{item}
			setState(doc, "partial")
			got := evaluateFixture(t, doc, Policy{})
			if got.Status != Inconclusive || !hasCode(got, "incomplete_coverage") {
				t.Fatalf("partial %s: %+v", class, got)
			}
			doc["comments"] = []any{map[string]any{"severity": "high"}, map[string]any{"severity": "unknown"}}
			failures(doc, "code_comment")
			got = evaluateFixture(t, doc, Policy{FailOnSeverity: "high"})
			if got.Status != Fail || !hasCode(got, "severity_threshold") || !hasCode(got, "incomplete_coverage") || !hasCode(got, "comment_delivery_unverified") {
				t.Fatalf("lost independent failure evidence: %+v", got)
			}
		})
	}
}

func TestInvalidEvidenceNeverPasses(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing manifest", func(d map[string]any) { delete(d, "manifest") }},
		{"future schema", func(d map[string]any) { manifest(d)["schema_version"] = "ocr.run-manifest/v2" }},
		{"missing run ID", func(d map[string]any) { delete(manifest(d), "run_id") }},
		{"unsupported operation", func(d map[string]any) { manifest(d)["operation"] = "scan" }},
		{"unsupported input", func(d map[string]any) { manifest(d)["input"].(map[string]any)["mode"] = "other" }},
		{"missing coverage", func(d map[string]any) { delete(manifest(d), "coverage") }},
		{"null coverage array", func(d map[string]any) { coverage(d)["failed"] = nil }},
		{"absent coverage array", func(d map[string]any) { delete(coverage(d), "waived") }},
		{"empty item ID", func(d map[string]any) { coverage(d)["selected"].([]any)[0].(map[string]any)["item_id"] = "" }},
		{"empty path", func(d map[string]any) { coverage(d)["selected"].([]any)[0].(map[string]any)["path"] = "" }},
		{"duplicate selected", func(d map[string]any) {
			c := coverage(d)
			c["selected"] = append(c["selected"].([]any), c["selected"].([]any)[0])
		}},
		{"duplicate outcome", func(d map[string]any) { c := coverage(d); c["reused"] = c["completed"] }},
		{"unknown outcome item", func(d map[string]any) { coverage(d)["completed"].([]any)[0].(map[string]any)["item_id"] = "other" }},
		{"mismatched outcome path", func(d map[string]any) { coverage(d)["completed"].([]any)[0].(map[string]any)["path"] = "other.go" }},
		{"mismatched old path", func(d map[string]any) { coverage(d)["completed"].([]any)[0].(map[string]any)["old_path"] = "old.go" }},
		{"mismatched fingerprint", func(d map[string]any) { coverage(d)["completed"].([]any)[0].(map[string]any)["fingerprint"] = "other" }},
		{"unaccounted item", func(d map[string]any) { coverage(d)["completed"] = []any{} }},
		{"status mismatch", func(d map[string]any) { d["status"] = "partial" }},
		{"terminal mismatch", func(d map[string]any) { manifest(d)["terminal_state"] = "failed" }},
		{"missing findings", func(d map[string]any) { delete(d, "comments") }},
		{"wrong findings shape", func(d map[string]any) { d["comments"] = map[string]any{} }},
		{"null finding", func(d map[string]any) { d["comments"] = []any{nil} }},
		{"missing tool calls", func(d map[string]any) { delete(d, "tool_calls") }},
		{"missing failure count", func(d map[string]any) { delete(calls(d), "failure") }},
		{"negative failure count", func(d map[string]any) { calls(d)["failure"] = -1 }},
		{"missing failure details", func(d map[string]any) { delete(calls(d), "failure_details") }},
		{"missing per-tool counts", func(d map[string]any) { delete(calls(d), "failure_by_tool") }},
		{"zero per-tool count without failures", func(d map[string]any) {
			calls(d)["failure_by_tool"] = map[string]any{"code_comment": 0}
		}},
		{"extra zero per-tool count", func(d map[string]any) {
			failures(d, "file_read")
			calls(d)["failure_by_tool"].(map[string]any)["code_search"] = 0
		}},
		{"negative per-tool count", func(d map[string]any) {
			calls(d)["failure_by_tool"] = map[string]any{"code_comment": -1}
		}},
		{"hidden failure details", func(d map[string]any) { failures(d, "code_comment"); calls(d)["failure"] = 0 }},
		{"missing tool name", func(d map[string]any) { failures(d, "") }},
		{"wrong per-tool counts", func(d map[string]any) {
			failures(d, "code_comment")
			calls(d)["failure_by_tool"] = map[string]any{"code_comment": 2}
		}},
		{"hidden per-tool failure", func(d map[string]any) { failures(d, "code_comment"); calls(d)["failure_by_tool"] = map[string]any{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fixture(t)
			tc.edit(doc)
			got := evaluateFixture(t, doc, Policy{})
			if got.Status != Inconclusive {
				t.Fatalf("invalid evidence: %+v", got)
			}
		})
	}
}

func TestMalformedDocuments(t *testing.T) {
	for _, input := range []string{"", "null", "[]", "{}", "{", completeResult + `{}`, completeResult + " trailing", `{"manifest":false}`} {
		got, err := Evaluate(strings.NewReader(input), Policy{})
		if err != nil || got.Status != Inconclusive {
			t.Fatalf("input %q: %+v, %v", input, got, err)
		}
	}
	got, err := Evaluate(io.MultiReader(strings.NewReader(completeResult), brokenReader{}), Policy{})
	if err != nil || got.Status != Inconclusive {
		t.Fatalf("read failure: %+v, %v", got, err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestPolicy(t *testing.T) {
	for _, policy := range []Policy{
		{FailOnSeverity: "info"}, {ExpectedHead: "HEAD"}, {ExpectedBase: "abc123"}, {ExpectedHead: strings.Repeat("B", 40)},
	} {
		if _, err := Evaluate(brokenReader{}, policy); err == nil {
			t.Fatalf("accepted invalid policy %+v", policy)
		}
	}
	p, err := (Policy{FailOnSeverity: " HIGH ", ExpectedHead: strings.Repeat("c", 64)}).Normalize()
	if err != nil || p.FailOnSeverity != "high" {
		t.Fatalf("normalization: %+v, %v", p, err)
	}
}

func TestDeterministicResults(t *testing.T) {
	doc := fixture(t)
	failures(doc, "file_read", "code_search", "file_read")
	first := evaluateFixture(t, doc, Policy{FailOnSeverity: "high"})
	for range 10 {
		if next := evaluateFixture(t, doc, first.Policy); !reflect.DeepEqual(first, next) {
			t.Fatalf("unstable result: %+v != %+v", first, next)
		}
	}
}
