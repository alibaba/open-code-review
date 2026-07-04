// Package reviewbundle builds versioned, deterministic review inputs for
// external agent control planes.
package reviewbundle

import "github.com/open-code-review/open-code-review/internal/model"

const (
	// BundleSchemaVersion identifies the review bundle protocol.
	BundleSchemaVersion = "agent-review-bundle/v1"
	// CommentsSchemaVersion identifies the external review comments protocol.
	CommentsSchemaVersion = "agent-review-comments/v1"
	// ValidationSchemaVersion identifies validated external comments.
	ValidationSchemaVersion = "agent-review-validation/v1"
)

// TargetMode identifies how the reviewed Git change is selected.
type TargetMode string

const (
	TargetWorkspace TargetMode = "workspace"
	TargetRange     TargetMode = "range"
	TargetCommit    TargetMode = "commit"
	TargetScan      TargetMode = "scan"
)

// Bundle is the complete deterministic input for an external reviewer.
type Bundle struct {
	SchemaVersion  string           `json:"schema_version"`
	BundleID       string           `json:"bundle_id"`
	Target         Target           `json:"target"`
	WorkspaceState *WorkspaceState  `json:"workspace_state,omitempty"`
	Summary        Summary          `json:"summary"`
	Rules          map[string]Rule  `json:"rules"`
	Files          []File           `json:"files"`
	Contract       Contract         `json:"contract"`
	Warnings       []ProtocolNotice `json:"warnings,omitempty"`
}

// Target records both requested refs and their resolved immutable identities.
type Target struct {
	Mode         TargetMode `json:"mode"`
	From         string     `json:"from"`
	To           string     `json:"to"`
	Commit       string     `json:"commit"`
	BaseSHA      string     `json:"base_sha"`
	HeadSHA      string     `json:"head_sha"`
	MergeBaseSHA string     `json:"merge_base_sha"`
	DiffSHA256   string     `json:"diff_sha256"`
}

// WorkspaceState fingerprints each component of a dirty working tree.
type WorkspaceState struct {
	HeadSHA         string `json:"head_sha"`
	StagedSHA256    string `json:"staged_sha256"`
	UnstagedSHA256  string `json:"unstaged_sha256"`
	UntrackedSHA256 string `json:"untracked_sha256"`
}

// Summary reports the complete target size and filtering outcome.
type Summary struct {
	TotalFiles      int   `json:"total_files"`
	ReviewableFiles int   `json:"reviewable_files"`
	ExcludedFiles   int   `json:"excluded_files"`
	Insertions      int64 `json:"insertions"`
	Deletions       int64 `json:"deletions"`
}

// Rule is a deduplicated resolved review rule.
type Rule struct {
	Source  string `json:"source"`
	Pattern string `json:"pattern"`
	Content string `json:"content"`
}

// File is one changed file and its review evidence.
type File struct {
	Path          string              `json:"path"`
	OldPath       string              `json:"old_path"`
	Status        string              `json:"status"`
	Reviewable    bool                `json:"reviewable"`
	ExcludeReason model.ExcludeReason `json:"exclude_reason,omitempty"`
	Insertions    int64               `json:"insertions"`
	Deletions     int64               `json:"deletions"`
	ContentSHA256 string              `json:"content_sha256"`
	RuleID        string              `json:"rule_id"`
	Patch         string              `json:"patch"`
	Content       string              `json:"content,omitempty"`
	Hunks         []Hunk              `json:"hunks"`
}

// Hunk records the old and new line bounds of one unified-diff hunk.
type Hunk struct {
	OldStart int `json:"old_start"`
	OldCount int `json:"old_count"`
	NewStart int `json:"new_start"`
	NewCount int `json:"new_count"`
}

// Contract tells an external reviewer which output protocol is accepted.
type Contract struct {
	CommentSchema      string   `json:"comment_schema"`
	LineNumbers        string   `json:"line_numbers"`
	AllowedPriorities  []string `json:"allowed_priorities"`
	AllowedCategories  []string `json:"allowed_categories"`
	MaxBundleBytes     int64    `json:"max_bundle_bytes"`
	BundleSizeBytes    int64    `json:"bundle_size_bytes"`
	RequiresReflection bool     `json:"requires_reflection"`
}

// DefaultContract returns the mandatory Phase 1 review-output constraints.
func DefaultContract() Contract {
	return Contract{
		CommentSchema:     CommentsSchemaVersion,
		LineNumbers:       "one_based_new_file",
		AllowedPriorities: []string{"high", "medium", "low"},
		AllowedCategories: []string{
			"bug",
			"security",
			"performance",
			"concurrency",
			"maintainability",
			"test",
		},
		RequiresReflection: true,
	}
}

// ProtocolNotice is a structured non-fatal protocol warning.
type ProtocolNotice struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Comments is the output protocol expected from an external reviewer.
type Comments struct {
	SchemaVersion string           `json:"schema_version"`
	BundleID      string           `json:"bundle_id"`
	Summary       CommentsSummary  `json:"summary"`
	Comments      []ReviewComment  `json:"comments"`
	Warnings      []ProtocolNotice `json:"warnings,omitempty"`
	sourceSHA256  string
}

// CommentsSummary reports the external review result size.
type CommentsSummary struct {
	FilesReviewed int `json:"files_reviewed"`
	IssuesFound   int `json:"issues_found"`
}

// ReviewComment is one line-level finding produced by the external reviewer.
type ReviewComment struct {
	Path             string  `json:"path"`
	StartLine        int     `json:"start_line"`
	EndLine          int     `json:"end_line"`
	Priority         string  `json:"priority"`
	Category         string  `json:"category"`
	Title            string  `json:"title"`
	Content          string  `json:"content"`
	Recommendation   string  `json:"recommendation"`
	ExistingCode     string  `json:"existing_code,omitempty"`
	SuggestionCode   string  `json:"suggestion_code,omitempty"`
	Confidence       float64 `json:"confidence"`
	FileLevelComment bool    `json:"file_level_comment,omitempty"`
}
