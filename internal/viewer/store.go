// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package viewer provides a read-only WebUI for browsing session records
// produced by open-code-review runs. It scans JSONL files under
// $HOME/.opencodereview/sessions/, parses them, and exposes structured data.
package viewer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/session"
)

// SessionsRoot returns the root directory where session JSONL files are stored.
func SessionsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "sessions"), nil
}

// RepoInfo represents a discovered repository from the sessions directory.
type RepoInfo struct {
	EncodedPath  string // encoded directory name on disk
	SessionCount int
	LastModified time.Time
}

// DiscoverRepos walks the sessions root and returns one entry per repository.
// Legacy session directories may contain records for multiple repositories
// because their path encoding was not collision-resistant; those records are
// split by the cwd recorded in each session_start event.
func DiscoverRepos(root string) ([]RepoInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	repoGroups := make(map[string]*RepoInfo)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoDir := filepath.Join(root, e.Name())
		subEntries, err := os.ReadDir(repoDir)
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() || !strings.HasSuffix(se.Name(), ".jsonl") {
				continue
			}
			key := e.Name()
			if !isVersionedRepoKey(key) {
				cwd, readErr := readSessionCWD(filepath.Join(repoDir, se.Name()))
				if readErr != nil {
					continue
				}
				if cwd != "" {
					key = session.RepoSessionKey(cwd)
				}
			}
			info := repoGroups[key]
			if info == nil {
				info = &RepoInfo{EncodedPath: key}
				repoGroups[key] = info
			}
			info.SessionCount++
			if fi, statErr := se.Info(); statErr == nil && fi.ModTime().After(info.LastModified) {
				info.LastModified = fi.ModTime()
			}
		}
	}

	var repos []RepoInfo
	for _, info := range repoGroups {
		if info.SessionCount > 0 {
			repos = append(repos, *info)
		}
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].LastModified.After(repos[j].LastModified)
	})
	return repos, nil
}

// SessionSummary is built from session_start and session_end records.
type SessionSummary struct {
	SessionID      string
	Timestamp      time.Time
	CWD            string
	GitBranch      string
	Model          string
	ReviewMode     string
	DiffFrom       string
	DiffTo         string
	DiffCommit     string
	FilesReviewed  []string
	DurationSec    float64
	FileCount      int
	LLMFailures    int
	CommentCount   int
	Aborted        bool
	Legacy         bool
	TerminalState  string
	SelectedCount  int
	CompletedCount int
	ReusedCount    int
	FailedCount    int
	WaivedCount    int
	RunManifest    *session.RunManifest
}

// ListSessions returns lightweight summaries for all sessions in a repo subdir.
func ListSessions(root, encodedRepo string) ([]SessionSummary, error) {
	repoDir := filepath.Join(root, encodedRepo)
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read repo dir: %w", err)
		}
		if isVersionedRepoKey(encodedRepo) {
			return listLegacySessions(root, encodedRepo)
		}
		return nil, fmt.Errorf("read repo dir: %w", err)
	}

	var summaries []SessionSummary
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
		s, err := peekSession(filepath.Join(repoDir, e.Name()))
		if err != nil {
			continue // skip unreadable files
		}
		s.SessionID = sessionID
		summaries = append(summaries, s)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Timestamp.After(summaries[j].Timestamp)
	})
	if isVersionedRepoKey(encodedRepo) {
		legacy, legacyErr := listLegacySessions(root, encodedRepo)
		if legacyErr != nil {
			return nil, legacyErr
		}
		summaries = append(summaries, legacy...)
		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].Timestamp.After(summaries[j].Timestamp)
		})
	}
	return summaries, nil
}

func isVersionedRepoKey(encodedRepo string) bool {
	return session.IsRepoSessionKey(encodedRepo)
}

