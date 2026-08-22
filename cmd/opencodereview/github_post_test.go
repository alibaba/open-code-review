// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/github"
	"github.com/alibaba/open-code-review/internal/model"
)

type postedGitHubComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine *int   `json:"start_line"`
	StartSide string `json:"start_side"`
	Position  *int   `json:"position"`
}

type postedGitHubReview struct {
	CommitID string                `json:"commit_id"`
	Body     string                `json:"body"`
	Comments []postedGitHubComment `json:"comments"`
}

func newGitHubPostingTestRepo(t *testing.T) (string, string) {
	t.Helper()
	repoDir := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	runGit("init")
	runGit("-c", "user.name=OCR Test", "-c", "user.email=ocr@example.com", "commit", "--allow-empty", "-m", "initial")
	runGit("branch", "-M", "main")
	runGit("branch", "master")
	runGit("remote", "add", "origin", "https://github.com/acme/widget.git")
	runGit("update-ref", "refs/remotes/origin/master", "HEAD")
	return repoDir, runGit("rev-parse", "HEAD")
}

func writeGitHubJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode GitHub response: %v", err)
	}
}

func newGitHubPostingTestConfiguration(t *testing.T, server *httptest.Server) github.Configuration {
	t.Helper()
	configuration, err := github.NewTestConfiguration("github.com", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewTestConfiguration: %v", err)
	}
	return configuration
}

func TestPostCommentsToGitHubUsesVerifiedRightSideFileLines(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	requests := make(chan postedGitHubReview, 1)
	summaries := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			if got := r.URL.Query().Get("base"); got != "master" {
				t.Errorf("base query = %q, want master", got)
			}
			if got := r.URL.Query().Get("head"); got != "" {
				t.Errorf("head query = %q, want empty because discovery matches the reviewed SHA", got)
			}
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"filename": "range.go", "patch": "@@ -40,3 +40,3 @@\n line40\n line41\n line42"},
				map[string]any{"filename": "end.go", "patch": "@@ -48,3 +48,3 @@\n line48\n line49\n line50"},
				map[string]any{"filename": "start.go", "patch": "@@ -60 +60 @@\n line60"},
			})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			var payload postedGitHubReview
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode review request: %v", err)
			}
			requests <- payload
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
		case "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode summary request: %v", err)
			}
			summaries <- payload.Body
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 2, "html_url": "https://github.example/summary/2"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	comments := []model.LlmComment{
		{Path: "range.go", Content: "range finding", StartLine: 40, EndLine: 42, Side: model.CommentSideRight},
		{Path: "end.go", Content: "end-only finding", EndLine: 50, Side: model.CommentSideRight},
		{Path: "start.go", Content: "start-only finding", StartLine: 60, Side: model.CommentSideRight},
		{Path: "outside.go", Content: "outside finding", StartLine: 200, EndLine: 200, Side: model.CommentSideRight},
		{Path: "start.go", Content: "legacy finding with unknown side", StartLine: 60, EndLine: 60},
	}
	if err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		comments,
		"test-token",
		configuration,
	); err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}

	payload := <-requests
	if payload.CommitID != headSHA {
		t.Errorf("commit_id = %q, want %s", payload.CommitID, headSHA)
	}
	if len(payload.Comments) != 3 {
		t.Fatalf("posted %d comments, want 3 verified inline comments", len(payload.Comments))
	}
	summaryBody := <-summaries
	if !strings.Contains(summaryBody, "outside finding") {
		t.Errorf("summary batch omitted unverified finding: %q", summaryBody)
	}
	if !strings.Contains(summaryBody, "legacy finding with unknown side") {
		t.Errorf("summary batch omitted unknown-side finding: %q", summaryBody)
	}

	if got := payload.Comments[0]; got.Line != 42 || got.Side != "RIGHT" || got.StartLine == nil || *got.StartLine != 40 || got.StartSide != "RIGHT" {
		t.Errorf("range comment location = %+v, want start_line 40 through line 42 on RIGHT", got)
	}
	for i, wantLine := range []int{50, 60} {
		got := payload.Comments[i+1]
		if got.Line != wantLine || got.Side != "RIGHT" || got.StartLine != nil || got.StartSide != "" {
			t.Errorf("single-line comment %d location = %+v, want line %d on RIGHT", i, got, wantLine)
		}
	}
	for i, got := range payload.Comments {
		if got.Position != nil {
			t.Errorf("comment %d sent deprecated diff position %d", i, *got.Position)
		}
	}
}

