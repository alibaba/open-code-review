// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// pathRuleStub resolves per-path system rules so a test can inflate one
// file's rendered prompt without touching its content - the reported shape
// where pre-selection on raw content passes but the rendered prompt does
// not fit the token limit.
type pathRuleStub map[string]string

func (s pathRuleStub) Resolve(path string) string { return s[path] }

// promptLimitTemplate renders MAIN_TASK as "review {rule} {content}" under
// the given max_tokens, so the 80% pre-flight limit is maxTokens*4/5.
func promptLimitTemplate(maxTokens int) template.ScanTemplate {
	return template.ScanTemplate{
		MaxTokens:           maxTokens,
		MaxToolRequestTimes: 5,
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "scan"},
				{Role: "user", Content: "review {{system_rule}} {{file_content}}"},
			},
		},
	}
}

// oversizedRule is far above the 80-token limit of a 100-token template
// under both the real tokenizer and the bytes/4 fallback.
func oversizedRule() string {
	return strings.Repeat("flag everything suspicious ", 100)
}

func newPromptLimitAgent(t *testing.T, rules pathRuleStub, items int) (*Agent, *fakeBudgetClient) {
	t.Helper()
	fake := &fakeBudgetClient{perCallTokens: 1}
	a := NewAgent(Args{
		Template:         promptLimitTemplate(100),
		SystemRule:       rules,
		LLMClient:        fake,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MaxConcurrency:   1,
		Session:          session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: session.ReviewModeFullScan}),
		SkipPlan:         true,
		SkipDedup:        true,
		SkipSummary:      true,
	})
	a.items = makeScanItems(items)
	a.args.Tools.Freeze()
	return a, fake
}

func assertFileWarning(t *testing.T, a *Agent, warningType, file string) {
	t.Helper()
	for _, w := range a.Warnings() {
		if w.Type == warningType && w.File == file {
			return
		}
	}
	t.Errorf("missing %s warning for %s", warningType, file)
}

// TestPromptLimitAllFilesFail pins the all-failed outcome: when every
// dispatched file's rendered prompt exceeds the pre-flight token limit, the
// scan must return the all-files-failed error instead of exiting clean with
// zero MAIN_TASK requests and a "looks good" result.
func TestPromptLimitAllFilesFail(t *testing.T) {
	a, fake := newPromptLimitAgent(t, pathRuleStub{"f0.go": oversizedRule()}, 1)

	_, err := a.dispatchSubtasks(context.Background())
	if err == nil {
		t.Fatal("expected the all-files-failed error, got nil")
	}
	if !strings.Contains(err.Error(), "all 1 file scan(s) failed") {
		t.Errorf("unexpected error: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 0 {
		t.Errorf("MAIN_TASK must not be called when every prompt is over the limit, got %d call(s)", calls)
	}
	assertFileWarning(t, a, "token_threshold_exceeded", "f0.go")
	assertFileWarning(t, a, "scan_subtask_error", "f0.go")
}

// TestPromptLimitMixedBatch pins the mixed outcome: a file rejected by the
// rendered-prompt limit must be recorded as failed (scan_subtask_error) so
// output layers report an incomplete review, while the healthy sibling still
// completes and the scan as a whole does not error.
func TestPromptLimitMixedBatch(t *testing.T) {
	a, fake := newPromptLimitAgent(t, pathRuleStub{"f1.go": oversizedRule()}, 2)

	comments, err := a.dispatchSubtasks(context.Background())
	if err != nil {
		t.Fatalf("a single rejected file must not fail the whole scan: %v", err)
	}
	if calls := atomic.LoadInt64(&fake.calls); calls != 1 {
		t.Errorf("exactly the healthy file must reach MAIN_TASK, got %d call(s)", calls)
	}
	if len(comments) != 0 {
		t.Errorf("fake client produces no comments, got %d", len(comments))
	}
	assertFileWarning(t, a, "token_threshold_exceeded", "f1.go")
	assertFileWarning(t, a, "scan_subtask_error", "f1.go")
	for _, w := range a.Warnings() {
		if w.File == "f0.go" {
			t.Errorf("the healthy file must not carry warnings: %+v", w)
		}
	}
}
