// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

func TestReportTabsShareHeader(t *testing.T) {
	root := sessionFixture(t, `{"type":"session_start","reviewMode":"range","model":"example","analysis":{"enabled":true}}`, fixtureEnd)
	mux := newMux(root)
	parts := func(tab string) (string, string) {
		t.Helper()
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, httptest.NewRequest("GET", "/r/repo/sess1?tab="+tab, nil))
		header, panel, ok := strings.Cut(out.Body.String(), "</header>")
		active := `href="?tab=` + tab + `" aria-current="page"`
		if out.Code != http.StatusOK || !ok || !strings.Contains(header, active) || strings.Count(header, `aria-current="page"`) != 1 {
			t.Fatalf("%s: missing report header or incorrect current link", tab)
		}
		return strings.Replace(header, ` aria-current="page"`, "", 1), panel
	}
	reviewHeader, review := parts("review")
	analysisHeader, analysis := parts("analysis")
	if reviewHeader != analysisHeader {
		t.Error("the breadcrumb, report context and navigation must match across tabs")
	}
	for _, label := range []string{"Status:", "Files:", "Duration:"} {
		if strings.Contains(reviewHeader, label) || !strings.Contains(review, label) || strings.Contains(analysis, label) {
			t.Errorf("%q should only appear in the review panel", label)
		}
	}
}

func TestCombinedReportViews(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	directory := filepath.Join(repo, "review-id", "analysis")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repo, "review-id.jsonl"),
		`{"type":"session_start","timestamp":"2026-01-01T00:00:00Z","cwd":"/repo","reviewMode":"range","analysis":{"enabled":true}}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","content":"review-only turn"}`)
	writeJSONL(t, filepath.Join(repo, "plain.jsonl"), `{"type":"session_start","cwd":"/repo"}`)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string, status int, want, absent string) string {
		t.Helper()
		out := httptest.NewRecorder()
		newMux(root).ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if out.Code != status || !strings.Contains(out.Body.String(), want) || (absent != "" && strings.Contains(out.Body.String(), absent)) {
			t.Fatalf("%s: status=%d want=%q absent=%q body=%s", path, out.Code, want, absent, out.Body.String())
		}
		return out.Body.String()
	}
	const page = "/r/repo/review-id"
	const tab = page + "?tab=analysis"
	get("/r/repo", 200, `<td class="col-mode">range</td>`, "Review + Analysis")
	get("/r/repo", 200, page, "trace.jsonl")
	get("/r/repo/plain?tab=analysis", 404, "", "analysis-prose")
	get(tab, 200, "missing or unreadable", "legacy")
	write("result.json", `{"execution_status":"started","session_id":"review-id"}`)
	get(tab, 200, "No final Analysis status", `id="analysis-conversations"`)
	writeJSONL(t, filepath.Join(directory, "trace.jsonl"),
		`{"type":"llm_request","filePath":"comparison","taskType":"main_task","request_no":1}`,
		`{"type":"llm_response","filePath":"comparison","taskType":"main_task","content":"first analysis turn","tool_calls":[{"name":"file_read","arguments":"read arguments"}]}`,
		`{"type":"tool_call","filePath":"comparison","taskType":"main_task","tool_name":"file_read","result":"private tool result"}`,
		`{"type":"llm_request","filePath":"comparison","taskType":"memory_compression_task","request_no":2}`,
		`{"type":"llm_response","filePath":"comparison","taskType":"memory_compression_task","content":"compressed history"}`,
		`{"type":"llm_request","filePath":"comparison","taskType":"main_task","request_no":3}`,
		`{"type":"llm_error","filePath":"comparison","taskType":"main_task","error":"analysis request failed"}`,
		`{"type":`)
	body := get(tab, 200, `<details class="file-accordion section-accordion" id="analysis-conversations">`, "review-only turn")
	for _, want := range []string{"3 requests", "Request #1", "first analysis turn", "file_read", "read arguments", "private tool result", "Request #2", "Memory compression", "compressed history", "Request #3", "analysis request failed", "Open execution log"} {
		var found bool
		_, body, found = strings.Cut(body, want)
		if !found {
			t.Fatalf("missing or out-of-order analysis detail %q", want)
		}
	}
	write("result.json", `{"execution_status":"skipped","errors":["Unsupported comparison"],"session_id":"review-id"}`)
	get(tab, 200, "Unsupported comparison", `id="evidence-index"`)
	write("result.json", `{"comparison":{"id":"comparison"},"session_id":"review-id","execution_status":"completed","submission_status":"accepted","scope":[{}, {}, {"excluded_reason":"binary"}],"usage":{"total_tokens":100},"observation_ids":["obs-000001","obs-000002","obs-000003"],"cited_observation_ids":["obs-000003","obs-000001"]}`)
	get(tab, 200, "accepted report file is missing", "panic")
	write("report.md", "## Report body\n\nSee [[obs-000003]] and [[obs-000001]].\n\n![external image](https://assets.example.org/unrequested.png)")
	write("observations.jsonl", `{"observation_id":"obs-000001","tool_name":"file_read","locations":[{"side":"head","path":"main.go","start_line":2,"end_line":8}],"truncated":true,"returned_content":"evidence text"}`)
	get(page, 200, "review-only turn", "first analysis turn")
	get(page, 200, "?tab=analysis", "Report body")
	get(tab, 200, "Report body", `"type":"llm_request"`)
	get(tab, 200, "<h2>Report body</h2>", "unrequested.png")
	get(tab, 200, "Changed files in scope</dt><dd>2", "evidence text")
	index := get(tab, 200, `<details class="file-accordion section-accordion" id="evidence-index">`, "evidence text")
	for _, id := range []string{"obs-000001", "obs-000002", "obs-000003"} {
		if strings.Count(index, `data-observation-id="`+id+`"`) != 1 {
			t.Fatalf("expected exactly one evidence index link for %s", id)
		}
	}
	for _, want := range []string{"2 cited &middot; 3 total", "Cited in the report", `data-observation-id="obs-000003"`, `data-observation-id="obs-000001"`, "Not cited in the report", `data-observation-id="obs-000002"`} {
		var found bool
		_, index, found = strings.Cut(index, want)
		if !found {
			t.Fatalf("missing or misplaced evidence index entry: %s", want)
		}
	}
	detail := get(tab+"&evidence=obs-000001", 200, "evidence text", "review-only turn")
	for _, want := range []string{`#evidence" aria-current="true">obs-000001`, "main.go:2–8", "Truncated result."} {
		if !strings.Contains(detail, want) {
			t.Fatalf("missing selected evidence detail: %s", want)
		}
	}
	get(tab+"&evidence=no-such-id", 404, "", "evidence text")
	get(tab+"&log=trace", 200, "private tool result", "Report body")
	var exported strings.Builder
	if err := ExportSession(&exported, root, "repo", "review-id"); err != nil || !strings.Contains(exported.String(), "Report body") || strings.Contains(exported.String(), "private tool result") || strings.Contains(exported.String(), "evidence text") {
		t.Fatalf("unexpected report export: %v", err)
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
	// Chmod(0000) on Windows only sets the read-only bit, so ReadDir still
	// succeeds and the handler returns 200. (The Getuid guard below cannot cover
	// this: Getuid returns -1 on Windows, never 0.)
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions not enforced on Windows")
	}
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
	t.Cleanup(func() { _ = os.Chmod(root, 0755) })

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

