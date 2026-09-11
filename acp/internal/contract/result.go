// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract

// ReviewResult represents the JSON output from `ocr review --format json`.
// Based on CLI Contract from phase 1 investigation.
type ReviewResult struct {
	// Status values observed in phase 1: "completed", "partial", "failed".
	// "cancelled" is a reserved value for adapter-side cancellation; the OCR CLI
	// itself reports cancellation as a partial result or non-zero exit.
	Status   string    `json:"status"`
	Summary  string    `json:"summary"`  // Overall summary
	Comments []Comment `json:"comments"` // Can be null
	Thinking string    `json:"thinking"` // Internal reasoning, not for display
}

// ScanResult represents the JSON output from `ocr scan --format json`.
type ScanResult struct {
	// Status values observed in phase 1: "completed", "skipped".
	// A scan with no manifest produces a valid empty result, not an error.
	Status   string    `json:"status"`
	Comments []Comment `json:"comments"` // Can be null
}

// Comment represents a single finding in the review or scan output.
// Severity is an open set: the phase 1 contract documents "error", "warning"
// and "info", but consumers must tolerate unknown values without failing.
type Comment struct {
	Path     string `json:"path"`               // Relative to target root
	Line     int    `json:"line"`               // 1-indexed; 0 means file-level
	Message  string `json:"message"`            // Finding description
	Severity string `json:"severity,omitempty"` // "error", "warning", "info"
}