func TestPostCommentsToGitHubDoesNotInferPRNumberFromBranch(t *testing.T) {
	repoDir, _ := newGitHubPostingTestRepo(t)
	cmd := exec.Command("git", "-C", repoDir, "branch", "123-feature")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create numbered branch: %v\n%s", err, output)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/repos/acme/widget/pulls" {
			writeGitHubJSON(t, w, http.StatusOK, []any{})
			return
		}
		t.Errorf("unexpected request after no matching PR: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "origin/master", to: "123-feature"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "no open PR") {
		t.Fatalf("error = %v, want no matching open PR", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("made %d API requests, want discovery only", got)
	}
}

func TestNormalizeGitHubBranchPreservesLocalBranchSlashes(t *testing.T) {
	remoteNames := []string{"origin", "upstream"}
	if got := normalizeGitHubBranch("refs/heads/origin/main", remoteNames); got != "origin/main" {
		t.Fatalf("local branch normalized to %q, want origin/main", got)
	}
	if got := normalizeGitHubBranch("refs/remotes/origin/main", remoteNames); got != "main" {
		t.Fatalf("remote-tracking branch normalized to %q, want main", got)
	}
}

func TestFormatCommentBodyUsesPlainCodeWhenSuggestionContainsFence(t *testing.T) {
	code := "value := `literal`\n```go\nfmt.Println(value)\n```"
	body := formatCommentBody(model.LlmComment{Content: "Keep the complete example.", SuggestionCode: code})
	if strings.Contains(body, "```suggestion") {
		t.Fatalf("unsafe suggestion fence emitted: %q", body)
	}
	for _, line := range strings.Split(code, "\n") {
		if !strings.Contains(body, "    "+line) {
			t.Fatalf("plain code omitted line %q: %q", line, body)
		}
	}
}

func TestFormatCommentBodyFormatsSafeSuggestionAndBadge(t *testing.T) {
	body := formatCommentBody(model.LlmComment{
		Content:        "Use the guarded assignment.",
		SuggestionCode: "value := guarded()",
		Category:       "security",
		Severity:       "high",
	})
	for _, want := range []string{
		"**[security · high]**",
		"Use the guarded assignment.",
		"```suggestion\nvalue := guarded()\n```",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("formatted body omitted %q: %q", want, body)
		}
	}

	withTrailingNewline := formatCommentBody(model.LlmComment{SuggestionCode: "value := guarded()\n"})
	if strings.Contains(withTrailingNewline, "\n\n```") {
		t.Fatalf("safe suggestion gained an extra blank line: %q", withTrailingNewline)
	}
}

func TestBuildSummaryOverviewIncludesSortedSeverityBreakdown(t *testing.T) {
	body := buildSummaryOverview(
		[]model.LlmComment{{Severity: "medium"}, {Severity: "high"}, {Severity: "high"}},
		2,
		1,
	)
	if !strings.Contains(body, "- Inline findings: 2") || !strings.Contains(body, "- Summary-only findings: 1") {
		t.Fatalf("summary counts missing: %q", body)
	}
	if strings.Index(body, "**high**: 2") > strings.Index(body, "**medium**: 1") {
		t.Fatalf("severity breakdown is not sorted: %q", body)
	}
}

func TestPostCommentsToGitHubUsesValidatedProductionConfiguration(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files", "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			writes.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/summary/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	runGit := exec.Command("git", "-C", repoDir, "remote", "set-url", "origin", server.URL+"/acme/widget.git")
	if output, err := runGit.CombinedOutput(); err != nil {
		t.Fatalf("set TLS GitHub remote: %v\n%s", err, output)
	}
	t.Setenv("GITHUB_SERVER_URL", server.URL)
	t.Setenv("GITHUB_API_URL", server.URL)
	previousTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	if err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "summary finding", Side: model.CommentSideLeft}},
		"test-token",
	); err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("production configuration writes = %d, want 1", got)
	}
}

