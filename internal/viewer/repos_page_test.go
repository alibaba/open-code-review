// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertReposLandingMarkup(t *testing.T, body string, encodedPaths []string) {
	t.Helper()
	required := []string{
		"<title>Repositories",
		"<h2>Repositories</h2>",
		`id="repository-search-input"`,
		`id="repositories-table"`,
		">Action<",
		`id="repositories-pagination"`,
		`src="/static/repos.js"`,
		`class="repos-page"`,
		`class="repo-check"`,
	}
	for _, path := range encodedPaths {
		required = append(required,
			path,
			`href="/r/`+path+`"`,
			">Check</a>",
		)
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("repositories page missing %q", want)
		}
	}
	if strings.Contains(body, `onclick=`) || strings.Contains(body, `oninput=`) || strings.Contains(body, `onchange=`) {
		t.Error("repositories page has inline event handlers")
	}
	if strings.Contains(body, `style=`) {
		t.Error("repositories page has inline style attributes")
	}
	if strings.Contains(body, "<script>") {
		t.Error("repositories page has an inline <script> block")
	}
	if strings.Contains(body, `<td data-repository-name><a `) {
		t.Error("repository name should be plain text; Check is the row action")
	}
	for _, path := range encodedPaths {
		if !strings.Contains(body, `href="/r/`+path+`">Check</a>`) {
			t.Errorf("Check control missing for %q", path)
		}
	}
}

func assertReposEmptyMarkup(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "No session data found") {
		t.Error("expected empty-state message in body")
	}
	if !strings.Contains(body, "<h2>Repositories</h2>") {
		t.Error("empty repositories page missing title")
	}
	for _, forbidden := range []string{
		`id="repository-search-input"`,
		`id="repositories-table"`,
		`id="repositories-pagination"`,
		">Check</a>",
		`src="/static/repos.js"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("empty repositories page should not contain %q", forbidden)
		}
	}
}

func writeRepoFixture(t *testing.T, root, encodedPath string) {
	t.Helper()
	repoDir := filepath.Join(root, encodedPath)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "s1.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T10:00:00Z"}`)
}

// GET / is served through newMux (the real viewer HTTP path), not by calling
// handleRepos directly. Hitting it twice guards against handlers that only
// succeed on the first render (template parse cache, leftover pagination
// state, etc.).
func TestMux_GetRootReposPage(t *testing.T) {
	root := t.TempDir()
	writeRepoFixture(t, root, "fixture-repo")

	mux := newMux(root)
	capture := os.Getenv("OCR_SCRATCH")

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET / pass %d: status = %d, want 200", i, rr.Code)
		}
		body := rr.Body.String()
		if body == "" {
			t.Fatalf("GET / pass %d: empty body", i)
		}
		assertReposLandingMarkup(t, body, []string{"fixture-repo"})
		if capture != "" {
			name := filepath.Join(capture, fmt.Sprintf("get-root-%d.html", i))
			if err := os.WriteFile(name, rr.Body.Bytes(), 0644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}
}

// Issue #1322 and the repositories.png mockup specify a green Check control.
// --accent is still indigo until #1320, so the repos page sets the brand green
// on .repo-check directly. This reads the shipped stylesheet, not a copy of
// the hex in the test.
func TestReposPageCSS_CheckIsMockupGreen(t *testing.T) {
	css, err := assets.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read shipped style.css: %v", err)
	}
	src := string(css)
	marker := ".repos-page #repositories-table .repo-check {"
	idx := strings.Index(src, marker)
	if idx < 0 {
		t.Fatalf("shipped style.css missing %q", marker)
	}
	end := idx + 240
	if end > len(src) {
		end = len(src)
	}
	block := src[idx:end]
	if !strings.Contains(block, "#2BDE5E") {
		t.Errorf("Check color in shipped CSS is not the mockup green #2BDE5E:\n%s", block)
	}
	if strings.Contains(block, "var(--accent)") {
		t.Errorf("Check still uses indigo --accent; the mockup and #1322 ask for green:\n%s", block)
	}
}

func TestHandleRepos_RendersEveryRepoForClientPagination(t *testing.T) {
	root := t.TempDir()
	var names []string
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("repo-%02d", i)
		names = append(names, name)
		writeRepoFixture(t, root, name)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handleRepos(rr, req, root)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	assertReposLandingMarkup(t, rr.Body.String(), names)
}
