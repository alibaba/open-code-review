package model

// ScanItem represents a single file enumerated by full-scan mode. Unlike
// model.Diff (which carries a unified diff text), ScanItem carries the
// entire file content because scan reviews whole files with no diff
// context.
type ScanItem struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	IsBinary  bool   `json:"is_binary,omitempty"`
	LineCount int    `json:"line_count,omitempty"`
}

// AsDiff returns a Diff suitable for handing to code that expects the
// diff-based shape (line-number resolver, file_read_diff tool). The Diff
// field stays empty since scan mode has no unified diff; NewFileContent
// carries the whole file so resolver.resolveFromFileContent and similar
// fallbacks can still find the source lines.
func (s *ScanItem) AsDiff() *Diff {
	if s == nil {
		return nil
	}
	return &Diff{
		OldPath:        s.Path,
		NewPath:        s.Path,
		NewFileContent: s.Content,
		IsBinary:       s.IsBinary,
		Insertions:     int64(s.LineCount),
	}
}
