// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package ghpost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

type postedReview struct {
	CommitID string `json:"commit_id"`
	Body     string `json:"body"`
	Comments []struct {
		Path      string `json:"path"`
		Body      string `json:"body"`
		Line      int    `json:"line"`
		Side      string `json:"side"`
		StartLine int    `json:"start_line"`
		StartSide string `json:"start_side"`
	} `json:"comments"`
}

type postServerState struct {
	mu               sync.Mutex
	headSHA          string
	baseRef          string
	baseSHAForCall   func(int) string
	mergeBaseForCall func(int) string
	patch            string
	compareCalls     int
	prCalls          int
	reviewPosts      []postedReview
	issueBodies      []string
	issuePosts       []string
	failReviewPost   int
	reviewPostStatus int
	acceptFailedPost bool
}

func newPostServer(t *testing.T, state *postServerState) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		write := func(status int, value any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if err := json.NewEncoder(w).Encode(value); err != nil {
				t.Errorf("encode response: %v", err)
			}
		}
		switch {
		case r.URL.Path == "/repos/acme/widget/pulls":
			write(http.StatusOK, []any{map[string]any{
				"number": 14,
				"state":  "open",
				"head":   map[string]any{"sha": state.headSHA},
				"base":   map[string]any{"ref": state.baseRef, "sha": "base-tip"},
			}})
		case r.URL.Path == "/repos/acme/widget/pulls/14":
			state.prCalls++
			baseSHA := "base-tip"
			if state.baseSHAForCall != nil {
				baseSHA = state.baseSHAForCall(state.prCalls)
			}
			write(http.StatusOK, map[string]any{
				"number": 14,
				"state":  "open",
				"head":   map[string]any{"sha": state.headSHA},
				"base":   map[string]any{"ref": state.baseRef, "sha": baseSHA},
			})
		case strings.HasPrefix(r.URL.Path, "/repos/acme/widget/compare/"):
			state.compareCalls++
			mergeBase := "reviewed-base"
			if state.mergeBaseForCall != nil {
				mergeBase = state.mergeBaseForCall(state.compareCalls)
			}
			write(http.StatusOK, map[string]any{"merge_base_commit": map[string]any{"sha": mergeBase}})
		case r.URL.Path == "/repos/acme/widget/pulls/14/files":
			write(http.StatusOK, []any{map[string]any{"filename": "main.go", "patch": state.patch}})
		case r.URL.Path == "/repos/acme/widget/pulls/14/reviews" && r.Method == http.MethodGet:
			reviews := make([]map[string]any, len(state.reviewPosts))
			for i, review := range state.reviewPosts {
				reviews[i] = map[string]any{"id": i + 1, "body": review.Body, "commit_id": state.headSHA}
			}
			write(http.StatusOK, reviews)
		case r.URL.Path == "/repos/acme/widget/pulls/14/reviews" && r.Method == http.MethodPost:
			var payload postedReview
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode review: %v", err)
			}
			if state.failReviewPost > 0 && len(state.reviewPosts)+1 == state.failReviewPost {
				write(http.StatusUnprocessableEntity, map[string]string{"message": "inline rejected"})
				return
			}
			if state.reviewPostStatus != 0 {
				if state.acceptFailedPost {
					state.reviewPosts = append(state.reviewPosts, payload)
				}
				write(state.reviewPostStatus, map[string]string{"message": "ambiguous response"})
				return
			}
			state.reviewPosts = append(state.reviewPosts, payload)
			write(http.StatusCreated, map[string]any{"id": len(state.reviewPosts), "html_url": "https://example/review"})
		case r.URL.Path == "/repos/acme/widget/issues/14/comments" && r.Method == http.MethodGet:
			comments := make([]map[string]any, len(state.issueBodies))
			for i, body := range state.issueBodies {
				comments[i] = map[string]any{"id": i + 1, "body": body, "html_url": fmt.Sprintf("https://example/comment/%d", i+1)}
			}
			write(http.StatusOK, comments)
		case r.URL.Path == "/repos/acme/widget/issues/14/comments" && r.Method == http.MethodPost:
			var payload struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode issue comment: %v", err)
			}
			state.issuePosts = append(state.issuePosts, payload.Body)
			state.issueBodies = append(state.issueBodies, payload.Body)
			write(http.StatusCreated, map[string]any{"id": len(state.issueBodies), "html_url": "https://example/comment/new"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = rewriteTransport{target: target, base: server.Client().Transport}
	t.Cleanup(func() { http.DefaultTransport = previous })
	t.Setenv("GITHUB_SERVER_URL", "")
	t.Setenv("GITHUB_API_URL", "")
	return server
}

func newPostRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dir}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("init")
	run("-c", "user.name=OCR Test", "-c", "user.email=ocr@example.com", "commit", "--allow-empty", "-m", "initial")
	run("branch", "-M", "main")
	run("branch", "master")
	run("remote", "add", "origin", "https://github.com/acme/widget.git")
	run("update-ref", "refs/remotes/origin/master", "HEAD")
	return dir, run("rev-parse", "HEAD")
}

