// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"net"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStartServer_SessionsRootError forces os.UserHomeDir to fail by clearing
// HOME so StartServer returns before binding a socket.
func TestStartServer_SessionsRootError(t *testing.T) {
	t.Setenv("HOME", "")
	// On unix os.UserHomeDir errors when HOME is empty.
	if _, err := SessionsRoot(); err == nil {
		t.Skip("home dir resolvable despite empty HOME; platform-specific")
	}
	if err := StartServer("127.0.0.1:0"); err == nil {
		t.Fatal("expected StartServer to fail when sessions root cannot resolve")
	}
}

// TestStartServer_AddrInUse runs the full setup path (routes, host guard,
// security headers, server construction) and then fails fast on ListenAndServe
// because the port is already bound — no goroutine leak.
func TestStartServer_AddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()

	err = StartServer(ln.Addr().String())
	if err == nil {
		t.Fatal("expected StartServer to fail binding an in-use address")
	}
}

// TestParseTemplate_SessionWithComments renders session.html with review
// comments spanning several severities and categories so the template helpers
// (severityCounts, categoryCounts, severityClass, categoryClass,
// groupCommentsByFile, and the normalization helpers) execute.
func TestParseTemplate_SessionWithComments(t *testing.T) {
	tmpl, err := parseTemplate("session.html")
	if err != nil {
		t.Fatalf("parseTemplate: %v", err)
	}

	comments := []*ReviewComment{
		{FilePath: "a.go", Content: "c1", Category: "bug", Severity: "critical", StartLine: 1, EndLine: 2},
		{FilePath: "a.go", Content: "c2", Category: "security", Severity: "high"},
		{FilePath: "b.go", Content: "c3", Category: "performance", Severity: "medium"},
		{FilePath: "b.go", Content: "c4", Category: "docs", Severity: "low"},
	}
	vs := &ViewSession{
		Summary:  SessionSummary{SessionID: "s", CWD: "/p"},
		Comments: comments,
		Files: []*FileGroup{
			{FilePath: "a.go", Tasks: map[TaskType][]*TaskCard{
				MainTask: {{RequestNo: 1, ResponseContent: "ok", DurationMs: 1500, PromptTokens: 1200, CompletionTokens: 2_000_000}},
			}},
		},
	}

	rr := httptest.NewRecorder()
	if err := tmpl.Execute(rr, sessionPageData{EncodedRepo: "r", RepoName: "R", Session: vs}); err != nil {
		t.Fatalf("execute session.html with comments: %v", err)
	}
	if !strings.Contains(rr.Body.String(), "Review Comments") {
		t.Error("rendered page missing Review Comments section")
	}
	body := rr.Body.String()
	for _, want := range []string{
		`<span class="comment-filter-label">Severity:</span>`,
		`<span class="comment-filter-label">Category:</span>`,
		`data-filter-kind="severity" data-filter-value="all"`,
		`data-filter-kind="category" data-filter-value="all"`,
		`data-filter-kind="severity" data-filter-value="critical"`,
		`data-filter-kind="category" data-filter-value="bug"`,
		`data-filter-kind="category" data-filter-value="other"`,
		`data-comment-card data-category="bug" data-severity="critical"`,
		`data-comment-card data-category="other" data-severity="low"`,
		`data-comment-filter-empty`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
}

func TestCategoryCounts_NormalizesUnknownCategories(t *testing.T) {
	counts := categoryCounts([]*ReviewComment{
		{Category: "bug"},
		{Category: "MAINTAINABILITY"},
		{Category: ""},
		{Category: "not-a-category"},
	})
	if counts.Bug != 1 || counts.Maintainability != 1 || counts.Other != 2 {
		t.Fatalf("unexpected category counts: %+v", counts)
	}
}
