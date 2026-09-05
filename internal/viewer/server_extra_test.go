// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseTemplate_ReposHTML(t *testing.T) {
	tmpl, err := parseTemplate("repos.html")
	if err != nil {
		t.Fatalf("parseTemplate(repos.html) error: %v", err)
	}
	if tmpl == nil {
		t.Fatal("parseTemplate returned nil template")
	}
}

func TestParseTemplate_SessionsHTML(t *testing.T) {
	tmpl, err := parseTemplate("sessions.html")
	if err != nil {
		t.Fatalf("parseTemplate(sessions.html) error: %v", err)
	}
	if tmpl == nil {
		t.Fatal("parseTemplate returned nil template")
	}
}

func TestParseTemplate_SessionHTML(t *testing.T) {
	tmpl, err := parseTemplate("session.html")
	if err != nil {
		t.Fatalf("parseTemplate(session.html) error: %v", err)
	}
	if tmpl == nil {
		t.Fatal("parseTemplate returned nil template")
	}
}

func TestParseTemplate_NonExistent(t *testing.T) {
	_, err := parseTemplate("nonexistent.html")
	if err == nil {
		t.Error("expected error for non-existent template")
	}
}

// Execute each page independently: parsing alone misses undefined partials,
// and parsing every page together can overwrite page-specific breadcrumbs.
func TestParseTemplate_SharedHeader(t *testing.T) {
	tests := []struct {
		name       string
		data       any
		breadcrumb string
	}{
		{
			name:       "repos.html",
			data:       map[string]any{"Repos": []RepoInfo{{EncodedPath: "my-repo", SessionCount: 1}}},
			breadcrumb: "",
		},
		{
			name: "sessions.html",
			data: sessionsData{
				EncodedRepo: "my-repo",
				RepoName:    "MyRepo",
				Sessions:    []SessionSummary{{SessionID: "0123456789abcdef"}},
			},
			breadcrumb: `<span class="sep">/</span><span class="current">MyRepo</span>`,
		},
		{
			name: "session.html",
			data: sessionPageData{
				EncodedRepo: "my-repo",
				RepoName:    "MyRepo",
				Session:     &ViewSession{Summary: SessionSummary{SessionID: "0123456789abcdef"}},
			},
			breadcrumb: `<span class="sep">/</span><a href="/r/my-repo">MyRepo</a><span class="sep">/</span><span class="current">0123456789ab…</span>`,
		},
		{
			name: "compare.html",
			data: comparePageData{
				EncodedRepo: "my-repo",
				RepoName:    "MyRepo",
				Before:      SessionSummary{SessionID: "before"},
				After:       SessionSummary{SessionID: "after"},
			},
			breadcrumb: `<span class="sep">/</span><a href="/r/my-repo">MyRepo</a><span class="sep">/</span><span class="current">compare</span>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := parseTemplate(tt.name)
			if err != nil {
				t.Fatalf("parseTemplate: %v", err)
			}
			if tmpl.Lookup("app-header") == nil {
				t.Fatal("shared app-header template is missing")
			}
			var output strings.Builder
			if err := tmpl.Execute(&output, tt.data); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			body := output.String()
			for _, marker := range []string{`<nav class="breadcrumb">`, `class="nav-brand"`, `class="brand-icon"`} {
				if count := strings.Count(body, marker); count != 1 {
					t.Errorf("count of %q = %d, want 1", marker, count)
				}
			}
			// The brand-icon inlines the logo SVG, so assert the surrounding
			// structure plus an inline <svg> rather than an exact glyph body.
			const head = `<nav class="breadcrumb"><a href="/" class="nav-brand"><span class="brand-icon" aria-hidden="true"><svg`
			tail := `</span>Open Code Review Viewer</a>` + tt.breadcrumb + `</nav>`
			if !strings.Contains(body, head) || !strings.Contains(body, tail) {
				t.Error("expected shared home link, inline logo, wordmark and page-specific breadcrumbs")
			}
		})
	}
}

func TestRenderTemplate_Success(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "repos.html", map[string]any{
		"Repos": []RepoInfo{},
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "No session data found") {
		t.Errorf("expected empty repos message in rendered output")
	}
	// The search input is useless without a table and repos.js only loads
	// alongside rows, so it must stay inside the {{if .Repos}} branch.
	if strings.Contains(rr.Body.String(), "repository-search-input") {
		t.Error("empty repositories page should not render the search input")
	}
}

func TestRenderTemplate_WithRepos(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "repos.html", map[string]any{
		"Repos": []RepoInfo{
			{EncodedPath: "my-project", SessionCount: 3},
			{EncodedPath: "other-project", SessionCount: 1},
		},
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, required := range []string{
		"my-project",
		"other-project",
		`id="repository-search-input"`,
		`id="repositories-table"`,
		"data-repository-name",
		`src="/static/repos.js"`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("rendered repository page missing %q", required)
		}
	}
}

func TestRenderTemplate_BadTemplate(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "nonexistent.html", nil)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "template error") {
		t.Errorf("expected template error message")
	}
}

func TestRenderTemplate_Sessions(t *testing.T) {
	tests := []struct {
		name     string
		sessions []SessionSummary
	}{
		{name: "empty", sessions: []SessionSummary{}},
		{name: "populated", sessions: []SessionSummary{{SessionID: "session-123", GitBranch: "main"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			renderTemplate(rr, "sessions.html", sessionsData{
				EncodedRepo: "test-repo",
				RepoName:    "MyProject",
				Sessions:    tt.sessions,
			})

			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
			}
			body := rr.Body.String()
			if len(tt.sessions) > 0 && !strings.Contains(body, `href="/r/test-repo/session-123"`) {
				t.Errorf("expected populated session link in rendered output")
			}
			if !strings.Contains(body, "MyProject") {
				t.Errorf("expected repo name in sessions template")
			}
			if !strings.Contains(body, `<a class="back-link" href="/" aria-label="Back to repositories">`) {
				t.Errorf("expected back link to repositories in sessions template")
			}
			if !strings.Contains(body, `<a href="/" class="nav-brand">`) {
				t.Errorf("expected breadcrumb navigation to remain in sessions template")
			}
		})
	}
}

func TestRenderTemplate_SessionsTableMockup(t *testing.T) {
	const fullID = "b029c726-7b6b-46aa-b923-9fea9f012345"
	rr := httptest.NewRecorder()
	renderTemplate(rr, "sessions.html", sessionsData{
		EncodedRepo: "my-repo",
		RepoName:    "my-project",
		Sessions: []SessionSummary{
			{
				SessionID:     fullID,
				GitBranch:     "refactor/rename-runprofile",
				ReviewMode:    "range",
				Model:         "claude-opus-5",
				FileCount:     8,
				TerminalState: "complete",
				CommentCount:  5,
				DurationSec:   290,
			},
			{SessionID: "older-session"},
		},
	})
	body := rr.Body.String()

	const header = `<thead><tr><th>Session ID</th><th>Branch</th><th>Mode</th><th>Model</th>` +
		`<th>Files</th><th>Status</th><th>Comments</th><th>Duration</th><th>Started At</th><th class="col-action">Action</th></tr></thead>`
	for _, want := range []string{
		header,
		`id="sessions-table"`,
		`<a class="back-link" href="/" aria-label="Back to repositories"><svg`,
		`<td class="col-session"><a class="session-id" href="/r/my-repo/` + fullID + `" title="` + fullID + `">Session: b029c726-7b6b-46aa-b923-9fea9f…</a></td>`,
		`<td class="col-branch">refactor/rename-runprofile</td>`,
		`<td class="col-mode">range</td>`,
		`<td class="col-model">claude-opus-5</td>`,
		`<td class="col-files">8</td>`,
		`<td>complete</td>`,
		`<td class="col-comments">5</td>`,
		`<td class="col-duration">4m50s</td>`,
		`<a href="/r/my-repo/compare?before=older-session&amp;after=` + fullID + `">Check</a>`,
		`id="sessions-pagination"`,
		`data-page-step="-1"`,
		`data-page-step="1"`,
		`id="sessions-page-numbers"`,
		`src="/static/sessions.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered sessions page missing %q", want)
		}
	}
	if strings.Contains(body, "<code>claude-opus-5</code>") {
		t.Error("the model column should render as plain text, as in the mockup")
	}
	if strings.Contains(body, "<script>") {
		t.Error("sessions page must not carry an inline script")
	}
}