// readSessionCWD reads only through the session_start record. Repository
// discovery and legacy lookup need the owning repository, not the potentially
// large request and response records that follow it.
func readSessionCWD(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		var rec struct {
			Type string `json:"type"`
			CWD  string `json:"cwd"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.Type == "session_start" {
			return rec.CWD, nil
		}
		if readErr == io.EOF {
			return "", nil
		}
		if readErr != nil {
			return "", readErr
		}
	}
}

func listLegacySessions(root, encodedRepo string) ([]SessionSummary, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read sessions root: %w", err)
	}
	var summaries []SessionSummary
	for _, dirEntry := range entries {
		if !dirEntry.IsDir() {
			continue
		}
		// Current-format directories are already read directly. Scanning them
		// here would duplicate their sessions in the legacy compatibility pass.
		if isVersionedRepoKey(dirEntry.Name()) {
			continue
		}
		dir := filepath.Join(root, dirEntry.Name())
		files, readErr := os.ReadDir(dir)
		if readErr != nil {
			continue
		}
		for _, fileEntry := range files {
			if fileEntry.IsDir() || !strings.HasSuffix(fileEntry.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(dir, fileEntry.Name())
			summary, peekErr := peekSession(path)
			if peekErr != nil || summary.CWD == "" || session.RepoSessionKey(summary.CWD) != encodedRepo {
				continue
			}
			summary.SessionID = strings.TrimSuffix(fileEntry.Name(), ".jsonl")
			summaries = append(summaries, summary)
		}
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Timestamp.After(summaries[j].Timestamp)
	})
	return summaries, nil
}

// peekSession reads the first record, all review_item records (for comment
// count), and the last record of a JSONL file.
func peekSession(path string) (SessionSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer f.Close()

	summary := SessionSummary{Aborted: true}
	var lastLine []byte
	readErr := readJSONLLines(f, func(line []byte) {
		lastLine = append([]byte(nil), line...)

		if summary.Timestamp.IsZero() {
			var rec map[string]any
			if err := json.Unmarshal(line, &rec); err != nil {
				return
			}
			if ts, ok := rec["timestamp"].(string); ok {
				summary.Timestamp, _ = time.Parse(time.RFC3339, ts)
			}
			if cwd, ok := rec["cwd"].(string); ok {
				summary.CWD = cwd
			}
			if branch, ok := rec["gitBranch"].(string); ok {
				summary.GitBranch = branch
			}
			if model, ok := rec["model"].(string); ok {
				summary.Model = model
			}
			if rm, ok := rec["reviewMode"].(string); ok {
				summary.ReviewMode = rm
			}
			if v, ok := rec["diffFrom"].(string); ok {
				summary.DiffFrom = v
			}
			if v, ok := rec["diffTo"].(string); ok {
				summary.DiffTo = v
			}
			if v, ok := rec["diffCommit"].(string); ok {
				summary.DiffCommit = v
			}
			return
		}

		// Count comments from review_item_done/reused records
		if len(line) > 0 && (bytes.Contains(line, []byte(`"review_item_done"`)) || bytes.Contains(line, []byte(`"review_item_reused"`))) {
			var rec map[string]any
			if err := json.Unmarshal(line, &rec); err == nil {
				if comments, ok := rec["comments"].([]any); ok {
					summary.CommentCount += len(comments)
				}
			}
		}
	})

	if len(lastLine) > 0 {
		var rec map[string]any
		if err := json.Unmarshal(lastLine, &rec); err == nil {
			if typ, _ := rec["type"].(string); typ == "session_end" {
				applySessionEnd(&summary, rec)
			}
		}
	}
	return summary, readErr
}

// readJSONLLines visits each physical JSONL record without bufio.Scanner's
// fixed token ceiling. session_end embeds the complete run manifest and can
// legitimately exceed the former 10 MiB scanner limit on very large reviews.
func readJSONLLines(r io.Reader, visit func([]byte)) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		switch err {
		case nil:
			visit(line)
			continue
		case io.EOF:
			if len(line) > 0 {
				visit(line)
			}
			return nil
		default:
			// ReadBytes may return partial data together with a non-EOF error.
			// Discard it rather than presenting a corrupt fragment as a JSONL
			// record to callers.
			return err
		}
	}
}

// ReviewComment represents a single code review finding from a session.
type ReviewComment struct {
	FilePath       string
	Content        string
	SuggestionCode string
	ExistingCode   string
	StartLine      int
	EndLine        int
	Category       string // bug, security, performance, maintainability, test, style, documentation, other
	Severity       string // critical, high, medium, low
}

// ViewSession holds fully parsed records for one session.
type ViewSession struct {
	Summary      SessionSummary
	TokenUsage   TokenUsageSummary
	Files        []*FileGroup     // ordered by file path
	SessionTasks []*FileGroup     // session-level tasks (grouping, etc.) separated from file-level
	Comments     []*ReviewComment // review findings from review_item_done/reused records
}

// TokenUsageSummary aggregates token counts across the session.
type TokenUsageSummary struct {
	TotalPromptTokens     int
	TotalCompletionTokens int
	TotalCacheReadTokens  int
	TotalCacheWriteTokens int
	RequestCount          int
	FileTokenBreakdown    []FileTokenUsage
}

// FileTokenUsage tracks token totals for a single file within a session.
type FileTokenUsage struct {
	FilePath         string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
}

// FileGroup aggregates records for a single file.
type FileGroup struct {
	FilePath string
	Tasks    map[TaskType][]*TaskCard
}

// TaskType mirrors session.TaskType.
type TaskType string

const (
	PlanTask              TaskType = "plan_task"
	MainTask              TaskType = "main_task"
	MemoryCompressionTask TaskType = "memory_compression_task"
	ReLocationTask        TaskType = "re_location_task"
	GroupingTask          TaskType = "grouping_task"
)

var sessionLevelPaths = map[string]bool{
	"__grouping__": true,
}

func isSessionLevelPath(fp string) bool {
	return sessionLevelPaths[fp]
}

// TaskCard links an LLM request with its response and tool calls.
type TaskCard struct {
	RequestMessages  any // preserved for display
	RequestNo        int
	ResponseContent  string
	ReasoningContent string // the model's reasoning/thinking text for this turn, if the provider exposed any
	ToolCalls        []ToolCallInfo
	DurationMs       int64
	Error            string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
}

// ToolCallInfo summarizes a single tool call.
type ToolCallInfo struct {
	Name       string
	Arguments  string
	Result     string
	Ok         bool
	DurationMs int64
}

// LoadSession fully parses a JSONL file into a ViewSession.
func LoadSession(root, encodedRepo, sessionID string) (*ViewSession, error) {
	path := filepath.Join(root, encodedRepo, sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("open session file: %w", err)
		}
		path, err = findLegacySession(root, encodedRepo, sessionID)
		if err != nil {
			return nil, fmt.Errorf("open session file: %w", err)
		}
		f, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open session file: %w", err)
		}
	}
	defer f.Close()

	vs := &ViewSession{Files: make([]*FileGroup, 0)}
	vs.Summary.Aborted = true
	fileIndex := make(map[string]*FileGroup)

	readErr := readJSONLLines(f, func(line []byte) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			return // skip malformed lines
		}
		typ, _ := rec["type"].(string)

		switch typ {
		case "session_start":
			if ts, ok := rec["timestamp"].(string); ok {
				vs.Summary.Timestamp, _ = time.Parse(time.RFC3339, ts)
			}
			if cwd, ok := rec["cwd"].(string); ok {
				vs.Summary.CWD = cwd
			}
			if branch, ok := rec["gitBranch"].(string); ok {
				vs.Summary.GitBranch = branch
			}
			if model, ok := rec["model"].(string); ok {
				vs.Summary.Model = model
			}
			if rm, ok := rec["reviewMode"].(string); ok {
				vs.Summary.ReviewMode = rm
			}
			if v, ok := rec["diffFrom"].(string); ok {
				vs.Summary.DiffFrom = v
			}
			if v, ok := rec["diffTo"].(string); ok {
				vs.Summary.DiffTo = v
			}
			if v, ok := rec["diffCommit"].(string); ok {
				vs.Summary.DiffCommit = v
			}

		case "llm_request":
			fp, _ := rec["filePath"].(string)
			tt, _ := rec["taskType"].(string)
			reqNo := 0
			if n, ok := rec["request_no"].(float64); ok {
				reqNo = int(n)
			}
			msgs := rec["messages"]

			tc := &TaskCard{RequestMessages: msgs, RequestNo: reqNo}

			fg := fileIndex[fp]
			if fg == nil {
				fg = &FileGroup{FilePath: fp, Tasks: make(map[TaskType][]*TaskCard)}
				fileIndex[fp] = fg
				vs.Files = append(vs.Files, fg)
			}
			fg.Tasks[TaskType(tt)] = append(fg.Tasks[TaskType(tt)], tc)

		case "llm_response":
			fp, _ := rec["filePath"].(string)
			content, _ := rec["content"].(string)
			reasoning, _ := rec["reasoning_content"].(string)
			durationMs := int64(0)
			if d, ok := rec["duration_ms"].(float64); ok {
				durationMs = int64(d)
			}
			model, _ := rec["model"].(string)
			errStr, _ := rec["error"].(string)

			promptTok := 0
			completionTok := 0
			cacheReadTok := 0
			cacheWriteTok := 0
			if usage, ok := rec["usage"].(map[string]any); ok {
				if v, ok := usage["prompt_tokens"].(float64); ok {
					promptTok = int(v)
				}
				if v, ok := usage["completion_tokens"].(float64); ok {
					completionTok = int(v)
				}
				if v, ok := usage["cache_read_tokens"].(float64); ok {
					cacheReadTok = int(v)
				}
				if v, ok := usage["cache_write_tokens"].(float64); ok {
					cacheWriteTok = int(v)
				}
			}

			tt, _ := rec["taskType"].(string)
			fg := fileIndex[fp]
			if fg != nil {
				cards := fg.Tasks[TaskType(tt)]
				if len(cards) > 0 && cards[len(cards)-1].ResponseContent == "" {
					card := cards[len(cards)-1]
					card.ResponseContent = content
					card.ReasoningContent = reasoning
					card.DurationMs = durationMs
					card.Model = model
					card.Error = errStr
					card.PromptTokens = promptTok
					card.CompletionTokens = completionTok
					card.CacheReadTokens = cacheReadTok
					card.CacheWriteTokens = cacheWriteTok
				}
			}

			// Also attach tool_calls to the same card
			if tcs, ok := rec["tool_calls"].([]any); ok && fg != nil {
				tt, _ := rec["taskType"].(string)
				cards := fg.Tasks[TaskType(tt)]
				if len(cards) > 0 {
					card := cards[len(cards)-1]
					for _, tc := range tcs {
						if tm, ok := tc.(map[string]any); ok {
							name, _ := tm["name"].(string)
							args, _ := tm["arguments"].(string)
							info := ToolCallInfo{Name: name, Arguments: args}
							if name == "task_done" {
								info.Ok = taskDoneSucceeded(args)
							} else if name == "report_incorrect_comments" || name == "approve_all_comments" {
								info.Ok = true
							}
							card.ToolCalls = append(card.ToolCalls, info)
						}
					}
				}
			}

		case "llm_error":
			fp, _ := rec["filePath"].(string)
			tt, _ := rec["taskType"].(string)
			errStr, _ := rec["error"].(string)
			durationMs := int64(0)
			if d, ok := rec["duration_ms"].(float64); ok {
				durationMs = int64(d)
			}

			fg := fileIndex[fp]
			if fg != nil {
				cards := fg.Tasks[TaskType(tt)]
				if len(cards) > 0 && cards[len(cards)-1].Error == "" {
					card := cards[len(cards)-1]
					card.Error = errStr
					card.DurationMs = durationMs
				}
			}

		case "tool_call":
			toolName, _ := rec["tool_name"].(string)
			result, _ := rec["result"].(string)
			okVal := true
			if b, hasOk := rec["ok"].(bool); hasOk {
				okVal = b
			}
			fp, _ := rec["filePath"].(string)
			tt, _ := rec["taskType"].(string)
			durationMs := int64(0)
			if d, ok2 := rec["duration_ms"].(float64); ok2 {
				durationMs = int64(d)
			}

			fg := fileIndex[fp]
			if fg != nil {
				cards := fg.Tasks[TaskType(tt)]
				if len(cards) > 0 {
					card := cards[len(cards)-1]
					for ti := range card.ToolCalls {
						// Older session records omitted tool_name, so retain
						// positional matching only for those records.
						if (toolName == "" || card.ToolCalls[ti].Name == toolName) &&
							card.ToolCalls[ti].Result == "" && !card.ToolCalls[ti].Ok {
							card.ToolCalls[ti].Result = result
							card.ToolCalls[ti].Ok = okVal
							card.ToolCalls[ti].DurationMs = durationMs
							break
						}
					}
				}
			}

		case "review_item_done", "review_item_reused":
			fp, _ := rec["filePath"].(string)
			if comments, ok := rec["comments"].([]any); ok {
				for _, c := range comments {
					cm, ok := c.(map[string]any)
					if !ok {
						continue
					}
					rc := &ReviewComment{FilePath: fp}
					if v, ok := cm["path"].(string); ok && v != "" {
						rc.FilePath = v
					}
					if v, ok := cm["content"].(string); ok {
						rc.Content = v
					}
					if v, ok := cm["suggestion_code"].(string); ok {
						rc.SuggestionCode = v
					}
					if v, ok := cm["existing_code"].(string); ok {
						rc.ExistingCode = v
					}
					if v, ok := cm["start_line"].(float64); ok {
						rc.StartLine = int(v)
					}
					if v, ok := cm["end_line"].(float64); ok {
						rc.EndLine = int(v)
					}
					if v, ok := cm["category"].(string); ok {
						rc.Category = v
					}
					if v, ok := cm["severity"].(string); ok {
						rc.Severity = v
					}
					vs.Comments = append(vs.Comments, rc)
				}
			}

		case "session_end":
			applySessionEnd(&vs.Summary, rec)
		}
	})

	// Aggregate token usage across all task cards
	fileBreakdown := make([]FileTokenUsage, 0, len(vs.Files))
	for _, fg := range vs.Files {
		ft := FileTokenUsage{FilePath: fg.FilePath}
		for _, cards := range fg.Tasks {
			for _, c := range cards {
				vs.TokenUsage.TotalPromptTokens += c.PromptTokens
				vs.TokenUsage.TotalCompletionTokens += c.CompletionTokens
				vs.TokenUsage.TotalCacheReadTokens += c.CacheReadTokens
				vs.TokenUsage.TotalCacheWriteTokens += c.CacheWriteTokens
				if c.ResponseContent != "" || c.PromptTokens > 0 {
					vs.TokenUsage.RequestCount++
				}
				ft.PromptTokens += c.PromptTokens
				ft.CompletionTokens += c.CompletionTokens
				ft.CacheReadTokens += c.CacheReadTokens
				ft.CacheWriteTokens += c.CacheWriteTokens
			}
		}
		fileBreakdown = append(fileBreakdown, ft)
	}
	sort.Slice(fileBreakdown, func(i, j int) bool {
		return fileBreakdown[i].PromptTokens+fileBreakdown[i].CompletionTokens > fileBreakdown[j].PromptTokens+fileBreakdown[j].CompletionTokens
	})
	vs.TokenUsage.FileTokenBreakdown = fileBreakdown

	// Separate session-level virtual paths from real file paths.
	realFiles := make([]*FileGroup, 0, len(vs.Files))
	for _, fg := range vs.Files {
		if isSessionLevelPath(fg.FilePath) {
			vs.SessionTasks = append(vs.SessionTasks, fg)
		} else {
			realFiles = append(realFiles, fg)
		}
	}
	vs.Files = realFiles

	sort.Slice(vs.Files, func(i, j int) bool {
		return vs.Files[i].FilePath < vs.Files[j].FilePath
	})

	vs.Summary.SessionID = sessionID
	vs.Summary.CommentCount = len(vs.Comments)
	return vs, readErr
}

func findLegacySession(root, encodedRepo, sessionID string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	name := sessionID + ".jsonl"
	for _, dirEntry := range entries {
		if !dirEntry.IsDir() {
			continue
		}
		if isVersionedRepoKey(dirEntry.Name()) {
			continue
		}
		path := filepath.Join(root, dirEntry.Name(), name)
		if _, statErr := os.Stat(path); statErr != nil {
			continue
		}
		cwd, readErr := readSessionCWD(path)
		if readErr == nil && cwd != "" && session.RepoSessionKey(cwd) == encodedRepo {
			return path, nil
		}
	}
	return "", os.ErrNotExist
}

func applySessionEnd(summary *SessionSummary, rec map[string]any) {
	summary.Aborted = false
	if dur, ok := rec["duration_seconds"].(float64); ok {
		summary.DurationSec = dur
	}
	if files, ok := rec["files_reviewed"].([]any); ok {
		summary.FilesReviewed = make([]string, 0, len(files))
		for _, fv := range files {
			if s, ok := fv.(string); ok {
				summary.FilesReviewed = append(summary.FilesReviewed, s)
			}
		}
	}
	if f, ok := rec["llm_failures"].(float64); ok {
		summary.LLMFailures = int(f)
	}

	if raw, ok := rec["run_manifest"]; ok {
		data, err := json.Marshal(raw)
		if err == nil {
			var manifest session.RunManifest
			if err := json.Unmarshal(data, &manifest); err == nil && manifest.SchemaVersion == session.ManifestSchemaVersion {
				summary.RunManifest = &manifest
				summary.TerminalState = string(manifest.TerminalState)
				summary.FilesReviewed = filesReviewedFromSelected(manifest.Coverage.Selected)
				summary.SelectedCount = len(manifest.Coverage.Selected)
				summary.CompletedCount = len(manifest.Coverage.Completed)
				summary.ReusedCount = len(manifest.Coverage.Reused)
				summary.FailedCount = len(manifest.Coverage.Failed)
				summary.WaivedCount = len(manifest.Coverage.Waived)
				summary.FileCount = summary.SelectedCount
			}
		}
	}
	if summary.RunManifest == nil {
		summary.Legacy = true
		summary.FileCount = len(summary.FilesReviewed)
	}
}

func filesReviewedFromSelected(selected []session.CoverageItem) []string {
	files := make([]string, 0, len(selected))
	for _, item := range selected {
		files = append(files, item.Path)
	}
	return files
}

func taskDoneSucceeded(arguments string) bool {
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return false
	}
	state, hasState := args["state"]
	if !hasState {
		return true
	}
	stateString, ok := state.(string)
	return ok && stateString == "DONE"
}
