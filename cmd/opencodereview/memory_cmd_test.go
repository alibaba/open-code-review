// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestMemoryAcrossReviews(t *testing.T) {
	startFakeLLM(t, newFakeLLM())
	repo := initResumeRepo(t)
	database := filepath.Join(t.TempDir(), "knowledge with space.db")
	schema := `{"properties":{"unit":{"const":"cent"}}}`
	commitFile(t, repo, "money.json", schema, "define amount unit")
	base := revParse(t, repo, "HEAD")
	commitFile(t, repo, "main.go", "package main\n// MARKER_ALPHA\nfunc main() {}\n", "use amount")
	run := func(args ...string) []byte {
		t.Helper()
		c := newMemoryCommand()
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs(append([]string{"--memory-db", database, "--repo", repo}, args...))
		if err := c.Execute(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out.String())
		}
		return out.Bytes()
	}
	learn := []string{"learn", "money.json", "--scope", "main.go"}
	var facts []knowledge.Fact
	if err := json.Unmarshal(run(learn...), &facts); err != nil || len(facts) != 1 {
		t.Fatalf("learn: %v %v", facts, err)
	}
	id := facts[0].ID
	run("decide", "money.json", "--scope", "main.go", "--review-id", "MR-43", "--finding", "multiply the amount", "--reason", "contract already uses cents")
	records := func(opts ...string) []knowledge.Checked {
		t.Helper()
		var results []knowledge.Checked
		if err := json.Unmarshal(run(append([]string{"list"}, opts...)...), &results); err != nil {
			t.Fatal(err)
		}
		return results
	}
	if got := records("--review-id", "other", "--search", "cent"); len(got) != 1 || got[0].ID != id || got[0].Status != "verified" {
		t.Fatalf("scope or search: %+v", got)
	}
	if got := records("--review-id", "MR-43"); len(got) != 2 || got[0].ReviewID != "MR-43" || got[0].Status != "historical" {
		t.Fatalf("decision: %+v", got)
	}
	if got := records("--ref", base); len(got) != 1 || got[0].Status != "unverified" {
		t.Fatalf("evidence from future: %+v", got)
	}

	out := filepath.Join(t.TempDir(), "review.json")
	args := []string{"--repo", repo, "--commit", "HEAD", "--memory-db", database, "--review-id", "MR-43", "--audience", "agent", "--format", "json", "--output", out}
	if err := runReview(args); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var report jsonOutput
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	resolvedRepo, err := resolveRepoDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	tracePath, err := session.SessionFilePath(resolvedRepo, report.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	trace, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(trace, []byte(id)) || !bytes.Contains(trace, []byte("contract already uses cents")) {
		t.Fatal("normal review did not record verified evidence in the model request")
	}

	run("revoke", id)
	if err = runReview(append(args, "--resume", report.SessionID)); err == nil || !strings.Contains(err.Error(), "rule identity") {
		t.Fatalf("resume after revocation: %v", err)
	}
	run(learn...)
	if got := records(); len(got) != 0 {
		t.Fatalf("re-import revived revoked fact: %+v", got)
	}
	commitFile(t, repo, "main.go", "package main\nfunc main() { println(1) }\n", "different code")
	run(learn...)
	if got := records(); len(got) != 0 {
		t.Fatalf("a new commit revived an unchanged revoked constraint: %+v", got)
	}
	if got := records("--review-id", "MR-43"); len(got) != 1 || got[0].Detail != "reviewed file changed" {
		t.Fatalf("changed code: %+v", got)
	}
	commitFile(t, repo, "money.json", strings.Replace(schema, "cent", "dollar", 1), "change contract")
	if got := records("--review-id", "MR-43"); len(got) != 1 || got[0].Status != "changed" {
		t.Fatalf("changed rule: %+v", got)
	}
}

