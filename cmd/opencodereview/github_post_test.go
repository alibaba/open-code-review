// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
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
	runGit("remote", "add", "origin", "https://github.com/acme/widget.git")
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

func TestPostCommentsToGitHubUsesVerifiedRightSideFileLines(t *testing.T) {
	repoDir, headSHA := newGitHubPostingTestRepo(t)
	requests := make(chan postedGitHubReview, 1)
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": headSHA}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"filename": "range.go", "patch": "@@ -40,3 +40,3 @@\n line40\n line41\n line42"},
				map[string]any{"filename": "end.go", "patch": "@@ -48,3 +48,3 @@\n line48\n line49\n line50"},
				map[string]any{"filename": "start.go", "patch": "@@ -60 +60 @@\n line60"},
			})
		case "/repos/acme/widget/pulls/14/reviews":
			var payload postedGitHubReview
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode review request: %v", err)
			}
			requests <- payload
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_API_URL", server.URL)

	comments := []model.LlmComment{
		{Path: "range.go", Content: "range finding", StartLine: 40, EndLine: 42, Side: model.CommentSideRight},
		{Path: "end.go", Content: "end-only finding", EndLine: 50, Side: model.CommentSideRight},
		{Path: "start.go", Content: "start-only finding", StartLine: 60, Side: model.CommentSideRight},
		{Path: "outside.go", Content: "outside finding", StartLine: 200, EndLine: 200, Side: model.CommentSideRight},
		{Path: "start.go", Content: "legacy finding with unknown side", StartLine: 60, EndLine: 60},
	}
	if err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		comments,
		"test-token",
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
	if !strings.Contains(payload.Body, "outside finding") {
		t.Errorf("review summary omitted unverified finding: %q", payload.Body)
	}
	if !strings.Contains(payload.Body, "legacy finding with unknown side") {
		t.Errorf("review summary omitted unknown-side finding: %q", payload.Body)
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
	t.Setenv("GITHUB_API_URL", server.URL)

	err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "origin/master", to: "123-feature"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
	)
	if err == nil || !strings.Contains(err.Error(), "no open PR") {
		t.Fatalf("error = %v, want no matching open PR", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("made %d API requests, want discovery only", got)
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": "pushed-after-review"}})
		default:
			writes.Add(1)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_API_URL", server.URL)

	err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
	)
	if err == nil || !strings.Contains(err.Error(), "changed from reviewed commit") {
		t.Fatalf("error = %v, want stale PR head rejection", err)
	}
	if got := writes.Load(); got != 0 {
		t.Errorf("performed %d write requests after stale-head detection", got)
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": headSHA}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{
				map[string]any{"filename": "file.go", "patch": "@@ -1,3 +1,2 @@\n context\n-deleted\n context2"},
			})
		case "/repos/acme/widget/pulls/14/reviews":
			inlineWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
		case "/repos/acme/widget/issues/14/comments":
			summaryWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_API_URL", server.URL)

	if err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		resolved,
		"test-token",
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": sha}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/issues/14/comments":
			summaryWrites.Add(1)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"html_url": "https://github.example/summary"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_API_URL", server.URL)

	err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "old-side finding", StartLine: 1, EndLine: 1, Side: model.CommentSideLeft}},
		"test-token",
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": headSHA}})
		case "/repos/upstream/widget/pulls/9/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/upstream/widget/pulls/9/reviews":
			posted.Store(true)
			writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_API_URL", server.URL)

	err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "upstream/master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
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
	t.Setenv("GITHUB_API_URL", server.URL)

	err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
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
	t.Setenv("GITHUB_API_URL", server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := postCommentsToGitHub(
		ctx,
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": headSHA}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
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
	t.Setenv("GITHUB_API_URL", server.URL)

	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("finding %d", i), StartLine: 1, EndLine: 1, Side: model.CommentSideRight}
	}
	if err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		comments,
		"test-token",
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": sha}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
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
	t.Setenv("GITHUB_API_URL", server.URL)

	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("finding %d", i), StartLine: 1, EndLine: 1, Side: model.CommentSideRight}
	}
	err := postCommentsToGitHub(context.Background(), repoDir, reviewOptions{from: "master", to: "main"}, comments, "test-token")
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": headSHA}})
		case "/repos/acme/widget/pulls/14/files":
			writeGitHubJSON(t, w, http.StatusOK, []any{map[string]any{"filename": "file.go", "patch": "@@ -1 +1 @@\n line1"}})
		case "/repos/acme/widget/pulls/14/reviews":
			if reviewWrites.Add(1) == 1 {
				writeGitHubJSON(t, w, http.StatusCreated, map[string]any{"id": 1, "html_url": "https://github.example/review/1"})
				return
			}
			writeGitHubJSON(t, w, http.StatusUnprocessableEntity, map[string]any{"message": "Line could not be resolved"})
		case "/repos/acme/widget/issues/14/comments":
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
	t.Setenv("GITHUB_API_URL", server.URL)

	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "file.go", Content: fmt.Sprintf("finding %d", i), StartLine: 1, EndLine: 1, Side: model.CommentSideRight}
	}
	if err := postCommentsToGitHub(context.Background(), repoDir, reviewOptions{from: "master", to: "main"}, comments, "test-token"); err != nil {
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
			writeGitHubJSON(t, w, http.StatusOK, map[string]any{"head": map[string]any{"sha": headSHA}})
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
	t.Setenv("GITHUB_API_URL", server.URL)

	err := postCommentsToGitHub(
		context.Background(),
		repoDir,
		reviewOptions{from: "master", to: "main"},
		[]model.LlmComment{{Path: "file.go", Content: "finding", StartLine: 1, EndLine: 1, Side: model.CommentSideRight}},
		"test-token",
	)
	if err != nil {
		t.Fatalf("postCommentsToGitHub: %v", err)
	}
	if summaryWrites.Load() != 0 {
		t.Fatal("ambiguous batch that was found on GitHub was duplicated into the fallback summary")
	}
}
