package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Summary is a compact digest of one persisted session, suitable for
// listing recent runs.
type Summary struct {
	SessionID      string        `json:"session_id"`
	FilePath       string        `json:"file_path"`
	RepoDir        string        `json:"repo_dir"`
	GitBranch      string        `json:"git_branch,omitempty"`
	Model          string        `json:"model,omitempty"`
	ReviewMode     string        `json:"review_mode,omitempty"`
	DiffFrom       string        `json:"diff_from,omitempty"`
	DiffTo         string        `json:"diff_to,omitempty"`
	DiffCommit     string        `json:"diff_commit,omitempty"`
	ResumedFrom    string        `json:"resumed_from,omitempty"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time,omitempty"`
	Duration       time.Duration `json:"duration_ns,omitempty"`
	CompletedFiles int           `json:"completed_files"`
	FailedFiles    int           `json:"failed_files"`
	ReusedFiles    int           `json:"reused_files"`
	TotalComments  int           `json:"total_comments"`
	LLMFailures    int64         `json:"llm_failures"`
	Aborted        bool          `json:"aborted"`
}

// ItemDetail describes one file-level record within a session, used by `ocr session show`.
type ItemDetail struct {
	Type            string    `json:"type"`
	Timestamp       time.Time `json:"timestamp"`
	FilePath        string    `json:"file_path"`
	OldPath         string    `json:"old_path,omitempty"`
	NewPath         string    `json:"new_path,omitempty"`
	Fingerprint     string    `json:"fingerprint,omitempty"`
	Comments        int       `json:"comments"`
	SourceSessionID string    `json:"source_session_id,omitempty"`
	Error           string    `json:"error,omitempty"`
}

// summaryRecord is a superset of resumeRecord that also carries session_end fields.
type summaryRecord struct {
	Type            string          `json:"type"`
	SessionID       string          `json:"sessionId"`
	Timestamp       string          `json:"timestamp"`
	Cwd             string          `json:"cwd"`
	GitBranch       string          `json:"gitBranch"`
	Model           string          `json:"model"`
	ReviewMode      string          `json:"reviewMode"`
	DiffFrom        string          `json:"diffFrom"`
	DiffTo          string          `json:"diffTo"`
	DiffCommit      string          `json:"diffCommit"`
	ResumedFrom     string          `json:"resumedFrom"`
	FilePath        string          `json:"filePath"`
	OldPath         string          `json:"oldPath"`
	NewPath         string          `json:"newPath"`
	Fingerprint     string          `json:"fingerprint"`
	SourceSessionID string          `json:"sourceSessionId"`
	Error           string          `json:"error"`
	Comments        json.RawMessage `json:"comments"`
	FilesReviewed   []string        `json:"files_reviewed"`
	DurationSeconds float64         `json:"duration_seconds"`
	LLMFailures     int64           `json:"llm_failures"`
}

// SessionsDir returns the on-disk directory that holds JSONL session files
// for a given repository. It does not create the directory.
func SessionsDir(repoDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", sessionSubDir, encodeRepoPath(repoDir)), nil
}

// ListSessions enumerates all persisted sessions for the given repository
// directory, sorted by StartTime descending (most recent first). Missing
// directories return an empty slice with no error.
func ListSessions(repoDir string) ([]Summary, error) {
	dir, err := SessionsDir(repoDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir %q: %w", dir, err)
	}
	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".jsonl")
		summary, err := loadSummaryFromFile(filepath.Join(dir, name), sessionID, repoDir)
		if err != nil {
			continue
		}
		summaries = append(summaries, *summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].StartTime.After(summaries[j].StartTime)
	})
	return summaries, nil
}

// LoadSummary loads a single session's Summary. Errors when the session
// file is missing or unreadable.
func LoadSummary(repoDir, sessionID string) (*Summary, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	return loadSummaryFromFile(path, sessionID, repoDir)
}

