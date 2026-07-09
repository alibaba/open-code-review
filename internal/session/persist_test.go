package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/model"
)

func init() { UseTestSessions() }

func TestEncodeRepoPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "empty",
		},
		{
			name:     "relative path",
			input:    "relative/path/to/repo",
			expected: "relative-path-to-repo",
		},
		{
			name:     "path with mixed separators",
			input:    "path/to\\mixed",
			expected: "path-to-mixed",
		},
	}

	// Add platform-specific test cases
	if runtime.GOOS == "windows" {
		tests = append(tests, []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "windows drive path",
				input:    "D:\\Users\\admin\\project",
				expected: "D_Users-admin-project",
			},
			{
				name:     "windows C drive",
				input:    "C:\\code\\myapp",
				expected: "C_code-myapp",
			},
			{
				name:     "windows relative path",
				input:    "relative\\path\\to\\repo",
				expected: "relative-path-to-repo",
			},
			{
				name:     "windows drive only",
				input:    "C:",
				expected: "C_",
			},
			{
				name:     "windows drive with separator only",
				input:    "D:\\",
				expected: "D_",
			},
		}...)
	} else {
		tests = append(tests, []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "unix absolute path",
				input:    "/home/user/project",
				expected: "home-user-project",
			},
			{
				name:     "unix nested path",
				input:    "/Users/john/dev/myapp",
				expected: "Users-john-dev-myapp",
			},
			{
				name:     "unix root only",
				input:    "/",
				expected: "empty",
			},
		}...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeRepoPath(tt.input)
			if result != tt.expected {
				t.Errorf("encodeRepoPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func readJSONLRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer func() { _ = f.Close() }()

	var records []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatalf("unmarshal jsonl line: %v", err)
		}
		records = append(records, rec)
	}
	return records
}

func sessionJSONLPath(t *testing.T, repoDir, sessionID string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}
	return filepath.Join(home, ".opencodereview", "test-sessions", encodeRepoPath(repoDir), sessionID+".jsonl")
}

func TestSetErrorIncrementsCounter(t *testing.T) {
	sh := &SessionHistory{
		SessionID:    generateUUID(),
		FileSessions: make(map[string]*FileSession),
	}
	fs := sh.GetOrCreateFileSession("test.go")

	rec1 := &TaskRecord{Type: MainTask, RequestNo: 1, fileSession: fs}
	rec1.SetError(fmt.Errorf("timeout"), 1*time.Second)

	if got := sh.LLMFailures(); got != 1 {
		t.Errorf("after 1 error: LLMFailures() = %d, want 1", got)
	}

	rec2 := &TaskRecord{Type: PlanTask, RequestNo: 1, fileSession: fs}
	rec2.SetError(fmt.Errorf("rate limit"), 2*time.Second)

	if got := sh.LLMFailures(); got != 2 {
		t.Errorf("after 2 errors: LLMFailures() = %d, want 2", got)
	}
}

func TestSetErrorWritesJSONL(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	defer sh.Finalize()

	fs := sh.GetOrCreateFileSession("foo.go")
	rec := fs.AppendTaskRecord(MainTask, nil)
	rec.SetError(fmt.Errorf("connection refused"), 500*time.Millisecond)

	if sh.persist != nil {
		sh.persist.mu.Lock()
		err := sh.persist.writer.Flush()
		sh.persist.mu.Unlock()
		if err != nil {
			t.Fatalf("flush: %v", err)
		}
	}

	path := sessionJSONLPath(t, repoDir, sh.SessionID)
	records := readJSONLRecords(t, path)

	var found bool
	for _, r := range records {
		if r["type"] == "llm_error" {
			found = true
			if r["filePath"] != "foo.go" {
				t.Errorf("filePath = %v, want foo.go", r["filePath"])
			}
			if r["taskType"] != string(MainTask) {
				t.Errorf("taskType = %v, want %s", r["taskType"], MainTask)
			}
			if r["error"] != "connection refused" {
				t.Errorf("error = %v, want connection refused", r["error"])
			}
			break
		}
	}
	if !found {
		t.Error("no llm_error record found in JSONL")
	}
}

func TestSessionFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions not enforced on Windows")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repoDir := t.TempDir()
	sessionID := generateUUID()

	jw, err := newJSONLWriter(sessionID, repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})
	if err != nil {
		t.Fatalf("newJSONLWriter: %v", err)
	}
	jw.WriteSessionStart(time.Now())
	defer jw.flushAndClose()

	sessionDir := filepath.Join(tmpHome, ".opencodereview", "test-sessions", encodeRepoPath(repoDir))
	sessionFile := filepath.Join(sessionDir, sessionID+".jsonl")

	dirInfo, err := os.Stat(sessionDir)
	if err != nil {
		t.Fatalf("stat session dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != os.FileMode(0700) {
		t.Errorf("session dir mode = %04o, want 0700", got)
	}

	fileInfo, err := os.Stat(sessionFile)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != os.FileMode(0600) {
		t.Errorf("session file mode = %04o, want 0600", got)
	}
}

