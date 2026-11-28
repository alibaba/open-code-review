// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestSquashedDiscussionOriginalLandedAndCurrentEvidence(t *testing.T) {
	ctx := t.Context()
	sourceRepo := newDiscoveryRepo(t)
	sourceRepo.git("branch", "-M", "main")
	const path = "shipping/queue.py"
	const prefix = "def eligible(shipment):\n"
	const line = "    return shipment.is_ready()"
	const body = "A shipment may leave after its readiness check passes."
	sourceRepo.write(path, prefix+"    return True\n")
	sourceRepo.write("billing/charge.py", "def charge(order):\n    return order.total\n")
	base := sourceRepo.commit("establish dispatch")
	sourceRepo.git("switch", "-c", "review")
	sourceRepo.write(path, prefix+line+"\n")
	proposal := sourceRepo.commit("discuss dispatch eligibility")
	originalBlob := sourceRepo.git("rev-parse", proposal.Commit+":"+path)
	sourceRepo.git("switch", "main")
	sourceRepo.git("merge", "--squash", "review")
	landed := sourceRepo.commit("squash approved dispatch changes")
	sourceRepo.write(path, "# Dispatch implementation\n"+prefix+line+"\n")
	final := sourceRepo.commit("move dispatch function down a line")
	dest := filepath.Join(t.TempDir(), "accepted")
	sourceRepo.git("clone", "--no-local", "--single-branch", "--branch", "main", sourceRepo.dir, dest)
	repo := discoveryRepo{t, dest}
	tree := Tree{Repo: dest, Commit: final.Commit, Git: gitcmd.New(1)}
	if repo.git("rev-list", "--parents", "-n", "1", landed.Commit) != landed.Commit+" "+base.Commit {
		t.Fatal("fixture did not create a single-parent squash commit")
	}
	if strings.Contains(repo.git("rev-list", "--all"), proposal.Commit) {
		t.Fatal("the original review commit is reachable from main")
	}
	const edited = "2025-05-01T08:00:00Z"
	const merged = "2025-05-01T09:00:00Z"
	early, _ := time.Parse(time.RFC3339, edited)
	after, _ := time.Parse(time.RFC3339, merged)
	hunk := "@@ -1,2 +1,2 @@\n def eligible(shipment):\n-    return True\n+" + line
	for _, provider := range []string{"gh", "glab"} {
		t.Run(provider, func(t *testing.T) {
			link := "https://code.example/team/project/pull/7"
			if provider == "glab" {
				link = "https://code.example/team/project/-/merge_requests/7"
			}
			pr, err := parseDiscussionURL(link)
			if err != nil {
				t.Fatal(err)
			}
			repo.git("remote", "set-url", "origin", "https://code.example/team/project.git")
			if err := pr.checkRemote(ctx, dest); err != nil {
				t.Fatal(err)
			}
			target, mergeID, mergedTime, remoteHead := "team/project", landed.Commit, merged, proposal.Commit
			status := "merged"
			originalBody := prefix + line + "\n"
			originalReads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var v any
				switch r.URL.EscapedPath() {
				case "/repos/team/project/pulls/7/comments":
					v = []any{map[string]any{"id": 91, "path": path, "body": body, "updated_at": edited, "commit_id": remoteHead, "original_commit_id": remoteHead, "side": "RIGHT", "original_line": 2, "diff_hunk": hunk}}
				case "/repos/team/project/pulls/7/reviews", "/repos/team/project/issues/7/comments":
					v = []any{}
				case "/repos/team/project/pulls/7":
					v = map[string]any{"merged": status == "merged", "merged_at": mergedTime, "merge_commit_sha": mergeID, "number": 7, "base": map[string]any{"repo": map[string]any{"full_name": target}}, "head": map[string]any{"repo": map[string]any{"full_name": "contributor/fork"}}}
				case "/projects/team%2Fproject/merge_requests/7/discussions":
					v = []any{map[string]any{"notes": []any{map[string]any{"id": 91, "body": body, "updated_at": edited, "position": map[string]any{"position_type": "text", "new_path": path, "head_sha": remoteHead, "new_line": 2,
						"line_range": map[string]any{"start": map[string]any{"old_line": 1, "new_line": 1}, "end": map[string]any{"type": "new", "new_line": 2}}}}}}}
				case "/projects/team%2Fproject/merge_requests/7":
					v = map[string]any{"iid": 7, "state": status, "web_url": "https://code.example/" + target + "/-/merge_requests/7", "project_id": 18, "target_project_id": 18, "source_project_id": 21, "merged_at": mergedTime, "squash_commit_sha": mergeID}
				case "/repos/team/project/contents/" + path, "/projects/team%2Fproject/repository/files/shipping%2Fqueue.py":
					originalReads++
					if r.URL.Query().Get("ref") != remoteHead {
						t.Errorf("original lookup was not pinned: %s", r.URL)
					}
					v = map[string]any{"type": "file", "path": path, "file_path": path, "commit_id": remoteHead, "sha": originalBlob, "blob_id": originalBlob, "encoding": "base64", "size": len(originalBody), "content": base64.StdEncoding.EncodeToString([]byte(originalBody))}
				default:
					http.NotFound(w, r)
					return
				}
				if err := json.NewEncoder(w).Encode(v); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			api := func(ctx context.Context, cli, host, endpoint string) ([]byte, error) {
				if cli != provider || host != "code.example" {
					return nil, fmt.Errorf("unexpected provider %s %s", cli, host)
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/"+endpoint, nil)
				if err != nil {
					return nil, err
				}
				resp, err := server.Client().Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
				}
				return io.ReadAll(resp.Body)
			}
			load := func() []Discussion {
				notes, err := pr.load(ctx, api)
				if err != nil || len(notes) != 1 {
					t.Fatalf("provider import: %+v, %v", notes, err)
				}
				return notes
			}
			first, second := load(), load()
			if first[0].LandingReceipt(early) != "" || first[0].LandingReceipt(after) == "" || first[0].LandingReceipt(after) != second[0].LandingReceipt(after) {
				t.Fatal("receipt differed by pointer identity or exposed a future merge")
			}
			notes := append(first, second...)
			opts := DiscoverOptions{AsOf: after, Discussions: notes}
			result, err := tree.Discover(ctx, opts, func(_ context.Context, _ string, corpus string) (string, error) {
				var ss []discoverySource
				if err := json.Unmarshal([]byte(corpus), &ss); err != nil {
					return "", err
				}
				var current, archive string
				for _, s := range ss {
					if s.Kind == "code" && s.Path == path && strings.Contains(s.Text, line) {
						current = s.ID
					}
					if s.Kind == "discussion" && s.Landing != nil && s.Landing.OriginalRevision == proposal.Commit {
						archive = s.ID
					}
				}
				return fmt.Sprintf(`{"records":[{"category":"business-rule","claim":%q,"scope":%q,"citations":[{"source":%q,"quote":%q},{"source":%q,"quote":%q}]}]}`, body, path, current, line, archive, body), nil
			})
			if err != nil || !result.Cacheable() || len(result.Facts) != 1 || len(result.DiscussionSources) != 1 || originalReads != 1 {
				t.Fatalf("squash import sources=%d rules=%d original reads=%d: %+v, %v", len(result.DiscussionSources), len(result.Facts), originalReads, result, err)
			}
			c := result.DiscussionSources[0].Citations[0]
			wantStart, wantQuote := 2, line
			if provider == "glab" {
				wantStart, wantQuote = 1, prefix+line
			}
			if c.Revision != landed.Commit || c.Landing == nil || c.Landing.OriginalRevision != proposal.Commit || c.Landing.OriginalStart != wantStart || c.Landing.OriginalEnd != 2 || c.Landing.LandedStart != wantStart || c.Landing.Quote != wantQuote || c.Landing.MergedAt != merged {
				t.Fatalf("landing provenance: %+v %+v", c, c.Landing)
			}
			store := testStore(t)
			if err := store.Save(ctx, append(result.Facts, result.DiscussionObservations()...)); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				at    time.Time
				count int
			}{{early, 0}, {after, 2}} {
				atTree := tree
				atTree.Now = tc.at
				facts, err := store.ListAt(ctx, "", "", tc.at)
				if err != nil || len(facts) != tc.count {
					t.Fatalf("stored at %s: %+v, %v", tc.at, facts, err)
				}
				checked := checkedFacts(t, atTree, storedFacts(t, store))
				prompt := ReviewSnapshot(checked).ForPaths([]string{path})
				if len(checked) != tc.count || strings.Contains(prompt, body) != (tc.count == 2) {
					t.Fatalf("review at %s: %s", tc.at, prompt)
				}
			}
			without := func(label string, tr Tree, at time.Time, supplied []Discussion) {
				t.Helper()
				tr.Now = at
				res := discoverWithoutRules(t, tr, DiscoverOptions{AsOf: at, Discussions: supplied})
				if len(res.DiscussionSources) != 0 || res.SourceKinds["discussion"] != 0 {
					t.Fatalf("%s admitted a source: %+v", label, res)
				}
			}
			without("future merge", tree, early, first)
			older := tree
			older.Commit = base.Commit
			without("merge outside accepted tree", older, after, first)
			excluded := tree
			excluded.AllowPath = func(p string) bool { return p != path }
			without("excluded file", excluded, after, first)
			if originalReads != 1 {
				t.Fatal("ineligible inputs fetched an original file")
			}
			t.Run("scope excludes remote read and still retires accepted moves", func(t *testing.T) {
				note := first[0]
				remote := *note.remote
				attempts := 0
				remote.api = func(context.Context, string, string, string) ([]byte, error) {
					attempts++
					return nil, fmt.Errorf("original file unavailable")
				}
				note.remote = &remote
				scoped := DiscoverOptions{Scope: "billing", AsOf: after, Discussions: []Discussion{note}}
				res := discoverWithoutRules(t, tree, scoped)
				if attempts != 0 || !res.Cacheable() || res.SourceKinds["discussion"] != 0 {
					t.Fatalf("outside scope: attempted %d reads, cacheable %v", attempts, res.Cacheable())
				}
				db := testStore(t)
				if err := db.SaveDiscovery(ctx, "billing-only", res.DiscussionObservations()); err != nil {
					t.Fatal(err)
				}
				if cached, err := db.Scanned(ctx, "billing-only"); err != nil || !cached {
					t.Fatalf("missing scoped receipt: %v %v", cached, err)
				}
				scoped.Scope = "shipping"
				res = discoverWithoutRules(t, tree, scoped)
				if attempts != 1 || res.Cacheable() {
					t.Fatalf("in-scope read failure: %d attempts, cacheable %v", attempts, res.Cacheable())
				}
				old := Discussion{ID: note.ID, Path: "billing/charge.py", Body: "Keep the invoice open.", UpdatedAt: "2025-04-30T00:00:00Z"}
				scoped.Scope, scoped.Discussions = "billing", []Discussion{old}
				if err := db.Save(ctx, discoverWithoutRules(t, tree, scoped).DiscussionObservations()); err != nil {
					t.Fatal(err)
				}
				if len(checkedFacts(t, tree, storedFacts(t, db))) != 1 {
					t.Fatal("original scoped source is missing")
				}
				note.Revision = landed.Commit
				scoped.Discussions = []Discussion{note}
				if err := db.Save(ctx, discoverWithoutRules(t, tree, scoped).DiscussionObservations()); err != nil {
					t.Fatal(err)
				}
				if got := checkedFacts(t, tree, storedFacts(t, db)); len(got) != 0 || attempts != 1 {
					t.Fatalf("accepted move left %d sources or fetched a file (%d)", len(got), attempts)
				}
			})
			t.Run("matching export and landed source", func(t *testing.T) {
				unpinned := Discussion{ID: first[0].ID, Path: path, Body: body, URL: first[0].URL, UpdatedAt: edited}
				weak := discoverWithoutRules(t, tree, DiscoverOptions{AsOf: after, Discussions: []Discussion{unpinned}}).DiscussionSources
				if len(weak) != 1 {
					t.Fatal("the independent dated export was not imported")
				}
				for _, pair := range [][]Discussion{{unpinned, first[0]}, {first[0], unpinned}} {
					_, err := tree.Discover(ctx, DiscoverOptions{AsOf: after, Discussions: pair}, func(_ context.Context, _, corpus string) (string, error) {
						var rows []discoverySource
						if err := json.Unmarshal([]byte(corpus), &rows); err != nil {
							return "", err
						}
						var found []discoverySource
						for _, s := range rows {
							if s.Kind == "discussion" {
								found = append(found, s)
							}
						}
						if len(found) != 1 || found[0].Landing == nil || found[0].Revision != landed.Commit {
							t.Errorf("equal mixed editions yielded %d model sources with landing=%v", len(found), len(found) == 1 && found[0].Landing != nil)
						}
						return `{"records":[]}`, nil
					})
					if err != nil {
						t.Fatal(err)
					}
				}
				check := func(t *testing.T, copy Fact, conflict bool) {
					t.Helper()
					db := testStore(t)
					if err := db.Save(ctx, []Fact{copy}); err != nil {
						t.Fatal(err)
					}
					if err := db.Save(ctx, append(result.Facts, result.DiscussionObservations()...)); err != nil {
						t.Fatal(err)
					}
					var raw, strong, dependent bool
					for _, c := range checkedFacts(t, tree, storedFacts(t, db)) {
						if c.Status == StatusDiscussionSource {
							raw = true
							strong = c.Citations[0].Landing != nil
						}
						if c.Category == CategoryBusinessRule && c.Status == StatusCandidate {
							dependent = true
						}
					}
					if !conflict && (!strong || !dependent) || conflict && (raw || dependent) {
						t.Errorf("conflicting=%v: raw=%v verified landing=%v inferred rule=%v", conflict, raw, strong, dependent)
					}
				}
				check(t, weak[0], false)
				alternate := unpinned
				alternate.Revision = base.Commit
				opposing := discoverWithoutRules(t, tree, DiscoverOptions{AsOf: after, Discussions: []Discussion{alternate}}).DiscussionSources
				if len(opposing) != 1 {
					t.Fatal("the opposing explicit revision was not eligible")
				}
				check(t, opposing[0], true)
				alternate = unpinned
				alternate.Body = "Dispatch readiness is optional."
				other := discoverWithoutRules(t, tree, DiscoverOptions{AsOf: after, Discussions: []Discussion{alternate}}).DiscussionSources
				check(t, other[0], true)
			})
			status = "opened"
			without("proposed merge", tree, after, load())
			status = "merged"
			target = "elsewhere/project"
			without("another target project", tree, after, load())
			target = "team/project"
			originalBody = prefix + "    return 999\n"
			without("incorrect immutable blob", tree, after, first)
			originalBody = prefix + line + "\n"
			offline, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			var imported []Discussion
			if err = decodeDiscoveryJSON(offline, &imported); err != nil {
				t.Fatal(err)
			}
			without("JSON export without provider proof", tree, after, imported)
			illicit := bytes.Replace(offline, []byte(`"body":`), []byte(`"landing":{"merged_at":"2025-05-01T09:00:00Z"},"body":`), 1)
			if err := decodeDiscoveryJSON(illicit, &imported); err == nil {
				t.Fatal("local export could mint a provider proof")
			}
			repo.write(path, prefix+line+"\n"+prefix+line+"\n")
			ambiguous := repo.commit("make original expression ambiguous")
			without("ambiguous source", ambiguous, after, first)
			checked := checkedFacts(t, ambiguous, storedFacts(t, store))
			if prompt := ReviewSnapshot(checked).ForPaths([]string{path}); strings.Contains(prompt, body) {
				t.Fatalf("ambiguous proof remained usable: %s", prompt)
			}
			repo.git("switch", "--detach", final.Commit)
		})
	}
}