func TestSessionsJS_PagerContract(t *testing.T) {
	script, err := assets.ReadFile("static/sessions.js")
	if err != nil {
		t.Fatalf("read static/sessions.js: %v", err)
	}
	rr := httptest.NewRecorder()
	renderTemplate(rr, "sessions.html", sessionsData{
		EncodedRepo: "my-repo",
		RepoName:    "my-project",
		Sessions:    []SessionSummary{{SessionID: "s-new"}, {SessionID: "s-old"}},
	})
	body := rr.Body.String()
	if !strings.Contains(body, `<nav id="sessions-pagination" class="pagination" aria-label="Session pages" hidden>`) {
		t.Error("pager should render hidden until sessions.js enables it")
	}
	for _, id := range []string{"sessions-table", "sessions-pagination", "sessions-page-numbers"} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("sessions.html does not render #%s", id)
		}
		if !strings.Contains(string(script), `"`+id+`"`) {
			t.Errorf("sessions.js does not look up #%s", id)
		}
	}
	if !strings.Contains(string(script), "data-page-step") {
		t.Error("sessions.js should drive the template's page-step buttons")
	}
}

func TestSessionsCSS_PagerStaysHiddenUntilScripted(t *testing.T) {
	css, err := assets.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read static/style.css: %v", err)
	}
	guard := regexp.MustCompile(`\.sessions-page \.pagination\[hidden\] \{\s*display: none;`)
	if !guard.Match(css) {
		t.Error("style.css lost the .sessions-page .pagination[hidden] { display: none } guard: " +
			"the pager's own display: flex is an author rule, so it outranks the UA [hidden] rule " +
			"and the control would paint before sessions.js reveals it, and stay up with scripting off")
	}
}