func TestMemoryLateReviewPreservesIndependentFindings(t *testing.T) {
	const (
		doc       = "queue.json"
		spec      = "queue-schema.json"
		unitLine  = `"unit":  "minutes",`
		channel   = `"channel": "broadcast",`
		workers   = `"workers": 0`
		badDraft  = "The unit is seconds and will mismeasure expiry."
		ordinary  = "A zero worker count leaves the queue without a consumer."
		violation = "Broadcast delivery bypasses the contract's local queue."
	)
	repo := initResumeRepo(t)
	commitFile(t, repo, spec, `{"properties":{"unit":{"const":"minutes"},"channel":{"const":"local"}}}`, "specify queue settings")
	commitFile(t, repo, doc, "{\n  \"unit\": \"minutes\",\n  \"channel\": \"local\",\n  \"workers\": 1\n}\n", "configure queue")
	db := filepath.Join(t.TempDir(), "review.db")
	var learned bytes.Buffer
	cmd := newMemoryCommand()
	cmd.SetOut(&learned)
	cmd.SetArgs([]string{"--repo", repo, "--memory-db", db, "learn", spec, "--scope", doc})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var facts []knowledge.Fact
	if err := json.Unmarshal(learned.Bytes(), &facts); err != nil || len(facts) != 2 {
		t.Fatalf("learn queue fields: %s, %v", learned.Bytes(), err)
	}
	rule := make(map[string]string)
	for _, f := range facts {
		rule[f.Property] = f.ID
	}
	commitFile(t, repo, doc, "{\n  "+unitLine+"\n  "+channel+"\n  "+workers+"\n}\n", "change queue")
	drafts := []model.LlmComment{
		{Path: doc, Content: badDraft, ExistingCode: unitLine, Category: "bug", Severity: "medium"},
		{Path: doc, Content: ordinary, ExistingCode: workers, Category: "bug", Severity: "high"},
	}
	checks, err := json.Marshal(map[string]any{"checks": []map[string]any{
		{"id": rule["channel"], "outcome": "violation", "path": doc, "line": 3, "quote": channel, "severity": "high", "reason": violation},
		{"id": rule["unit"], "outcome": "refuted", "path": doc, "line": 2, "quote": unitLine, "candidate": "c-0",
			"refuted_text": badDraft, "reason": "The current unit is the contract's accepted minutes."},
		{"id": rule["unit"], "outcome": "refuted", "path": doc, "line": 4, "quote": workers, "candidate": "c-1",
			"refuted_text": ordinary, "reason": "The accepted unit does not address the worker count."},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		noFilter bool
	}{
		{name: "normal"},
		{name: "no-filter", noFilter: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var businessCalls, ordinaryCalls, reassessments atomic.Int32
			fallback := newFakeLLM()
			startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Error(err)
					return
				}
				send := func(stop string, block map[string]any) {
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(map[string]any{
						"id": "queue_review", "type": "message", "role": "assistant", "model": "claude-test",
						"content": []any{block}, "stop_reason": stop, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5},
					}); err != nil {
						t.Error(err)
					}
				}
				sendTool := func(name string, input map[string]any) {
					send("tool_use", map[string]any{"type": "tool_use", "id": "call_" + name, "name": name, "input": input})
				}
				contains := func(s string) bool { return bytes.Contains(body, []byte(s)) }
				switch {
				case contains("Original findings:"):
					reassessments.Add(1)
					if tc.noFilter || !contains(badDraft) || contains(ordinary) {
						t.Error("independent reassessment received the wrong candidates")
					}
					verdict, err := json.Marshal(map[string]any{"checks": []map[string]any{{
						"id": rule["unit"], "outcome": "refuted", "path": doc, "line": 2, "quote": unitLine,
						"candidate": "c-0", "refuted_text": badDraft, "trigger": "The unit is read.", "effect": "Seconds would mismeasure expiry.",
						"reason": "The reviewed field contains minutes, as required by the current schema.",
					}}})
					if err != nil {
						t.Error(err)
					}
					send("end_turn", map[string]any{"type": "text", "text": string(verdict)})
				case contains("Provisional findings:"):
					businessCalls.Add(1)
					if !contains(badDraft) || !contains(ordinary) || !contains(rule["unit"]) || !contains(rule["channel"]) {
						t.Error("late review didn't receive both ordinary drafts and matching knowledge")
					}
					send("end_turn", map[string]any{"type": "text", "text": string(checks)})
				case contains(`"name":"approve_all_comments"`):
					if tc.noFilter {
						t.Error("ordinary review filter ran with --no-filter")
					}
					sendTool("approve_all_comments", map[string]any{})
				case contains(`"name":"code_comment"`):
					if contains(rule["unit"]) || contains(rule["channel"]) {
						t.Error("business evidence reached the ordinary reviewer")
					}
					if contains(`"type":"tool_result"`) {
						sendTool("task_done", map[string]any{"state": "DONE"})
					} else {
						ordinaryCalls.Add(1)
						sendTool("code_comment", map[string]any{"comments": drafts})
					}
				default:
					req.Body = io.NopCloser(bytes.NewReader(body))
					fallback.ServeHTTP(w, req)
				}
			}))
			flags := []string{"--memory-db", db, "--concurrency", "1", "--effort", "low", "--audience", "agent"}
			if tc.noFilter {
				flags = append(flags, "--no-filter")
			}
			out, stderr, err := runReviewCapturingBoth(t, repo, "json", flags...)
			if err != nil {
				t.Fatalf("review: %v\n%s", err, stderr)
			}
			var report jsonOutput
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatalf("review output: %v\n%s", err, out)
			}
			has := func(line int, text string) bool {
				return slices.ContainsFunc(report.Comments, func(c model.LlmComment) bool {
					return c.Path == doc && c.StartLine == line && strings.Contains(c.Content, text)
				})
			}
			wantCount := 2
			if tc.noFilter {
				wantCount++
			}
			wantReassessments := int32(1)
			if tc.noFilter {
				wantReassessments = 0
			}
			if ordinaryCalls.Load() == 0 || businessCalls.Load() != 1 || reassessments.Load() != wantReassessments || len(report.Comments) != wantCount ||
				!has(4, ordinary) || !has(3, violation) || has(2, badDraft) != tc.noFilter {
				t.Fatalf("unexpected report after %d ordinary and %d late requests: %+v", ordinaryCalls.Load(), businessCalls.Load(), report.Comments)
			}
		})
	}
}