func TestGitHubReviewRunIDIsStableForRetry(t *testing.T) {
	target := githubPostingTarget{
		githubRepository: githubRepository{repository: github.Repository{Owner: "acme", Name: "widget"}},
		prNumber:         14,
	}
	comments := []model.LlmComment{{Path: "main.go", Content: "finding", Side: model.CommentSideRight}}
	first := newGitHubReviewRunID(target, "main", "abc123", comments)
	second := newGitHubReviewRunID(target, "main", "abc123", comments)
	if first != second {
		t.Fatalf("retry marker changed from %q to %q", first, second)
	}
	if changed := newGitHubReviewRunID(target, "main", "abc123", []model.LlmComment{{Path: "main.go", Content: "different"}}); changed == first {
		t.Fatal("different review content reused the same run marker")
	}
}

func TestPostCommentsToGitHubRejectsChangedPRHead(t *testing.T) {
	repoDir, reviewedSHA := newGitHubPostingTestRepo(t)
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": reviewedSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": "pushed-after-review"}, "base": map[string]any{"ref": "master"}})
		default:
			writes.Add(1)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "changed from reviewed commit") {
		t.Fatalf("error = %v, want stale PR head rejection", err)
	}
	if got := writes.Load(); got != 0 {
		t.Errorf("performed %d write requests after stale-head detection", got)
	}
}

func TestPostCommentsToGitHubRejectsChangedBaseOrClosedPR(t *testing.T) {
	for _, tc := range []struct {
		name      string
		base      string
		state     string
		wantError string
	}{
		{name: "base changed", base: "release", state: "open", wantError: "base changed"},
		{name: "pull request closed", base: "master", state: "closed", wantError: "is closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoDir, reviewedSHA := newGitHubPostingTestRepo(t)
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/widget/pulls":
					writeGitHubJSON(t, w, http.StatusOK, []any{
						map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": reviewedSHA}, "base": map[string]any{"ref": "master"}},
					})
				case "/repos/acme/widget/pulls/14":
					writeGitHubJSON(t, w, http.StatusOK, map[string]any{
						"state": tc.state,
						"head":  map[string]any{"sha": reviewedSHA},
						"base":  map[string]any{"ref": tc.base},
					})
				case "/repos/acme/widget/pulls/14/files":
					writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
				case "/repos/acme/widget/pulls/14/reviews", "/repos/acme/widget/issues/14/comments":
					writes.Add(1)
					writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			configuration := newGitHubPostingTestConfiguration(t, server)

			err := postCommentsToGitHubWithConfiguration(
				context.Background(),
				repoDir,
				reviewOptions{from: "master", to: "main"},
				[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
				"test-token",
				configuration,
			)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			if got := writes.Load(); got != 0 {
				t.Fatalf("performed %d write(s) after target validation failed", got)
			}
		})
	}
}

func TestPostCommentsToGitHubRoutesOldSideResolutionToSummary(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	resolved := diff.ResolveLineNumbers([]model.LlmComment{{
		Path:         "file.go",
		Content:      "deleted-line finding",
		ExistingCode: "deleted",
	}}, []model.Diff{{
		OldPath: "file.go",
		NewPath: "file.go",
		Diff:    "@@ -1,3 +1,2 @@\n context\n-deleted\n context2",
	}})
	if len(resolved) != 1 || resolved[0].StartLine != 2 {
		t.Fatalf("old-side fixture did not resolve as expected: %+v", resolved)
	}

	var inlineWrites atomic.Int32
	var summaryWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"filename": "file.go", "patch": "@@ -1,3 +1,2 @@\n context\n-deleted\n context2"},
			})
		case "/repos/acme/widget/pulls/14/reviews":
			inlineWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
		case "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			summaryWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	if err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		resolved,
		"test-token",
		configuration,
	); err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}
	if got := inlineWrites.Load(); got != 0 {
		t.Errorf("posted %d old-side findings inline", got)
	}
	if got := summaryWrites.Load(); got != 1 {
		t.Errorf("posted %d summaries, want 1", got)
	}
}

