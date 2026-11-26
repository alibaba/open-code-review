// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/knowledge"
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
	if got := records("--review-id", "MR-43"); len(got) != 2 || got[0].ReviewID != "MR-43" || got[0].Status != "verified" {
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
	tracePath, err := session.SessionFilePath(repo, report.SessionID)
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
