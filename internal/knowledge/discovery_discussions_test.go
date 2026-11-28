// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("OCR_TEST_DISCUSSION_CLI") == "1" && len(os.Args) > 1 && os.Args[1] == "api" {
		if len(os.Args) != 7 || !slices.Equal(os.Args[1:6], []string{"api", "--hostname", "host.example", "--method", "GET"}) {
			os.Exit(2)
		}
		switch os.Args[6] {
		case "/ok":
			fmt.Print("[]")
		case "/large":
			fmt.Print(strings.Repeat("x", (2<<20)+1))
		default:
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDiscussionCLIReadBoundary(t *testing.T) {
	t.Setenv("OCR_TEST_DISCUSSION_CLI", "1")
	cli, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := runDiscussionAPI(t.Context(), cli, "host.example", "/ok")
	if err != nil || string(got) != "[]" {
		t.Fatalf("GET request: %s, %v", got, err)
	}
	if _, err := runDiscussionAPI(t.Context(), cli, "host.example", "/large"); err == nil || !strings.Contains(err.Error(), "2 MiB") {
		t.Fatalf("provider size limit: %v", err)
	}
	if _, err := runDiscussionAPI(t.Context(), cli, "host.example", "/missing"); err == nil {
		t.Fatal("provider failure was hidden")
	}
}

func TestDiscussionExportAndRemoteBinding(t *testing.T) {
	repo := newDiscoveryRepo(t)
	repo.write("comments.json", `[{"id":"c1","body":"A recorded decision.","url":"https://example.org/pull/3#c1","updated_at":"2024-01-01T00:00:00Z"}]`)
	path := filepath.Join(repo.dir, "comments.json")
	notes, err := LoadDiscussions(t.Context(), repo.dir, path)
	if err != nil || len(notes) != 1 || notes[0].ID != "c1" {
		t.Fatalf("normalized export: %+v %v", notes, err)
	}
	for _, raw := range []string{
		`null`, `[{"id":"x","body":"ok","trusted":true}]`, `[{"body":"text"}]`,
		`[{"id":"c1","body":"a"},{"id":"c1","body":"b"}]`, `[{"id":"c","body":"x","url":"https://login@example.org/"}]`,
		`[{"id":"c","body":"x","path":"../x.go"}]`, `[{"id":"c","body":"x","revision":"HEAD"}]`,
		`[{"id":"c","body":"a","created_at":"tomorrow"}]`, `[{"id":"c","body":"a","updated_at":"2023-01-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z"}]`,
	} {
		repo.write("comments.json", raw)
		if _, err := LoadDiscussions(t.Context(), repo.dir, path); err == nil {
			t.Errorf("accepted invalid export %s", raw)
		}
	}
	pr, err := parseDiscussionURL("https://git.example.org/team/sub/project/-/merge_requests/19")
	if err != nil || pr.cli != "glab" || pr.project != "team/sub/project" {
		t.Fatalf("GitLab URL: %+v %v", pr, err)
	}
	repo.git("remote", "add", "upstream", "git@git.example.org:team/sub/project.git")
	if err := pr.checkRemote(t.Context(), repo.dir); err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{"https://git.example.org:8443/team/sub/project.git", "https://git.example.org/team/sub/other", "file:///team/sub/project"} {
		if pr.matchesRemote(other) {
			t.Errorf("accepted unrelated remote %s", other)
		}
	}
	for _, bad := range []string{"https://login@git.example.org/group/repo/pull/1", "https://git.example.org/a/b/pull/-2", "http://git.example.org/a/b/pull/3", "https://git.example.org/../../pull/4"} {
		if _, err := parseDiscussionURL(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := LoadDiscussions(t.Context(), repo.dir, "https://elsewhere.example.org/team/sub/project/-/merge_requests/19"); err == nil || !strings.Contains(err.Error(), "configured Git remote") {
		t.Fatalf("provider should not be invoked across unrelated remotes: %v", err)
	}
}

func TestNormalizedDiscussionThreadExport(t *testing.T) {
	repo := newDiscoveryRepo(t)
	file := filepath.Join(repo.dir, "comments.json")
	const review = "https://github.com/owner/project/pull/42"
	const edit = "2024-03-11T09:10:00Z"
	const published = "2024-03-11T11:30:00Z"
	revision := strings.Repeat("a", 40)
	root := Discussion{ID: "root", ReviewID: review, URL: review + "#discussion_r1", Body: "Review note", Path: "main.go",
		OriginalRevision: revision, StartLine: 4, EndLine: 5, SourceQuote: "func start() {\n\trun()"}
	reply := Discussion{ID: "reply", ReviewID: review, URL: review + "#discussion_r2", Body: "Follow up", Path: "main.go",
		ReplyTo: root.ID, CreatedAt: edit, UpdatedAt: edit, PublishedAt: published, PublishedRevision: revision}
	legacy := Discussion{ID: "legacy", Body: "Old flat export", UpdatedAt: edit}
	local := Discussion{ID: "standalone", ReviewID: "local:ticket/7", Body: "Local review"}
	opaque := Discussion{ID: "mirrored", ReviewID: "github:org/repo#12", Body: "Imported review"}
	truncated := Discussion{ID: "truncated", ReviewID: review, Body: "Another review note", Path: "main.go",
		OriginalRevision: revision, StartLine: 7, EndLine: 7, ReportedOriginalLine: 9, SourceQuote: "\trun()"}
	input := []Discussion{root, reply, legacy, local, opaque, truncated}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	repo.write("comments.json", string(raw))
	notes, err := LoadDiscussions(t.Context(), repo.dir, file)
	if err != nil || len(notes) != len(input) {
		t.Fatalf("read normalized export: %+v, %v", notes, err)
	}
	for i, note := range notes {
		if note.editionKey() != input[i].editionKey() || note.exportPath != "comments.json" || note.remote != nil {
			t.Fatalf("export record %d: %+v", i, note)
		}
	}
	for _, change := range []func(*Discussion){
		func(d *Discussion) { d.ReplyTo = d.ID },
		func(d *Discussion) { d.ReviewID = "https://login@github.com/owner/project/pull/42" },
		func(d *Discussion) { d.ReviewID += "#discussion_r1" },
		func(d *Discussion) { d.ReviewID = "//github.com/owner/project/pull/42" },
		func(d *Discussion) { d.ReviewID = "local\nreview" },
		func(d *Discussion) { d.URL = "https://github.com/owner/project/pull/43#discussion_r1" },
		func(d *Discussion) { d.PublishedRevision = "HEAD" },
		func(d *Discussion) { d.PublishedAt = "2024-03-11T09:09:59Z" },
		func(d *Discussion) { d.EndLine = maxEvidenceBytes + 1 },
		func(d *Discussion) { d.ReportedOriginalLine = 9 },
		func(d *Discussion) {
			d.OriginalRevision = revision
			d.StartLine, d.EndLine, d.SourceQuote = 4, 5, "one line"
		},
	} {
		invalid := reply
		change(&invalid)
		data, err := json.Marshal([]Discussion{root, invalid})
		if err != nil {
			t.Fatal(err)
		}
		repo.write("comments.json", string(data))
		if _, err := LoadDiscussions(t.Context(), repo.dir, file); err == nil {
			t.Errorf("accepted invalid export %s", data)
		}
	}
	if got := DiscussionReviewID("https://GitHub.COM/Owner/Project/pull/042/#discussion_r1"); got != review {
		t.Errorf("GitHub review identity = %q", got)
	}
	if got := DiscussionReviewID("https://gitlab.example/group/project/-/merge_requests/07#note_1"); got != "https://gitlab.example/group/project/-/merge_requests/7" {
		t.Errorf("GitLab review identity = %q", got)
	}
}

func TestGitHubDiscussionPaginationAndPermalinks(t *testing.T) {
	pr, err := parseDiscussionURL("https://github.com/owner/issues/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	notes, err := pr.load(t.Context(), func(_ context.Context, cli, host, endpoint string) ([]byte, error) {
		calls++
		if cli != "gh" || host != "github.com" || !strings.HasPrefix(endpoint, "repos/owner/issues/") {
			t.Fatalf("unexpected API call: %s %s %s", cli, host, endpoint)
		}
		switch endpoint {
		case "repos/owner/issues/pulls/42/comments?per_page=25&page=1":
			var rows []map[string]any
			for i := 1; i <= 25; i++ {
				rows = append(rows, map[string]any{"id": i, "path": "code.ts", "body": "review note", "commit_id": strings.Repeat("b", 40), "updated_at": "2024-01-01T00:00:00Z"})
			}
			return json.Marshal(rows)
		case "repos/owner/issues/pulls/42/comments?per_page=25&page=2":
			return []byte(`[{"id":26,"body":"reply","path":"code.ts"}]`), nil
		case "repos/owner/issues/pulls/42/reviews?per_page=25&page=1":
			return []byte(`[{"id":3,"body":""},{"id":4,"body":"Overall issue","submitted_at":"2024-01-01T00:00:00Z"}]`), nil
		case "repos/owner/issues/issues/42/comments?per_page=25&page=1":
			return []byte(`[{"id":77,"body":"Follow up","updated_at":"2024-01-02T00:00:00Z"}]`), nil
		default:
			t.Fatalf("unexpected endpoint: %s", endpoint)
			return nil, nil
		}
	})
	if err != nil || calls != 4 || len(notes) != 28 || notes[0].Revision == "" || notes[26].UpdatedAt != "" ||
		notes[26].URL != pr.link+"#pullrequestreview-4" || notes[27].URL != pr.link+"#issuecomment-77" || notes[0].URL != pr.link+"#discussion_r1" {
		t.Fatalf("GitHub comments: %+v (%d calls), %v", notes, calls, err)
	}
}

func TestGitHubRepliesUseSubmittedReview(t *testing.T) {
	pr, err := parseDiscussionURL("https://github.com/owner/project/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	original, published, mutable := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	const created = "2024-03-11T10:00:00Z"
	const edit = "2024-03-11T10:30:00Z"
	const submitted = "2024-03-11T11:30:00Z"
	const rootCreated = "2024-03-11T09:00:01Z"
	const anchor = "        and (item := next(items, None))"
	const pullAPI = "https://api.github.com/repos/owner/project/pulls/42"
	notes, err := pr.load(t.Context(), func(_ context.Context, _, _, endpoint string) ([]byte, error) {
		switch endpoint {
		case "repos/owner/project/pulls/42/comments?per_page=25&page=1":
			root := map[string]any{"id": 1, "pull_request_review_id": 100, "path": "queue.py", "body": "Check whether items can be empty",
				"side": "RIGHT", "diff_hunk": "@@ -2,4 +5,6 @@\n     while pending:\n+        if pending\n+" + anchor, "original_commit_id": original,
				"original_line": 9, "commit_id": mutable, "created_at": rootCreated, "updated_at": rootCreated, "pull_request_url": pullAPI}
			rows := []any{root}
			for id := 2; id <= 7; id++ {
				pull := pullAPI
				if id == 7 {
					pull = "https://api.github.com/repos/other/project/pulls/42"
				}
				rows = append(rows, map[string]any{"id": id, "in_reply_to_id": 1, "pull_request_review_id": 99 + id, "body": "Follow up",
					"path": "queue.py", "commit_id": mutable, "created_at": created, "updated_at": edit, "pull_request_url": pull})
			}
			return json.Marshal(rows)
		case "repos/owner/project/pulls/42/reviews?per_page=25&page=1":
			return json.Marshal([]map[string]any{
				{"id": 100, "state": "COMMENTED", "submitted_at": "2024-03-11T09:00:00Z", "commit_id": original, "body": ""},
				{"id": 101, "state": "COMMENTED", "submitted_at": submitted, "commit_id": published, "body": "", "pull_request_url": pullAPI},
				{"id": 102, "state": "PENDING", "commit_id": published, "body": "Draft"},
				{"id": 103, "state": "COMMENTED", "submitted_at": submitted, "commit_id": published, "body": "", "html_url": "https://github.com/owner/project/pull/43#pullrequestreview-103"},
				{"id": 105, "state": "COMMENTED", "submitted_at": submitted, "commit_id": "HEAD", "body": ""},
			})
		case "repos/owner/project/issues/42/comments?per_page=25&page=1":
			return []byte(`[]`), nil
		default:
			t.Fatalf("unexpected endpoint: %s", endpoint)
			return nil, nil
		}
	})
	if err != nil || len(notes) != 5 {
		t.Fatalf("provider discussions: %+v, %v", notes, err)
	}
	root, reply := notes[0], notes[1]
	if root.ReviewID != pr.link || root.ReplyTo != "" || root.PublishedAt != rootCreated || root.PublishedRevision != original || root.OriginalRevision != original ||
		root.StartLine != 7 || root.EndLine != 7 || root.ReportedOriginalLine != 9 || root.SourceQuote != anchor || root.position.valid() {
		t.Errorf("original review anchor: %+v", root)
	}
	if reply.ReviewID != root.ReviewID || reply.ReplyTo != root.ID || reply.PublishedAt != submitted || reply.PublishedRevision != published ||
		reply.Revision != mutable || reply.CreatedAt != created || reply.UpdatedAt != edit || reply.SourceQuote != "" {
		t.Errorf("reply publication and edition: %+v", reply)
	}
	for _, note := range notes[2:4] {
		if note.ReplyTo != root.ID || note.PublishedAt != "" || note.PublishedRevision != "" {
			t.Errorf("unattested reply acquired publication metadata: %+v", note)
		}
	}
	if note := notes[4]; note.PublishedAt != submitted || note.PublishedRevision != "" {
		t.Errorf("reply with missing commit: %+v", note)
	}
}

func TestGitLabDiscussionThreadAndLimits(t *testing.T) {
	pr, _ := parseDiscussionURL("https://gitlab.com/group/team/project/-/merge_requests/7")
	rev := strings.Repeat("a", 40)
	notes, err := pr.load(t.Context(), func(_ context.Context, cli, host, endpoint string) ([]byte, error) {
		if cli != "glab" || host != "gitlab.com" || endpoint != "projects/group%2Fteam%2Fproject/merge_requests/7/discussions?per_page=25&page=1" {
			t.Fatalf("GitLab endpoint: %s %s %s", cli, host, endpoint)
		}
		return fmt.Appendf(nil, `[{"notes":[{"id":1,"body":"status changed","system":true},{"id":2,"body":"review","position":{"new_path":"src/entry.rs","head_sha":"%s"}},{"id":3,"body":"confirmed","updated_at":"2024-01-06T01:02:03Z"}]}]`, rev), nil
	})
	if err != nil || len(notes) != 2 || notes[1].URL != pr.link+"#note_3" || notes[1].Path != "src/entry.rs" || notes[1].Revision != rev || notes[1].UpdatedAt == "" {
		t.Fatalf("GitLab thread: %+v %v", notes, err)
	}
	calls := 0
	_, err = pr.load(t.Context(), func(context.Context, string, string, string) ([]byte, error) {
		calls++
		return []byte("[" + strings.TrimSuffix(strings.Repeat(`{"notes":[]},`, 25), ",") + "]"), nil
	})
	if err == nil || calls != 5 || !strings.Contains(err.Error(), "100 items") {
		t.Fatalf("provider pagination was not bounded: %d calls, %v", calls, err)
	}
}