func TestHandleSession_GroupingRendersPaths(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// json.Marshal each record so the embedded newlines in the file list and the
	// quotes in the response JSON are escaped correctly, instead of hand-writing
	// the escapes into a raw JSONL literal.
	mustJSON := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	fileList := "[0] MODIFIED   internal/agent/grouping.go (+66/-24)\n" +
		"[1] ADDED   internal/viewer/store.go (+120/-0)\n"
	writeJSONL(t, filepath.Join(repoDir, "grp.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj","model":"claude"}`,
		mustJSON(map[string]any{
			"type": "llm_request", "filePath": "__grouping__", "taskType": "grouping_task", "request_no": 1,
			"messages": []any{map[string]any{"role": "user", "content": fileList}},
		}),
		mustJSON(map[string]any{
			"type": "llm_response", "filePath": "__grouping__", "taskType": "grouping_task",
			"content": `[{"label":"grouping index switch","files":[0,1]}]`,
		}),
		`{"type":"session_end","duration_seconds":30}`)

	req := httptest.NewRequest("GET", "/r/repo/grp", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "grp")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	// The resolved paths and label must appear in the grouping-view markup.
	for _, want := range []string{"grouping-view", "internal/agent/grouping.go", "internal/viewer/store.go", "grouping index switch"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered session missing %q", want)
		}
	}
	// The raw index JSON is still available (collapsed) for audit, but the
	// primary view is the path list, not a bare "files":[0,1].
	if !strings.Contains(body, "Raw LLM response") {
		t.Error("raw LLM response fallback should still be present for audit")
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

// writeMarkIdentityFixture writes a session jsonl with a uuid-bearing
// review_item_done record (two comments, identical content) plus a legacy
// uuid-less record (also with a duplicate), so both MarkID identity forms
// and their duplicate handling are exercised end to end.
func writeMarkIdentityFixture(t *testing.T, root, repo, sid string) {
	t.Helper()
	repoDir := filepath.Join(root, repo)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, sid+".jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"review_item_done","uuid":"11111111-2222-3333-4444-555555555555","filePath":"main.go","comments":[`+
			`{"content":"same finding","path":"main.go"},`+
			`{"content":"same finding","path":"main.go"}]}`,
		`{"type":"review_item_done","filePath":"legacy.go","comments":[`+
			`{"content":"legacy finding","path":"legacy.go"},`+
			`{"content":"legacy finding","path":"legacy.go"}]}`,
		`{"type":"session_end","duration_seconds":5,"files_reviewed":["main.go"]}`,
	)
}

