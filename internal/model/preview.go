package model

// ExcludeReason describes why a file was excluded from review. Shared by
// both diff review (internal/agent) and full-file scan (internal/scan).
type ExcludeReason string

const (
	ExcludeNone        ExcludeReason = ""
	ExcludeUserRule    ExcludeReason = "user_exclude"
	ExcludeExtension   ExcludeReason = "unsupported_ext"
	ExcludeDefaultPath ExcludeReason = "default_path"
	ExcludeDeleted     ExcludeReason = "deleted"
	ExcludeBinary      ExcludeReason = "binary"
)

// PreviewEntry is one file's preview record (mode-agnostic).
type PreviewEntry struct {
	Path          string        `json:"path"`
	Status        string        `json:"status"`
	Insertions    int64         `json:"insertions"`
	Deletions     int64         `json:"deletions"`
	WillReview    bool          `json:"will_review"`
	ExcludeReason ExcludeReason `json:"exclude_reason,omitempty"`
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
