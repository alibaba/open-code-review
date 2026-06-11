//go:build gitflic_e2e

package gitflic

// End-to-end test against a live GitFlic instance (e.g. a local GitFlic CE
// in Docker). Excluded from regular `make test` by the gitflic_e2e build tag.
//
// Run:
//   GITFLIC_TOKEN=<token> go test -tags gitflic_e2e -count=1 -v ./internal/publish/gitflic/
//
// Environment:
//   GITFLIC_TOKEN             access token (required; test skips when unset)
//   GITFLIC_E2E_API_URL       REST API base URL (default http://localhost:8080/rest-api)
//   GITFLIC_E2E_GIT_URL       web/git base URL  (default http://localhost:8080)
//   GITFLIC_E2E_GIT_USER      git push login    (default adminuser@admin.local)
//   GITFLIC_E2E_GIT_PASSWORD  git push password (default qwerty123)
//
// GitFlic CE does not accept REST tokens for git push, hence the separate
// login/password pair (defaults match the stock CE admin account).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/model"
)

const (
	e2eOldMain = `package main

import "fmt"

func main() {
	name := "world"
	fmt.Println("hello", name)
	fmt.Println("done")
}
`
	e2eNewMain = `package main

import "fmt"

func main() {
	name := "GitFlic"
	greet(name)
	fmt.Println("hello", name)
	fmt.Println("done")
}
`
	e2eUtil = `package main

import "fmt"

func greet(name string) {
	fmt.Println("greetings,", name)
}
`
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// e2eAPI is a tiny helper around the GitFlic REST API for test fixtures.
type e2eAPI struct {
	t       *testing.T
	baseURL string
	token   string
}

func (a *e2eAPI) request(method, path string, body any, out any) error {
	var reqBody *strings.Reader = strings.NewReader("")
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, a.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+a.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (a *e2eAPI) must(method, path string, body any, out any) {
	a.t.Helper()
	if err := a.request(method, path, body, out); err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// e2eDiscussion is the discussion shape we assert on. The _embedded list key
// differs between GitFlic cloud and CE versions, so the envelope is decoded
// generically.
type e2eDiscussion struct {
	RootNote struct {
		RawMessage string `json:"rawMessage"`
		NewLine    *int   `json:"newLine"`
		OldLine    *int   `json:"oldLine"`
		NewPath    string `json:"newPath"`
		OldPath    string `json:"oldPath"`
	} `json:"rootNote"`
}

func listDiscussions(t *testing.T, api *e2eAPI, owner, alias string, mrID int) []e2eDiscussion {
	t.Helper()
	var envelope struct {
		Embedded map[string]json.RawMessage `json:"_embedded"`
	}
	api.must("GET", fmt.Sprintf("/project/%s/%s/merge-request/%d/discussions?size=50", owner, alias, mrID), nil, &envelope)
	for _, raw := range envelope.Embedded {
		var items []e2eDiscussion
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatalf("decode discussions: %v", err)
		}
		return items
	}
	return nil
}

func TestE2EPublishToGitFlic(t *testing.T) {
	token := os.Getenv("GITFLIC_TOKEN")
	if token == "" {
		t.Skip("GITFLIC_TOKEN not set; skipping GitFlic e2e")
	}
	apiURL := envOr("GITFLIC_E2E_API_URL", "http://localhost:8080/rest-api")
	gitURL := envOr("GITFLIC_E2E_GIT_URL", "http://localhost:8080")
	gitUser := envOr("GITFLIC_E2E_GIT_USER", "adminuser@admin.local")
	gitPassword := envOr("GITFLIC_E2E_GIT_PASSWORD", "qwerty123")

	api := &e2eAPI{t: t, baseURL: strings.TrimRight(apiURL, "/"), token: token}

	// 1. Resolve current user and create a throwaway project.
	var me struct {
		Username string `json:"username"`
	}
	api.must("GET", "/user/me", nil, &me)
	owner := me.Username

	alias := "ocr-e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	api.must("POST", "/project", map[string]any{
		"title":          "OCR publish e2e",
		"alias":          alias,
		"isPrivate":      true,
		"ownerAlias":     owner,
		"ownerAliasType": "USER",
	}, nil)
	t.Cleanup(func() {
		var proj struct {
			ID string `json:"id"`
		}
		if err := api.request("GET", "/project/"+owner+"/"+alias, nil, &proj); err == nil && proj.ID != "" {
			_ = api.request("DELETE", "/project/"+proj.ID+"/delete", nil, nil)
		}
	})

	// 2. Seed the repo: master, then a feature branch with a modified line,
	// an inserted line and a new file.
	repoDir := t.TempDir()
	pushTarget, err := url.Parse(gitURL)
	if err != nil {
		t.Fatalf("parse GITFLIC_E2E_GIT_URL: %v", err)
	}
	pushTarget.User = url.UserPassword(gitUser, gitPassword)
	pushTarget.Path = fmt.Sprintf("/project/%s/%s.git", owner, alias)

	runGit(t, repoDir, "init", "-q", "-b", "master")
	runGit(t, repoDir, "config", "user.email", "e2e@test.local")
	runGit(t, repoDir, "config", "user.name", "OCR e2e")
	writeFile(t, repoDir, "main.go", e2eOldMain)
	runGit(t, repoDir, "add", "-A")
	runGit(t, repoDir, "commit", "-qm", "initial")
	runGit(t, repoDir, "remote", "add", "origin", pushTarget.String())
	runGit(t, repoDir, "push", "-q", "-u", "origin", "master:master")

	runGit(t, repoDir, "checkout", "-qb", "feature")
	writeFile(t, repoDir, "main.go", e2eNewMain)
	writeFile(t, repoDir, "util.go", e2eUtil)
	runGit(t, repoDir, "add", "-A")
	runGit(t, repoDir, "commit", "-qm", "feature: add greet")
	runGit(t, repoDir, "push", "-q", "-u", "origin", "feature:feature")

	// 3. Open a merge request feature -> master.
	var proj struct {
		ID string `json:"id"`
	}
	api.must("GET", "/project/"+owner+"/"+alias, nil, &proj)
	var mr struct {
		LocalID int `json:"localId"`
	}
	api.must("POST", "/project/"+owner+"/"+alias+"/merge-request", map[string]any{
		"title":         "OCR e2e MR",
		"description":   "merge request created by the gitflic_e2e test",
		"sourceProject": map[string]string{"id": proj.ID},
		"targetProject": map[string]string{"id": proj.ID},
		"sourceBranch":  map[string]string{"id": "feature"},
		"targetBranch":  map[string]string{"id": "master"},
	}, &mr)
	if mr.LocalID == 0 {
		t.Fatal("merge request was not created")
	}

	// 4. Publish a synthetic review result.
	result := &ReviewResult{
		Comments: []model.LlmComment{
			{Path: "main.go", Content: "e2e: comment on a modified line", StartLine: 6, EndLine: 6,
				ExistingCode: `name := "GitFlic"`, SuggestionCode: "name := os.Args[1]"},
			{Path: "main.go", Content: "e2e: comment on an added line", StartLine: 7, EndLine: 7},
			{Path: "util.go", Content: "e2e: comment in a new file", StartLine: 6, EndLine: 6},
			{Path: "ghost.go", Content: "e2e: comment that must fall back", StartLine: 1, EndLine: 1},
		},
	}
	ctx := context.Background()
	diffs, err := diff.NewProvider(repoDir, "master", "feature", nil).GetDiff(ctx)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}

	publisher := NewPublisher(NewClient(apiURL, token))
	stats, err := publisher.Publish(ctx, Options{
		Owner:   owner,
		Project: alias,
		MRID:    strconv.Itoa(mr.LocalID),
	}, result, diffs)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if stats.Inline != 3 || stats.Fallback != 1 {
		t.Fatalf("stats = %+v, want 3 inline / 1 fallback", stats)
	}

	// 5. Verify what GitFlic actually stored.
	discussions := listDiscussions(t, api, owner, alias, mr.LocalID)
	if len(discussions) != 5 {
		t.Fatalf("expected 5 discussions (3 inline + fallback + summary), got %d", len(discussions))
	}

	type position struct {
		newLine, oldLine int
		oldPath          string
	}
	inline := map[string]position{}
	var general []string
	for _, d := range discussions {
		n := d.RootNote
		if n.NewPath == "" {
			general = append(general, n.RawMessage)
			continue
		}
		if n.NewLine == nil || n.OldLine == nil {
			t.Errorf("inline discussion on %s lacks line info", n.NewPath)
			continue
		}
		inline[fmt.Sprintf("%s:%d", n.NewPath, *n.NewLine)] = position{*n.NewLine, *n.OldLine, n.OldPath}
	}

	expectations := map[string]position{
		"main.go:6": {6, 6, "main.go"}, // modified line maps to its old position
		"main.go:7": {7, 6, "main.go"}, // added line anchors to the preceding old line
		"util.go:6": {6, 1, "util.go"}, // new file anchors to its own path, line 1
	}
	for key, want := range expectations {
		got, ok := inline[key]
		if !ok {
			t.Errorf("missing inline discussion %s (have %v)", key, inline)
			continue
		}
		if got != want {
			t.Errorf("%s position = %+v, want %+v", key, got, want)
		}
	}

	if len(general) != 2 {
		t.Fatalf("expected 2 general notes (fallback + summary), got %d", len(general))
	}
	joined := strings.Join(general, "\n===\n")
	if !strings.Contains(joined, "could not be posted inline") || !strings.Contains(joined, "ghost.go") {
		t.Errorf("fallback note missing or incomplete:\n%s", joined)
	}
	if !strings.Contains(joined, "**4** issue(s)") {
		t.Errorf("summary note missing or incomplete:\n%s", joined)
	}
}
