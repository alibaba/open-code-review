// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package gate evaluates saved review results without running a review or
// contacting Git, a model, or a hosting platform.
package gate

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

const SchemaVersion = "ocr.gate/v1"

type Status string

const (
	Pass         Status = "pass"
	Fail         Status = "fail"
	Inconclusive Status = "inconclusive"
)

// Policy describes checks in addition to the always-required coverage and
// delivery checks. Empty fields disable their respective optional checks.
type Policy struct {
	FailOnSeverity string `json:"fail_on_severity,omitempty"`
	ExpectedBase   string `json:"expected_base,omitempty"`
	ExpectedHead   string `json:"expected_head,omitempty"`
}

var commitID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

// Normalize rejects invalid policies before reading input. Expected revisions
// must be full object IDs; resolving mutable refs is the caller's responsibility.
func (p Policy) Normalize() (Policy, error) {
	p.FailOnSeverity = strings.ToLower(strings.TrimSpace(p.FailOnSeverity))
	if p.FailOnSeverity != "" && severityRank(p.FailOnSeverity) == 0 {
		return p, fmt.Errorf("fail-on-severity must be critical, high, medium, or low")
	}
	for _, expected := range []struct{ name, value string }{
		{"expected-base", p.ExpectedBase}, {"expected-head", p.ExpectedHead},
	} {
		if expected.value != "" && !commitID.MatchString(expected.value) {
			return p, fmt.Errorf("%s must be a full lowercase Git object ID (40 or 64 hex characters)", expected.name)
		}
	}
	return p, nil
}

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Result is separate from the review manifest: gate policy never rewrites the
// coverage-derived review status. A pass applies only to the selected input,
// not to excluded files or to a PR head that changes after evaluation.
type Result struct {
	SchemaVersion string  `json:"schema_version"`
	Status        Status  `json:"status"`
	RunID         string  `json:"run_id,omitempty"`
	Policy        Policy  `json:"policy"`
	Checks        []Check `json:"checks"`
}

func (r *Result) add(name string, status Status, code, message string) {
	r.Checks = append(r.Checks, Check{name, status, code, message})
	// An established policy violation takes precedence over insufficient
	// evidence. Keep all checks so an incomplete run's blocking findings remain
	// visible alongside its missing coverage or delivery evidence.
	if status == Fail || (status == Inconclusive && r.Status == Pass) {
		r.Status = status
	}
}

type report struct {
	Status    string               `json:"status"`
	Manifest  *session.RunManifest `json:"manifest"`
	Comments  json.RawMessage      `json:"comments"`
	ToolCalls *toolCalls           `json:"tool_calls"`
}

// Only consume delivery evidence, not raw tool arguments or error messages.
type toolCalls struct {
	Failure        *int64           `json:"failure"`
	FailureByTool  map[string]int64 `json:"failure_by_tool"`
	FailureDetails []struct {
		ToolName string `json:"tool_name"`
	} `json:"failure_details"`
}

// Evaluate reads one OCR JSON result. Invalid/missing evidence produces an
// inconclusive result; only an invalid policy returns an error. Additional
// result fields are tolerated, but unknown manifest versions never pass.
func Evaluate(input io.Reader, policy Policy) (Result, error) {
	p, err := policy.Normalize()
	if err != nil {
		return Result{}, err
	}
	r := Result{SchemaVersion: SchemaVersion, Status: Pass, Policy: p, Checks: []Check{}}
	var doc report
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&doc); err != nil {
		r.add("input", Inconclusive, "invalid_json", "Cannot read an OCR result JSON object.")
		return r, nil
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		r.add("input", Inconclusive, "invalid_json", "Expected exactly one OCR result JSON object.")
		return r, nil
	}
	if reason := validateReport(doc); reason != "" {
		r.add("input", Inconclusive, "invalid_report", reason)
		return r, nil
	}
	var comments []*model.LlmComment
	// The producer legitimately emits null for an empty comment slice. An
	// absent field is different: it may be a manifest without its findings.
	if len(doc.Comments) == 0 || json.Unmarshal(doc.Comments, &comments) != nil {
		r.add("input", Inconclusive, "missing_findings", "The result must contain a comments array or null.")
		return r, nil
	}
	for _, comment := range comments {
		if comment == nil {
			r.add("input", Inconclusive, "missing_findings", "Each comments entry must be a finding object, not null.")
			return r, nil
		}
	}
	r.RunID = doc.Manifest.RunID
	r.add("input", Pass, "valid_report", "The result has a supported, consistent review manifest.")
	evaluateCoverage(&r, doc.Manifest)
	for _, revision := range []struct{ name, expected, actual string }{
		{"base", p.ExpectedBase, doc.Manifest.Input.ResolvedBase},
		{"head", p.ExpectedHead, doc.Manifest.Input.ResolvedHead},
	} {
		if revision.expected == "" {
			continue
		}
		if doc.Manifest.Input.Mode == session.InputModeWorkspace || revision.actual != revision.expected {
			r.add(revision.name, Inconclusive, "revision_mismatch", "The reviewed "+revision.name+" does not match the expected immutable revision.")
		} else {
			r.add(revision.name, Pass, "revision_matches", "The reviewed "+revision.name+" matches the expected revision.")
		}
	}
	evaluateDelivery(&r, doc.ToolCalls)
	if p.FailOnSeverity != "" {
		evaluateSeverity(&r, comments, p.FailOnSeverity)
	}
	return r, nil
}

