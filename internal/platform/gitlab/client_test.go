package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListChangedFiles_DiffsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/merge_requests/42/diffs") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]string{
			{"old_path": "a.go", "new_path": "a.go", "diff": "@@ -1 +1 @@\n-old\n+new"},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	files, err := client.ListChangedFiles(context.Background(), "proj", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].NewPath != "a.go" {
		t.Fatalf("expected NewPath a.go, got %s", files[0].NewPath)
	}
	if files[0].Diff == "" {
		t.Fatal("expected non-empty diff")
	}
}

func TestListChangedFiles_FallsBackToChangesOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"404 Not Found"}`))
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/changes") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("access_raw_diffs") != "true" {
			t.Fatalf("missing access_raw_diffs query param: %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"changes": []map[string]string{
				{"old_path": "b.go", "new_path": "b.go", "diff": "@@ -1 +1 @@\n-x\n+y"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	files, err := client.ListChangedFiles(context.Background(), "proj", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].NewPath != "b.go" {
		t.Fatalf("expected NewPath b.go, got %s", files[0].NewPath)
	}
}

func TestListChangedFiles_ProjectPathEscaping(t *testing.T) {
	var capturedRawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path decodes %2F; r.URL.RawPath preserves the wire encoding.
		capturedRawPath = r.URL.RawPath
		json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListChangedFiles(context.Background(), "group/sub/project", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRawPath == "" {
		t.Fatal("expected non-empty raw path (path escaping may not be active)")
	}
	if !strings.Contains(capturedRawPath, "group%2Fsub%2Fproject") {
		t.Fatalf("project path not escaped in raw path, got: %s", capturedRawPath)
	}
}

func TestListChangedFiles_SendsPrivateTokenHeader(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "my-secret-token")
	_, err := client.ListChangedFiles(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != "my-secret-token" {
		t.Fatalf("expected PRIVATE-TOKEN my-secret-token, got %s", gotToken)
	}
}

func TestListChangedFiles_Non2xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListChangedFiles(context.Background(), "proj", 1)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected error to contain status code 403, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("expected error to contain response body, got: %v", err)
	}
}

func TestNewClient_DefaultBaseURL(t *testing.T) {
	c := NewClient("", "tok")
	if c.baseURL != "https://gitlab.com" {
		t.Fatalf("expected default baseURL https://gitlab.com, got %s", c.baseURL)
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	c := NewClient("https://gitlab.example.com/", "tok")
	if strings.HasSuffix(c.baseURL, "/") {
		t.Fatalf("expected trailing slash trimmed, got %s", c.baseURL)
	}
}

func TestListDiscussions_FetchesTwoPages(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.String())
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Next-Page", "2")
			json.NewEncoder(w).Encode([]Discussion{
				{ID: "disc-1", Notes: []Note{{ID: 1, Body: "note-1"}}},
			})
		case "2":
			w.Header().Set("X-Next-Page", "")
			json.NewEncoder(w).Encode([]Discussion{
				{ID: "disc-2", Notes: []Note{{ID: 2, Body: "note-2"}}},
			})
		default:
			t.Fatalf("unexpected page: %s", r.URL.Query().Get("page"))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	discs, err := client.ListDiscussions(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(discs) != 2 {
		t.Fatalf("expected 2 discussions, got %d", len(discs))
	}
	if discs[0].ID != "disc-1" {
		t.Fatalf("expected first discussion disc-1, got %s", discs[0].ID)
	}
	if discs[1].ID != "disc-2" {
		t.Fatalf("expected second discussion disc-2, got %s", discs[1].ID)
	}
}

func TestListDiscussions_RequestsIncludePerPageAndPage(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		json.NewEncoder(w).Encode([]Discussion{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListDiscussions(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if !strings.Contains(requests[0], "per_page=100") {
		t.Fatalf("expected per_page=100 in query, got %s", requests[0])
	}
}

func TestListDiscussions_SecondPageRequestIncludesPageParam(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Next-Page", "2")
			json.NewEncoder(w).Encode([]Discussion{{ID: "d1"}})
		case "2":
			json.NewEncoder(w).Encode([]Discussion{{ID: "d2"}})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListDiscussions(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if !strings.Contains(requests[1], "page=2") {
		t.Fatalf("expected page=2 in second request, got %s", requests[1])
	}
	if !strings.Contains(requests[1], "per_page=100") {
		t.Fatalf("expected per_page=100 in second request, got %s", requests[1])
	}
}

func TestListDiscussions_FollowsNonSequentialNextPage(t *testing.T) {
	var requestedPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPages = append(requestedPages, r.URL.Query().Get("page"))
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Next-Page", "3")
			json.NewEncoder(w).Encode([]Discussion{{ID: "d1"}})
		case "3":
			w.Header().Set("X-Next-Page", "")
			json.NewEncoder(w).Encode([]Discussion{{ID: "d3"}})
		default:
			t.Fatalf("unexpected page: %s", r.URL.Query().Get("page"))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	discs, err := client.ListDiscussions(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(discs) != 2 {
		t.Fatalf("expected 2 discussions, got %d", len(discs))
	}
	if discs[0].ID != "d1" || discs[1].ID != "d3" {
		t.Fatalf("expected [d1, d3], got [%s, %s]", discs[0].ID, discs[1].ID)
	}
	// The second request must use page=3, not page=2.
	if len(requestedPages) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requestedPages))
	}
	if requestedPages[1] != "3" {
		t.Fatalf("expected second request page=3 (from X-Next-Page), got page=%s", requestedPages[1])
	}
}

func TestListDiscussions_SecondPageErrorReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("X-Next-Page", "2")
			json.NewEncoder(w).Encode([]Discussion{{ID: "d1"}})
		case "2":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message":"500 Internal Server Error"}`))
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListDiscussions(context.Background(), "proj", 1)
	if err == nil {
		t.Fatal("expected error from second page, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error to contain 500, got: %v", err)
	}
}

func TestListDiscussions_SinglePageStillWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next-Page", "")
		json.NewEncoder(w).Encode([]Discussion{
			{ID: "only", Notes: []Note{{ID: 1, Body: "single"}}},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	discs, err := client.ListDiscussions(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(discs) != 1 {
		t.Fatalf("expected 1 discussion, got %d", len(discs))
	}
	if discs[0].ID != "only" {
		t.Fatalf("expected discussion ID only, got %s", discs[0].ID)
	}
}

func TestListChangedFiles_Diffs404FallbackSendsCorrectPaths(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/diffs") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"404"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"changes": []any{}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	_, err := client.ListChangedFiles(context.Background(), "p", 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 requests, got %d: %v", len(paths), paths)
	}
	if !strings.HasSuffix(paths[0], "/diffs") {
		t.Fatalf("first request should be /diffs, got %s", paths[0])
	}
	if !strings.HasSuffix(paths[1], "/changes") {
		t.Fatalf("second request should be /changes, got %s", paths[1])
	}
}
