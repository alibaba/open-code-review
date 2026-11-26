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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/knowledge"
)

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