// The session page is the marks UI's only server dependency: it must render
// each card's stable data-mark-id plus the client-side buttons, and carry no
// mark state itself — marks are browser state, applied by session.js.
func TestHandleSession_RendersMarkIdentityNotState(t *testing.T) {
	root := t.TempDir()
	writeMarkIdentityFixture(t, root, "repo", "s1")

	req := httptest.NewRequest("GET", "/r/repo/s1", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "s1")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`data-mark-id="11111111-2222-3333-4444-555555555555#0"`,
		`data-mark-id="11111111-2222-3333-4444-555555555555#1"`,
		`data-set-mark="fixed"`, `data-set-mark="ignored"`,
		`data-hide-marked`, `data-clear-all-marks`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("render missing %s", want)
		}
	}
	if strings.Contains(body, `data-mark="`) {
		t.Error("render carries a mark state; state must be client-side only")
	}
	if strings.Contains(body, `data-set-mark="solved"`) {
		t.Error("render still offers the collapsed solved state")
	}
}

// The viewer is read-only: no route may accept a write. The mux registers its
// document routes with GET-only patterns, so any state-changing method is
// answered by the ServeMux itself with 405 + Allow, and unmatched paths keep
// 404ing. Driving newMux directly — rather than a live StartServer, whose
// root comes from SessionsRoot() and whose goroutine outlives the test —
// keeps the fixture root real, binds no ports, and leaks nothing. New routes
// must register method-qualified patterns or these rows stop holding.
func TestMux_HasNoWriteRoutes(t *testing.T) {
	root := t.TempDir()
	writeMarkIdentityFixture(t, root, "repo", "s1")

	mux := newMux(root)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"POST session route", http.MethodPost, "/r/repo/s1", http.StatusMethodNotAllowed},
		{"PUT session route", http.MethodPut, "/r/repo/s1", http.StatusMethodNotAllowed},
		{"DELETE session route", http.MethodDelete, "/r/repo/s1", http.StatusMethodNotAllowed},
		{"PATCH session route", http.MethodPatch, "/r/repo/s1", http.StatusMethodNotAllowed},
		{"POST repo route", http.MethodPost, "/r/repo", http.StatusMethodNotAllowed},
		{"PUT repo route", http.MethodPut, "/r/repo", http.StatusMethodNotAllowed},
		{"DELETE repo route", http.MethodDelete, "/r/repo", http.StatusMethodNotAllowed},
		{"PATCH repo route", http.MethodPatch, "/r/repo", http.StatusMethodNotAllowed},
		{"POST compare route", http.MethodPost, "/r/repo/compare", http.StatusMethodNotAllowed},
		{"DELETE compare route", http.MethodDelete, "/r/repo/compare", http.StatusMethodNotAllowed},
		{"POST root", http.MethodPost, "/", http.StatusMethodNotAllowed},
		{"PUT root", http.MethodPut, "/", http.StatusMethodNotAllowed},
		{"DELETE root", http.MethodDelete, "/", http.StatusMethodNotAllowed},
		{"PATCH root", http.MethodPatch, "/", http.StatusMethodNotAllowed},
		{"OPTIONS root", http.MethodOptions, "/", http.StatusMethodNotAllowed},
		{"POST static asset", http.MethodPost, "/static/session.js", http.StatusMethodNotAllowed},
		{"PUT static asset", http.MethodPut, "/static/session.js", http.StatusMethodNotAllowed},
		{"DELETE static asset", http.MethodDelete, "/static/session.js", http.StatusMethodNotAllowed},
		{"GET session route still served", http.MethodGet, "/r/repo/s1", http.StatusOK},
		{"HEAD session route still served", http.MethodHead, "/r/repo/s1", http.StatusOK},
		{"GET repo route still served", http.MethodGet, "/r/repo", http.StatusOK},
		{"GET static asset still served", http.MethodGet, "/static/session.js", http.StatusOK},
		{"GET sessions list script still served", http.MethodGet, "/static/sessions.js", http.StatusOK},
		{"POST to unknown write-looking path stays 404", http.MethodPost, "/r/repo/s1/marks", http.StatusNotFound},
		{"GET unknown path stays 404", http.MethodGet, "/nope", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("{}"))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rr.Code, tt.want)
			}
		})
	}
}

// TestStaticAssetRefs_OnReposAndSessionsPages holds the two listing pages to the
// same a11y script reference the session detail page already asserts. Both
// repos.html and sessions.html hand their scrollable table regions to
// window.ocrArrowScroll, so dropping the tag kills arrow-key scrolling with
// nothing failing: the page still renders, only the keyboard path is dead.
func TestStaticAssetRefs_OnReposAndSessionsPages(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "s1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj","model":"claude"}`,
		`{"type":"session_end","duration_seconds":30,"files_reviewed":["main.go"]}`)

	tests := []struct {
		name string
		call func(w http.ResponseWriter, r *http.Request)
	}{
		{"repos", func(w http.ResponseWriter, r *http.Request) { handleRepos(w, r, root) }},
		{"sessions", func(w http.ResponseWriter, r *http.Request) { handleSessions(w, r, root, "repo") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			rr := httptest.NewRecorder()
			tt.call(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), `src="/static/a11y.js"`) {
				t.Errorf("%s page is missing src=\"/static/a11y.js\": "+
					"the scrollable table regions lose arrow-key scrolling", tt.name)
			}
		})
	}
}