func TestPostCommentsToGitHubStopsBeforeSummaryWhenHeadChanges(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var headReads atomic.Int32
	var summaryWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			sha := headSHA
			if headReads.Add(1) >= 3 {
				sha = "new-head-before-summary"
			}
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": sha}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			summaryWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "old-side finding", StartLine: 1, EndLine: 1, Side: model.CommentSideLeft}},
		"test-token",
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "changed from reviewed commit") {
		t.Fatalf("error = %v, want changed-head rejection", err)
	}
	if got := summaryWrites.Load(); got != 0 {
		t.Errorf("summary writes = %d, want none after the head changed", got)
	}
}

func TestPostCommentsToGitHubFindsForkPRThroughUpstreamRemote(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("remote", "set-url", "origin", "https://github.com/acme/widget-fork.git")
	runGit("remote", "add", "upstream", "https://github.com/upstream/widget.git")
	runGit("update-ref", "refs/remotes/upstream/master", "HEAD")
	runGit("remote", "add", "gitlab", "https://gitlab.com/acme/not-github.git")

	var posted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget-fork/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{})
		case "/repos/upstream/widget/pulls":
			if got := r.URL.Query().Get("base"); got != "master" {
				t.Errorf("base query = %q, want master", got)
			}
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 9, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/upstream/widget/pulls/9":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/upstream/widget/pulls/9/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/upstream/widget/pulls/9/reviews":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			posted.Store(true)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "upstream/master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}
	if !posted.Load() {
		t.Fatal("review was not posted to the upstream PR")
	}
}

func TestPostCommentsToGitHubRejectsGitLabOnlyRemote(t *testing.T) {
	repoDir, _ := newGitHubPostingTestRepo(t)
	cmd := exec.Command("git", "-C", repoDir, "remote", "set-url", "origin", "https://gitlab.com/acme/widget.git")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("replace origin URL: %v\n%s", err, output)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "no parseable GitHub repository") {
		t.Fatalf("error = %v, want non-GitHub remote rejection", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("made %d GitHub API requests for a GitLab remote", got)
	}
}

func TestPostCommentsToGitHubHonorsCanceledContext(t *testing.T) {
	repoDir, _ := newGitHubPostingTestRepo(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := postCommentsToGitHubWithConfiguration(
		ctx,
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("made %d HTTP requests after cancellation", got)
	}
}

func TestPostCommentsToGitHubBatchesInlineFindings(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var mu sync.Mutex
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			var payload postedGitHubReview
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode review request: %v", err)
			}
			mu.Lock()
			batchSizes = append(batchSizes, len(payload.Comments))
			id := len(batchSizes)
			mu.Unlock()
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": id, "html_url": "https://github.example/review"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("finding %d", i), StartLine: 1, EndLine: 1, Side: model.CommentSideRight}
	}
	if err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		comments,
		"test-token",
		configuration,
	); err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(batchSizes) != "[50 1]" {
		t.Errorf("review batch sizes = %v, want [50 1]", batchSizes)
	}
}

func TestPostCommentsToGitHubStopsWhenHeadChangesBetweenBatches(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var headReads atomic.Int32
	var reviewWrites atomic.Int32
	var summaryWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			sha := headSHA
			if headReads.Add(1) >= 4 {
				sha = "new-head-after-first-batch"
			}
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": sha}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			reviewWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review"})
		case "/repos/acme/widget/issues/14/comments":
			summaryWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("finding %d", i), StartLine: 1, EndLine: 1, Side: model.CommentSideRight}
	}
	err := postCommentsToGitHubWithConfiguration(context.Background(), repoDir, reviewOptions{from: "master", to: "main"}, comments, "test-token", configuration)
	if err == nil || !strings.Contains(err.Error(), "changed from reviewed commit") {
		t.Fatalf("error = %v, want changed-head rejection", err)
	}
	if got := reviewWrites.Load(); got != 1 {
		t.Errorf("review writes = %d, want only the first batch", got)
	}
	if got := summaryWrites.Load(); got != 0 {
		t.Errorf("summary writes = %d, want none after the head changed", got)
	}
}