func TestRenderTemplate_SessionPage(t *testing.T) {
	rr := httptest.NewRecorder()
	vs := &ViewSession{
		Summary: SessionSummary{
			SessionID: "abc",
			Model:     "gpt-4",
			CWD:       "/test",
		},
		Files: []*FileGroup{
			{
				FilePath: "main.go",
				Tasks: map[TaskType][]*TaskCard{
					MainTask: {
						{
							RequestNo:        1,
							ResponseContent:  "looks good",
							Model:            "gpt-4",
							PromptTokens:     100,
							CompletionTokens: 50,
						},
					},
				},
			},
		},
	}
	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "repo",
		RepoName:    "MyRepo",
		Session:     vs,
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<a class="back-link" href="/r/repo" aria-label="Back to sessions">`) {
		t.Errorf("expected back link to repository sessions in session template")
	}
	if !strings.Contains(body, `<a href="/r/repo">MyRepo</a>`) {
		t.Errorf("expected breadcrumb navigation to remain in session template")
	}
}

func TestRenderTemplate_SessionHeaderMockup(t *testing.T) {
	rr := httptest.NewRecorder()
	vs := &ViewSession{
		Summary: SessionSummary{
			SessionID:     "b029c726-7b6b",
			Model:         "claude-opus-5",
			CWD:           "/Users/kite/Documents/code/github/open-code-review",
			GitBranch:     "refactor/rename-runprofile",
			ReviewMode:    "range",
			DiffFrom:      "05af664",
			DiffTo:        "HEAD",
			TerminalState: "complete",
		},
	}
	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "my-repo",
		RepoName:    "MyRepo",
		Session:     vs,
	})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, required := range []string{
		`<main class="session-page">`,
		`aria-label="Back to sessions"><svg`,
		`<span class="meta-truncate" title="/Users/kite/Documents/code/github/open-code-review">`,
		`<span class="meta-truncate" title="refactor/rename-runprofile">`,
		`<strong>From:</strong> <code>05af664</code>`,
		`<strong>To:</strong> <code>HEAD</code>`,
		`<span class="meta-status status-complete">complete</span>`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("rendered session header missing %q", required)
		}
	}
	if strings.Contains(body, "<script>") {
		t.Error("session page must not contain inline <script> elements (CSP)")
	}
}

func TestRenderTemplate_SessionHeaderStatusClasses(t *testing.T) {
	cases := []struct {
		name      string
		aborted   bool
		legacy    bool
		termState string
		wantClass string
	}{
		{"complete", false, false, "complete", "status-complete"},
		{"partial", false, false, "partial", "status-partial"},
		{"failed", false, false, "failed", "status-failed"},
		{"skipped", false, false, "skipped", "status-legacy"},
		{"aborted", true, false, "complete", "status-aborted"},
		{"legacy", false, true, "complete", "status-legacy"},
		{"unknown state stays unclassed", false, false, "odd state", `>odd state</span>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			renderTemplate(rr, "session.html", sessionPageData{
				EncodedRepo: "repo",
				RepoName:    "MyRepo",
				Session: &ViewSession{
					Summary: SessionSummary{
						SessionID:     "abc",
						CWD:           "/test",
						Aborted:       tc.aborted,
						Legacy:        tc.legacy,
						TerminalState: tc.termState,
					},
				},
			})
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.wantClass) {
				t.Errorf("expected %q in rendered status markup", tc.wantClass)
			}
		})
	}
}

