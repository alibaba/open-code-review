// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestReviewFindingFilterFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--min-severity", "urgent"},
		{"--min-severity", "high,low"},
		{"--exclude-categories", "style,typo"},
	} {
		if _, err := parseReviewFlags(args); err == nil {
			t.Errorf("expected rejection for %v", args)
		}
	}
	opts, err := parseReviewFlags([]string{"--min-severity", " HIGH ", "--exclude-categories", " Style, TEST ,style ", "--no-filter"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.minSeverity != "high" || opts.excludeCategories != "style,test,style" || !opts.noFilter {
		t.Fatalf("filters not parsed and normalized: %+v", opts)
	}
}

func TestFilterReviewComments(t *testing.T) {
	comments := []model.LlmComment{
		{Content: "low", Severity: "low", Category: "bug"},
		{Content: "medium", Severity: "medium", Category: "bug"},
		{Content: "high", Severity: "high", Category: "bug"},
		{Content: "critical", Severity: "critical", Category: "security"},
		{Content: "style", Severity: "high", Category: "style"},
		{Content: "test", Severity: "medium", Category: "test"},
		{Content: "other", Severity: "low", Category: "other"},
		{Content: "unknown severity", Severity: "future", Category: "style"},
		{Content: "unknown category", Severity: "low", Category: "future"},
		{Content: "missing severity", Category: "style"},
		{Content: "missing category", Severity: "low"},
	}
	original := append([]model.LlmComment(nil), comments...)
	unknown := []string{"unknown severity", "unknown category", "missing severity", "missing category"}
	for _, tc := range []struct {
		name, minimum, excluded string
		want                    []string
	}{
		{"disabled", "", "", []string{"low", "medium", "high", "critical", "style", "test", "other"}},
		{"low", "low", "", []string{"low", "medium", "high", "critical", "style", "test", "other"}},
		{"medium", "medium", "", []string{"medium", "high", "critical", "style", "test"}},
		{"high", "high", "", []string{"high", "critical", "style"}},
		{"critical", "critical", "", []string{"critical"}},
		{"categories", "", "style,test,other", []string{"low", "medium", "high", "critical"}},
		{"combined", "high", "style,test,other", []string{"high", "critical"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := filterReviewComments(comments, reviewOptions{minSeverity: tc.minimum, excludeCategories: tc.excluded})
			var contents []string
			for _, c := range got {
				contents = append(contents, c.Content)
			}
			want := append(tc.want, unknown...)
			if !reflect.DeepEqual(contents, want) {
				t.Fatalf("got %v, want %v", contents, want)
			}
			if !reflect.DeepEqual(comments, original) {
				t.Fatal("session comments were mutated")
			}
		})
	}
	if got := filterReviewComments(nil, reviewOptions{}); got != nil {
		t.Fatalf("disabled nil = %v", got)
	}
	if got := filterReviewComments(comments[:1], reviewOptions{minSeverity: "high"}); len(got) != 0 {
		t.Fatalf("all filtered = %v", got)
	}
	mixed := []model.LlmComment{{Severity: " HIGH ", Category: " Style "}}
	if got := filterReviewComments(mixed, reviewOptions{excludeCategories: "style"}); len(got) != 0 {
		t.Fatalf("metadata should be normalized for comparison: %v", got)
	}
}

func TestReviewFindingFiltersKeepUnknownParsedMetadata(t *testing.T) {
	for _, metadata := range []map[string]any{
		{"severity": "urgent", "category": "style"},
		{"severity": "low", "category": "correctness"},
		{"severity": 42, "category": "style"},
		{"severity": "low", "category": nil},
		{},
	} {
		metadata["path"], metadata["content"] = "a.go", "keep unknown metadata"
		comments, msg := tool.ParseComments(map[string]any{"comments": []any{metadata}})
		if msg != "" {
			t.Fatal(msg)
		}
		got := filterReviewComments(comments, reviewOptions{minSeverity: "critical", excludeCategories: "style,other"})
		if len(got) != 1 {
			t.Fatalf("unknown metadata dropped: %v", metadata)
		}
	}
}

func TestReviewFindingFiltersEndToEnd(t *testing.T) {
	// Git expands Windows short paths; use the same repository key as the CLI
	// when reading the saved session directly below.
	repo, err := resolveRepoDir(retryTestRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeLLM()
	startFakeLLM(t, fake)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if bytes.Contains(body, []byte(`"tools"`)) && bytes.Contains(body, []byte("MARKER_ALPHA")) && !bytes.Contains(body, []byte(`"tool_result"`)) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"msg_comments","type":"message","role":"assistant","model":"claude-test",
                "content":[{"type":"tool_use","id":"comments_1","name":"code_comment","input":{"comments":[
                    {"path":"a.go","content":"KEEP_HIGH_BUG","severity":"high","category":"bug","existing_code":"return 2"},
                    {"path":"a.go","content":"DROP_LOW_BUG","severity":"low","category":"bug","existing_code":"return 2"},
                    {"path":"a.go","content":"DROP_HIGH_STYLE","severity":"high","category":"style","existing_code":"return 2"},
                    {"path":"a.go","content":"KEEP_UNKNOWN","severity":"urgent","category":"style","existing_code":"return 2"}
                ]}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`)
			return
		}
		fake.ServeHTTP(w, r)
	}))
	defer server.Close()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
	for _, format := range []string{"json", "text", "sarif"} {
		t.Run(format, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "report."+format)
			args := []string{"--repo", repo, "--from", "HEAD~1", "--to", "HEAD", "--format", format,
				"--audience", "agent", "--output", output, "--no-filter", "--effort", "low",
				"--min-severity", "high", "--exclude-categories", "style"}
			if err := runReview(args); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"KEEP_HIGH_BUG", "KEEP_UNKNOWN"} {
				if !strings.Contains(string(data), want) {
					t.Errorf("missing %s in %s", want, data)
				}
			}
			if strings.Contains(string(data), "DROP_") {
				t.Fatalf("excluded finding in output: %s", data)
			}
			if format == "json" {
				var report jsonOutput
				if err := json.Unmarshal(data, &report); err != nil {
					t.Fatal(err)
				}
				if len(report.Comments) != 2 || report.Summary.Comments != 2 {
					t.Fatalf("wrong comment count: %s", data)
				}
				if report.Manifest.TerminalState != session.StateComplete || report.Summary.FilesReviewed != 4 {
					t.Fatalf("filter changed coverage: %s", data)
				}
				saved, err := session.LoadComments(repo, report.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if len(saved) != 4 {
					t.Fatalf("session lost findings: %v", saved)
				}
				// A resume can recover findings hidden by the previous run's policy.
				resumeOutput := filepath.Join(t.TempDir(), "resume.json")
				if err := runReview([]string{"--repo", repo, "--from", "HEAD~1", "--to", "HEAD",
					"--resume", report.SessionID, "--no-filter", "--effort", "low",
					"--format", "json", "--audience", "agent", "--output", resumeOutput}); err != nil {
					t.Fatal(err)
				}
				resumed, err := os.ReadFile(resumeOutput)
				if err != nil {
					t.Fatal(err)
				}
				var resumedReport jsonOutput
				if err := json.Unmarshal(resumed, &resumedReport); err != nil {
					t.Fatal(err)
				}
				if len(resumedReport.Comments) != 4 || len(resumedReport.Manifest.Coverage.Reused) != 4 {
					t.Fatalf("resume did not recover filtered findings: %s", resumed)
				}
			}
		})
	}
	t.Run("partial", func(t *testing.T) {
		fake.mu.Lock()
		fake.hardFail["b.go"] = true
		fake.mu.Unlock()
		output := filepath.Join(t.TempDir(), "partial.json")
		if err := runReview([]string{"--repo", repo, "--from", "HEAD~1", "--to", "HEAD",
			"--format", "json", "--audience", "agent", "--output", output,
			"--no-filter", "--effort", "low", "--min-severity", "high", "--exclude-categories", "style"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		var report jsonOutput
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if len(report.Comments) != 2 || report.Summary.Comments != 2 || report.Manifest.TerminalState != session.StatePartial {
			t.Fatalf("partial review lost filtered findings or coverage: %s", data)
		}
	})

}