func TestPostCommentsToGitHubFallbackIncludesOnlyUnpostedBatches(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var reviewWrites atomic.Int32
	var fallbackBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			if reviewWrites.Add(1) == 1 {
				writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
				return
			}
			writeGitHubJSON(t, w, http.StatusUnprocessableEntity, map[string]any{"message": "Line could not be resolved"})
		case "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodGet {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode fallback summary: %v", err)
			}
			fallbackBody = payload.Body
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("finding %d", i), StartLine: 1, EndLine: 1, Side: model.CommentSideRight}
	}
	if err := postCommentsToGitHubWithConfiguration(context.Background(), repoDir, reviewOptions{from: "master", to: "main"}, comments, "test-token", configuration); err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}
	if !strings.Contains(fallbackBody, "finding 50") {
		t.Errorf("fallback omitted unposted second-batch finding: %q", fallbackBody)
	}
	if strings.Contains(fallbackBody, "finding 0") {
		t.Errorf("fallback duplicated an already-posted first-batch finding: %q", fallbackBody)
	}
}

func TestPostCommentsToGitHubReconcilesAmbiguousBatchFailure(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var attemptedReviewBody string
	var summaryWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodPost {
				var payload postedGitHubReview
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode attempted review: %v", err)
				}
				attemptedReviewBody = payload.Body
				writeGitHubJSON(t, w, http.StatusInternalServerError, map[string]any{"message": "response lost after write"})
				return
			}
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"body": attemptedReviewBody, "commit_id": headSHA}})
		case "/repos/acme/widget/issues/14/comments":
			summaryWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}
	if summaryWrites.Load() != 0 {
		t.Fatal("ambiguous batch that was found on GitHub was duplicated into the fallback summary")
	}
}

func TestPostCommentsToGitHubRecognizesPreviouslyAcceptedRetryBatch(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	comments := []model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}}
	target := githubPostingTarget{
		githubRepository: githubRepository{repository: github.Repository{Owner: "acme", Name: "widget"}},
		prNumber:         14,
	}
	runID := newGitHubReviewRunID(target, "master", headSHA, comments)
	marker := fmt.Sprintf("<!-- ocr-review-%s-review-0 -->", runID)
	var reviewWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodPost {
				reviewWrites.Add(1)
				writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 2, "html_url": "https://github.example/review/2"})
				return
			}
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"id": 1, "html_url": "https://github.example/review/1", "body": marker, "commit_id": headSHA},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	if err := postCommentsToGitHubWithConfiguration(
		context.Background(), repoDir, reviewOptions{from: "master", to: "main"}, comments, "test-token", configuration,
	); err != nil {
		t.Fatalf("postCommentsToGitHubWithConfiguration: %v", err)
	}
	if got := reviewWrites.Load(); got != 0 {
		t.Fatalf("retry posted %d duplicate review batch(es)", got)
	}
}

func TestPostCommentsToGitHubPollsForDelayedAmbiguousSummaryWrite(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var attemptedBody string
	var commentReads atomic.Int32
	var commentWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{})
		case "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodPost {
				commentWrites.Add(1)
				var payload struct {
					Body string `json:"body"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode summary request: %v", err)
				}
				attemptedBody = payload.Body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"html_url":`))
				return
			}
			if commentReads.Add(1) < 2 {
				writeGitHubJSON(t, w, http.StatusOK, []any{})
				return
			}
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"id": 8, "html_url": "https://github.example/comment/8", "body": attemptedBody},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "summary finding", StartLine: 1, EndLine: 1, Side: model.CommentSideLeft}},
		"test-token",
		configuration,
	)
	if err != nil {
		t.Fatalf("postCommentsToGitHubWithConfiguration: %v", err)
	}
	if got := commentWrites.Load(); got != 1 {
		t.Fatalf("summary writes = %d, want one ambiguous write", got)
	}
	if got := commentReads.Load(); got < 2 {
		t.Fatalf("summary reconciliation reads = %d, want bounded polling", got)
	}
	if !strings.Contains(attemptedBody, "<!-- ocr-review-") {
		t.Fatalf("summary body omitted reconciliation marker: %q", attemptedBody)
	}
}

