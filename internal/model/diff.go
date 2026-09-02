// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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

	// UndecodedCharset is the charset detection's answer for a file whose
	// bytes could NOT be turned into UTF-8 text. It is set whenever decoding
	// was attempted and skipped, whether or not the file is still reviewed,
	// so a consumer of the JSON output can always tell that the diff text it
	// is reading may contain replacement characters.
	UndecodedCharset string `json:"undecoded_charset,omitempty"`
	// Unreviewable marks a file whose raw bytes are past reviewing as text —
	// binary, or so much of it would render as U+FFFD that a review of it is
	// worthless. A merely imperfect file (a stray smart quote, a handful of
	// Latin-1 accents) is marked via UndecodedCharset but stays reviewable.
	Unreviewable bool `json:"unreviewable,omitempty"`
}
