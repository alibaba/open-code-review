// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
)

func TestMemorySearchUsesTheDiscussionEditionAtTheReviewTime(t *testing.T) {
	repo := initResumeRepo(t)
	const source = "delivery.go"
	commitFile(t, repo, source, "package delivery\nfunc dispatch() {}\n", "dispatch deliveries")
	tree := knowledge.Tree{Repo: repo, Commit: revParse(t, repo, "HEAD"), Git: gitcmd.New(1)}
	const early = "2025-01-01T00:00:00Z"
	const late = "2025-04-01T00:00:00Z"
	notes := []knowledge.Discussion{
		{ID: "timing", Path: source, UpdatedAt: early, Body: "Keep the shipment pending."},
		{ID: "timing", Path: source, UpdatedAt: late, Body: "Release the shipment immediately. M\u00f6glich."},
	}
	result, err := tree.Discover(t.Context(), knowledge.DiscoverOptions{Discussions: notes}, func(context.Context, string, string) (string, error) {
		return `{"records":[]}`, nil
	})
	if err != nil || len(result.DiscussionSources) != 2 {
		t.Fatalf("importing two editions: %+v, %v", result.DiscussionSources, err)
	}
	db := filepath.Join(t.TempDir(), "editions.db")
	store, err := openMemory(t.Context(), db, repo, tree.Git, true)
	if err != nil {
		t.Fatal(err)
	}
	// Persist the original first, as in separate imports, so only its text
	// occupies the immutable fact_search row for this external source ID.
	for _, stamp := range []string{early, late} {
		for _, f := range result.DiscussionSources {
			if f.ObservedAt == stamp {
				if err := store.Save(t.Context(), []knowledge.Fact{f}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	lateCutoff, _ := time.Parse(time.RFC3339, late)
	if matches, err := store.ListAt(t.Context(), "", "M\u00d6GLICH", lateCutoff); err != nil || len(matches) == 0 {
		t.Fatalf("case-insensitive search did not locate the edited discussion: %v %v", matches, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ at, search, body string }{
		{early, "PENDING", notes[0].Body}, {early, "immediately", ""},
		{late, "pending", ""}, {late, "immediately", notes[1].Body}, {late, source, notes[1].Body},
		{early, "M\u00d6GLICH", ""}, {late, "M\u00d6GLICH", notes[1].Body},
	} {
		c := newMemoryCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetArgs([]string{"--repo", repo, "--memory-db", db, "list", "--as-of", tc.at, "--search", tc.search})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		var records []knowledge.Checked
		if err := json.Unmarshal(out.Bytes(), &records); err != nil {
			t.Fatal(err)
		}
		if tc.body == "" && len(records) != 0 || tc.body != "" && (len(records) != 1 || records[0].DiscussionBody != tc.body) {
			t.Errorf("search for %q at %s returned %+v", tc.search, tc.at, records)
		}
	}
}

func TestMemorySyncHistoricalSchemaRetainsWithdrawal(t *testing.T) {
	ctx := t.Context()
	startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"schema","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"records\":[]}"}],"stop_reason":"end_turn"}`)
	}))
	repo := initResumeRepo(t)
	commitFile(t, repo, "delivery.go", "package delivery\nfunc ship() {}\n", "deliver packages")
	commitFile(t, repo, "policy.json", `{"properties":{"window":{"const":12}}}`, "set dispatch window")
	base := revParse(t, repo, "HEAD")
	commitFile(t, repo, "policy.json", `{"properties":{"window":{"const":24}}}`, "change dispatch window")
	head := revParse(t, repo, "HEAD")
	for _, treeClock := range []bool{false, true} {
		db := filepath.Join(t.TempDir(), "withdrawal.db")
		memory := func(args ...string) []byte {
			t.Helper()
			var out bytes.Buffer
			cmd := newMemoryCommand()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"--repo", repo, "--memory-db", db}, args...))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			return out.Bytes()
		}
		var old []knowledge.Fact
		if err := json.Unmarshal(memory("learn", "policy.json", "--scope", "delivery.go", "--ref", base), &old); err != nil || len(old) != 1 {
			t.Fatalf("learn returned %v: %v", old, err)
		}
		stamp := time.Now().UTC()
		memory("revoke", old[0].ID)
		if treeClock {
			cc, err := loadCommonContext(repo, "", head, 0, 0, true)
			if err != nil {
				t.Fatal(err)
			}
			rt, err := loadLLMRuntime(cc.Template, llm.ResolveOptions{Context: ctx})
			if err != nil {
				t.Fatal(err)
			}
			store, err := openMemory(ctx, db, repo, cc.GitRunner, true)
			if err != nil {
				t.Fatal(err)
			}
			tree := knowledge.Tree{Repo: repo, Git: cc.GitRunner, Commit: head, Now: stamp}
			_, err = syncKnowledge(ctx, store, tree, cc, rt, knowledge.DiscoverOptions{}, nil, nil)
			if closeErr := store.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil {
				t.Fatal(err)
			}
		} else {
			for range 2 {
				memory("sync", "--ref", head, "--as-of", stamp.Format(time.RFC3339Nano))
			}
		}
		var then []knowledge.Checked
		if err := json.Unmarshal(memory("list", "--ref", head, "--as-of", stamp.Format(time.RFC3339Nano)), &then); err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		for _, c := range then {
			values[string(c.Value)] = c.Status
		}
		if values["12"] != knowledge.StatusChanged || values["24"] != knowledge.StatusVerified {
			t.Errorf("tree clock %v: historical window did not refresh: %v", treeClock, values)
		}
		memory("sync", "--ref", head)
		var now []knowledge.Checked
		if err := json.Unmarshal(memory("list", "--ref", head), &now); err != nil || len(now) != 0 {
			t.Fatalf("tree clock %v: historical replay revived a withdrawn mapping: %v, %v", treeClock, now, err)
		}
	}
}

func TestMemorySyncRefreshesOlderDiscussionReceipts(t *testing.T) {
	ctx := t.Context()
	const first = "Queue before closing the batch."
	const second = "Queue only after closing the batch."
	var extractions atomic.Int32
	var reviewedBoth atomic.Bool
	ordinary := newFakeLLM()
	startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if bytes.Contains(body, []byte("Extract repository knowledge")) {
			extractions.Add(1)
			if !bytes.Contains(body, []byte(first)) || !bytes.Contains(body, []byte(second)) || bytes.Contains(body, []byte("proposed*2")) || bytes.Contains(body, []byte("omitted-value-123456")) {
				t.Error("wrong accepted-side discovery corpus")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"sync","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"records\":[]}"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
			return
		}
		if bytes.Contains(body, []byte("An independent code review of this change has finished")) && bytes.Contains(body, []byte(first)) && bytes.Contains(body, []byte(second)) {
			reviewedBoth.Store(true)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		ordinary.ServeHTTP(w, r)
	}))
	repo := initResumeRepo(t)
	const path = "queue.go"
	commitFile(t, repo, path, "package queue\nfunc submit(proposed int) int { return proposed }\n", "open queue")
	base := revParse(t, repo, "HEAD")
	commitFile(t, repo, path, "package queue\nfunc submit(proposed int) int { return proposed*2 }\n", "change queue")
	root := t.TempDir()
	archive := filepath.Join(root, "notes.json")
	cutoff := time.Now().UTC().Truncate(time.Second)
	expiring := knowledge.Discussion{ID: "archived-window", Body: "Keep the original queue window.", UpdatedAt: cutoff.Add(-5 * time.Hour).Format(time.RFC3339), ExpiresAt: cutoff.Add(-4 * time.Hour).Format(time.RFC3339)}
	notes := []knowledge.Discussion{
		{ID: "discussion-1", Path: path, UpdatedAt: cutoff.Add(-3 * time.Hour).Format(time.RFC3339), Body: first},
		{ID: "discussion-2", Path: path, UpdatedAt: cutoff.Add(-2 * time.Hour).Format(time.RFC3339), Body: second},
		{ID: "tomorrow", Path: path, UpdatedAt: cutoff.Add(24 * time.Hour).Format(time.RFC3339), Body: "A future change."},
		expiring,
	}
	b, err := json.Marshal(notes)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, b, 0600); err != nil {
		t.Fatal(err)
	}
	cc, err := loadCommonContext(repo, "", base, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := loadLLMRuntime(cc.Template, llm.ResolveOptions{Context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	pathToDB := filepath.Join(root, "previous.db")
	store, err := openMemory(ctx, pathToDB, repo, cc.GitRunner, true)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce earlier receipts for the same ref, selector and export.
	// Version 2 could omit raw decisions and version 3 did not bind names.
	prior, err := json.Marshal(struct {
		Version, Revision, Provider, Model string
		Live                               bool
		Options                            knowledge.DiscoverOptions
		Sources                            []string
	}{"business-discovery-v2", base, rt.Provider, rt.Model, false, knowledge.DiscoverOptions{Paths: []string{path}, FocusLines: map[string][]int{}, AsOf: cutoff, Discussions: notes, FileFilter: cc.FileFilter}, []string{archive}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDiscovery(ctx, knowledge.Digest(prior), nil); err != nil {
		t.Fatal(err)
	}
	v3 := bytes.Replace(prior, []byte("business-discovery-v2"), []byte("business-discovery-v3"), 1)
	if err := store.SaveDiscovery(ctx, knowledge.Digest(v3), nil); err != nil {
		t.Fatal(err)
	}
	v4 := bytes.Replace(prior, []byte("business-discovery-v2"), []byte("business-discovery-v4"), 1)
	if err := store.SaveDiscovery(ctx, knowledge.Digest(v4), nil); err != nil {
		t.Fatal(err)
	}
	// V5 could save a checked inference from this pathless note without
	// retaining its original record or including the deadline in the rule.
	tree := knowledge.Tree{Repo: repo, Commit: base, Git: cc.GitRunner}
	claim, err := tree.Prepare(ctx, knowledge.Fact{Origin: "discovery", Scope: path, Source: path, Category: knowledge.CategoryBusinessRule, Authority: knowledge.AuthorityInferred, Claim: expiring.Body,
		Citations: []knowledge.Citation{{Kind: "code", Path: path, StartLine: 2, EndLine: 2}, {Kind: "discussion", Reference: expiring.ID, ObservedAt: expiring.UpdatedAt,
			Quote: expiring.Body, Digest: knowledge.Digest([]byte(expiring.Body)), StartLine: 1, EndLine: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	oldRules := []knowledge.Fact{claim}
	v5 := bytes.Replace(prior, []byte("business-discovery-v2"), []byte("business-discovery-v5"), 1)
	if err := store.SaveDiscovery(ctx, knowledge.Digest(v5), oldRules); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", pathToDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.ExecContext(ctx, "DROP TABLE discussion_edits"); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	memory := func(db string, args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		c := newMemoryCommand()
		c.SetOut(&out)
		c.SetArgs(append([]string{"--repo", repo, "--memory-db", db}, args...))
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	args := []string{"sync", "--ref", base, "--path", path, "--discussions", archive, "--as-of", cutoff.Format(time.RFC3339)}
	var before []knowledge.Checked
	if err := json.Unmarshal(memory(pathToDB, "list", "--ref", base, "--as-of", cutoff.Format(time.RFC3339)), &before); err != nil || len(before) != 1 || before[0].Status != knowledge.StatusCandidate || before[0].ExpiresAt != "" {
		t.Fatalf("expected a v5 rule without its deadline before importing the archive: %+v %v", before, err)
	}
	for _, cached := range []bool{false, true} {
		var report memorySyncReport
		if err := json.Unmarshal(memory(pathToDB, args...), &report); err != nil || report.Cached != cached || report.Facts != 0 || !cached && report.DiscussionSources != 2 {
			t.Fatalf("upgrade then idempotent sync: %+v, %v", report, err)
		}
	}
	checkList := func(db string) {
		t.Helper()
		var list []knowledge.Checked
		if err := json.Unmarshal(memory(db, "list", "--ref", base, "--as-of", cutoff.Format(time.RFC3339)), &list); err != nil {
			t.Fatalf("stored sources: %+v, %v", list, err)
		}
		sources, migrated := 0, false
		for _, item := range list {
			if item.ID == oldRules[0].ID {
				if item.ExpiresAt != expiring.ExpiresAt || item.Status != knowledge.StatusExpired {
					t.Fatalf("cached v5 rule survived reimport after expiry: %+v", item)
				}
				migrated = true
				continue
			}
			sources++
			if item.Status != knowledge.StatusDiscussionSource || item.Authority != knowledge.AuthorityHistorical || item.Revision != base || item.Scope != path || len(item.Citations) != 1 || item.Citations[0].Reference == "tomorrow" {
				t.Fatalf("invalid source from CLI: %+v", item)
			}
		}
		if sources != 2 || migrated != (db == pathToDB) {
			t.Fatalf("stored sources or migrated rule: %+v", list)
		}
	}
	checkList(pathToDB)
	if err := json.Unmarshal(memory(pathToDB, "list", "--ref", base, "--as-of", expiring.UpdatedAt), &before); err != nil || len(before) != 1 || before[0].ID != oldRules[0].ID || before[0].Status != knowledge.StatusCandidate || before[0].ExpiresAt != expiring.ExpiresAt {
		t.Fatalf("import did not preserve the old ID and pre-expiry availability: %+v %v", before, err)
	}
	fresh := filepath.Join(root, "new-review.db")
	// A prior regular review had the same source bytes while the second
	// note was still future-dated. Replaying that receipt now must sync.
	priorLive, err := json.Marshal(struct {
		Version, Revision, Provider, Model string
		Live                               bool
		Options                            knowledge.DiscoverOptions
		Sources                            []string
		NextEdit                           string `json:",omitempty"`
	}{"business-discovery-v6", base, rt.Provider, rt.Model, true, knowledge.DiscoverOptions{Paths: []string{path}, FocusLines: map[string][]int{path: {2}}, Discussions: notes, FileFilter: cc.FileFilter}, []string{archive}, notes[1].UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	liveStore, err := openMemory(ctx, fresh, repo, cc.GitRunner, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := liveStore.SaveDiscovery(ctx, knowledge.Digest(priorLive), nil); err != nil {
		t.Fatal(err)
	}
	if err := liveStore.Close(); err != nil {
		t.Fatal(err)
	}
	review := []string{"--repo", repo, "--commit", "HEAD", "--memory-sync", "--memory-discussions", archive,
		"--memory-db", fresh, "--audience", "agent", "--format", "json", "--output", filepath.Join(root, "review.json")}
	if err := runReview(review); err != nil {
		t.Fatal(err)
	}
	checkList(fresh)
	liveStore, err = openMemory(ctx, fresh, repo, cc.GitRunner, false)
	if err != nil {
		t.Fatal(err)
	}
	defer liveStore.Close()
	var receipt map[string]json.RawMessage
	if err := json.Unmarshal(priorLive, &receipt); err != nil {
		t.Fatal(err)
	}
	// Keep the original JSON field order while substituting the next actual
	// unobserved timestamp in the same regular review's receipt.
	newLive := bytes.Replace(priorLive, append([]byte(`"NextEdit":"`), notes[1].UpdatedAt...), append([]byte(`"NextEdit":"`), notes[2].UpdatedAt...), 1)
	if seen, err := liveStore.Scanned(ctx, knowledge.Digest(newLive)); err != nil || !seen || len(receipt["NextEdit"]) == 0 {
		t.Fatalf("live receipt failed to advance to the remaining future edition: %t, %v", seen, err)
	}
	if extractions.Load() != 2 || !reviewedBoth.Load() {
		t.Fatalf("ordinary review did not receive both independent original notes after empty extraction: %d extractions, visible=%t", extractions.Load(), reviewedBoth.Load())
	}
	t.Run("v6 scoped pathless update", func(t *testing.T) {
		old := knowledge.Discussion{ID: "dispatch-slot", Body: "Dispatch after the slot opens.", UpdatedAt: notes[0].UpdatedAt}
		edit := old
		edit.Body, edit.UpdatedAt = "api_key = 'omitted-value-123456'", notes[1].UpdatedAt
		historical := knowledge.DiscoverOptions{Scope: path, AsOf: cutoff, Discussions: []knowledge.Discussion{old}, FileFilter: cc.FileFilter}
		original, err := tree.Discover(ctx, historical, func(context.Context, string, string) (string, error) { return `{"records":[]}`, nil })
		if err != nil || len(original.DiscussionSources) != 1 {
			t.Fatalf("missing original binding: %+v, %v", original, err)
		}
		historical.Discussions = append([]knowledge.Discussion{notes[0], notes[1], old}, edit)
		historical.FocusLines = map[string][]int{}
		scopeArchive := filepath.Join(root, "scoped-notes.json")
		data, err := json.Marshal(historical.Discussions)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(scopeArchive, data, 0600); err != nil {
			t.Fatal(err)
		}
		v6, err := json.Marshal(struct {
			Version, Revision, Provider, Model string
			Live                               bool
			Options                            knowledge.DiscoverOptions
			Sources                            []string
		}{"business-discovery-v6", base, rt.Provider, rt.Model, false, historical, []string{scopeArchive}})
		if err != nil {
			t.Fatal(err)
		}
		database := filepath.Join(root, "v6-scoped.db")
		oldStore, err := openMemory(ctx, database, repo, cc.GitRunner, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := oldStore.SaveDiscovery(ctx, knowledge.Digest(v6), original.DiscussionObservations()); err != nil {
			t.Fatal(err)
		}
		if err := oldStore.Close(); err != nil {
			t.Fatal(err)
		}
		for _, cached := range []bool{false, true} {
			var report memorySyncReport
			if err := json.Unmarshal(memory(database, "sync", "--ref", base, "--scope", path, "--discussions", scopeArchive, "--as-of", cutoff.Format(time.RFC3339)), &report); err != nil || report.Cached != cached {
				t.Fatalf("v6 replay cached=%t: %+v, %v", cached, report, err)
			}
		}
		upgraded, err := openMemory(ctx, database, repo, cc.GitRunner, false)
		if err != nil {
			t.Fatal(err)
		}
		v7 := bytes.Replace(v6, []byte("business-discovery-v6"), []byte("business-discovery-v7"), 1)
		if found, err := upgraded.Scanned(ctx, knowledge.Digest(v7)); err != nil || !found {
			t.Fatalf("upgraded receipt did not match the original inputs: %t, %v", found, err)
		}
		if err := upgraded.Close(); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			at     string
			active bool
		}{{old.UpdatedAt, true}, {edit.UpdatedAt, false}} {
			records := memory(database, "list", "--ref", base, "--as-of", tc.at)
			if bytes.Contains(records, []byte(old.Body)) != tc.active || bytes.Contains(records, []byte("omitted-value")) {
				t.Fatalf("v6 source after upgrade at %s: %s", tc.at, records)
			}
		}
	})
}

func TestLiveDiscussionReceiptChangesAtVisibilityBoundary(t *testing.T) {
	at := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	middle, last := at.Add(time.Hour), at.Add(2*time.Hour)
	notes := []knowledge.Discussion{{UpdatedAt: last.Format(time.RFC3339Nano)}, {CreatedAt: middle.Format(time.RFC3339Nano)}, {UpdatedAt: at.Format(time.RFC3339Nano)}}
	if nextDiscussionEdit(notes, at) != middle.Format(time.RFC3339Nano) || nextDiscussionEdit(notes, middle.Add(-time.Nanosecond)) != middle.Format(time.RFC3339Nano) ||
		nextDiscussionEdit(notes, middle) != last.Format(time.RFC3339Nano) || nextDiscussionEdit(notes, last) != "" {
		t.Fatal("live receipt changed between scheduled edits or stayed valid across a newly admissible observation")
	}
}

func TestStandaloneDiscussionReceiptFollowsCheckout(t *testing.T) {
	startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sync","type":"message","role":"assistant","model":"test","content":[{"type":"text","text":"{\"records\":[]}"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	repo := initResumeRepo(t)
	t.Setenv("GIT_COMMITTER_DATE", "2024-01-02T10:00:00Z")
	commitFile(t, repo, "queue.go", "package queue\nfunc submit() {}\n", "open queue")
	base := revParse(t, repo, "HEAD")
	t.Setenv("GIT_COMMITTER_DATE", "2024-01-05T10:00:00Z")
	commitFile(t, repo, "next.go", "package queue\n", "next revision")
	dir := t.TempDir()
	archive := filepath.Join(dir, "discussions.json")
	if err := os.WriteFile(archive, []byte(`[{"id":"dispatch","path":"queue.go","body":"Queue a parcel before departure.","updated_at":"2024-01-03T10:00:00Z"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	sync := func() memorySyncReport {
		t.Helper()
		c := newMemoryCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetArgs([]string{"--repo", repo, "--memory-db", filepath.Join(dir, "db"), "sync", "--ref", base, "--discussions", archive})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		var report memorySyncReport
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	if got := sync(); got.Cached || got.DiscussionSources != 0 {
		t.Fatalf("later decision entered a historical sync: %+v", got)
	}
	gitIn(t, repo, "checkout", "--detach", base)
	if got := sync(); got.Cached || got.DiscussionSources != 1 {
		t.Fatalf("historical receipt hid the now eligible decision at HEAD: %+v", got)
	}
	if got := sync(); !got.Cached {
		t.Fatal("identical live sync did not reuse its own receipt")
	}
}

func TestMemorySyncRefreshAndReviewBase(t *testing.T) {
	const accepted = "return cents"
	const proposed = "return cents * 100"
	var extractions atomic.Int32
	ordinaryReview := newFakeLLM()
	startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if !bytes.Contains(body, []byte("Extract repository knowledge")) {
			r.Body = io.NopCloser(bytes.NewReader(body))
			ordinaryReview.ServeHTTP(w, r)
			return
		}
		extractions.Add(1)
		if bytes.Contains(body, []byte(proposed)) || !bytes.Contains(body, []byte(accepted)) {
			t.Error("extraction should see the accepted base implementation only")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sync","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"records\":[]}"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	repo := initResumeRepo(t)
	commitFile(t, repo, "pay.go", "package pay\nfunc units(cents int) int { "+accepted+" }\n", "establish payment units")
	commitFile(t, repo, "billing.json", `{"properties":{"unit":{"const":"cents"}}}`, "define the contract")
	db := filepath.Join(t.TempDir(), "memory.db")
	memory := func(args ...string) []byte {
		t.Helper()
		var b bytes.Buffer
		c := newMemoryCommand()
		c.SetOut(&b)
		c.SetArgs(append([]string{"--repo", repo, "--memory-db", db}, args...))
		if err := c.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return b.Bytes()
	}
	memory("learn", "billing.json", "--scope", "pay.go")
	commitFile(t, repo, "billing.json", `{"properties":{"unit":{"const":"mills"}}}`, "accept a different unit")
	base := revParse(t, repo, "HEAD")
	commitFile(t, repo, "pay.go", "package pay\nfunc units(cents int) int { "+proposed+" }\n", "propose conversion")
	syncArgs := []string{"sync", "--ref", base, "--at", "pay.go:2"}
	for _, cached := range []bool{false, true} {
		var report memorySyncReport
		if err := json.Unmarshal(memory(syncArgs...), &report); err != nil || report.Cached != cached {
			t.Fatalf("sync report %+v: %v", report, err)
		}
	}
	if extractions.Load() != 1 {
		t.Fatalf("expected one extraction for an identical input, got %d", extractions.Load())
	}
	var records []knowledge.Checked
	if err := json.Unmarshal(memory("list", "--ref", base), &records); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, r := range records {
		values[string(r.Value)] = r.Status
	}
	if values[`"mills"`] != "verified" || values[`"cents"`] != "changed" {
		t.Fatalf("schema was not refreshed at the accepted revision: %v", values)
	}
	out := filepath.Join(t.TempDir(), "review.json")
	if err := runReview([]string{"--repo", repo, "--commit", "HEAD", "--memory-db", db, "--memory-sync", "--max-tokens-budget", "1", "--output", out, "--format", "json", "--audience", "agent"}); err == nil {
		t.Fatal("one token cannot cover the selected review")
	}
	report, err := os.ReadFile(out)
	if err != nil || !bytes.Contains(report, []byte(`"budget_exceeded": true`)) {
		t.Fatalf("expected the review engine's budget report: %s, %v", report, err)
	}
	if extractions.Load() != 1 {
		t.Fatal("an unaffordable extraction was sent")
	}
	if err := runReview([]string{"--repo", repo, "--commit", "HEAD", "--memory-db", db, "--memory-sync", "--output", out, "--format", "json", "--audience", "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil || extractions.Load() != 2 {
		t.Fatalf("normal review: %v, %d extractions", err, extractions.Load())
	}
}

func TestMemorySyncPersistsTheDateOfAnUnavailableEdit(t *testing.T) {
	startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sync","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"records\":[]}"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	repo := initResumeRepo(t)
	commitFile(t, repo, "queue.go", "package queue\nfunc Next() int { return 1 }\n", "create queue")
	commitFile(t, repo, "report.go", "package queue\nfunc Count() int { return 2 }\n", "add reports")
	dir := t.TempDir()
	db, archive := filepath.Join(dir, "knowledge.db"), filepath.Join(dir, "notes.json")
	call := func(args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		cmd := newMemoryCommand()
		cmd.SetOut(&out)
		cmd.SetArgs(append([]string{"--repo", repo, "--memory-db", db}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	first := knowledge.Discussion{ID: "note-5", Path: "queue.go", Body: "The queue is FIFO.", UpdatedAt: "2025-02-01T01:00:00Z"}
	edit := first
	edit.Path, edit.Body, edit.UpdatedAt = "missing.py", "New external-only policy.", "2025-03-01T01:00:00Z"
	for _, d := range []knowledge.Discussion{first, edit} {
		raw, _ := json.Marshal([]knowledge.Discussion{d})
		if err := os.WriteFile(archive, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if d == edit {
			call("sync", "--scope", "report.go", "--discussions", archive, "--as-of", edit.UpdatedAt)
			var kept []knowledge.Checked
			if err := json.Unmarshal(call("list", "--as-of", edit.UpdatedAt), &kept); err != nil || len(kept) != 1 || kept[0].DiscussionBody != first.Body {
				t.Fatalf("an unrelated scoped sync retired the original queue source: %+v, %v", kept, err)
			}
		}
		call("sync", "--scope", "queue.go", "--discussions", archive, "--as-of", edit.UpdatedAt)
	}
	for _, tc := range []struct {
		when string
		n    int
	}{{first.UpdatedAt, 1}, {edit.UpdatedAt, 0}} {
		var got []knowledge.Checked
		if err := json.Unmarshal(call("list", "--as-of", tc.when), &got); err != nil || len(got) != tc.n {
			t.Fatalf("sources at %s: %+v %v", tc.when, got, err)
		}
	}
}

func TestMemorySyncFlags(t *testing.T) {
	for _, args := range [][]string{{"--memory-sync"}, {"--memory-discussions", "comments.json"}, {"--memory-as-of", "2025-01-02T10:00:00Z"}} {
		if _, err := parseReviewFlags(args); err == nil || !strings.Contains(err.Error(), "requires") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if opts, err := parseReviewFlags([]string{"--memory-db", "sample.db", "--memory-as-of", "2025-01-02T10:00:00Z"}); err != nil || opts.memoryTime.IsZero() {
		t.Fatalf("valid review cutoff: %v", err)
	}
	if _, err := parseReviewFlags([]string{"--memory-db", "sample.db", "--memory-as-of", "yesterday"}); err == nil {
		t.Fatal("accepted an ambiguous replay date")
	}
	for _, lines := range []string{"x", "-2", "2-1", "0", "5-7-9"} {
		if _, _, err := parseEvidenceLines(lines); err == nil {
			t.Errorf("accepted %q", lines)
		}
	}
	for _, lines := range []string{"", "4", "4-6"} {
		if _, _, err := parseEvidenceLines(lines); err != nil {
			t.Errorf("%q: %v", lines, err)
		}
	}
}

func TestMemoryFeedbackAndTimeLimitedDecisionCommands(t *testing.T) {
	startFakeLLM(t, newFakeLLM())
	repo := initResumeRepo(t)
	commitFile(t, repo, "money.json", `{"properties":{"unit":{"const":"cents"}}}`, "define units")
	commitFile(t, repo, "pay.go", "package pay\nfunc process(cents int) int { return cents }\n", "process cents")
	db := filepath.Join(t.TempDir(), "rules.db")
	call := func(args ...string) ([]byte, error) {
		c := newMemoryCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs(append([]string{"--repo", repo, "--memory-db", db}, args...))
		err := c.Execute()
		return out.Bytes(), err
	}
	out, err := call("learn", "money.json", "--scope", "pay.go")
	var facts []knowledge.Fact
	if err != nil || json.Unmarshal(out, &facts) != nil || len(facts) != 1 {
		t.Fatalf("learn: %s, %v", out, err)
	}
	id := facts[0].ID
	cutoff := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := call("feedback", id, "--action", "reject", "--reason", "wrong target for the contract"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		flags  []string
		status string
	}{
		{nil, knowledge.StatusRejected}, {[]string{"--as-of", cutoff}, knowledge.StatusVerified},
	} {
		out, err := call(append([]string{"list"}, tc.flags...)...)
		var checked []knowledge.Checked
		if err != nil || json.Unmarshal(out, &checked) != nil || len(checked) != 1 || checked[0].Status != tc.status {
			t.Fatalf("list %v: %s, %v", tc.flags, out, err)
		}
	}
	args := []string{"decide", "pay.go", "--scope", "pay.go", "--review-id", "refund", "--finding", "convert units", "--reason", "already in cents"}
	if _, err := call("revoke", id); err != nil {
		t.Fatal(err)
	}
	out, err = call("list", "--as-of", cutoff, "--search", "cents")
	var prior []knowledge.Checked
	if err != nil || json.Unmarshal(out, &prior) != nil || len(prior) != 1 || prior[0].ID != id || prior[0].Status != knowledge.StatusVerified {
		t.Fatalf("a later withdrawal hid the original verified mapping at the earlier cutoff: %s, %v", out, err)
	}
	if _, err := call(append(args, "--evidence-lines", "2", "--expires-at", "2001-01-01T00:00:00Z")...); err != nil {
		t.Fatal(err)
	}
	out, err = call("list", "--review-id", "refund")
	if err != nil || !bytes.Contains(out, []byte(`"status":"expired"`)) {
		t.Fatalf("expired decision: %s, %v", out, err)
	}
	for _, flags := range [][]string{
		{"list", "--as-of", "tomorrow"}, {"feedback", id, "--action", "replace", "--reason", "no replacement"},
		append(args, "--evidence-lines", "9-3"), append(args, "--expires-at", "in a week"),
		{"sync", "--at", "pay.go:0"}, {"sync", "--as-of", "yesterday"},
		{"sync", "--discussions", filepath.Join(t.TempDir(), "missing.json")},
		{"learn", "missing.json", "--scope", "pay.go"}, {"list", "--ref", "missing-oid"},
	} {
		if _, err := call(flags...); err == nil {
			t.Errorf("unexpected success for %v", flags)
		}
	}
}
