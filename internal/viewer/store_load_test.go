package viewer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionsRoot(t *testing.T) {
	root, err := SessionsRoot()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".opencodereview", "sessions")
	if root != expected {
		t.Errorf("SessionsRoot() = %q, want %q", root, expected)
	}
}

func TestLoadSession_FullParse(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3","reviewMode":"commit","diffFrom":"aaa","diffTo":"bbb","diffCommit":"ccc"}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1,"messages":[{"role":"user","content":"review this"}]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","content":"Code looks good","duration_ms":1500,"model":"claude-3","usage":{"prompt_tokens":100,"completion_tokens":50,"cache_read_tokens":10,"cache_write_tokens":5},"tool_calls":[{"name":"search","arguments":"query"}]}`,
		`{"type":"tool_call","filePath":"main.go","taskType":"main_task","result":"found 3 results","ok":true,"duration_ms":20}`,
		`{"type":"llm_request","filePath":"util.go","taskType":"plan_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"util.go","taskType":"plan_task","content":"planning","duration_ms":800,"model":"claude-3","usage":{"prompt_tokens":200,"completion_tokens":80,"cache_read_tokens":0,"cache_write_tokens":0}}`,
		`{"type":"session_end","duration_seconds":120.5,"files_reviewed":["main.go","util.go"],"llm_failures":1}`,
	)

	vs, err := LoadSession(root, "repo", "sess1")
	if err != nil {
		t.Fatal(err)
	}

	// Check summary
	if vs.Summary.SessionID != "sess1" {
		t.Errorf("SessionID = %q", vs.Summary.SessionID)
	}
	if vs.Summary.CWD != "/home/dev/proj" {
		t.Errorf("CWD = %q", vs.Summary.CWD)
	}
	if vs.Summary.GitBranch != "feat" {
		t.Errorf("GitBranch = %q", vs.Summary.GitBranch)
	}
	if vs.Summary.Model != "claude-3" {
		t.Errorf("Model = %q", vs.Summary.Model)
	}
	if vs.Summary.ReviewMode != "commit" {
		t.Errorf("ReviewMode = %q", vs.Summary.ReviewMode)
	}
	if vs.Summary.DiffFrom != "aaa" {
		t.Errorf("DiffFrom = %q", vs.Summary.DiffFrom)
	}
	if vs.Summary.DiffTo != "bbb" {
		t.Errorf("DiffTo = %q", vs.Summary.DiffTo)
	}
	if vs.Summary.DiffCommit != "ccc" {
		t.Errorf("DiffCommit = %q", vs.Summary.DiffCommit)
	}
	if vs.Summary.DurationSec != 120.5 {
		t.Errorf("DurationSec = %f", vs.Summary.DurationSec)
	}
	if vs.Summary.FileCount != 2 {
		t.Errorf("FileCount = %d", vs.Summary.FileCount)
	}
	if vs.Summary.LLMFailures != 1 {
		t.Errorf("LLMFailures = %d", vs.Summary.LLMFailures)
	}

	// Check files are sorted
	if len(vs.Files) != 2 {
		t.Fatalf("Files count = %d, want 2", len(vs.Files))
	}
	if vs.Files[0].FilePath != "main.go" {
		t.Errorf("Files[0] = %q, want main.go", vs.Files[0].FilePath)
	}
	if vs.Files[1].FilePath != "util.go" {
		t.Errorf("Files[1] = %q, want util.go", vs.Files[1].FilePath)
	}

	// Check main.go task card
	mainCards := vs.Files[0].Tasks[MainTask]
	if len(mainCards) != 1 {
		t.Fatalf("main.go main_task cards = %d", len(mainCards))
	}
	card := mainCards[0]
	if card.RequestNo != 1 {
		t.Errorf("RequestNo = %d", card.RequestNo)
	}
	if card.ResponseContent != "Code looks good" {
		t.Errorf("ResponseContent = %q", card.ResponseContent)
	}
	if card.DurationMs != 1500 {
		t.Errorf("DurationMs = %d", card.DurationMs)
	}
	if card.Model != "claude-3" {
		t.Errorf("Model = %q", card.Model)
	}
	if card.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d", card.PromptTokens)
	}
	if card.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d", card.CompletionTokens)
	}
	if card.CacheReadTokens != 10 {
		t.Errorf("CacheReadTokens = %d", card.CacheReadTokens)
	}
	if card.CacheWriteTokens != 5 {
		t.Errorf("CacheWriteTokens = %d", card.CacheWriteTokens)
	}

	// Check tool calls
	if len(card.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d", len(card.ToolCalls))
	}
	tc := card.ToolCalls[0]
	if tc.Name != "search" {
		t.Errorf("ToolCall.Name = %q", tc.Name)
	}
	if !tc.Ok {
		t.Error("ToolCall.Ok = false, want true")
	}
	if tc.Result != "found 3 results" {
		t.Errorf("ToolCall.Result = %q", tc.Result)
	}
	if tc.DurationMs != 20 {
		t.Errorf("ToolCall.DurationMs = %d", tc.DurationMs)
	}

	// Check token usage
	if vs.TokenUsage.TotalPromptTokens != 300 {
		t.Errorf("TotalPromptTokens = %d, want 300", vs.TokenUsage.TotalPromptTokens)
	}
	if vs.TokenUsage.TotalCompletionTokens != 130 {
		t.Errorf("TotalCompletionTokens = %d, want 130", vs.TokenUsage.TotalCompletionTokens)
	}
	if vs.TokenUsage.TotalCacheReadTokens != 10 {
		t.Errorf("TotalCacheReadTokens = %d", vs.TokenUsage.TotalCacheReadTokens)
	}
	if vs.TokenUsage.TotalCacheWriteTokens != 5 {
		t.Errorf("TotalCacheWriteTokens = %d", vs.TokenUsage.TotalCacheWriteTokens)
	}
	if vs.TokenUsage.RequestCount != 2 {
		t.Errorf("RequestCount = %d, want 2", vs.TokenUsage.RequestCount)
	}

	// Check file token breakdown
	if len(vs.TokenUsage.FileTokenBreakdown) != 2 {
		t.Fatalf("FileTokenBreakdown count = %d", len(vs.TokenUsage.FileTokenBreakdown))
	}
	// Sorted by total tokens (descending), util.go (200+80=280) > main.go (100+50=150)
	if vs.TokenUsage.FileTokenBreakdown[0].FilePath != "util.go" {
		t.Errorf("top token file = %q, want util.go", vs.TokenUsage.FileTokenBreakdown[0].FilePath)
	}
}

