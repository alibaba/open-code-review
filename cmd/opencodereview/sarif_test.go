// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// mustUnmarshal is a test helper that unmarshals SARIF output and fails the
// test on error, so malformed output produces a clear failure instead of a
// nil-map panic downstream.
func mustUnmarshal(t *testing.T, out string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal SARIF output: %v\noutput: %s", err, out)
	}
	return doc
}

// mustGetResult extracts the first result from a SARIF document, failing the
// test if the structure is wrong.
func mustGetResult(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	runs := doc["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	results := runs[0].(map[string]any)["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	return results[0].(map[string]any)
}

// --- AC-1: Basic SARIF output structure ---

func TestOutputSARIF_BasicStructure(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "fix", StartLine: 1, EndLine: 1, Category: "bug", Severity: "high"},
	}
	out := captureStdout(t, func() {
		if err := outputSARIF(comments, "test-version", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)

	if doc["$schema"] != sarifSchema {
		t.Errorf("$schema = %v, want %q", doc["$schema"], sarifSchema)
	}
	if doc["version"] != sarifVersion {
		t.Errorf("version = %v, want %q", doc["version"], sarifVersion)
	}
	runs, ok := doc["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("runs should be array of length 1, got %T len=%d", doc["runs"], len(runs))
	}
	run := runs[0].(map[string]any)
	driver := run["tool"].(map[string]any)["driver"].(map[string]any)
	if driver["name"] != sarifToolName {
		t.Errorf("driver.name = %v, want %q", driver["name"], sarifToolName)
	}
	if driver["version"] != "test-version" {
		t.Errorf("driver.version = %v, want test-version", driver["version"])
	}
	if driver["informationUri"] != sarifInformationURI {
		t.Errorf("driver.informationUri = %v", driver["informationUri"])
	}
	rules, ok := driver["rules"].([]any)
	if !ok || len(rules) != 8 {
		t.Errorf("rules should have 8 entries, got %d", len(rules))
	}
	results, ok := run["results"].([]any)
	if !ok || len(results) != 1 {
		t.Errorf("results should have 1 entry, got %d", len(results))
	}
}

// --- AC-2: Empty comments produces empty results array (not null) ---

func TestOutputSARIF_EmptyComments(t *testing.T) {
	out := captureStdout(t, func() {
		if err := outputSARIF(nil, "test-version", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)

	run := doc["runs"].([]any)[0].(map[string]any)
	results, ok := run["results"].([]any)
	if !ok {
		t.Fatalf("results should be an array, got %T", run["results"])
	}
	if len(results) != 0 {
		t.Errorf("results should be empty, got %d", len(results))
	}
	rules := run["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	if len(rules) != 8 {
		t.Errorf("rules should still have 8 entries, got %d", len(rules))
	}
}

// --- AC-3: No-files scenario via emitRunResult ---

func TestEmitRunResult_SarifNoFiles(t *testing.T) {
	ag := &mockResultProvider{filesReviewed: 0}
	out := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, nil, time.Now(), "sarif", "developer", nil, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)

	run := doc["runs"].([]any)[0].(map[string]any)
	results, ok := run["results"].([]any)
	if !ok || len(results) != 0 {
		t.Errorf("results should be empty array, got %T len=%d", run["results"], len(results))
	}
}

// --- AC-4: Full field mapping ---

func TestOutputSARIF_FullFieldMapping(t *testing.T) {
	comment := model.LlmComment{
		Path:           "internal/agent/agent.go",
		Content:        "Potential nil pointer dereference",
		SuggestionCode: "if err != nil { return err }",
		ExistingCode:   "return err",
		StartLine:      42,
		EndLine:        42,
		Category:       "bug",
		Severity:       "high",
	}
	out := captureStdout(t, func() {
		if err := outputSARIF([]model.LlmComment{comment}, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)
	result := mustGetResult(t, doc)

	if result["ruleId"] != "bug" {
		t.Errorf("ruleId = %v, want bug", result["ruleId"])
	}
	if result["level"] != "error" {
		t.Errorf("level = %v, want error", result["level"])
	}
	msg := result["message"].(map[string]any)
	if msg["text"] != "Potential nil pointer dereference" {
		t.Errorf("message.text = %v", msg["text"])
	}
	// locations (array, not location object)
	locs, ok := result["locations"].([]any)
	if !ok || len(locs) != 1 {
		t.Fatalf("locations should be array of length 1, got %T", result["locations"])
	}
	physLoc := locs[0].(map[string]any)["physicalLocation"].(map[string]any)
	artLoc := physLoc["artifactLocation"].(map[string]any)
	if artLoc["uri"] != "internal/agent/agent.go" {
		t.Errorf("uri = %v", artLoc["uri"])
	}
	region := physLoc["region"].(map[string]any)
	if region["startLine"] != float64(42) {
		t.Errorf("startLine = %v, want 42", region["startLine"])
	}
	if region["endLine"] != float64(42) {
		t.Errorf("endLine = %v, want 42", region["endLine"])
	}
	// fixes
	fixes := result["fixes"].([]any)
	fix := fixes[0].(map[string]any)
	artChange := fix["artifactChanges"].([]any)[0].(map[string]any)
	fixArtLoc := artChange["artifactLocation"].(map[string]any)
	if fixArtLoc["uri"] != "internal/agent/agent.go" {
		t.Errorf("fix uri = %v", fixArtLoc["uri"])
	}
	rep := artChange["replacements"].([]any)[0].(map[string]any)
	delRegion := rep["deletedRegion"].(map[string]any)
	if delRegion["startLine"] != float64(42) {
		t.Errorf("deletedRegion.startLine = %v, want 42", delRegion["startLine"])
	}
	inserted := rep["insertedContent"].(map[string]any)
	if inserted["text"] != "if err != nil { return err }" {
		t.Errorf("insertedContent.text = %v", inserted["text"])
	}
	// partialFingerprints
	fp := result["partialFingerprints"].(map[string]any)
	if _, ok := fp[sarifFingerprintKey]; !ok {
		t.Errorf("partialFingerprints should contain %q", sarifFingerprintKey)
	}
}

// --- AC-5 & AC-6: Severity → Level mapping ---

func TestSarifSeverityLevel(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		want     string
	}{
		{"critical", "critical", "error"},
		{"high", "high", "error"},
		{"medium", "medium", "warning"},
		{"low", "low", "note"},
		{"empty", "", "note"},
		{"unknown", "unknown", "note"},
		{"case-sensitive", "CRITICAL", "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sarifSeverityLevel(tc.severity)
			if got != tc.want {
				t.Errorf("sarifSeverityLevel(%q) = %q, want %q", tc.severity, got, tc.want)
			}
		})
	}
}

// --- AC-7: Empty Category defaults to "other" ---

func TestOutputSARIF_EmptyCategory(t *testing.T) {
	comment := model.LlmComment{
		Path:      "a.go",
		Content:   "test",
		StartLine: 1,
		EndLine:   1,
		Category:  "",
		Severity:  "medium",
	}
	out := captureStdout(t, func() {
		if err := outputSARIF([]model.LlmComment{comment}, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)
	result := mustGetResult(t, doc)
	if result["ruleId"] != "other" {
		t.Errorf("ruleId = %v, want other", result["ruleId"])
	}
}

// --- AC-8: Zero line numbers omit region AND fixes (deletedRegion is required) ---

func TestOutputSARIF_ZeroLineNumbers(t *testing.T) {
	comment := model.LlmComment{
		Path:           "a.go",
		Content:        "test",
		SuggestionCode: "new code",
		ExistingCode:   "old code",
		StartLine:      0,
		EndLine:        0,
		Category:       "bug",
		Severity:       "high",
	}
	out := captureStdout(t, func() {
		if err := outputSARIF([]model.LlmComment{comment}, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)
	result := mustGetResult(t, doc)

	// locations must exist (Path is non-empty) but region must be omitted.
	locs, ok := result["locations"].([]any)
	if !ok || len(locs) != 1 {
		t.Fatal("locations should exist when Path is non-empty")
	}
	physLoc := locs[0].(map[string]any)["physicalLocation"].(map[string]any)
	if _, exists := physLoc["region"]; exists {
		t.Error("region should be omitted when line numbers are 0")
	}
	// fixes must NOT exist — deletedRegion is required by SARIF schema and
	// cannot be omitted, so the entire fixes block is suppressed.
	if _, exists := result["fixes"]; exists {
		t.Error("fixes should be omitted when line numbers are 0 (deletedRegion is required)")
	}
}

// --- AC-9: Missing suggestion/existing code omits fixes ---

func TestOutputSARIF_NoFixes(t *testing.T) {
	tests := []struct {
		name     string
		suggest  string
		existing string
	}{
		{"both empty", "", ""},
		{"suggest empty", "", "old code"},
		{"existing empty", "new code", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comment := model.LlmComment{
				Path:           "a.go",
				Content:        "test",
				SuggestionCode: tc.suggest,
				ExistingCode:   tc.existing,
				StartLine:      1,
				EndLine:        1,
				Category:       "bug",
				Severity:       "high",
			}
			out := captureStdout(t, func() {
				if err := outputSARIF([]model.LlmComment{comment}, "v1", nil, nil); err != nil {
					t.Fatalf("outputSARIF: %v", err)
				}
			})
			doc := mustUnmarshal(t, out)
			result := mustGetResult(t, doc)
			if _, exists := result["fixes"]; exists {
				t.Errorf("fixes should be omitted when suggestion or existing code is empty")
			}
		})
	}
}

// --- AC-10: Rule definition completeness ---

func TestSarifRules(t *testing.T) {
	rules := sarifRules()
	if len(rules) != 8 {
		t.Fatalf("expected 8 rules, got %d", len(rules))
	}
	expectedIDs := map[string]bool{
		"bug": false, "security": false, "performance": false, "maintainability": false,
		"test": false, "style": false, "documentation": false, "other": false,
	}
	for _, rule := range rules {
		if rule.ID == "" || rule.Name == "" || rule.ShortDescription.Text == "" {
			t.Errorf("rule %q has empty field: id=%q name=%q desc=%q", rule.ID, rule.ID, rule.Name, rule.ShortDescription.Text)
		}
		if _, ok := expectedIDs[rule.ID]; ok {
			expectedIDs[rule.ID] = true
		} else {
			t.Errorf("unexpected rule id: %q", rule.ID)
		}
	}
	for id, found := range expectedIDs {
		if !found {
			t.Errorf("missing rule id: %q", id)
		}
	}
}

// --- AC-11: --format flag includes sarif ---

func TestAddOutputFlags_IncludesSarif(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	var format, audience string
	addOutputFlags(cmd, &format, &audience)

	flag := cmd.Flags().Lookup("format")
	if flag == nil {
		t.Fatal("format flag not found")
	}
	if !strings.Contains(flag.Usage, "sarif") {
		t.Errorf("format flag usage should mention sarif, got: %s", flag.Usage)
	}
}

// --- AC-12: emitRunResult with sarif format produces valid SARIF ---

func TestEmitRunResult_Sarif(t *testing.T) {
	ag := &mockResultProvider{
		filesReviewed: 1,
		manifest:      mockManifest(session.StateComplete),
	}
	comments := []model.LlmComment{
		{Path: "main.go", Content: "nil deref", StartLine: 10, EndLine: 10, Category: "bug", Severity: "critical"},
	}
	out := captureStdout(t, func() {
		if err := emitRunResult(context.Background(), ag, comments, time.Now(), "sarif", "developer", nil, nil); err != nil {
			t.Fatalf("emitRunResult: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)

	if doc["version"] != sarifVersion {
		t.Errorf("version = %v, want %q", doc["version"], sarifVersion)
	}
	run := doc["runs"].([]any)[0].(map[string]any)
	results := run["results"].([]any)
	if len(results) != 1 {
		t.Errorf("results length = %d, want 1", len(results))
	}
}

// --- AC-13: Schema compliance (field types) ---

func TestOutputSARIF_SchemaCompliance(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "test", StartLine: 5, EndLine: 10, Category: "security", Severity: "medium"},
	}
	out := captureStdout(t, func() {
		if err := outputSARIF(comments, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)

	if _, ok := doc["version"].(string); !ok {
		t.Errorf("version should be string, got %T", doc["version"])
	}
	runs, ok := doc["runs"].([]any)
	if !ok {
		t.Fatalf("runs should be array, got %T", doc["runs"])
	}
	run := runs[0].(map[string]any)
	results, ok := run["results"].([]any)
	if !ok {
		t.Fatalf("results should be array, got %T", run["results"])
	}
	result := results[0].(map[string]any)
	if _, ok := result["ruleId"].(string); !ok {
		t.Errorf("ruleId should be string, got %T", result["ruleId"])
	}
	if _, ok := result["level"].(string); !ok {
		t.Errorf("level should be string, got %T", result["level"])
	}
	// locations must be array (not location object)
	locs, ok := result["locations"].([]any)
	if !ok {
		t.Fatalf("locations should be array, got %T", result["locations"])
	}
	region := locs[0].(map[string]any)["physicalLocation"].(map[string]any)["region"].(map[string]any)
	if _, ok := region["startLine"].(float64); !ok {
		t.Errorf("startLine should be number, got %T", region["startLine"])
	}
	if _, ok := region["endLine"].(float64); !ok {
		t.Errorf("endLine should be number, got %T", region["endLine"])
	}
	// Every replacement must carry a deletedRegion (required by schema).
	if fixes, ok := result["fixes"].([]any); ok {
		for _, f := range fixes {
			for _, ac := range f.(map[string]any)["artifactChanges"].([]any) {
				for _, r := range ac.(map[string]any)["replacements"].([]any) {
					if r.(map[string]any)["deletedRegion"] == nil {
						t.Error("replacement must have deletedRegion (required by SARIF schema)")
					}
				}
			}
		}
	}
}

// --- AC-14: JSON formatting (2-space indent, trailing newline) ---

func TestOutputSARIF_JSONFormatting(t *testing.T) {
	out := captureStdout(t, func() {
		if err := outputSARIF(nil, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output should end with newline, got last char: %q", out[len(out)-1:])
	}
	if !strings.Contains(out, "\n  \"") {
		t.Errorf("output should contain 2-space indented keys, got:\n%s", out)
	}
	mustUnmarshal(t, out)
}

// --- AC-15: Multiple comments ---

func TestOutputSARIF_MultipleComments(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "bug here", StartLine: 1, EndLine: 1, Category: "bug", Severity: "critical"},
		{Path: "b.go", Content: "slow query", StartLine: 10, EndLine: 20, Category: "performance", Severity: "medium"},
		{Path: "c.go", Content: "bad naming", StartLine: 5, EndLine: 5, Category: "style", Severity: "low"},
	}
	out := captureStdout(t, func() {
		if err := outputSARIF(comments, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)
	results := doc["runs"].([]any)[0].(map[string]any)["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("results length = %d, want 3", len(results))
	}
	expected := []struct {
		ruleID string
		level  string
	}{
		{"bug", "error"},
		{"performance", "warning"},
		{"style", "note"},
	}
	for i, exp := range expected {
		result := results[i].(map[string]any)
		if result["ruleId"] != exp.ruleID {
			t.Errorf("result[%d].ruleId = %v, want %q", i, result["ruleId"], exp.ruleID)
		}
		if result["level"] != exp.level {
			t.Errorf("result[%d].level = %v, want %q", i, result["level"], exp.level)
		}
	}
}

// --- newQuietHandle: sarif silences stdout ---

func TestNewQuietHandle_Sarif(t *testing.T) {
	h := newQuietHandle("sarif", "human")
	if h.fn == nil {
		t.Error("newQuietHandle should silence stdout for sarif format")
	}
	h.Restore() // restore so subsequent tests see real stdout
}

// --- Helper: sarifResultFromComment edge cases ---

func TestSarifResultFromComment_EmptyPath(t *testing.T) {
	c := model.LlmComment{Content: "test", Category: "bug", Severity: "high"}
	result := sarifResultFromComment(c)
	if result.Locations != nil {
		t.Errorf("Locations should be nil when Path is empty, got %+v", result.Locations)
	}
	if result.Fixes != nil {
		t.Errorf("Fixes should be nil when Path is empty, got %+v", result.Fixes)
	}
}

// Inverted line range (StartLine > EndLine) must omit region AND fixes
// (deletedRegion is required, so fixes cannot be emitted without a valid region).
func TestSarifResultFromComment_InvertedLineNumbers(t *testing.T) {
	c := model.LlmComment{
		Path:           "a.go",
		Content:        "test",
		SuggestionCode: "new code",
		ExistingCode:   "old code",
		StartLine:      10,
		EndLine:        5,
		Category:       "bug",
		Severity:       "high",
	}
	result := sarifResultFromComment(c)

	// Locations should exist (Path is non-empty) but region must be omitted.
	if result.Locations == nil {
		t.Fatal("Locations should exist when Path is non-empty")
	}
	if result.Locations[0].PhysicalLocation.Region != nil {
		t.Errorf("Region should be nil when StartLine > EndLine")
	}

	// Fixes must NOT exist — deletedRegion is required and cannot be constructed
	// from an inverted range.
	if result.Fixes != nil {
		t.Errorf("Fixes should be nil when line range is inverted (deletedRegion is required)")
	}
}

// Fixes must be omitted when Path is empty even if SuggestionCode and
// ExistingCode are non-empty, because fixes reference Path as the
// artifactLocation URI — an empty URI is structurally invalid for SARIF.
func TestSarifResultFromComment_FixesWithEmptyPath(t *testing.T) {
	c := model.LlmComment{
		Path:           "",
		Content:        "test",
		SuggestionCode: "new code",
		ExistingCode:   "old code",
		StartLine:      1,
		EndLine:        1,
		Category:       "bug",
		Severity:       "high",
	}
	result := sarifResultFromComment(c)
	if result.Locations != nil {
		t.Errorf("Locations should be nil when Path is empty")
	}
	if result.Fixes != nil {
		t.Errorf("Fixes should be nil when Path is empty")
	}
}

// --- partialFingerprints stability ---

func TestSarifFingerprints_Stable(t *testing.T) {
	c1 := model.LlmComment{Path: "a.go", Category: "bug", ExistingCode: "old code"}
	c2 := model.LlmComment{Path: "a.go", Category: "bug", ExistingCode: "old code"}
	// Different message text should NOT change the fingerprint.
	c3 := model.LlmComment{Path: "a.go", Category: "bug", ExistingCode: "old code", Content: "different message"}

	fp1 := sarifFingerprints(c1, "bug")
	fp2 := sarifFingerprints(c2, "bug")
	fp3 := sarifFingerprints(c3, "bug")

	if fp1[sarifFingerprintKey] != fp2[sarifFingerprintKey] {
		t.Error("same finding should produce same fingerprint")
	}
	if fp1[sarifFingerprintKey] != fp3[sarifFingerprintKey] {
		t.Error("fingerprint should not depend on message text")
	}
}

// When ExistingCode is empty, fingerprint must fall back to StartLine to
// avoid collisions between same-file, same-category findings.
func TestSarifFingerprints_EmptyExistingCodeFallback(t *testing.T) {
	// Two findings in the same file with the same category but different
	// line numbers — without the StartLine fallback they would collide.
	c1 := model.LlmComment{Path: "a.go", Category: "bug", ExistingCode: "", StartLine: 10}
	c2 := model.LlmComment{Path: "a.go", Category: "bug", ExistingCode: "", StartLine: 20}

	fp1 := sarifFingerprints(c1, "bug")
	fp2 := sarifFingerprints(c2, "bug")

	if fp1[sarifFingerprintKey] == fp2[sarifFingerprintKey] {
		t.Error("fingerprints should differ when ExistingCode is empty but StartLine differs")
	}

	// Same line + same file + same category + empty ExistingCode → same fingerprint
	c3 := model.LlmComment{Path: "a.go", Category: "bug", ExistingCode: "", StartLine: 10}
	fp3 := sarifFingerprints(c3, "bug")
	if fp1[sarifFingerprintKey] != fp3[sarifFingerprintKey] {
		t.Error("same finding (empty ExistingCode, same StartLine) should produce same fingerprint")
	}
}

// --- invocations: executionSuccessful derived from manifest ---

func TestSarifInvocation_ExecutionSuccessful(t *testing.T) {
	// No manifest (scan mode) → successful
	inv := sarifInvocationFromRun(nil, nil, 0)
	if !inv.ExecutionSuccessful {
		t.Error("executionSuccessful should be true when manifest is nil")
	}

	// Complete manifest → successful
	inv = sarifInvocationFromRun(nil, mockManifest(session.StateComplete), 0)
	if !inv.ExecutionSuccessful {
		t.Error("executionSuccessful should be true for StateComplete")
	}

	// Skipped manifest → successful (empty run, e.g. only excluded files)
	inv = sarifInvocationFromRun(nil, mockManifest(session.StateSkipped), 0)
	if !inv.ExecutionSuccessful {
		t.Error("executionSuccessful should be true for StateSkipped (not a failure)")
	}

	// Partial manifest → successful (budget truncation is publishable)
	inv = sarifInvocationFromRun(nil, mockManifest(session.StatePartial), 0)
	if !inv.ExecutionSuccessful {
		t.Error("executionSuccessful should be true for StatePartial (publishable outcome)")
	}

	// Failed manifest → not successful
	inv = sarifInvocationFromRun(nil, mockManifest(session.StateFailed), 0)
	if inv.ExecutionSuccessful {
		t.Error("executionSuccessful should be false for StateFailed")
	}
}

// --- invocations: warnings become toolExecutionNotifications ---

func TestSarifInvocation_WarningsAsNotifications(t *testing.T) {
	warnings := []agent.AgentWarning{
		{Type: "warning", File: "x.go", Message: "slow"},
		{Type: "subtask_error", File: "y.go", Message: "failed"},
	}

	// With manifest: subtask_error is filtered (already in coverage.failed).
	inv := sarifInvocationFromRun(warnings, mockManifest(session.StateComplete), 0)
	// 1 notification from warning ("slow"); subtask_error filtered;
	// StateComplete adds no manifest notification.
	if len(inv.ToolExecutionNotifications) != 1 {
		t.Fatalf("expected 1 notification with manifest (subtask_error filtered), got %d", len(inv.ToolExecutionNotifications))
	}
	notif := inv.ToolExecutionNotifications[0]
	if notif.Message.Text != "slow" {
		t.Errorf("notification message = %v", notif.Message.Text)
	}
	// Level must be a plain string, not a message object (SARIF spec).
	if notif.Level != "warning" {
		t.Errorf("notification level = %v, want \"warning\" (string)", notif.Level)
	}

	// Without manifest: subtask_error is the only record of failure, kept.
	inv = sarifInvocationFromRun(warnings, nil, 0)
	if len(inv.ToolExecutionNotifications) != 2 {
		t.Fatalf("expected 2 notifications without manifest (subtask_error kept), got %d", len(inv.ToolExecutionNotifications))
	}
}

// Non-complete manifest states add a manifest notification so consumers can
// distinguish "clean scan" from "partial/skipped".
func TestSarifInvocation_NonCompleteAddsManifestNotification(t *testing.T) {
	// Partial → 1 manifest notification
	inv := sarifInvocationFromRun(nil, mockManifest(session.StatePartial), 3)
	if len(inv.ToolExecutionNotifications) != 1 {
		t.Fatalf("expected 1 manifest notification for StatePartial, got %d", len(inv.ToolExecutionNotifications))
	}

	// Skipped → 1 manifest notification
	inv = sarifInvocationFromRun(nil, mockManifest(session.StateSkipped), 0)
	if len(inv.ToolExecutionNotifications) != 1 {
		t.Fatalf("expected 1 manifest notification for StateSkipped, got %d", len(inv.ToolExecutionNotifications))
	}

	// Complete → no manifest notification
	inv = sarifInvocationFromRun(nil, mockManifest(session.StateComplete), 5)
	if len(inv.ToolExecutionNotifications) != 0 {
		t.Errorf("expected 0 manifest notifications for StateComplete, got %d", len(inv.ToolExecutionNotifications))
	}
}

// Duplicate fingerprints (same file + category + ExistingCode) get an
// occurrence index appended so GitHub tracks them as separate alerts.
func TestSarifResults_DuplicateFingerprints(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "a.go", Content: "SQL injection 1", Category: "security", ExistingCode: "query('SELECT * FROM users WHERE id=' + id)", StartLine: 10, EndLine: 10},
		{Path: "a.go", Content: "SQL injection 2", Category: "security", ExistingCode: "query('SELECT * FROM users WHERE id=' + id)", StartLine: 20, EndLine: 20},
	}
	out := captureStdout(t, func() {
		if err := outputSARIF(comments, "v1", nil, nil); err != nil {
			t.Fatalf("outputSARIF: %v", err)
		}
	})
	doc := mustUnmarshal(t, out)
	results := doc["runs"].([]any)[0].(map[string]any)["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	fp0 := results[0].(map[string]any)["partialFingerprints"].(map[string]any)[sarifFingerprintKey].(string)
	fp1 := results[1].(map[string]any)["partialFingerprints"].(map[string]any)[sarifFingerprintKey].(string)

	// First occurrence keeps the base fingerprint; second gets "#1" appended.
	if fp0 == fp1 {
		t.Errorf("duplicate findings should have different fingerprints, both = %q", fp0)
	}
	if !strings.HasSuffix(fp1, "#1") {
		t.Errorf("second duplicate fingerprint should end with #1, got %q", fp1)
	}
}

// --preview --format sarif should error, not emit a non-SARIF document.
func TestOutputPreview_SarifRejects(t *testing.T) {
	p := &agent.DiffPreview{TotalFiles: 0}
	err := outputPreview(p, "sarif")
	if err == nil {
		t.Error("outputPreview should return an error for sarif format")
	}
}