func TestRenderTemplate_SecondarySectionsCollapsedByDefault(t *testing.T) {
	rr := httptest.NewRecorder()
	vs := &ViewSession{
		Summary: SessionSummary{
			SessionID:     "abc",
			CWD:           "/test",
			FilesReviewed: []string{"main.go"},
		},
		TokenUsage: TokenUsageSummary{
			FileTokenBreakdown: []FileTokenUsage{{FilePath: "main.go"}},
		},
		Files: []*FileGroup{{FilePath: "main.go", Tasks: map[TaskType][]*TaskCard{}}},
		Comments: []*ReviewComment{{
			FilePath: "main.go",
			Content:  "Keep this visible",
		}},
	}

	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "repo",
		RepoName:    "MyRepo",
		Session:     vs,
	})

	body := rr.Body.String()
	if count := strings.Count(body, `<details class="file-accordion section-accordion">`); count != 2 {
		t.Fatalf("collapsed secondary section count = %d, want 2", count)
	}
	if strings.Contains(body, `<details class="file-accordion section-accordion" open>`) {
		t.Fatal("secondary sections should be collapsed by default")
	}
	if !strings.Contains(body, `<details class="token-breakdown">`) || strings.Contains(body, `<details class="token-breakdown" open>`) {
		t.Fatal("file token breakdown should be rendered and collapsed by default")
	}
	if !strings.Contains(body, `<details class="comment-file-group" open>`) {
		t.Fatal("review comment groups should remain expanded")
	}
}

func TestRenderTemplate_HidesEmptyConversationsSection(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "repo",
		RepoName:    "MyRepo",
		Session: &ViewSession{
			Summary: SessionSummary{SessionID: "abc", CWD: "/test"},
		},
	})

	if strings.Contains(rr.Body.String(), `<span class="section-title">Conversations</span>`) {
		t.Fatal("empty conversations section should not be rendered")
	}
}

func TestRenderTemplate_RendersConversationToolCallDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "repo",
		RepoName:    "MyRepo",
		Session: &ViewSession{
			Summary: SessionSummary{SessionID: "abc", CWD: "/test"},
			Files: []*FileGroup{{
				FilePath: "internal/viewer/server.go",
				Tasks: map[TaskType][]*TaskCard{
					MainTask: {{
						RequestNo:        1,
						Model:            "model-a",
						PromptTokens:     10,
						CompletionTokens: 20,
						DurationMs:       30,
						ToolCalls: []ToolCallInfo{{
							Name:      "code_search",
							Arguments: `{"query":"viewer"}`,
							Result:    "matched server.go",
							Ok:        true,
						}},
					}},
				},
			}},
		},
	})

	body := rr.Body.String()
	if strings.Contains(body, "1 files") || strings.Contains(body, "1 requests") {
		t.Error("single-item counts should use singular labels")
	}
	if strings.Contains(body, "&#9881;") {
		t.Error("tool call controls should use the shared SVG icon, not a Unicode glyph")
	}
	for _, want := range []string{
		"Conversations", "1 file", "1 request", "internal/viewer/server.go", "Request #1", "model-a",
		"Tool Calls (1)", "code_search", "Arguments", "matched server.go", "<svg",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered session page missing %q", want)
		}
	}
}

func TestRenderTemplate_ExecutionError(t *testing.T) {
	rr := httptest.NewRecorder()
	// Pass wrong data type to trigger template execution error
	// repos.html expects .Repos to be rangeable; passing a string causes execution error
	renderTemplate(rr, "repos.html", map[string]any{
		"Repos": "not-a-slice",
	})
	// Template execution may partially write before failing, so we just check it didn't panic
	// and that something was written (the header was set before Execute)
	ct := rr.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestStaticFS(t *testing.T) {
	sfs := staticFS()
	if sfs == nil {
		t.Fatal("staticFS() returned nil")
	}
	// Should be able to open style.css
	f, err := sfs.Open("style.css")
	if err != nil {
		t.Fatalf("failed to open style.css from staticFS: %v", err)
	}
	_ = f.Close()
}

func TestResolveAllowedHostsFromEnv(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "custom.host,other.host")
	allowed := resolveAllowedHostsFromEnv("192.168.1.5:5483")

	if _, ok := allowed["localhost"]; !ok {
		t.Error("missing localhost")
	}
	if _, ok := allowed["192.168.1.5"]; !ok {
		t.Error("missing bind host")
	}
	if _, ok := allowed["custom.host"]; !ok {
		t.Error("missing custom.host from env")
	}
	if _, ok := allowed["other.host"]; !ok {
		t.Error("missing other.host from env")
	}
}

