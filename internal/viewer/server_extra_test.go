package viewer

import (
	"net/http"
	"net/http/httptest"
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
}

func TestRenderTemplate_WithRepos(t *testing.T) {
	rr := httptest.NewRecorder()
	renderTemplate(rr, "repos.html", map[string]any{
		"Repos": []RepoInfo{
			{EncodedPath: "my-project", SessionCount: 3},
		},
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "my-project") {
		t.Errorf("expected repo name in rendered output")
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
	rr := httptest.NewRecorder()
	renderTemplate(rr, "sessions.html", sessionsData{
		EncodedRepo: "test-repo",
		RepoName:    "MyProject",
		Sessions:    []SessionSummary{},
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "MyProject") {
		t.Errorf("expected repo name in sessions template")
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
	f.Close()
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
