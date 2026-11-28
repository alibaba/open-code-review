// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package capacity defines the structured change-set capacity error returned
// before the first model request when a review or scan input exceeds its
// configured file-count or byte limits (#2125).
package capacity

import "fmt"

// Code is the stable machine-readable error code carried by every capacity
// rejection, regardless of command or stage. Callers branch on Stage for
// specifics; the code alone answers "was this rejected for size?".
const Code = "OCR_CAPACITY_EXCEEDED"

// Stage names the check that rejected the input.
const (
	StageReviewChangeset = "review_changeset_limits"
	StageScanChangeset   = "scan_changeset_limits"
)

// Error is a typed capacity rejection. It carries the stable error code, the
// configured limits, and the observed change-set size so automation can
// pre-reject with specific diagnostics instead of parsing timeouts or empty
// reports. All fields are always serialized (0 = that class had no configured
// limit) so consumers can rely on the shape.
type Error struct {
	Code           string `json:"code"`
	Stage          string `json:"stage"`
	RequestedFiles int    `json:"requested_files"`
	LimitFiles     int    `json:"limit_files"`
	RequestedBytes int64  `json:"requested_bytes"`
	LimitBytes     int64  `json:"limit_bytes"`
}

// Error renders the human-readable rejection, including actual-vs-limit for
// whatever class fired, so text-mode output is actionable without parsing JSON.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s rejected %d file(s) / %d bytes (limits: %d file(s) / %d bytes)",
		Code, e.Stage, e.RequestedFiles, e.RequestedBytes, e.LimitFiles, e.LimitBytes)
}

// Check returns a typed *Error when the change set strictly exceeds either
// configured limit. A limit of 0 means "no limit for that class" and never
// fires, so an unconfigured run behaves exactly as before.
func Check(stage string, requestedFiles, limitFiles int, requestedBytes, limitBytes int64) error {
	if limitFiles <= 0 && limitBytes <= 0 {
		return nil
	}
	if (limitFiles > 0 && requestedFiles > limitFiles) ||
		(limitBytes > 0 && requestedBytes > limitBytes) {
		return &Error{
			Code:           Code,
			Stage:          stage,
			RequestedFiles: requestedFiles,
			LimitFiles:     limitFiles,
			RequestedBytes: requestedBytes,
			LimitBytes:     limitBytes,
		}
	}
	return nil
}