func TestGitHubTruncatedHunkUsesDisplayedRightSideLine(t *testing.T) {
	revision := strings.Repeat("a", 40)
	const quote = "    return item.ready()"
	const hunk = "@@ -10,6 +40,8 @@\n def process(item):\n-    check_old(item)\n-    return False\n+    check(item)\n+" + quote + "\n"
	for _, tc := range []struct {
		reported, wantLine int
		exact              bool
	}{{42, 42, true}, {46, 42, false}, {48, 0, false}} {
		position, exact := githubDiscussionPosition(revision, 0, tc.reported, hunk)
		if position.start != tc.wantLine || position.end != tc.wantLine || exact != tc.exact || tc.wantLine != 0 && position.hunkQuote != quote {
			t.Errorf("reported line %d gave position %+v, exact=%t", tc.reported, position, exact)
		}
	}
}

func TestGitLabDiscussionRangeRequiresConsistentNewSideLines(t *testing.T) {
	rev := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name               string
		line               int
		span               *gitlabDiscussionRange
		wantStart, wantEnd int
	}{
		{"single line", 5, nil, 5, 5},
		{"range without end hint", 0, &gitlabDiscussionRange{gitlabRangeLine{"", 3}, gitlabRangeLine{"new", 5}}, 3, 5},
		{"end disagrees", 4, &gitlabDiscussionRange{gitlabRangeLine{"new", 3}, gitlabRangeLine{"new", 5}}, 0, 0},
		{"starts on deletion", 5, &gitlabDiscussionRange{gitlabRangeLine{"old", 3}, gitlabRangeLine{"new", 5}}, 0, 0},
		{"no new end", 0, &gitlabDiscussionRange{gitlabRangeLine{"new", 3}, gitlabRangeLine{"old", 0}}, 0, 0},
		{"reversed", 3, &gitlabDiscussionRange{gitlabRangeLine{"new", 5}, gitlabRangeLine{"new", 3}}, 0, 0},
		{"over source limit", 15, &gitlabDiscussionRange{gitlabRangeLine{"new", 3}, gitlabRangeLine{"new", 15}}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := gitlabDiscussionPosition(rev, tc.line, tc.span)
			if got.start != tc.wantStart || got.end != tc.wantEnd {
				t.Fatalf("parsed range %+v", got)
			}
		})
	}
}