func TestBuildAllowedHosts_BracketedIPv6(t *testing.T) {
	a := buildAllowedHosts("[fe80::1]", "")
	if _, ok := a["fe80::1"]; !ok {
		t.Errorf("bracketed IPv6 bind host not stripped: %v", a)
	}
}

func TestResolveAllowedHostsFromEnv_NoEnv(t *testing.T) {
	t.Setenv(EnvAllowedHosts, "")
	allowed := resolveAllowedHostsFromEnv(":5483")

	if len(allowed) != 3 {
		t.Errorf("expected 3 default hosts, got %d: %v", len(allowed), allowed)
	}
}

func TestTemplateFuncTaskTypeClass(t *testing.T) {
	tmpl, err := parseTemplate("session.html")
	if err != nil {
		t.Fatal(err)
	}

	// Verify we can execute with task data that exercises taskTypeClass
	rr := httptest.NewRecorder()
	vs := &ViewSession{
		Summary: SessionSummary{SessionID: "x", CWD: "/p"},
		Files: []*FileGroup{
			{
				FilePath: "f.go",
				Tasks: map[TaskType][]*TaskCard{
					PlanTask:              {{RequestNo: 1, ResponseContent: "plan"}},
					MainTask:              {{RequestNo: 2, ResponseContent: "main"}},
					MemoryCompressionTask: {{RequestNo: 3, ResponseContent: "mem"}},
					ReLocationTask:        {{RequestNo: 4, ResponseContent: "reloc"}},
					TaskType("custom"):    {{RequestNo: 5, ResponseContent: "custom"}},
				},
			},
		},
	}
	err = tmpl.Execute(rr, sessionPageData{
		EncodedRepo: "r",
		RepoName:    "R",
		Session:     vs,
	})
	if err != nil {
		t.Errorf("template execution with all task types: %v", err)
	}
}

func TestInlineIcon(t *testing.T) {
	// Known icons return their embedded SVG markup.
	for _, name := range []string{"logo", "search", "settings", "file", "chevron-left", "chevron-right", "chevron-down"} {
		got := string(inlineIcon(name))
		if !strings.Contains(got, "<svg") || !strings.Contains(got, "currentColor") {
			t.Errorf("inlineIcon(%q) = %q, want inline svg using currentColor", name, got)
		}
	}
	// Malformed or out-of-range names return empty markup instead of reading
	// arbitrary files. Uppercase, slashes, dots and traversal are all rejected
	// by the name guard; a well-formed but unknown name misses the embed.
	for _, name := range []string{"", "Search", "foo/bar", "../style", "a.b", "chevron_left", "missing"} {
		if got := inlineIcon(name); got != "" {
			t.Errorf("inlineIcon(%q) = %q, want empty", name, got)
		}
	}
}

func TestRenderTemplate_ReposSearchIcon(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "repos.html", map[string]any{
		"Repos": []RepoInfo{{EncodedPath: "my-project", SessionCount: 1}},
	})
	body := rr.Body.String()
	if !strings.Contains(body, `<span class="search-icon" aria-hidden="true"><svg`) {
		t.Error("repos search box should render the inline search icon")
	}
}

func TestRenderTemplate_ToolCallIconIsInlineSVG(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "repo",
		RepoName:    "MyRepo",
		Session: &ViewSession{
			Summary: SessionSummary{SessionID: "abc", CWD: "/test"},
			Files: []*FileGroup{{
				FilePath: "internal/viewer/server.go",
				Tasks: map[TaskType][]*TaskCard{
					MainTask: {{
						RequestNo: 1,
						Model:     "model-a",
						ToolCalls: []ToolCallInfo{{Name: "code_search", Ok: true}},
					}},
				},
			}},
		},
	})
	body := rr.Body.String()
	if strings.Contains(body, "&#9881;") || strings.Contains(body, "⚙") {
		t.Error("tool-call icon should no longer use the unicode gear glyph")
	}
	if !strings.Contains(body, `<span class="tool-calls-icon" aria-hidden="true"><svg`) {
		t.Error("tool-calls label should render the inline settings icon")
	}
	if strings.Contains(body, `class="tool-icon"`) {
		t.Error("individual tool-call rows should not render a duplicate settings icon")
	}
}

