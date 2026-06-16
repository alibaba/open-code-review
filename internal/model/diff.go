package model

// Diff represents a single file change in a git diff.
type Diff struct {
	OldPath        string `json:"old_path"`
	NewPath        string `json:"new_path"`
	Diff           string `json:"diff"`
	NewFileContent string `json:"new_file_content"`
	IsBinary       bool   `json:"is_binary"`
	IsDeleted      bool   `json:"is_deleted"`
	IsNew          bool   `json:"is_new"`
	IsRenamed      bool   `json:"is_renamed"`
	Insertions     int64  `json:"insertions"`
	Deletions      int64  `json:"deletions"`
}