func defaultPostState(headSHA string) *postServerState {
	return &postServerState{
		headSHA: headSHA,
		baseRef: "master",
		patch:   "@@ -1,1 +1,2 @@\n context\n+added",
	}
}

func postTarget(repoDir, headSHA string) Target {
	return Target{RepoDir: repoDir, BaseRef: "master", ResolvedBase: "reviewed-base", ResolvedHead: headSHA}
}

func TestCanonicalCommentsStabilizeMarkersAndBatches(t *testing.T) {
	comments := []model.LlmComment{
		{Path: "z.go", StartLine: 9, EndLine: 9, Content: "last", Category: "bug"},
		{Path: "a.go", StartLine: 2, EndLine: 2, Content: "second", Severity: "high"},
		{Path: "a.go", StartLine: 1, EndLine: 1, Content: "first", SuggestionCode: "fixed"},
	}
	original := slices.Clone(comments)
	reversed := slices.Clone(comments)
	slices.Reverse(reversed)
	first := canonicalComments(comments)
	second := canonicalComments(reversed)
	if !slices.Equal(first, second) {
		t.Fatalf("canonical order differs: %#v != %#v", first, second)
	}
	if !slices.Equal(comments, original) {
		t.Fatalf("canonicalComments mutated input: %#v", comments)
	}
	target := Target{ResolvedBase: "base-a", ResolvedHead: "head"}
	id1 := reviewRunID("acme", "widget", 14, "master", target, first)
	id2 := reviewRunID("acme", "widget", 14, "master", target, second)
	if id1 != id2 {
		t.Fatalf("stable IDs differ: %q != %q", id1, id2)
	}
	target.ResolvedBase = "base-b"
	if changed := reviewRunID("acme", "widget", 14, "master", target, first); changed == id1 {
		t.Fatal("resolved base did not affect marker identity")
	}
}

func TestPostRestoredFindingUsesRightOnlyInventory(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	newPostServer(t, state)
	raw, err := json.Marshal(model.LlmComment{Path: "main.go", StartLine: 2, EndLine: 2, Content: "restored", Category: "bug", Severity: "high"})
	if err != nil {
		t.Fatalf("marshal comment: %v", err)
	}
	var restored model.LlmComment
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal comment: %v", err)
	}
	result, err := Post(context.Background(), postTarget(repoDir, headSHA), []model.LlmComment{restored}, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if result.InlineComments != 1 || result.SummaryComments != 0 || len(state.reviewPosts) != 1 {
		t.Fatalf("result = %+v, reviews = %d", result, len(state.reviewPosts))
	}
	comment := state.reviewPosts[0].Comments[0]
	if comment.Line != 2 || comment.Side != "RIGHT" || !strings.HasPrefix(comment.Body, "![bug · high]") {
		t.Fatalf("posted comment = %+v", comment)
	}
}

