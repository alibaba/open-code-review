package viewer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRepos_Success(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "s1.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T10:00:00Z"}`)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "test-repo") {
		t.Errorf("response does not contain repo name")
	}
}

func TestHandleRepos_EmptyRoot(t *testing.T) {
	root := t.TempDir()
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "No session data found") {
		t.Errorf("expected empty-state message in body")
	}
}

func TestHandleRepos_NotFoundForNonRootPath(t *testing.T) {
	root := t.TempDir()
	req := httptest.NewRequest("GET", "/other", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleRepos_UnreadableRoot(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, "/nonexistent/definitely/missing/root")

	// DiscoverRepos returns nil for non-existent dirs (treated as empty)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (empty)", rr.Code)
	}
}

func TestHandleRepos_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	root := t.TempDir()
	// Create a directory that exists but cannot be read
	badDir := filepath.Join(root, "unreadable")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a .jsonl inside so it's a valid sessions dir
	writeJSONL(t, filepath.Join(badDir, "s.jsonl"), `{"type":"session_start"}`)
	// Remove read permission on root
	if err := os.Chmod(root, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0755) })

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestHandleSessions_Success(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-03-01T09:00:00Z","cwd":"/home/user/project","model":"gpt-4"}`,
		`{"type":"session_end","duration_seconds":60,"files_reviewed":["a.go"]}`)

	req := httptest.NewRequest("GET", "/r/myrepo", nil)
	rr := httptest.NewRecorder()
	handleSessions(rr, req, root, "myrepo")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "project") {
		t.Errorf("response does not contain repo display name derived from CWD")
	}
}

func TestHandleSessions_NoCWD(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo2")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "s.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","model":"m"}`)

	req := httptest.NewRequest("GET", "/r/repo2", nil)
	rr := httptest.NewRecorder()
	handleSessions(rr, req, root, "repo2")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestHandleSessions_ErrorOnBadDir(t *testing.T) {
	root := t.TempDir()
	req := httptest.NewRequest("GET", "/r/missing", nil)
	rr := httptest.NewRecorder()
	handleSessions(rr, req, root, "missing")

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestHandleSession_Success(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "abc123.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj","model":"claude"}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","content":"LGTM"}`,
		`{"type":"session_end","duration_seconds":30,"files_reviewed":["main.go"]}`)

	req := httptest.NewRequest("GET", "/r/repo/abc123", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "abc123")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "proj") {
		t.Errorf("response does not contain derived display name")
	}
}

func TestHandleSession_NotFound(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/r/repo/nonexistent", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "nonexistent")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleSession_EmptyCWD(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "s.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","model":"m"}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`)

	req := httptest.NewRequest("GET", "/r/repo/s", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "s")

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
