// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestParseRepoInfoValidatesConfiguredGitHubHost(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "")
	for _, remote := range []string{
		"https://github.com/acme/widget.git",
		"git@github.com:acme/widget.git",
		"ssh://git@github.com/acme/widget.git",
	} {
		owner, repo, err := ParseRepoInfo(remote)
		if err != nil {
			t.Errorf("ParseRepoInfo(%q): %v", remote, err)
			continue
		}
		if owner != "acme" || repo != "widget" {
			t.Errorf("ParseRepoInfo(%q) = %s/%s, want acme/widget", remote, owner, repo)
		}
	}
	if _, _, err := ParseRepoInfo("https://gitlab.com/acme/widget.git"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("GitLab remote accepted as GitHub: %v", err)
	}

	t.Setenv("GITHUB_SERVER_URL", "https://github.enterprise.example")
	owner, repo, err := ParseRepoInfo("git@github.enterprise.example:platform/service.git")
	if err != nil || owner != "platform" || repo != "service" {
		t.Fatalf("enterprise remote = %s/%s, %v", owner, repo, err)
	}
	t.Setenv("GITHUB_API_URL", "")
	repository, err := ResolveRepository("git@github.enterprise.example:platform/service.git")
	if err != nil {
		t.Fatalf("ResolveRepository: %v", err)
	}
	if got := NewClient("token", repository, 3).baseURL; got != "https://github.enterprise.example/api/v3" {
		t.Fatalf("enterprise API URL = %q", got)
	}
}

func TestClientRejectsMismatchedAPIHostBeforeSendingToken(t *testing.T) {
	const token = "must-not-leave-github"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("hostile server received Authorization header %q", got)
		}
		writeJSON(t, w, http.StatusCreated, map[string]string{"html_url": "https://attacker.example/comment"})
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_API_URL", server.URL)

	repository, err := ResolveRepository("https://github.com/acme/widget.git")
	if err == nil {
		_, err = NewClient(token, repository, 14).CreateIssueComment(context.Background(), "finding")
	}
	if err == nil {
		t.Fatal("mismatched API host was accepted")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("hostile API received %d request(s), want none", got)
	}
}

func TestClientRejectsHTTPProductionEndpoints(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "http://github.enterprise.example")
	t.Setenv("GITHUB_API_URL", "http://github.enterprise.example/api/v3")

	if _, _, err := ParseRepoInfo("git@github.enterprise.example:acme/widget.git"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("ParseRepoInfo error = %v, want HTTPS requirement", err)
	}
}

func TestClientRejectsEnterpriseAPIHostMismatch(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://github.enterprise.example")
	t.Setenv("GITHUB_API_URL", "https://attacker.example/api/v3")

	if _, err := ResolveRepository("git@github.enterprise.example:acme/widget.git"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("ResolveRepository error = %v, want mismatched-host rejection", err)
	}
}

func TestConfigurationRejectsMalformedCredentialBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		serverURL string
		apiURL    string
		wantError string
	}{
		{name: "server path", serverURL: "https://github.enterprise.example/root", wantError: "must not include a path"},
		{name: "server credentials", serverURL: "https://user@github.enterprise.example", wantError: "only a scheme and host"},
		{name: "API query", serverURL: "https://github.com", apiURL: "https://api.github.com?route=other", wantError: "query"},
		{name: "invalid server", serverURL: "://missing-host", wantError: "invalid GITHUB_SERVER_URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_SERVER_URL", tc.serverURL)
			t.Setenv("GITHUB_API_URL", tc.apiURL)
			if _, err := NewConfigurationFromEnvironment(); err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("NewConfigurationFromEnvironment error = %v, want %q", err, tc.wantError)
			}
		})
	}

	if _, err := NewTestConfiguration("", "http://127.0.0.1", http.DefaultClient); err == nil || !strings.Contains(err.Error(), "host is empty") {
		t.Fatalf("empty test host error = %v", err)
	}
	if _, err := NewTestConfiguration("github.com", "not-a-url", http.DefaultClient); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid test API error = %v", err)
	}
	if _, err := NewTestConfiguration("github.com", "http://127.0.0.1", nil); err == nil || !strings.Contains(err.Error(), "client is nil") {
		t.Fatalf("nil test client error = %v", err)
	}
}

