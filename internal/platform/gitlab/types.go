package gitlab

// ChangedFile represents a file changed in a merge request.
type ChangedFile struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	DeletedFile bool   `json:"deleted_file"`
	RenamedFile bool   `json:"renamed_file"`
}

// Discussion represents a GitLab MR discussion thread.
type Discussion struct {
	ID    string `json:"id"`
	Notes []Note `json:"notes"`
}

// Note represents a single note (comment) in a GitLab MR discussion.
type Note struct {
	ID     int    `json:"id"`
	Body   string `json:"body"`
	System bool   `json:"system"`
}

// Position represents a text position for inline discussions.
type Position struct {
	PositionType string `json:"position_type"`
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	NewPath      string `json:"new_path"`
	NewLine      int    `json:"new_line"`
}

// DiffVersion represents a GitLab MR diff version.
type DiffVersion struct {
	ID             int    `json:"id"`
	BaseCommitSHA  string `json:"base_commit_sha"`
	StartCommitSHA string `json:"start_commit_sha"`
	HeadCommitSHA  string `json:"head_commit_sha"`
}

// MergeRequest represents a minimal GitLab merge request.
type MergeRequest struct {
	IID         int    `json:"iid"`
	Description string `json:"description"`
}