func TestRenderTemplate_FilesReviewedUseFileIcon(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "session.html", sessionPageData{
		EncodedRepo: "repo",
		RepoName:    "MyRepo",
		Session: &ViewSession{
			Summary: SessionSummary{
				SessionID:     "abc",
				CWD:           "/test",
				FilesReviewed: []string{"internal/agent/agent.go"},
			},
		},
	})
	body := rr.Body.String()
	if !strings.Contains(body, `<span class="file-list-icon" aria-hidden="true"><svg`) {
		t.Error("Files Reviewed rows should render the inline file icon")
	}
	if !strings.Contains(body, "internal/agent/agent.go") {
		t.Error("Files Reviewed should still render the file path")
	}
}

func TestRenderTemplate_ReposTableMockup(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "repos.html", map[string]any{
		"Repos": []RepoInfo{{EncodedPath: "my-project", SessionCount: 3}},
	})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, required := range []string{
		`<main class="repos-page">`,
		`<th scope="col" class="col-action">Action</th>`,
		`<a class="repo-check" href="/r/my-project">Check</a>`,
		`<td class="col-repository" data-repository-name><a href="/r/my-project">my-project</a></td>`,
		`aria-label="Previous page"><svg`,
		`aria-label="Next page"><svg`,
		`<nav id="repos-pagination" class="pagination" aria-label="Repository pages" hidden>`,
		`data-page-step="-1"`,
		`data-page-step="1"`,
		`id="repos-page-numbers"`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("rendered repositories page missing %q", required)
		}
	}
	if strings.Contains(body, "<script>") {
		t.Error("repositories page must not contain inline <script> elements (CSP)")
	}
}

func TestReposJS_PagerContract(t *testing.T) {
	html, err := os.ReadFile(filepath.Join("templates", "repos.html"))
	if err != nil {
		t.Fatalf("read repos.html: %v", err)
	}
	js, err := os.ReadFile(filepath.Join("static", "repos.js"))
	if err != nil {
		t.Fatalf("read repos.js: %v", err)
	}
	for _, id := range []string{
		"repository-search-input",
		"repositories-table",
		"repos-pagination",
		"repos-page-numbers",
	} {
		if !strings.Contains(string(html), `id="`+id+`"`) {
			t.Errorf("repos.html is missing id %q", id)
		}
		if !strings.Contains(string(js), `getElementById("`+id+`")`) {
			t.Errorf("repos.js does not look up id %q", id)
		}
	}
	if !strings.Contains(string(js), "data-page-step") {
		t.Error("repos.js should drive the pagination step buttons via data-page-step")
	}
}

func TestReposCSS_PagerStaysHiddenUntilScripted(t *testing.T) {
	css, err := os.ReadFile(filepath.Join("static", "style.css"))
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	guard := regexp.MustCompile(`\.repos-page \.pagination\[hidden\] \{\s*display: none;`)
	if !guard.Match(css) {
		t.Error("style.css lost the .repos-page .pagination[hidden] { display: none } guard: " +
			"the pager's own display: flex is an author rule, so it outranks the UA [hidden] rule " +
			"and the pager would paint before repos.js reveals it")
	}
}

// TestHandleSession_ServedPageKeepsStaticRefs guards the other half of the
// export gates in session.html. sessionPageData.Static is false for every HTTP
// render, so the served page must still link the two /static/ assets and keep
// its breadcrumb anchors — inlining them over HTTP would defeat the browser
// cache, and the {{else}} branches are otherwise untested.
func TestHandleSession_ServedPageKeepsStaticRefs(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "srv1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-01T10:00:00Z","cwd":"/my/proj","model":"claude"}`,
		`{"type":"session_end","duration_seconds":30,"files_reviewed":["main.go"]}`)

	req := httptest.NewRequest("GET", "/r/repo/srv1", nil)
	rr := httptest.NewRecorder()
	handleSession(rr, req, root, "repo", "srv1")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`href="/static/style.css"`,
		`src="/static/session.js"`,
		`<a href="/" class="nav-brand"`,
		`<a href="/r/repo">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("served page missing %q", want)
		}
	}
	if strings.Contains(body, `<span class="crumb">`) {
		t.Error("served page de-linked the repo crumb; that is export-only")
	}
}
