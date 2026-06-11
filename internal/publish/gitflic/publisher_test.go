package gitflic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/model"
)

type recordedRequest struct {
	path string
	auth string
	body Discussion
}

func newRecordingServer(t *testing.T, requests *[]recordedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d Discussion
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		*requests = append(*requests, recordedRequest{
			path: r.URL.Path,
			auth: r.Header.Get("Authorization"),
			body: d,
		})
		w.WriteHeader(http.StatusOK)
	}))
}

func silentPublisher(client *Client) *Publisher {
	p := NewPublisher(client)
	p.logf = func(string, ...any) {}
	return p
}

func TestPublishInlineAndSummary(t *testing.T) {
	var requests []recordedRequest
	srv := newRecordingServer(t, &requests)
	defer srv.Close()

	result := &ReviewResult{
		Comments: []model.LlmComment{
			{Path: "main.go", Content: "possible nil dereference", StartLine: 6, EndLine: 6,
				ExistingCode: "x := y.Field", SuggestionCode: "if y != nil { x = y.Field }"},
		},
	}
	diffs := []model.Diff{
		{OldPath: "main.go", NewPath: "main.go", Diff: sampleDiff},
	}

	publisher := silentPublisher(NewClient(srv.URL, "secret-token"))
	stats, err := publisher.Publish(context.Background(), Options{Owner: "fedor", Project: "demo", MRID: "7"}, result, diffs)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stats.Inline != 1 || stats.Fallback != 0 {
		t.Fatalf("stats = %+v, want 1 inline / 0 fallback", stats)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (inline + summary), got %d", len(requests))
	}

	inline := requests[0]
	wantPath := "/project/fedor/demo/merge-request/7/discussions/create"
	if inline.path != wantPath {
		t.Errorf("path = %q, want %q", inline.path, wantPath)
	}
	if inline.auth != "token secret-token" {
		t.Errorf("auth header = %q, want %q", inline.auth, "token secret-token")
	}
	if inline.body.NewLine != 6 || inline.body.OldLine != 5 {
		t.Errorf("position = new %d / old %d, want new 6 / old 5", inline.body.NewLine, inline.body.OldLine)
	}
	if inline.body.NewPath != "main.go" || inline.body.OldPath != "main.go" {
		t.Errorf("paths = %q / %q, want main.go / main.go", inline.body.NewPath, inline.body.OldPath)
	}
	if !strings.Contains(inline.body.Message, "possible nil dereference") ||
		!strings.Contains(inline.body.Message, "**Suggestion:**") {
		t.Errorf("inline message missing content or suggestion: %q", inline.body.Message)
	}

	summary := requests[1]
	if summary.body.NewPath != "" {
		t.Errorf("summary should be a general comment, got path %q", summary.body.NewPath)
	}
	if !strings.Contains(summary.body.Message, "**1** issue(s)") {
		t.Errorf("summary message = %q", summary.body.Message)
	}
}

func TestPublishFallbackForUnmappedComment(t *testing.T) {
	var requests []recordedRequest
	srv := newRecordingServer(t, &requests)
	defer srv.Close()

	result := &ReviewResult{
		Comments: []model.LlmComment{
			{Path: "missing.go", Content: "issue in file absent from diff", StartLine: 1, EndLine: 1},
		},
		Warnings: []ReviewWarning{{File: "a.go", Message: "skipped", Type: "subtask_error"}},
	}

	publisher := silentPublisher(NewClient(srv.URL, "tok"))
	stats, err := publisher.Publish(context.Background(), Options{Owner: "o", Project: "p", MRID: "1"}, result, nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stats.Inline != 0 || stats.Fallback != 1 {
		t.Fatalf("stats = %+v, want 0 inline / 1 fallback", stats)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (fallback + summary), got %d", len(requests))
	}
	if !strings.Contains(requests[0].body.Message, "could not be posted inline") ||
		!strings.Contains(requests[0].body.Message, "`missing.go`") {
		t.Errorf("fallback message = %q", requests[0].body.Message)
	}
	if !strings.Contains(requests[1].body.Message, "1 warning(s)") {
		t.Errorf("summary message = %q", requests[1].body.Message)
	}
}

func TestPublishInlineErrorFallsBack(t *testing.T) {
	var requests []recordedRequest
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var d Discussion
		_ = json.NewDecoder(r.Body).Decode(&d)
		// Reject the inline attempt (first call), accept the rest.
		if calls == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		requests = append(requests, recordedRequest{path: r.URL.Path, body: d})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := &ReviewResult{
		Comments: []model.LlmComment{
			{Path: "main.go", Content: "finding", StartLine: 1, EndLine: 1},
		},
	}
	diffs := []model.Diff{{OldPath: "main.go", NewPath: "main.go", Diff: sampleDiff}}

	publisher := silentPublisher(NewClient(srv.URL, "tok"))
	stats, err := publisher.Publish(context.Background(), Options{Owner: "o", Project: "p", MRID: "1"}, result, diffs)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stats.Inline != 0 || stats.Fallback != 1 {
		t.Fatalf("stats = %+v, want 0 inline / 1 fallback", stats)
	}
	if len(requests) != 2 {
		t.Fatalf("expected fallback + summary after inline failure, got %d", len(requests))
	}
}

func TestPublishNoComments(t *testing.T) {
	var requests []recordedRequest
	srv := newRecordingServer(t, &requests)
	defer srv.Close()

	result := &ReviewResult{Message: "No comments generated. Looks good to me."}

	publisher := silentPublisher(NewClient(srv.URL, "tok"))
	stats, err := publisher.Publish(context.Background(), Options{Owner: "o", Project: "p", MRID: "1"}, result, nil)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stats.Inline != 0 || stats.Fallback != 0 {
		t.Fatalf("stats = %+v, want zero", stats)
	}
	if len(requests) != 1 || !strings.Contains(requests[0].body.Message, "Looks good to me") {
		t.Fatalf("expected single LGTM note, got %+v", requests)
	}
}

func TestPublishNewFileAnchorsToNewPath(t *testing.T) {
	var requests []recordedRequest
	srv := newRecordingServer(t, &requests)
	defer srv.Close()

	const newFileDiff = `diff --git a/added.go b/added.go
--- /dev/null
+++ b/added.go
@@ -0,0 +1,3 @@
+package main
+
+func main() {}
`
	result := &ReviewResult{
		Comments: []model.LlmComment{
			{Path: "added.go", Content: "empty main", StartLine: 3, EndLine: 3},
		},
	}
	diffs := []model.Diff{{OldPath: "/dev/null", NewPath: "added.go", IsNew: true, Diff: newFileDiff}}

	publisher := silentPublisher(NewClient(srv.URL, "tok"))
	stats, err := publisher.Publish(context.Background(), Options{Owner: "o", Project: "p", MRID: "1"}, result, diffs)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stats.Inline != 1 {
		t.Fatalf("stats = %+v, want 1 inline", stats)
	}
	inline := requests[0].body
	if inline.OldPath != "added.go" || inline.OldLine != 1 || inline.NewLine != 3 {
		t.Errorf("new-file position = old %s:%d new line %d, want added.go:1 / 3",
			inline.OldPath, inline.OldLine, inline.NewLine)
	}
}