func TestPostCommentsToGitHubDoesNotFallbackWhileReviewWriteIsUnknown(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	var fallbackWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodPost {
				writeGitHubJSON(t, w, http.StatusInternalServerError, map[string]string{"message": "unknown write outcome"})
				return
			}
			writeGitHubJSON(t, w, http.StatusOK, []any{})
		case "/repos/acme/widget/issues/14/comments":
			fallbackWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]string{"html_url": "https://github.example/fallback"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration := newGitHubPostingTestConfiguration(t, server)

	err := postCommentsToGitHubWithConfiguration(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "outcome remains unknown") {
		t.Fatalf("error = %v, want unresolved ambiguous-write error", err)
	}
	if got := fallbackWrites.Load(); got != 0 {
		t.Fatalf("fallback writes = %d while review outcome was unknown", got)
	}
}

func TestPostCommentsToGitHubBatchesSummaryPayloadsWithoutDroppingFindings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		comments []model.LlmComment
		verify   func(*testing.T, []string)
	}{
		{
			name: "finding count",
			comments: func() []model.LlmComment {
				comments := make([]model.LlmComment, 51)
				for i := range comments {
					comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("summary-finding-%02d", i), Side: model.CommentSideLeft}
				}
				return comments
			}(),
			verify: func(t *testing.T, bodies []string) {
				t.Helper()
				if len(bodies) < 2 {
					t.Fatalf("summary batches = %d, want at least two", len(bodies))
				}
				joined := strings.Join(bodies, "\n")
				for i := 0; i < 51; i++ {
					finding := fmt.Sprintf("summary-finding-%02d", i)
					if strings.Count(joined, finding) != 1 {
						t.Fatalf("%s was not preserved exactly once", finding)
					}
				}
			},
		},
		{
			name: "individually oversized finding",
			comments: []model.LlmComment{{
				Path:    "huge.go",
				Content: strings.Repeat("z", 100_000),
				Side:    model.CommentSideLeft,
			}},
			verify: func(t *testing.T, bodies []string) {
				t.Helper()
				if len(bodies) < 2 {
					t.Fatalf("oversized finding used %d batch(es), want multiple", len(bodies))
				}
				if got := strings.Count(strings.Join(bodies, ""), "z"); got != 100_000 {
					t.Fatalf("preserved %d oversized-finding bytes, want 100000", got)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoDir, headSHA := newGitHubPostingTestRepo(t)
			var bodies []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/acme/widget/pulls":
					writeGitHubJSON(t, w, http.StatusOK, []any{
						map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}},
					})
				case "/repos/acme/widget/pulls/14":
					writeGitHubJSON(t, w, http.StatusOK, map[string]any{"state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "master"}})
				case "/repos/acme/widget/pulls/14/files":
					writeGitHubJSON(t, w, http.StatusOK, []any{})
				case "/repos/acme/widget/issues/14/comments":
					if r.Method == http.MethodGet {
						writeGitHubJSON(t, w, http.StatusOK, []any{})
						return
					}
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read summary request: %v", err)
					}
					if len(raw) > 60*1024 {
						writeGitHubJSON(t, w, http.StatusRequestEntityTooLarge, map[string]string{"message": "payload too large"})
						return
					}
					var payload struct {
						Body string `json:"body"`
					}
					if err := json.Unmarshal(raw, &payload); err != nil {
						t.Errorf("decode summary request: %v", err)
					}
					if strings.Count(payload.Body, "#### `") > 50 {
						writeGitHubJSON(t, w, http.StatusUnprocessableEntity, map[string]string{"message": "too many findings"})
						return
					}
					bodies = append(bodies, payload.Body)
					writeGitHubJSON(t, w, http.StatusCreated, map[string]string{"html_url": fmt.Sprintf("https://github.example/summary/%d", len(bodies))})
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			configuration := newGitHubPostingTestConfiguration(t, server)

			if err := postCommentsToGitHubWithConfiguration(
				context.Background(), repoDir, reviewOptions{from: "master", to: "main"}, tc.comments, "test-token", configuration,
			); err != nil {
				t.Fatalf("postCommentsToGitHubWithConfiguration: %v", err)
			}
			tc.verify(t, bodies)
		})
	}
}
