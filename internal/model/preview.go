// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package model

// ExcludeReason describes why a file was excluded from review. Shared by
// both diff review (internal/agent) and full-file scan (internal/scan).
type ExcludeReason string

const (
	ExcludeNone        ExcludeReason = ""
	ExcludeUserRule    ExcludeReason = "user_exclude"
	ExcludeExtension   ExcludeReason = "unsupported_ext"
	ExcludeDefaultPath ExcludeReason = "default_path"
	// ExcludeSecret is a built-in credential path (for example .env or
	// id_rsa). Like provider_directory and unlike default_path, an include
	// rule cannot make the file reviewable.
	ExcludeSecret ExcludeReason = "secret_exclude"
	// ExcludeProviderDirectory is an unconditional diff-provider directory
	// exclusion (for example vendor/ or node_modules/). Unlike default_path,
	// an include rule cannot make the file reviewable.
	ExcludeProviderDirectory ExcludeReason = "provider_directory"
	ExcludeDeleted           ExcludeReason = "deleted"
	ExcludeBinary            ExcludeReason = "binary"
	ExcludeTooLarge          ExcludeReason = "too_large"
	// ExcludeUndecodable marks a file whose bytes are not valid text in any
	// charset we can decode AND are too far gone to review raw. A file that is
	// merely imperfect — a stray smart quote, a handful of Latin-1 accents — is
	// NOT excluded: it keeps the review it gets today and is only marked.
	ExcludeUndecodable ExcludeReason = "undecodable_encoding"
)

// PreviewEntry is one file's preview record (mode-agnostic).
type PreviewEntry struct {
	Path          string        `json:"path"`
	Status        string        `json:"status"`
	Insertions    int64         `json:"insertions"`
	Deletions     int64         `json:"deletions"`
	WillReview    bool          `json:"will_review"`
	ExcludeReason ExcludeReason `json:"exclude_reason,omitempty"`
	// DetectedCharset carries the charset detection's answer for any file the
	// decode step could not turn into UTF-8, whether that got the file excluded
	// as ExcludeUndecodable or left it marked and still reviewed. Empty for
	// files that decoded cleanly, so clean previews are unchanged.
	DetectedCharset string `json:"detected_charset,omitempty"`
}

// Preview is the full preview result, mode-agnostic so cmd/opencodereview
// can render it the same way for review and scan.
type Preview struct {
	Entries         []PreviewEntry `json:"files"`
	TotalInsertions int64          `json:"total_insertions"`
	TotalDeletions  int64          `json:"total_deletions"`
	TotalFiles      int            `json:"total_files"`
	ReviewableCount int            `json:"reviewable_count"`
	ExcludedCount   int            `json:"excluded_count"`
}