func TestPostRoutesBothSideLineToSummary(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	newPostServer(t, state)
	result, err := Post(context.Background(), postTarget(repoDir, headSHA), []model.LlmComment{{Path: "main.go", StartLine: 1, EndLine: 1, Content: "ambiguous"}}, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if result.InlineComments != 0 || result.SummaryComments != 1 || len(state.reviewPosts) != 0 || len(state.issuePosts) != 1 {
		t.Fatalf("result = %+v, reviews = %d, summaries = %d", result, len(state.reviewPosts), len(state.issuePosts))
	}
	if !strings.Contains(state.issuePosts[0], "line range is ambiguous across both sides of the PR diff") {
		t.Fatalf("summary = %q", state.issuePosts[0])
	}
}

func TestPostIncludesMixedSummaryFindingInReviewBody(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	newPostServer(t, state)
	comments := []model.LlmComment{
		{Path: "main.go", StartLine: 1, EndLine: 1, Content: "ambiguous"},
		{Path: "main.go", StartLine: 2, EndLine: 2, Content: "inline"},
	}
	result, err := Post(context.Background(), postTarget(repoDir, headSHA), comments, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if result.InlineComments != 1 || result.SummaryComments != 1 || len(state.reviewPosts) != 1 || len(state.issuePosts) != 0 {
		t.Fatalf("result = %+v, reviews = %d, issue comments = %d", result, len(state.reviewPosts), len(state.issuePosts))
	}
	body := state.reviewPosts[0].Body
	for _, want := range []string{
		"2 comment(s) generated.",
		"Inline findings: 1",
		"Summary-only findings: 1",
		"line range is ambiguous across both sides of the PR diff",
		"ambiguous",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review summary missing %q: %q", want, body)
		}
	}
}

func TestPostVerifiesMergeBaseBeforeEveryReviewWrite(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.mergeBaseForCall = func(call int) string {
		if call >= 4 {
			return "changed-base"
		}
		return "reviewed-base"
	}
	newPostServer(t, state)
	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "main.go", StartLine: 2, EndLine: 2, Content: fmt.Sprintf("finding-%02d", i)}
	}
	_, err := Post(context.Background(), postTarget(repoDir, headSHA), comments, Options{Token: "token"})
	if err == nil || !strings.Contains(err.Error(), "PR merge-base changed") {
		t.Fatalf("Post error = %v", err)
	}
	if len(state.reviewPosts) != 1 {
		t.Fatalf("review posts = %d, want first batch only", len(state.reviewPosts))
	}
}

func TestPostAllowsBaseTipChangeWhenMergeBaseIsStable(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.baseSHAForCall = func(call int) string { return fmt.Sprintf("base-tip-%d", call) }
	newPostServer(t, state)
	if _, err := Post(context.Background(), postTarget(repoDir, headSHA), []model.LlmComment{{Path: "main.go", StartLine: 2, EndLine: 2, Content: "finding"}}, Options{Token: "token"}); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(state.reviewPosts) != 1 {
		t.Fatalf("review posts = %d", len(state.reviewPosts))
	}
}

func TestPostVerifiesMergeBaseBeforeSummaryWrite(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.mergeBaseForCall = func(call int) string {
		if call >= 3 {
			return "changed-base"
		}
		return "reviewed-base"
	}
	newPostServer(t, state)
	_, err := Post(context.Background(), postTarget(repoDir, headSHA), []model.LlmComment{{Path: "main.go", StartLine: 1, EndLine: 1, Content: "summary"}}, Options{Token: "token"})
	if err == nil || !strings.Contains(err.Error(), "PR merge-base changed") {
		t.Fatalf("Post error = %v", err)
	}
	if len(state.issuePosts) != 0 {
		t.Fatalf("issue posts = %d", len(state.issuePosts))
	}
}

func TestPostVerifiesMergeBaseBeforeFallbackWrite(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.failReviewPost = 2
	state.mergeBaseForCall = func(call int) string {
		if call >= 5 {
			return "changed-base"
		}
		return "reviewed-base"
	}
	newPostServer(t, state)
	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "main.go", StartLine: 2, EndLine: 2, Content: fmt.Sprintf("finding-%02d", i)}
	}
	_, err := Post(context.Background(), postTarget(repoDir, headSHA), comments, Options{Token: "token"})
	if err == nil || !strings.Contains(err.Error(), "PR merge-base changed") {
		t.Fatalf("Post error = %v", err)
	}
	if len(state.reviewPosts) != 1 || len(state.issuePosts) != 0 {
		t.Fatalf("review posts = %d, issue posts = %d", len(state.reviewPosts), len(state.issuePosts))
	}
}

