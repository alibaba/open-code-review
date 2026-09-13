// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

// compareFinding renders one comment object for a review_item_done record.
// suggestion is the fix the LLM proposed; it is not part of session.Compare's
// finding key, but the page has to render it or a reader loses the patch the
// CLI prints.
func compareFinding(path, content, existing, suggestion string) string {
	return fmt.Sprintf(`{"path":%q,"content":%q,"existing_code":%q,"suggestion_code":%q,"category":"bug","severity":"high","start_line":7,"end_line":7}`,
		path, content, existing, suggestion)
}

// fullCoverage is a v1 manifest coverage object marking every given path as
// completed, which is what makes an unmatched before-finding read as resolved
// rather than not-reviewed.
func fullCoverage(paths ...string) string {
	var items []string
	for _, p := range paths {
		items = append(items, fmt.Sprintf(`{"item_id":%q,"path":%q}`, p, p))
	}
	joined := strings.Join(items, ",")
	return `{"selected":[` + joined + `],"completed":[` + joined + `],"reused":[],"failed":[],"waived":[]}`
}

// splitCoverage is fullCoverage's counterpart for a run that stopped early:
// every path is selected, only some are completed. It is what separates
// session.ReviewedPaths (completed+reused) from Coverage.Selected - a handler
// reading Selected would report the files the run never reached as resolved.
func splitCoverage(selected, completed []string) string {
	items := func(paths []string) string {
		var out []string
		for _, p := range paths {
			out = append(out, fmt.Sprintf(`{"item_id":%q,"path":%q}`, p, p))
		}
		return strings.Join(out, ",")
	}
	return `{"selected":[` + items(selected) + `],"completed":[` + items(completed) +
		`],"reused":[],"failed":[],"waived":[]}`
}

// writeCompareSession writes a minimal session JSONL: session_start, one
// review_item_done carrying the findings, and session_end. Pass an empty
// coverage string for a legacy session that recorded no run manifest.
func writeCompareSession(t *testing.T, dir, id, reviewMode, coverage string, findings ...string) {
	t.Helper()
	end := `{"type":"session_end","duration_seconds":1,"files_reviewed":["a.go"]}`
	if coverage != "" {
		end = `{"type":"session_end","duration_seconds":1,"run_manifest":{"schema_version":"ocr.run-manifest/v1",` +
			`"run_id":"r","operation":"review","terminal_state":"complete","repository":{},"input":{},` +
			`"execution":{},"coverage":` + coverage + `,"elapsed_ms":1}}`
	}
	writeJSONL(t, filepath.Join(dir, id+".jsonl"),
		fmt.Sprintf(`{"type":"session_start","timestamp":"2025-03-01T09:00:00Z","cwd":"/home/user/project","model":"gpt-4","reviewMode":%q}`, reviewMode),
		`{"type":"review_item_done","filePath":"a.go","comments":[`+strings.Join(findings, ",")+`]}`,
		end)
}