func validateReport(doc report) string {
	m := doc.Manifest
	if m == nil || m.SchemaVersion != session.ManifestSchemaVersion {
		return "A supported review manifest is required; legacy results cannot establish coverage."
	}
	if m.Operation != session.OperationReview || strings.TrimSpace(m.RunID) == "" {
		return "The manifest must identify a review operation and a run."
	}
	switch m.Input.Mode {
	case session.InputModeWorkspace, session.InputModeCommit, session.InputModeRange:
	default:
		return "The manifest input mode is unsupported."
	}
	c := m.Coverage
	sets := [][]session.CoverageItem{c.Selected, c.Completed, c.Reused, c.Failed, c.Waived}
	for _, items := range sets {
		if items == nil {
			return "All coverage arrays must be present; missing or null arrays are not evidence of zero failures."
		}
	}
	selected := make(map[string]session.CoverageItem, len(c.Selected))
	for _, item := range c.Selected {
		if item.ItemID == "" || item.Path == "" {
			return "Selected coverage items must identify a file and an item ID."
		}
		if _, exists := selected[item.ItemID]; exists {
			return "Selected coverage contains duplicate item IDs."
		}
		selected[item.ItemID] = item
	}
	seen := make(map[string]bool, len(selected))
	for _, items := range sets[1:] {
		for _, item := range items {
			original, exists := selected[item.ItemID]
			if !exists || seen[item.ItemID] || original.Path != item.Path || original.OldPath != item.OldPath || original.Fingerprint != item.Fingerprint {
				return "Coverage outcomes must be disjoint and match their selected items."
			}
			seen[item.ItemID] = true
		}
	}
	if len(seen) != len(selected) {
		return "Some selected items have no coverage outcome."
	}
	expected := session.StateComplete
	switch {
	case m.RunFailure != nil:
		expected = session.StateFailed
	case len(selected) == 0:
		expected = session.StateSkipped
	case len(c.Failed) == len(selected):
		expected = session.StateFailed
	case len(c.Failed) > 0:
		expected = session.StatePartial
	}
	if m.TerminalState != expected || doc.Status != string(expected) {
		return "The result status and manifest terminal state must agree with the coverage outcomes."
	}
	return ""
}

func evaluateCoverage(r *Result, m *session.RunManifest) {
	c := m.Coverage
	switch {
	case m.RunFailure != nil:
		r.add("coverage", Inconclusive, "run_failed", "The review recorded a run-level failure.")
	case len(c.Selected) == 0:
		r.add("coverage", Inconclusive, "no_selected_items", "No files were selected; this manifest cannot distinguish an empty change from excluded changes.")
	case len(c.Failed) > 0 || len(c.Waived) > 0:
		r.add("coverage", Inconclusive, "incomplete_coverage", fmt.Sprintf("%d selected item(s) failed and %d were waived; complete coverage is required, including budget stops.", len(c.Failed), len(c.Waived)))
	default:
		r.add("coverage", Pass, "complete_coverage", fmt.Sprintf("All %d selected item(s) completed or were reused (%d reused).", len(c.Selected), len(c.Reused)))
	}
}

func evaluateDelivery(r *Result, calls *toolCalls) {
	invalid := func() {
		r.add("delivery", Inconclusive, "missing_delivery_evidence", "Tool failure counts and details are missing or inconsistent.")
	}
	if calls == nil || calls.Failure == nil || calls.FailureByTool == nil || calls.FailureDetails == nil || *calls.Failure != int64(len(calls.FailureDetails)) {
		invalid()
		return
	}
	counts := make(map[string]int64)
	for _, failure := range calls.FailureDetails {
		if strings.TrimSpace(failure.ToolName) == "" {
			invalid()
			return
		}
		counts[failure.ToolName]++
	}
	for name, count := range calls.FailureByTool {
		if count <= 0 || count != counts[name] {
			invalid()
			return
		}
	}
	for name, count := range counts {
		if calls.FailureByTool[name] != count {
			invalid()
			return
		}
	}
	if counts["code_comment"] > 0 {
		r.add("delivery", Inconclusive, "comment_delivery_unverified", "A code_comment call failed; this result does not prove that the same findings were subsequently delivered.")
		return
	}
	r.add("delivery", Pass, "no_comment_delivery_failure", "No code_comment failures were recorded; exploration failures alone do not block this check.")
}

func evaluateSeverity(r *Result, comments []*model.LlmComment, threshold string) {
	blocking, unknown := 0, 0
	for _, comment := range comments {
		rank := severityRank(comment.Severity)
		if rank == 0 {
			unknown++
		} else if rank >= severityRank(threshold) {
			blocking++
		}
	}
	message := fmt.Sprintf("%d finding(s) meet the %s threshold; %d have missing or unknown severity.", blocking, threshold, unknown)
	switch {
	case blocking > 0:
		r.add("severity", Fail, "severity_threshold", message)
	case unknown > 0:
		r.add("severity", Inconclusive, "unknown_severity", message)
	default:
		r.add("severity", Pass, "below_threshold", message)
	}
}