func TestLoadSession_MissingFile(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSession(root, "repo", "nonexistent")
	if err == nil {
		t.Error("expected error for missing session file")
	}
}

func TestLoadSession_MalformedLines(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "bad.jsonl"),
		`not json at all`,
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{broken json`,
		`{"type":"session_end","duration_seconds":10,"files_reviewed":[],"llm_failures":0}`,
	)

	vs, err := LoadSession(root, "repo", "bad")
	if err != nil {
		t.Fatal(err)
	}
	if vs.Summary.CWD != "/x" {
		t.Errorf("CWD = %q, want /x (should skip malformed lines)", vs.Summary.CWD)
	}
}

func TestLoadSession_LLMError(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "errs.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_error","filePath":"a.go","taskType":"main_task","error":"rate limit exceeded","duration_ms":500}`,
		`{"type":"session_end","duration_seconds":5,"files_reviewed":["a.go"],"llm_failures":1}`,
	)

	vs, err := LoadSession(root, "repo", "errs")
	if err != nil {
		t.Fatal(err)
	}

	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 1 {
		t.Fatalf("cards count = %d", len(cards))
	}
	if cards[0].Error != "rate limit exceeded" {
		t.Errorf("Error = %q", cards[0].Error)
	}
	if cards[0].DurationMs != 500 {
		t.Errorf("DurationMs = %d", cards[0].DurationMs)
	}
}

func TestLoadSession_ToolCallWithOkFalse(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "tc.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"result","duration_ms":100,"model":"m","usage":{"prompt_tokens":10,"completion_tokens":5},"tool_calls":[{"name":"search","arguments":"query"}]}`,
		`{"type":"tool_call","filePath":"a.go","taskType":"main_task","result":"error: not found","ok":false,"duration_ms":50}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":["a.go"]}`,
	)

	vs, err := LoadSession(root, "repo", "tc")
	if err != nil {
		t.Fatal(err)
	}

	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	if len(cards[0].ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(cards[0].ToolCalls))
	}
	tc := cards[0].ToolCalls[0]
	if tc.Ok {
		t.Error("expected Ok=false")
	}
	if tc.Result != "error: not found" {
		t.Errorf("Result = %q", tc.Result)
	}
	if tc.DurationMs != 50 {
		t.Errorf("DurationMs = %d", tc.DurationMs)
	}
}