func TestPostResumesExistingFallbackWithoutRetryingInline(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	comments := []model.LlmComment{{Path: "main.go", StartLine: 2, EndLine: 2, Content: "finding"}}
	canonical := canonicalComments(comments)
	runID := reviewRunID("acme", "widget", 14, "master", postTarget(repoDir, headSHA), canonical)
	state.issueBodies = []string{postingMarker(runID, "fallback-0", 0) + "\nexisting"}
	newPostServer(t, state)
	result, err := Post(context.Background(), postTarget(repoDir, headSHA), comments, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(state.reviewPosts) != 0 || len(state.issuePosts) != 0 || result.SummaryComments != 1 {
		t.Fatalf("result = %+v, review posts = %d, issue posts = %d", result, len(state.reviewPosts), len(state.issuePosts))
	}
}

func TestPostRepairsPartialFallbackAndKeepsGlobalCounts(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	comments := make([]model.LlmComment, 51)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "main.go", StartLine: 2, EndLine: 2, Content: fmt.Sprintf("finding-%02d", i)}
	}
	canonical := canonicalComments(comments)
	runID := reviewRunID("acme", "widget", 14, "master", postTarget(repoDir, headSHA), canonical)
	state.issueBodies = []string{postingMarker(runID, "fallback-0", 1) + "\nexisting second batch"}
	newPostServer(t, state)
	result, err := Post(context.Background(), postTarget(repoDir, headSHA), comments, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(state.reviewPosts) != 0 || len(state.issuePosts) != 1 || result.SummaryComments != 51 {
		t.Fatalf("result = %+v, review posts = %d, issue posts = %d", result, len(state.reviewPosts), len(state.issuePosts))
	}
	if !strings.Contains(state.issuePosts[0], "51 comment(s) generated.") || !strings.Contains(state.issuePosts[0], "Summary-only findings: 51") {
		t.Fatalf("summary = %q", state.issuePosts[0])
	}
}

func TestPostFallbackHeaderUsesGlobalCounts(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.failReviewPost = 2
	newPostServer(t, state)
	comments := make([]model.LlmComment, 60)
	for i := range comments {
		comments[i] = model.LlmComment{Path: "main.go", StartLine: 2, EndLine: 2, Content: fmt.Sprintf("finding-%02d", i)}
	}
	_, err := Post(context.Background(), postTarget(repoDir, headSHA), comments, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(state.issuePosts) != 1 {
		t.Fatalf("issue posts = %d", len(state.issuePosts))
	}
	body := state.issuePosts[0]
	for _, want := range []string{"60 comment(s) generated.", "Inline findings: 50", "Summary-only findings: 10"} {
		if !strings.Contains(body, want) {
			t.Fatalf("summary missing %q: %q", want, body)
		}
	}
}

func TestPostReconcilesAcceptedAmbiguousReviewWrite(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.reviewPostStatus = http.StatusInternalServerError
	state.acceptFailedPost = true
	newPostServer(t, state)
	result, err := Post(context.Background(), postTarget(repoDir, headSHA), []model.LlmComment{{Path: "main.go", StartLine: 2, EndLine: 2, Content: "finding"}}, Options{Token: "token"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if result.InlineComments != 1 || len(state.reviewPosts) != 1 || len(state.issuePosts) != 0 {
		t.Fatalf("result = %+v, review posts = %d, issue posts = %d", result, len(state.reviewPosts), len(state.issuePosts))
	}
}

func TestPostRefusesFallbackWhileReviewWriteIsUnknown(t *testing.T) {
	repoDir, headSHA := newPostRepo(t)
	state := defaultPostState(headSHA)
	state.reviewPostStatus = http.StatusInternalServerError
	newPostServer(t, state)
	_, err := Post(context.Background(), postTarget(repoDir, headSHA), []model.LlmComment{{Path: "main.go", StartLine: 2, EndLine: 2, Content: "finding"}}, Options{Token: "token"})
	if err == nil || !strings.Contains(err.Error(), "outcome remains unknown") {
		t.Fatalf("Post error = %v", err)
	}
	if len(state.issuePosts) != 0 {
		t.Fatalf("issue posts = %d, want fail-closed without fallback", len(state.issuePosts))
	}
}