func TestClientPullRequestOperations(t *testing.T) {
	const headSHA = "abc123"
	var postedCommit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/repos/acme/widget/pulls":
			writeJSON(t, w, http.StatusOK, []any{
				map[string]any{"number": 14, "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "main"}},
			})
		case "/repos/acme/widget/pulls/14":
			writeJSON(t, w, http.StatusOK, map[string]any{"number": 14, "state": "open", "head": map[string]any{"sha": headSHA}, "base": map[string]any{"ref": "main"}})
		case "/repos/acme/widget/pulls/14/files":
			writeJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "main.go", "patch": "@@ -1 +1 @@\n line"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if r.Method == http.MethodPost {
				var payload struct {
					CommitID string `json:"commit_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode review: %v", err)
				}
				postedCommit = payload.CommitID
				writeJSON(t, w, http.StatusCreated, map[string]any{"id": 7, "html_url": "https://example/review/7"})
				return
			}
			writeJSON(t, w, http.StatusOK, []any{map[string]any{"id": 7, "body": "marker", "commit_id": headSHA}})
		case "/repos/acme/widget/issues/14/comments":
			if r.Method == http.MethodGet {
				writeJSON(t, w, http.StatusOK, []any{map[string]any{"id": 8, "html_url": "https://example/comment/8", "body": "fallback marker"}})
				return
			}
			writeJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://example/comment/8"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	configuration, err := NewTestConfiguration("github.com", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewTestConfiguration: %v", err)
	}
	repository, err := configuration.ResolveRepository("https://github.com/acme/widget.git")
	if err != nil {
		t.Fatalf("ResolveRepository: %v", err)
	}

	ctx := context.Background()
	prNumber, err := FindPRByHead(ctx, "test-token", repository, "main", headSHA)
	if err != nil || prNumber != 14 {
		t.Fatalf("FindPRByHead = %d, %v", prNumber, err)
	}
	client := NewClient("test-token", repository, prNumber)
	pull, err := client.GetPRInfo(ctx)
	if err != nil || pull.Head.SHA != headSHA {
		t.Fatalf("GetPRInfo = %+v, %v", pull, err)
	}
	files, err := client.ListChangedFiles(ctx)
	if err != nil || len(files) != 1 || files[0].Filename != "main.go" {
		t.Fatalf("ListChangedFiles = %+v, %v", files, err)
	}
	response, err := client.CreateReview(ctx, headSHA, ReviewRequest{
		Body:  "summary",
		Event: "COMMENT",
		Comments: []Comment{{
			Path: "main.go", Body: "finding", Line: 1, Side: "RIGHT",
		}},
	})
	if err != nil || response.ID != 7 || postedCommit != headSHA {
		t.Fatalf("CreateReview = %+v, posted commit %q, %v", response, postedCommit, err)
	}
	reviews, err := client.ListReviews(ctx)
	if err != nil || len(reviews) != 1 || reviews[0].CommitID != headSHA {
		t.Fatalf("ListReviews = %+v, %v", reviews, err)
	}
	commentURL, err := client.CreateIssueComment(ctx, "fallback")
	if err != nil || commentURL != "https://example/comment/8" {
		t.Fatalf("CreateIssueComment = %q, %v", commentURL, err)
	}
	issueComments, err := client.ListIssueComments(ctx)
	if err != nil || len(issueComments) != 1 || issueComments[0].Body != "fallback marker" {
		t.Fatalf("ListIssueComments = %+v, %v", issueComments, err)
	}
}

func TestClientReturnsTypedAPIError(t *testing.T) {
	const token = "sensitive-test-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnprocessableEntity, map[string]string{"message": "invalid line for " + token})
	}))
	t.Cleanup(server.Close)
	configuration, err := NewTestConfiguration("github.com", server.URL, server.Client())
	if err != nil {
		t.Fatalf("NewTestConfiguration: %v", err)
	}
	repository, err := configuration.ResolveRepository("https://github.com/acme/widget.git")
	if err != nil {
		t.Fatalf("ResolveRepository: %v", err)
	}

	_, err = NewClient(token, repository, 14).CreateReview(context.Background(), "head", ReviewRequest{})
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apiErr.Body, "invalid line") {
		t.Fatalf("error = %#v, want typed 422 APIError", err)
	}
	if strings.Contains(apiErr.Error(), token) || !strings.Contains(apiErr.Error(), "[REDACTED]") {
		t.Fatalf("error = %q, want token redacted", apiErr)
	}
}
