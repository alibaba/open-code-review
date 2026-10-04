// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestRun_PromptOverflowRecordsFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		systemWords   int
		content       string
		withSmallFile bool
	}{
		{name: "all files overflow", systemWords: 100, content: "package main\n"},
		{name: "one file completes", systemWords: 20, content: "package main\n// " + strings.Repeat("test", 70) + "\n", withSmallFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initTestRepo(t)
			writeFile(t, repo, "overflow.go", []byte(tc.content))
			if tc.withSmallFile {
				writeFile(t, repo, "small.go", []byte("package main\n"))
			}
			gitCommit(t, repo, "init")
			tpl := budgetTestTemplate()
			tpl.MaxTokens = 100
			tpl.MainTask.Messages = []template.ChatMessage{
				{Role: "system", Content: strings.Repeat("review ", tc.systemWords)},
				{Role: "user", Content: "review {{file_content}}"},
			}
			client := &fakeBudgetClient{}
			a := NewAgent(Args{
				RepoDir:        repo,
				Template:       tpl,
				LLMClient:      client,
				MaxConcurrency: 1,
				SkipPlan:       true,
				SkipDedup:      true,
				SkipSummary:    true,
				Session:        session.New(repo, "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
			})
			_, err := a.Run(context.Background())
			wantCalls := int64(0)
			if tc.withSmallFile {
				wantCalls = 1
				if err != nil {
					t.Fatalf("mixed scan: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "all 1 file scan(s) failed") {
				t.Fatalf("expected all-files-failed error, got %v", err)
			}
			if calls := atomic.LoadInt64(&client.calls); calls != wantCalls {
				t.Fatalf("MAIN_TASK calls = %d, want %d", calls, wantCalls)
			}
			var foundWarning bool
			for _, warning := range a.Warnings() {
				if warning.Type == "scan_subtask_error" && warning.File == "overflow.go" && strings.Contains(warning.Message, "prompt tokens") {
					foundWarning = true
				}
			}
			if !foundWarning {
				t.Fatalf("missing prompt-overflow failure warning: %+v", a.Warnings())
			}
			state, err := session.LoadResumeState(repo, a.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			if len(state.Items) != int(wantCalls) {
				t.Fatalf("reusable items = %d, want %d", len(state.Items), wantCalls)
			}
			for _, item := range state.Items {
				if item.FilePath != "small.go" {
					t.Errorf("incomplete file was marked reusable: %s", item.FilePath)
				}
			}
			path, err := session.SessionFilePath(repo, a.SessionID())
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var foundFailure bool
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var record struct {
					Type     string `json:"type"`
					FilePath string `json:"filePath"`
					Error    string `json:"error"`
				}
				if err := json.Unmarshal([]byte(line), &record); err != nil {
					t.Fatal(err)
				}
				if record.Type == "review_item_failed" && record.FilePath == "overflow.go" && strings.Contains(record.Error, "prompt tokens") {
					foundFailure = true
				}
			}
			if !foundFailure {
				t.Fatal("session omitted the prompt-overflow failure record")
			}
		})
	}
}