// LoadDetail returns the summary plus per-file item records for one session.
func LoadDetail(repoDir, sessionID string) (*Summary, []ItemDetail, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, nil, err
	}
	summary := &Summary{
		SessionID: sessionID,
		FilePath:  path,
		RepoDir:   repoDir,
		Aborted:   true,
	}
	var items []ItemDetail
	err = walkSessionFile(path, func(rec summaryRecord) {
		applyRecordToSummary(summary, rec)
		if item, ok := recordToItem(rec); ok {
			items = append(items, item)
		}
	})
	if err != nil {
		return nil, nil, err
	}
	if summary.SessionID == "" {
		summary.SessionID = sessionID
	}
	return summary, items, nil
}

func loadSummaryFromFile(path, sessionID, repoDir string) (*Summary, error) {
	summary := &Summary{
		SessionID: sessionID,
		FilePath:  path,
		RepoDir:   repoDir,
		Aborted:   true,
	}
	if err := walkSessionFile(path, func(rec summaryRecord) {
		applyRecordToSummary(summary, rec)
	}); err != nil {
		return nil, err
	}
	if summary.SessionID == "" {
		summary.SessionID = sessionID
	}
	return summary, nil
}

func walkSessionFile(path string, apply func(summaryRecord)) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open session %q: %w", path, err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var rec summaryRecord
			if err := json.Unmarshal(line, &rec); err == nil {
				apply(rec)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read session %q: %w", path, readErr)
		}
	}
}

func applyRecordToSummary(s *Summary, rec summaryRecord) {
	ts := parseRecordTime(rec.Timestamp)
	switch rec.Type {
	case "session_start":
		if rec.SessionID != "" {
			s.SessionID = rec.SessionID
		}
		if rec.Cwd != "" {
			s.RepoDir = rec.Cwd
		}
		s.GitBranch = rec.GitBranch
		s.Model = rec.Model
		s.ReviewMode = rec.ReviewMode
		s.DiffFrom = rec.DiffFrom
		s.DiffTo = rec.DiffTo
		s.DiffCommit = rec.DiffCommit
		s.ResumedFrom = rec.ResumedFrom
		if !ts.IsZero() {
			s.StartTime = ts
		}
	case "review_item_done":
		s.CompletedFiles++
		s.TotalComments += countCommentsRaw(rec.Comments)
	case "review_item_reused":
		s.ReusedFiles++
		s.TotalComments += countCommentsRaw(rec.Comments)
	case "review_item_failed":
		s.FailedFiles++
	case "session_end":
		s.Aborted = false
		if s.CompletedFiles == 0 && s.ReusedFiles == 0 && s.FailedFiles == 0 && len(rec.FilesReviewed) > 0 {
			s.CompletedFiles = len(rec.FilesReviewed)
		}
		if !ts.IsZero() {
			s.EndTime = ts
		}
		if rec.DurationSeconds > 0 {
			s.Duration = time.Duration(rec.DurationSeconds * float64(time.Second))
		} else if !s.EndTime.IsZero() && !s.StartTime.IsZero() {
			s.Duration = s.EndTime.Sub(s.StartTime)
		}
		s.LLMFailures = rec.LLMFailures
	}
}

func recordToItem(rec summaryRecord) (ItemDetail, bool) {
	switch rec.Type {
	case "review_item_done", "review_item_reused", "review_item_failed":
	default:
		return ItemDetail{}, false
	}
	kind := strings.TrimPrefix(rec.Type, "review_item_")
	filePath := rec.FilePath
	if filePath == "" {
		filePath = rec.NewPath
	}
	return ItemDetail{
		Type:            kind,
		Timestamp:       parseRecordTime(rec.Timestamp),
		FilePath:        filePath,
		OldPath:         rec.OldPath,
		NewPath:         rec.NewPath,
		Fingerprint:     rec.Fingerprint,
		Comments:        countCommentsRaw(rec.Comments),
		SourceSessionID: rec.SourceSessionID,
		Error:           rec.Error,
	}, true
}

func countCommentsRaw(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return 0
	}
	return len(arr)
}

func parseRecordTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}