func TestSessionEndIncludesFailures(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "main", "test-model", SessionOptions{ReviewMode: ReviewModeWorkspace})

	fs := sh.GetOrCreateFileSession("bar.go")
	for i := 0; i < 3; i++ {
		rec := &TaskRecord{Type: MainTask, RequestNo: i + 1, fileSession: fs}
		rec.SetError(fmt.Errorf("error %d", i), time.Second)
	}

	sh.Finalize()

	path := sessionJSONLPath(t, repoDir, sh.SessionID)
	records := readJSONLRecords(t, path)

	var endRec map[string]any
	for _, r := range records {
		if r["type"] == "session_end" {
			endRec = r
			break
		}
	}
	if endRec == nil {
		t.Fatal("no session_end record found")
	}

	failures, ok := endRec["llm_failures"].(float64)
	if !ok {
		t.Fatalf("llm_failures field missing or wrong type: %v", endRec["llm_failures"])
	}
	if int64(failures) != 3 {
		t.Errorf("llm_failures = %v, want 3", failures)
	}
}

func TestReviewItemResumeRoundTrip(t *testing.T) {
	repoDir := t.TempDir()
	sh := New(repoDir, "feature", "new-model", SessionOptions{
		ReviewMode:  ReviewModeRange,
		DiffFrom:    "main",
		DiffTo:      "feature",
		ResumedFrom: "old-session",
	})
	comments := []model.LlmComment{{
		Path:         "a.go",
		Content:      "fix this",
		ExistingCode: "old()",
	}}
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", "fp-a", comments)
	sh.RecordReviewItemFailed("b.go", "b.go", "b.go", "fp-b", "rate limit")
	sh.Finalize()

	state, err := LoadResumeState(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	if state.SessionID != sh.SessionID {
		t.Errorf("SessionID = %q, want %q", state.SessionID, sh.SessionID)
	}
	if state.ReviewMode != ReviewModeRange || state.DiffFrom != "main" || state.DiffTo != "feature" {
		t.Errorf("metadata mismatch: %+v", state)
	}
	if state.CompletedCount() != 1 {
		t.Fatalf("CompletedCount = %d, want 1", state.CompletedCount())
	}
	item, ok := state.Item("fp-a")
	if !ok {
		t.Fatal("missing fp-a")
	}
	if item.FilePath != "a.go" || len(item.Comments) != 1 || item.Comments[0].Content != "fix this" {
		t.Errorf("item mismatch: %+v", item)
	}
	if _, ok := state.Item("fp-b"); ok {
		t.Error("failed item should not be reusable")
	}
	if err := state.ValidateOptions(SessionOptions{ReviewMode: ReviewModeRange, DiffFrom: "main", DiffTo: "feature"}); err != nil {
		t.Fatalf("ValidateOptions: %v", err)
	}

	records := readJSONLRecords(t, sessionJSONLPath(t, repoDir, sh.SessionID))
	var foundStart bool
	for _, r := range records {
		if r["type"] == "session_start" {
			foundStart = true
			if r["resumedFrom"] != "old-session" {
				t.Errorf("resumedFrom = %v, want old-session", r["resumedFrom"])
			}
		}
	}
	if !foundStart {
		t.Fatal("missing session_start")
	}
}

func TestResumeStateValidateOptionsRejectsMismatchedRange(t *testing.T) {
	state := &ResumeState{
		SessionID:  "s1",
		ReviewMode: ReviewModeRange,
		DiffFrom:   "main",
		DiffTo:     "feature",
	}
	err := state.ValidateOptions(SessionOptions{ReviewMode: ReviewModeRange, DiffFrom: "main", DiffTo: "other"})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestResumeStateSessionStartKeepsRepoDirWhenCwdEmpty(t *testing.T) {
	state := &ResumeState{RepoDir: "/repo/from/caller"}

	state.applySessionStart(resumeRecord{
		SessionID:  "s1",
		ReviewMode: ReviewModeCommit,
		DiffCommit: "abc123",
	})

	if state.RepoDir != "/repo/from/caller" {
		t.Fatalf("RepoDir = %q, want caller-provided repo dir", state.RepoDir)
	}
}
