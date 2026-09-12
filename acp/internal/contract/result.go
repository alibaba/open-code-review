// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract

// This file models the subset of the OCR CLI JSON result envelope that the
// adapter needs. Field names and types mirror the real CLI output
// (jsonOutput in cmd/opencodereview/output.go and model.LlmComment in
// internal/model/review.go) so that decoding real `ocr --format json`
// output works. Unknown fields are ignored.
//
// `thinking` is deliberately not modeled. It is per-comment model reasoning
// that must never reach the user, and a field that does not exist on the DTO
// cannot leak by accident.
//
// Status values are an open set. Review and scan report different names, and
// a newer CLI may add more, so consumers must tolerate unknown values rather
// than fail on them.

// Known status values reported by the OCR CLI. Review uses the manifest
// terminal state (complete/partial/failed/skipped/completed_with_warnings/
// completed_with_errors) with `success` as the no-manifest fallback; scan has
// no manifest and uses success/completed_with_warnings/completed_with_errors/
// skipped.
const (
	StatusSuccess               = "success"
	StatusComplete              = "complete"
	StatusPartial               = "partial"
	StatusFailed                = "failed"
	StatusSkipped               = "skipped"
	StatusCompletedWithWarnings = "completed_with_warnings"
	StatusCompletedWithErrors   = "completed_with_errors"
)

// ReviewResult mirrors the JSON document `ocr review --format json` writes to
// stdout on a successful run. It is not a full mirror: only the fields the
// adapter consumes are modeled.
type ReviewResult struct {
	Status   string    `json:"status"`
	Message  string    `json:"message,omitempty"`
	Summary  *Summary  `json:"summary,omitempty"`
	Comments []Comment `json:"comments"` // may be null or []
	// Manifest is present for review but not for scan. The adapter reads
	// run_failure/coverage classifications from it to cross-check a local
	// cancel, because the CLI has no dedicated cancellation exit code.
	Manifest *Manifest `json:"manifest,omitempty"`
}

// ScanResult mirrors the JSON document `ocr scan --format json` writes to
// stdout. A scan has no run manifest, so its status is limited to the
// top-level fallback values; an empty or skipped result is still valid.
type ScanResult struct {
	Status   string    `json:"status"`
	Message  string    `json:"message,omitempty"`
	Summary  *Summary  `json:"summary,omitempty"`
	Comments []Comment `json:"comments"` // may be null or []
}

// Summary mirrors jsonSummary. Its fields are counts and timing only; the
// human-readable text lives in ReviewResult.Message.
type Summary struct {
	FilesReviewed int64  `json:"files_reviewed"`
	Comments      int64  `json:"comments"`
	TotalTokens   int64  `json:"total_tokens,omitempty"`
	Elapsed       string `json:"elapsed,omitempty"`
}

// Comment mirrors model.LlmComment. StartLine and EndLine are 1-indexed; when
// both are 0 the finding is file-level and the adapter must not invent a line
// number. Severity is an open set (critical, high, medium, low have been
// observed, but consumers must tolerate unknown values).
type Comment struct {
	Path           string `json:"path"`
	Content        string `json:"content"` // always emitted; mirrors model.LlmComment's tag
	SuggestionCode string `json:"suggestion_code,omitempty"`
	ExistingCode   string `json:"existing_code,omitempty"`
	StartLine      int    `json:"start_line"`
	EndLine        int    `json:"end_line"`
	Category       string `json:"category,omitempty"`
	Severity       string `json:"severity,omitempty"`
}

// Manifest mirrors the review run manifest (ocr.run-manifest/v1). Only the
// fields the adapter consults are modeled.
type Manifest struct {
	SchemaVersion string      `json:"schema_version,omitempty"`
	Operation     string      `json:"operation,omitempty"`
	TerminalState string      `json:"terminal_state,omitempty"`
	RunFailure    *RunFailure `json:"run_failure,omitempty"`
	Coverage      *Coverage   `json:"coverage,omitempty"`
}

// RunFailure is the run-level failure classification in a manifest.
type RunFailure struct {
	Classification string `json:"classification,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// Coverage is the manifest per-item coverage report.
type Coverage struct {
	Failed []CoverageItem `json:"failed,omitempty"`
}

// CoverageItem is one entry of manifest.coverage. Classification is the field
// that marks a cancelled item.
type CoverageItem struct {
	Path           string `json:"path,omitempty"`
	Classification string `json:"classification,omitempty"`
	Reason         string `json:"reason,omitempty"`
}