func TestLoadSession_MultipleRequests(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "multi.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"first pass","duration_ms":100,"model":"m","usage":{"prompt_tokens":50,"completion_tokens":20}}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":2,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"second pass","duration_ms":200,"model":"m","usage":{"prompt_tokens":60,"completion_tokens":30}}`,
		`{"type":"session_end","duration_seconds":10,"files_reviewed":["a.go"]}`,
	)

	vs, err := LoadSession(root, "repo", "multi")
	if err != nil {
		t.Fatal(err)
	}

	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}
	if cards[0].ResponseContent != "first pass" {
		t.Errorf("cards[0] content = %q", cards[0].ResponseContent)
	}
	if cards[1].ResponseContent != "second pass" {
		t.Errorf("cards[1] content = %q", cards[1].ResponseContent)
	}
	if vs.TokenUsage.RequestCount != 2 {
		t.Errorf("RequestCount = %d", vs.TokenUsage.RequestCount)
	}
}

func TestLoadSession_EmptyFile(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "empty.jsonl"))

	vs, err := LoadSession(root, "repo", "empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d, want 0", len(vs.Files))
	}
}

func TestLoadSession_ResponseWithoutRequest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// llm_response for a file that has no prior request - should not panic
	writeJSONL(t, filepath.Join(repoDir, "orphan.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_response","filePath":"unknown.go","taskType":"main_task","content":"orphan","duration_ms":100,"model":"m"}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`,
	)

	vs, err := LoadSession(root, "repo", "orphan")
	if err != nil {
		t.Fatal(err)
	}
	// No file groups should be created for orphan responses (no request created the fileIndex entry)
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d, want 0 (orphan response has no request)", len(vs.Files))
	}
}

func TestLoadSession_LLMErrorWithoutRequest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "orphanerr.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_error","filePath":"unknown.go","taskType":"main_task","error":"fail","duration_ms":10}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`,
	)

	// Should not panic
	vs, err := LoadSession(root, "repo", "orphanerr")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d", len(vs.Files))
	}
}

func TestLoadSession_ToolCallWithoutRequest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "orphantc.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"tool_call","filePath":"unknown.go","taskType":"main_task","result":"x","ok":true}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`,
	)

	// Should not panic
	vs, err := LoadSession(root, "repo", "orphantc")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d", len(vs.Files))
	}
}

func TestDiscoverRepos_SkipsUnreadableSubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	root := t.TempDir()
	badRepo := filepath.Join(root, "unreadable-repo")
	if err := os.MkdirAll(badRepo, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(badRepo, "s.jsonl"), `{"type":"session_start"}`)
	// Remove read permission so os.ReadDir(repoDir) fails
	if err := os.Chmod(badRepo, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(badRepo, 0755) })

	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	// Unreadable repo is skipped (continue)
	if len(repos) != 0 {
		t.Errorf("repos = %d, want 0 (unreadable dir skipped)", len(repos))
	}
}

func TestListSessions_SkipsUnreadableFiles(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a valid session file
	writeJSONL(t, filepath.Join(repoDir, "good.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`)

	// Create an unreadable jsonl file
	badPath := filepath.Join(repoDir, "bad.jsonl")
	if err := os.WriteFile(badPath, []byte(`{"type":"session_start"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(badPath, 0644) })

	sessions, err := ListSessions(root, "repo")
	if err != nil {
		t.Fatal(err)
	}
	// Should get 1 session (the good one), bad one is skipped
	if len(sessions) != 1 {
		t.Errorf("sessions = %d, want 1 (bad file skipped)", len(sessions))
	}
}

func TestLoadSession_MultipleTaskTypes(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "tasks.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"plan_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"plan_task","content":"planning","duration_ms":100,"model":"m","usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"reviewing","duration_ms":200,"model":"m","usage":{"prompt_tokens":20,"completion_tokens":10}}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"memory_compression_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"memory_compression_task","content":"compressed","duration_ms":50,"model":"m","usage":{"prompt_tokens":5,"completion_tokens":3}}`,
		`{"type":"session_end","duration_seconds":5,"files_reviewed":["a.go"]}`,
	)

	vs, err := LoadSession(root, "repo", "tasks")
	if err != nil {
		t.Fatal(err)
	}

	if len(vs.Files) != 1 {
		t.Fatalf("Files = %d", len(vs.Files))
	}
	fg := vs.Files[0]
	if len(fg.Tasks) != 3 {
		t.Errorf("task types = %d, want 3", len(fg.Tasks))
	}
	if len(fg.Tasks[PlanTask]) != 1 {
		t.Errorf("plan_task cards = %d", len(fg.Tasks[PlanTask]))
	}
	if len(fg.Tasks[MainTask]) != 1 {
		t.Errorf("main_task cards = %d", len(fg.Tasks[MainTask]))
	}
	if len(fg.Tasks[MemoryCompressionTask]) != 1 {
		t.Errorf("memory_compression_task cards = %d", len(fg.Tasks[MemoryCompressionTask]))
	}
}