func TestToLlmComments(t *testing.T) {
	t.Parallel() // pure function: no shared state, no t.Setenv, no temp dirs
	full := &ReviewComment{
		FilePath:       "a.go",
		Content:        "content",
		SuggestionCode: "sugg",
		ExistingCode:   "existing",
		StartLine:      3,
		EndLine:        5,
		Category:       "bug",
		Severity:       "high",
	}
	tests := []struct {
		name string
		in   []*ReviewComment
		want []model.LlmComment
	}{
		{name: "nil slice", in: nil, want: []model.LlmComment{}},
		{name: "empty slice", in: []*ReviewComment{}, want: []model.LlmComment{}},
		{name: "nil element is skipped", in: []*ReviewComment{nil}, want: []model.LlmComment{}},
		{name: "zero value", in: []*ReviewComment{{}}, want: []model.LlmComment{{}}},
		{
			// FilePath -> Path is the rename, and ExistingCode/Category are two
			// of the three inputs to session.Compare's finding key.
			name: "every field is carried",
			in:   []*ReviewComment{full},
			want: []model.LlmComment{{
				Path:           "a.go",
				Content:        "content",
				SuggestionCode: "sugg",
				ExistingCode:   "existing",
				StartLine:      3,
				EndLine:        5,
				Category:       "bug",
				Severity:       "high",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toLlmComments(tt.in)
			if got == nil {
				t.Fatal("toLlmComments returned nil, want non-nil slice")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("toLlmComments = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestModeWarning(t *testing.T) {
	t.Parallel() // pure function
	tests := []struct {
		name          string
		before, after string
		want          string
	}{
		{name: "same mode", before: "commit", after: "commit", want: ""},
		{name: "both empty", before: "", after: "", want: ""},
		{
			name: "different modes", before: "commit", after: "workspace",
			want: "review modes differ (commit vs workspace); the two runs may not have looked at the same files",
		},
		{
			name: "before empty renders a dash", before: "", after: "workspace",
			want: "review modes differ (- vs workspace); the two runs may not have looked at the same files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := modeWarning(SessionSummary{ReviewMode: tt.before}, SessionSummary{ReviewMode: tt.after})
			if got != tt.want {
				t.Fatalf("modeWarning = %q, want %q", got, tt.want)
			}
		})
	}
}

// compareFixture builds a sessions root with the sessions every TestHandleCompare
// row draws on and returns it.
//
// s1 (before) and s2 (after) share one finding and differ in one, so a compare
// of the two exercises New, Persisting and Resolved at once. s3 has no findings
// and no manifest; s4 is s2's twin with a different review mode.
func compareFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	kept := compareFinding("a.go", "still broken", "x := 1", "x := 2")
	gone := compareFinding("a.go", "was broken", "y := 2", "")
	added := compareFinding("b.go", "newly broken", "z := 3", "z := 4")
	writeCompareSession(t, repoDir, "s1", "commit", fullCoverage("a.go"), kept, gone)
	writeCompareSession(t, repoDir, "s2", "commit", fullCoverage("a.go", "b.go"), kept, added)
	writeCompareSession(t, repoDir, "s3", "commit", fullCoverage())
	// A legacy session recorded no run manifest at all, which is what makes
	// session.ReviewedPaths return nil.
	writeCompareSession(t, repoDir, "legacy", "commit", "")
	writeCompareSession(t, repoDir, "s4", "workspace", fullCoverage("a.go", "b.go"), kept, added)
	// An interrupted run: it meant to review a.go and b.go and reached only
	// b.go, so a.go carries no verdict.
	writeCompareSession(t, repoDir, "partial", "commit",
		splitCoverage([]string{"a.go", "b.go"}, []string{"b.go"}))
	// A session recorded in a different working directory. Written from literal
	// lines because writeCompareSession hardcodes one cwd for all six of its
	// call sites.
	writeJSONL(t, filepath.Join(repoDir, "otherrepo.jsonl"),
		`{"type":"session_start","timestamp":"2025-03-01T09:00:00Z","cwd":"/home/user/other","model":"gpt-4","reviewMode":"commit"}`,
		`{"type":"review_item_done","filePath":"a.go","comments":[`+kept+`]}`,
		`{"type":"session_end","duration_seconds":1,"run_manifest":{"schema_version":"ocr.run-manifest/v1",`+
			`"run_id":"r","operation":"review","terminal_state":"complete","repository":{},"input":{},`+
			`"execution":{},"coverage":`+fullCoverage("a.go")+`,"elapsed_ms":1}}`)
	writeCompareSession(t, repoDir, "xss", "commit", fullCoverage("a.go"),
		compareFinding("a.go", "<script>alert(1)</script>", "<b>x</b> := 1", "<i>x</i> := 2"))
	return root
}

func TestHandleCompare(t *testing.T) {
	// No t.Parallel: handler_test.go's TestHandleRepos_PermissionDenied
	// os.Chmods a shared root, so isolation across this package is not proven.
	root := compareFixture(t)
	tests := []struct {
		name     string
		query    string
		status   int
		contains []string
		absent   []string
	}{
		{
			name: "happy path", query: "before=s1&after=s2", status: http.StatusOK,
			contains: []string{"New (1)", "Persisting (1)", "Resolved (1)", "Not reviewed (0)",
				"newly broken", "still broken", "was broken", "none"},
		},
		{
			// The CLI renders a suggested patch as a diff (renderComment ->
			// buildDiffLines); dropping it here would leave the page showing
			// prose only, which is not the parity #1104 asked for.
			name:  "a suggested patch renders, not just the prose",
			query: "before=s1&after=s2", status: http.StatusOK,
			contains: []string{
				"Existing Code", "Suggested Change",
				"x := 1", "x := 2", // persisting finding keeps its patch
				"z := 3", "z := 4", // new finding carries one too
				"y := 2", // resolved finding shows the code it used to flag
			},
		},
		{
			name: "not reviewed when the after run never looked at the file",
			// s1 found something in a.go; the after run covered only b.go, so
			// the unmatched before-finding is undecided, not fixed.
			query: "before=s1&after=s3", status: http.StatusOK,
			contains: []string{"New (0)", "Persisting (0)", "Resolved (0)", "Not reviewed (2)"},
		},
		{
			name:  "legacy after run reports unmatched findings as resolved",
			query: "before=s1&after=legacy", status: http.StatusOK,
			contains: []string{"Resolved (2)", "Not reviewed (0)"},
		},
		{
			name: "self compare is all persisting", query: "before=s1&after=s1", status: http.StatusOK,
			contains: []string{"New (0)", "Persisting (2)", "Resolved (0)", "Not reviewed (0)"},
		},
		{
			name: "mode mismatch warns and still renders", query: "before=s1&after=s4", status: http.StatusOK,
			contains: []string{
				"review modes differ (commit vs workspace); the two runs may not have looked at the same files",
				"New (1)", "Persisting (1)",
			},
		},
		{
			name: "same mode does not warn", query: "before=s1&after=s2", status: http.StatusOK,
			absent: []string{"review modes differ"},
		},
		{
			name: "findings are html escaped", query: "before=s1&after=xss", status: http.StatusOK,
			contains: []string{"&lt;script&gt;", "&lt;i&gt;x&lt;/i&gt; := 2"},
			absent:   []string{"<script>alert(1)", "<i>x</i> := 2"},
		},
		{
			name:  "page has no form, which form-action 'none' would silently block",
			query: "before=s1&after=s2", status: http.StatusOK,
			absent: []string{"<form", "formaction"},
		},
		{
			// The falsifier for the Coverage.Selected bug: a.go is selected but
			// not completed, so a handler reading Selected would call both of
			// s1's findings resolved. Only completed+reused is a verdict.
			name:  "interrupted after run does not resolve the files it never reached",
			query: "before=s1&after=partial", status: http.StatusOK,
			contains: []string{"New (0)", "Persisting (0)", "Resolved (0)", "Not reviewed (2)"},
		},
		{
			// Reachable because encodeRepoPath collapses "/" to "-", so
			// "/home/user/project" and "/home/user-project" share a directory.
			name: "cross repository compare is refused", query: "before=s1&after=otherrepo",
			status:   http.StatusBadRequest,
			contains: []string{"sessions belong to different repositories"},
		},
		{
			name: "missing before", query: "after=s2", status: http.StatusBadRequest,
			contains: []string{"query parameters 'before' and 'after' are required"},
		},
		{
			name: "missing after", query: "before=s1", status: http.StatusBadRequest,
			contains: []string{"query parameters 'before' and 'after' are required"},
		},
		{
			name: "missing both", query: "", status: http.StatusBadRequest,
			contains: []string{"query parameters 'before' and 'after' are required"},
		},
		{
			name: "traversal in before", query: "before=../s1&after=s2", status: http.StatusBadRequest,
			contains: []string{"invalid session id"},
		},
		{
			name: "percent encoded traversal in before", query: "before=%2e%2e%2fs1&after=s2",
			status: http.StatusBadRequest, contains: []string{"invalid session id"},
		},
		{
			name: "slash in after", query: "before=s1&after=a%2fb", status: http.StatusBadRequest,
			contains: []string{"invalid session id"},
		},
		{
			name: "backslash in after", query: `before=s1&after=a\b`, status: http.StatusBadRequest,
			contains: []string{"invalid session id"},
		},
		{
			name: "unknown before", query: "before=nope&after=s2", status: http.StatusNotFound,
			contains: []string{"Failed to load session:"},
		},
		{
			name: "unknown after", query: "before=s1&after=nope", status: http.StatusNotFound,
			contains: []string{"Failed to load session:"},
		},
		{
			// A lone dot is not traversal: it resolves to a missing "..jsonl".
			name: "single dot id", query: "before=.&after=s2", status: http.StatusNotFound,
			contains: []string{"Failed to load session:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/r/myrepo/compare?"+tt.query, nil)
			rr := httptest.NewRecorder()
			handleCompare(rr, req, root, "myrepo")

			if rr.Code != tt.status {
				t.Fatalf("status = %d, want %d (body: %s)", rr.Code, tt.status, rr.Body.String())
			}
			body := rr.Body.String()
			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Errorf("body does not contain %q", want)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(body, unwanted) {
					t.Errorf("body unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}

// TestRenderTemplate_SessionsCompareLink pins the only entry point to the
// compare page: one link per row, pointing at the next-older session.
func TestRenderTemplate_SessionsCompareLink(t *testing.T) {
	tests := []struct {
		name     string
		sessions []SessionSummary
		contains []string
		absent   []string
	}{
		{
			name:     "two sessions link newest to next oldest",
			sessions: []SessionSummary{{SessionID: "s-new"}, {SessionID: "s-old"}},
			// Rows are newest-first, so the oldest row has no link.
			contains: []string{"/compare?before=s-old&amp;after=s-new", "<th>Compare</th>"},
		},
		{
			name:     "a single session has nothing to compare against",
			sessions: []SessionSummary{{SessionID: "only"}},
			contains: []string{"<th>Compare</th>"},
			absent:   []string{"/compare?"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			renderTemplate(rr, "sessions.html", sessionsData{
				EncodedRepo: "myrepo",
				RepoName:    "MyProject",
				Sessions:    tt.sessions,
			})
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			body := rr.Body.String()
			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Errorf("body does not contain %q", want)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(body, unwanted) {
					t.Errorf("body unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}

// TestNewMux_RouteDispatch exercises the real ServeMux, not the handlers
// directly: "compare" is a literal segment competing with the {sessionID}
// wildcard, and nothing else in the suite would notice if the wildcard started
// winning and turned /r/myrepo/compare into a lookup for a session named
// "compare".
func TestNewMux_RouteDispatch(t *testing.T) {
	mux := newMux(compareFixture(t))
	tests := []struct {
		name     string
		target   string
		status   int
		contains []string
		absent   []string
	}{
		{
			name: "compare wins over the sessionID wildcard", target: "/r/myrepo/compare?before=s1&after=s2",
			status: http.StatusOK,
			// Session Compare, not Session Detail: proves handleCompare ran.
			contains: []string{"<title>Session Compare", "New (1)", "Persisting (1)"},
			absent:   []string{"<title>Session Detail"},
		},
		{
			// The sharpest regression signal: if the wildcard captured
			// sessionID="compare" this would be a 404 for a missing file
			// instead of the handler's own argument check.
			name:   "compare without query is the compare handler's 400, not a session 404",
			target: "/r/myrepo/compare", status: http.StatusBadRequest,
			contains: []string{"query parameters 'before' and 'after' are required"},
		},
		{
			name: "the wildcard still serves a real session", target: "/r/myrepo/s1",
			status: http.StatusOK, contains: []string{"<title>Session Detail"},
		},
		{
			name: "session list", target: "/r/myrepo", status: http.StatusOK,
			contains: []string{"Sessions:", "<th>Compare</th>"},
		},
		{
			name: "repo list", target: "/", status: http.StatusOK,
		},
		{
			// %5C is a path separator on Windows, so the repo guard has to
			// reject it the same way it rejects %2F.
			name:   "backslash in repo is rejected on the compare route",
			target: "/r/my%5Crepo/compare?before=s1&after=s2", status: http.StatusBadRequest,
			contains: []string{"invalid repo path"},
		},
		{
			name:   "backslash in repo is rejected on the session list route",
			target: "/r/my%5Crepo", status: http.StatusBadRequest,
			contains: []string{"invalid repo path"},
		},
		{
			name: "backslash in the session id is rejected", target: "/r/myrepo/a%5Cb",
			status: http.StatusBadRequest, contains: []string{"invalid path"},
		},
		{
			name: "slash in the session id is rejected", target: "/r/myrepo/a%2Fb",
			status: http.StatusBadRequest, contains: []string{"invalid path"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest("GET", tt.target, nil))
			if rr.Code != tt.status {
				t.Fatalf("GET %s status = %d, want %d (body: %s)", tt.target, rr.Code, tt.status, rr.Body.String())
			}
			body := rr.Body.String()
			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Errorf("body does not contain %q", want)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(body, unwanted) {
					t.Errorf("body unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}
