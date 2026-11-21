// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

type scanRuleFunc func(string) string

func (f scanRuleFunc) Resolve(path string) string { return f(path) }

type scanRequestFunc func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)

func (f scanRequestFunc) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return f(ctx, req)
}

func TestScanItemFingerprintRuleIdentity(t *testing.T) {
	item := model.ScanItem{Path: "a.go", Content: "package a\n"}
	want := scanItemFingerprint(item, "policy")
	if got := scanItemFingerprint(item, "policy"); got != want {
		t.Fatalf("fingerprint is not stable: %q != %q", got, want)
	}
	for _, tc := range []struct {
		name string
		item model.ScanItem
		rule string
	}{
		{"path", model.ScanItem{Path: "b.go", Content: item.Content}, "policy"},
		{"content", model.ScanItem{Path: item.Path, Content: "package b\n"}, "policy"},
		{"rule", item, "new policy"},
		{"empty rule", item, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanItemFingerprint(tc.item, tc.rule); got == want {
				t.Fatal("changed input retained the fingerprint")
			}
		})
	}

	withNUL := model.ScanItem{Path: item.Path, Content: item.Content + "\x00suffix"}
	if scanItemFingerprint(withNUL, "policy") == scanItemFingerprint(item, "suffix\x00policy") {
		t.Fatal("embedded NUL shifted the content/rule boundary")
	}
	invalidA := model.ScanItem{Path: item.Path, Content: "\xff"}
	invalidB := model.ScanItem{Path: item.Path, Content: "\xfe"}
	if scanItemFingerprint(invalidA, "") == scanItemFingerprint(invalidB, "") {
		t.Fatal("distinct invalid UTF-8 bytes were normalized")
	}
	var nilAgent *Agent
	if got := nilAgent.scanItemFingerprint(item); got != scanItemFingerprint(item, "") {
		t.Fatalf("nil agent fingerprint = %q", got)
	}
}

func TestScanInputsFreezeRulesForPlanAndMain(t *testing.T) {
	for _, initial := range []string{"policy", ""} {
		t.Run(fmt.Sprintf("rule=%q", initial), func(t *testing.T) {
			var resolved int
			current := initial
			resolver := scanRuleFunc(func(string) string {
				resolved++
				return current
			})
			var requests []llm.ChatRequest
			client := scanRequestFunc(func(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				requests = append(requests, req)
				return scanTaskDoneResponse(), nil
			})
			tpl := makeTemplateWithFullScan()
			tpl.MaxTokens = 100000
			tpl.PlanTask = &template.LlmConversation{
				Messages: []template.ChatMessage{{Role: "user", Content: "plan rule={{system_rule}}"}},
			}
			a := NewAgent(Args{RepoDir: t.TempDir(), Template: tpl, SystemRule: resolver, LLMClient: client})
			t.Cleanup(func() { _ = a.Session().Finalize() })
			item := model.ScanItem{Path: "a.go", Content: "package a\n", LineCount: 1}
			a.initScanInputs([]model.ScanItem{item})
			current = "changed after checkpoint lookup"
			if got := a.scanItemFingerprint(item); got != scanItemFingerprint(item, initial) {
				t.Fatalf("fingerprint did not retain the resolved rule: %q", got)
			}
			completed, reason, err := a.executeSubtask(t.Context(), item)
			if err != nil || !completed || reason != "" {
				t.Fatalf("executeSubtask: completed=%v reason=%q err=%v", completed, reason, err)
			}
			if resolved != 1 || len(requests) != 2 {
				t.Fatalf("rule resolutions=%d requests=%d, want 1 and 2", resolved, len(requests))
			}
			for i, want := range []string{"plan rule=" + initial, "scan system rule=" + initial} {
				if got := requests[i].Messages[0].ExtractText(); got != want {
					t.Errorf("request %d rule = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestScanResumeRuleCheckpoints(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous string
		current  string
		legacy   bool
		reuse    bool
	}{
		{name: "unchanged", previous: "policy", current: "policy", reuse: true},
		{name: "changed", previous: "old policy", current: "new policy"},
		{name: "added", current: "policy"},
		{name: "removed", previous: "policy"},
		{name: "legacy with rule", current: "policy", legacy: true},
		{name: "legacy without rule", legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setTestHome(t, t.TempDir())
			repo := t.TempDir()
			item := model.ScanItem{Path: "a.go", Content: "package a\n", LineCount: 1}
			fingerprint := scanItemFingerprint(item, tc.previous)
			if tc.legacy {
				sum := sha256.Sum256([]byte(session.ReviewModeFullScan + "\x00" + item.Path + "\x00" + item.Content))
				fingerprint = fmt.Sprintf("%x", sum)
			}
			parent := session.New(repo, "main", "test-model", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
			parent.RecordReviewItemDone(item.Path, item.Path, item.Path, fingerprint,
				[]model.LlmComment{{Path: item.Path, Content: "prior finding"}})
			if err := parent.Finalize(); err != nil {
				t.Fatal(err)
			}
			path, err := session.SessionFilePath(repo, parent.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			state, err := session.LoadResumeState(repo, parent.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			wantComments := 0
			if tc.reuse {
				wantComments = 1
			}
			for round := 0; round < 2; round++ {
				client := &fakeScanClient{responses: []*llm.ChatResponse{scanTaskDoneResponse()}}
				a := NewAgent(Args{
					RepoDir: repo, Template: makeTemplateWithFullScan(), LLMClient: client,
					SystemRule: scanRuleFunc(func(string) string { return tc.current }),
					Resume:     state, MaxConcurrency: 1, SkipPlan: true, SkipDedup: true, SkipSummary: true,
				})
				t.Cleanup(func() { _ = a.Session().Finalize() })
				a.items = []model.ScanItem{item}
				comments, err := a.dispatchSubtasks(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(comments) != wantComments || (wantComments == 1 && comments[0].Content != "prior finding") {
					t.Fatalf("round %d comments = %+v, want %d prior findings", round, comments, wantComments)
				}
				wantCalls, wantReused := 1, int64(0)
				if round == 1 || tc.reuse {
					wantCalls, wantReused = 0, 1
				}
				info := a.ResumeInfo()
				if client.calls != wantCalls || info.ReusedFiles != wantReused || info.RerunFiles != 1-wantReused {
					t.Fatalf("round %d calls=%d resume=%+v, want calls=%d reused=%d", round, client.calls, info, wantCalls, wantReused)
				}
				if err := a.Session().Finalize(); err != nil {
					t.Fatal(err)
				}
				state, err = session.LoadResumeState(repo, a.SessionID())
				if err != nil {
					t.Fatal(err)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("resuming modified the parent session")
			}
		})
	}
}
