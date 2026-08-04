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

	"github.com/alibaba/open-code-review/internal/model"
)

// ResumeState is the replayed, read-only checkpoint index for one prior session.
type ResumeState struct {
	SessionID        string
	RepoDir          string
	GitBranch        string
	Model            string
	ReviewMode       string
	DiffFrom         string
	DiffTo           string
	DiffCommit       string
	ScanPaths        []string
	HasScanPathScope bool
	Items            map[string]ResumeItem
}

// ResumeItem is a completed file-level checkpoint, keyed by diff fingerprint.
type ResumeItem struct {
	FilePath    string
	OldPath     string
	NewPath     string
	Fingerprint string
	Comments    []model.LlmComment
}

type resumeRecord struct {
	Type            string             `json:"type"`
	SessionID       string             `json:"sessionId"`
	Cwd             string             `json:"cwd"`
	GitBranch       string             `json:"gitBranch"`
	Model           string             `json:"model"`
	ReviewMode      string             `json:"reviewMode"`
	DiffFrom        string             `json:"diffFrom"`
	DiffTo          string             `json:"diffTo"`
	DiffCommit      string             `json:"diffCommit"`
	ScanPaths       *[]string          `json:"scanPaths"`
	FilePath        string             `json:"filePath"`
	OldPath         string             `json:"oldPath"`
	NewPath         string             `json:"newPath"`
	Fingerprint     string             `json:"fingerprint"`
	SourceSessionID string             `json:"sourceSessionId"`
	Error           string             `json:"error"`
	Comments        []model.LlmComment `json:"comments"`
}

// SessionFilePath returns the JSONL path for a persisted session.
func SessionFilePath(repoDir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", sessionSubDir, encodeRepoPath(repoDir), sessionID+".jsonl"), nil
}

// LoadResumeState replays a previous session JSONL into a fingerprint index.
func LoadResumeState(repoDir, sessionID string) (*ResumeState, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open resume session %q: %w", sessionID, err)
	}
	defer f.Close()

	state := &ResumeState{
		SessionID: sessionID,
		RepoDir:   repoDir,
		Items:     make(map[string]ResumeItem),
	}
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if err := state.applyResumeLine(line); err != nil {
				return nil, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read resume session %q: %w", sessionID, readErr)
		}
	}
	if state.SessionID == "" {
		state.SessionID = sessionID
	}
	return state, nil
}

func (s *ResumeState) applyResumeLine(line []byte) error {
	var rec resumeRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return fmt.Errorf("parse resume session %q: %w", s.SessionID, err)
	}

	switch rec.Type {
	case "session_start":
		s.applySessionStart(rec)
	case "review_item_done", "review_item_reused":
		if rec.Fingerprint == "" {
			return nil
		}
		filePath := rec.FilePath
		if filePath == "" {
			filePath = rec.NewPath
		}
		s.Items[rec.Fingerprint] = ResumeItem{
			FilePath:    filePath,
			OldPath:     rec.OldPath,
			NewPath:     rec.NewPath,
			Fingerprint: rec.Fingerprint,
			Comments:    copyLlmComments(rec.Comments),
		}
	case "review_item_failed":
		if rec.Fingerprint != "" {
			delete(s.Items, rec.Fingerprint)
		}
	}
	return nil
}

func (s *ResumeState) applySessionStart(rec resumeRecord) {
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
	if rec.ScanPaths != nil {
		s.ScanPaths = normalizeScanPaths(*rec.ScanPaths)
		s.HasScanPathScope = true
	}
}

// CompletedCount returns the number of reusable file-level checkpoints.
func (s *ResumeState) CompletedCount() int {
	if s == nil {
		return 0
	}
	return len(s.Items)
}

// Item returns a copy of the checkpoint for fingerprint.
func (s *ResumeState) Item(fingerprint string) (ResumeItem, bool) {
	if s == nil {
		return ResumeItem{}, false
	}
	item, ok := s.Items[fingerprint]
	if !ok {
		return ResumeItem{}, false
	}
	item.Comments = copyLlmComments(item.Comments)
	return item, true
}

// ValidateOptions verifies that the requested review range matches the prior session.
func (s *ResumeState) ValidateOptions(opts SessionOptions) error {
	if s == nil {
		return nil
	}
	if opts.ReviewMode == "" || opts.ReviewMode == ReviewModeWorkspace {
		return fmt.Errorf("resume requires --from/--to or --commit; workspace resume is not supported")
	}
	if s.ReviewMode == "" {
		return fmt.Errorf("resume session %q is missing review mode metadata", s.SessionID)
	}
	if s.ReviewMode != opts.ReviewMode {
		return fmt.Errorf("resume session review mode %q does not match current mode %q", s.ReviewMode, opts.ReviewMode)
	}
	switch opts.ReviewMode {
	case ReviewModeRange:
		if s.DiffFrom != opts.DiffFrom || s.DiffTo != opts.DiffTo {
			return fmt.Errorf("resume session range %q..%q does not match current range %q..%q", s.DiffFrom, s.DiffTo, opts.DiffFrom, opts.DiffTo)
		}
	case ReviewModeCommit:
		if s.DiffCommit != opts.DiffCommit {
			return fmt.Errorf("resume session commit %q does not match current commit %q", s.DiffCommit, opts.DiffCommit)
		}
	default:
		return fmt.Errorf("resume mode %q is not supported", opts.ReviewMode)
	}
	return nil
}

// ValidateScanOptions verifies that the previous session was a full-file scan.
func (s *ResumeState) ValidateScanOptions(scanPaths []string) error {
	if s == nil {
		return nil
	}
	if s.ReviewMode == "" {
		return fmt.Errorf("resume session %q is missing review mode metadata", s.SessionID)
	}
	if s.ReviewMode != ReviewModeFullScan {
		return fmt.Errorf("resume session review mode %q does not match current mode %q", s.ReviewMode, ReviewModeFullScan)
	}
	current := normalizeScanPaths(scanPaths)
	if s.HasScanPathScope && !equalStringSlices(s.ScanPaths, current) {
		return fmt.Errorf("resume session scan path scope %q does not match current scope %q", formatScanScope(s.ScanPaths), formatScanScope(current))
	}
	return nil
}

func normalizeScanPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimSuffix(filepath.ToSlash(p), "/")
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatScanScope(paths []string) string {
	if len(paths) == 0 {
		return "<whole repo>"
	}
	return strings.Join(paths, ",")
}

func copyLlmComments(in []model.LlmComment) []model.LlmComment {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.LlmComment, len(in))
	copy(out, in)
	return out
}
