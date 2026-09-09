// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func newAgentForTest(t *testing.T, tpl template.ScanTemplate) *Agent {
	t.Helper()
	return NewAgent(Args{
		Template:         tpl,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test-model", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
	})
}

func makeTemplateWithFullScan() template.ScanTemplate {
	return template.ScanTemplate{
		MaxTokens:           1000,
		MaxToolRequestTimes: 5,
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "scan system rule={{system_rule}}"},
				{
					Role: "user",
					Content: "path={{current_file_path}}\n" +
						"date={{current_system_date_time}}\n" +
						"siblings=[{{change_files}}]\n" +
						"bg={{requirement_background}}\n" +
						"plan={{plan_guidance}}\n" +
						"<content>\n{{file_content}}\n</content>",
				},
			},
		},
	}
}

func TestFormatPlanGuidance_FullJSON(t *testing.T) {
	raw := "```json\n" + `{
  "summary": "this file orchestrates X.",
  "checkpoints": [
    {"focus": "race in cache", "lines": "45-78", "why": "writes under read lock"},
    {"focus": "error swallowing", "lines": "120-130", "why": "ignored Err return"}
  ]
}` + "\n```"
	got := formatPlanGuidance(raw)
	for _, want := range []string{
		"**Summary**: this file orchestrates X.",
		"1. `race in cache` (lines 45-78) — writes under read lock",
		"2. `error swallowing` (lines 120-130) — ignored Err return",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestFormatPlanGuidance_EmptyAndMalformed(t *testing.T) {
	if got := formatPlanGuidance(""); got != "" {
		t.Errorf("empty input should yield empty guidance, got %q", got)
	}
	// Malformed JSON falls back to the raw text so we don't lose what the
	// model said — better to feed bad text to the reviewer than nothing.
	raw := "the LLM forgot to use JSON: focus on error handling"
	if got := formatPlanGuidance(raw); got != raw {
		t.Errorf("malformed input should pass through raw, got %q", got)
	}
}

func TestFormatPlanGuidance_SummaryOnly(t *testing.T) {
	raw := `{"summary": "small helper file", "checkpoints": []}`
	got := formatPlanGuidance(raw)
	if !strings.Contains(got, "**Summary**: small helper file") {
		t.Errorf("missing summary header, got %q", got)
	}
	if strings.Contains(got, "Focus areas") {
		t.Errorf("should not render focus header when no checkpoints, got %q", got)
	}
}

// TestPreview_DoesNotMutateAgentItems guards against re-introducing a
// side-effect that pre-populated a.items, which made subsequent Run calls
// on the same Agent silently observe stale state.
func TestPreview_DoesNotMutateAgentItems(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "a.go", []byte("package a\n"))
	writeFile(t, repo, "b.go", []byte("package b\n"))
	gitCommit(t, repo, "init")

	a := NewAgent(Args{
		RepoDir:   repo,
		GitRunner: nil,
		Template:  makeTemplateWithFullScan(),
	})
	if got := a.items; got != nil {
		t.Fatalf("pre-Preview items should be nil, got %v", got)
	}
	if _, err := a.preview(t.Context()); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if a.items != nil {
		t.Errorf("Preview must not mutate a.items; got %d items", len(a.items))
	}
}

// TestPreview_EmptyResultEntriesIsNonNilSlice prevents `"files":null` in
// JSON output when there is nothing reviewable to enumerate.
func TestPreview_EmptyResultEntriesIsNonNilSlice(t *testing.T) {
	// Empty repo → empty Entries
	repo := initTestRepo(t)
	a := NewAgent(Args{
		RepoDir:  repo,
		Template: makeTemplateWithFullScan(),
	})
	got, err := a.preview(t.Context())
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if got.Entries == nil {
		t.Errorf("Entries must be non-nil even when empty (JSON would emit null)")
	}
	if len(got.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got.Entries))
	}
}

func TestBuildSummaryCommentsList_TruncatesAndOneLines(t *testing.T) {
	long := strings.Repeat("x", 400)
	cs := []model.LlmComment{
		{Path: "a.go", Content: "line one\nline two\nline three"},
		{Path: "b.go", Content: long},
	}
	got := buildSummaryCommentsList(cs)

	// Newlines in content should be collapsed to spaces.
	if strings.Contains(got, "line one\nline two") {
		t.Errorf("expected content newlines to be flattened, got:\n%s", got)
	}
	if !strings.Contains(got, "- `a.go`: line one line two line three") {
		t.Errorf("expected path-anchored prefix, got:\n%s", got)
	}
	// Long content truncated to ~280 + "..." marker.
	if !strings.Contains(got, "...") {
		t.Errorf("expected truncation marker on long content, got:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 320 { // 280 content + small path/prefix overhead
			t.Errorf("line not capped: len=%d %q", len(line), line)
		}
	}
}

func TestMaybeRunPlan_SkipPathsDoNotCallLLM(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	// no PlanTask attached → must return sentinel without crashing
	a := newAgentForTest(t, tpl)
	guidance := a.maybeRunPlan(t.Context(), model.ScanItem{Path: "x.go", Content: "package x"}, "rule")
	if !strings.Contains(guidance, "no pre-scan plan") {
		t.Errorf("expected fallback sentinel, got %q", guidance)
	}

	// PlanTask attached but SkipPlan set
	tpl.PlanTask = &template.LlmConversation{
		Messages: []template.ChatMessage{{Role: "user", Content: "plan {{file_content}}"}},
	}
	a2 := NewAgent(Args{
		Template:         tpl,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		Session: session.New(t.TempDir(), "main", "test-model", session.SessionOptions{
			ReviewMode: session.ReviewModeFullScan,
		}),
		SkipPlan: true,
	})
	guidance2 := a2.maybeRunPlan(t.Context(), model.ScanItem{Path: "x.go", Content: "package x"}, "rule")
	if !strings.Contains(guidance2, "no pre-scan plan") {
		t.Errorf("SkipPlan should suppress plan, got %q", guidance2)
	}
}

func TestRenderMessages(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	a := newAgentForTest(t, tpl)
	a.currentDate = "2026-06-09 10:00"
	a.args.Background = "ticket-123"

	it := model.ScanItem{
		Path:    "internal/foo/bar.go",
		Content: "package foo\n\nfunc Bar() {}\n",
	}
	msgs := a.renderMessages(it, "rule-text", "(no pre-scan plan; review the entire file as usual)")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	sysText := msgs[0].ExtractText()
	if !strings.Contains(sysText, "rule=rule-text") {
		t.Errorf("system missing system_rule: %q", sysText)
	}

	userText := msgs[1].ExtractText()
	checks := map[string]string{
		"path":     "path=internal/foo/bar.go",
		"date":     "date=2026-06-09 10:00",
		"siblings": "siblings=[" + changeFilesScanLiteral + "]",
		"bg":       "bg=ticket-123",
		"content":  "<content>\npackage foo\n\nfunc Bar() {}\n\n</content>",
	}
	for label, want := range checks {
		if !strings.Contains(userText, want) {
			t.Errorf("%s missing %q\nfull: %q", label, want, userText)
		}
	}
	for _, leak := range []string{"{{diff}}", "{{file_content}}", "{{change_files}}", "{{plan_guidance}}"} {
		if strings.Contains(userText, leak) {
			t.Errorf("placeholder %s leaked into prompt", leak)
		}
	}
}

func TestFilterLargeScans(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 40 // threshold = 32
	a := newAgentForTest(t, tpl)

	short := strings.Repeat("a ", 5)
	huge := strings.Repeat("token ", 200)
	in := []model.ScanItem{
		{Path: "a.go", Content: short},
		{Path: "huge.go", Content: huge},
		{Path: "b.go", Content: short},
	}
	out := a.filterLargeScans(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 kept, got %d", len(out))
	}
	for _, it := range out {
		if it.Path == "huge.go" {
			t.Errorf("huge.go should have been filtered")
		}
	}
}

// exactNTokens builds a string that llm.CountTokens reports as exactly n
// tokens, failing loudly if the tokenizer disagrees so fixture drift cannot
// silently weaken the boundary assertions below.
func exactNTokens(t *testing.T, n int) string {
	t.Helper()
	s := strings.TrimSpace(strings.Repeat("a ", n))
	if got := llm.CountTokens(s); got != n {
		t.Fatalf("fixture drift: llm.CountTokens(<%d-token string>) = %d, want %d", n, got, n)
	}
	return s
}

// TestFilterLargeScans_Boundary pins the 80% threshold exactly: with
// MaxTokens=100 the limit is 80, so an 80-token item is kept and an 81-token
// one is dropped. TestFilterLargeScans above uses margins wide enough to pass
// at any threshold, so it does not pin the value.
func TestFilterLargeScans_Boundary(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 100 // threshold = 80
	a := newAgentForTest(t, tpl)

	in := []model.ScanItem{
		{Path: "at-limit.go", Content: exactNTokens(t, 80)},
		{Path: "over-limit.go", Content: exactNTokens(t, 81)},
	}
	out := a.filterLargeScans(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 kept, got %d", len(out))
	}
	if out[0].Path != "at-limit.go" {
		t.Errorf("kept wrong file: got %s, want at-limit.go", out[0].Path)
	}
}

func TestFilterLargeScans_NoLimit(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 0
	a := newAgentForTest(t, tpl)
	in := []model.ScanItem{
		{Path: "a.go", Content: "anything"},
		{Path: "b.go", Content: strings.Repeat("x ", 1000)},
	}
	out := a.filterLargeScans(in)
	if len(out) != 2 {
		t.Errorf("with MaxTokens=0 nothing should be filtered, got %d", len(out))
	}
}

func TestInjectScanContentMap(t *testing.T) {
	tpl := makeTemplateWithFullScan()
	a := newAgentForTest(t, tpl)
	a.args.Tools.Register(tool.NewFileReadDiff(tool.DiffMap{}))

	a.items = []model.ScanItem{
		{Path: "x.go", Content: "package x"},
		{Path: "y.go", Content: "package y"},
	}
	a.injectScanContentMap()

	p, ok := a.args.Tools.Get(tool.FileReadDiff.Name())
	if !ok {
		t.Fatal("file_read_diff not registered")
	}
	frd := p.(*tool.FileReadDiffProvider)
	res, err := frd.Execute(t.Context(), map[string]any{
		"path_array": []any{"x.go", "y.go", "missing.go"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res, "package x") || !strings.Contains(res, "package y") {
		t.Errorf("missing scan content:\n%s", res)
	}
}

func TestNewAgent_SetsSessionMode(t *testing.T) {
	a := NewAgent(Args{Template: makeTemplateWithFullScan()})
	if a.session.ReviewMode != session.ReviewModeFullScan {
		t.Errorf("ReviewMode = %q, want %q", a.session.ReviewMode, session.ReviewModeFullScan)
	}
}

func TestRunner_Warnings_RoundTrip(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	a.recordWarning("foo", "x.go", "boom")
	ws := a.Warnings()
	if len(ws) != 1 || ws[0].Type != "foo" || ws[0].File != "x.go" {
		t.Errorf("warnings = %+v", ws)
	}
}

// Ensure llmloop.Runner is the underlying source of token counters so the
// public methods on scan.Agent are not stale (preventing accidental refactor
// regressions).
func TestTokenCountersDelegateToRunner(t *testing.T) {
	a := newAgentForTest(t, makeTemplateWithFullScan())
	if a.TotalInputTokens() != a.runner.TotalInputTokens() ||
		a.TotalOutputTokens() != a.runner.TotalOutputTokens() ||
		a.TotalCacheReadTokens() != a.runner.TotalCacheReadTokens() ||
		a.TotalCacheWriteTokens() != a.runner.TotalCacheWriteTokens() {
		t.Fatal("scan.Agent token getters must mirror runner")
	}
	_ = llmloop.AgentWarning{} // keep llmloop import meaningful
}

type blockingScanCompressionClient struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	round   int
}

func newBlockingScanCompressionClient() *blockingScanCompressionClient {
	return &blockingScanCompressionClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (c *blockingScanCompressionClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(req.Tools) == 0 {
		c.once.Do(func() { close(c.started) })
		<-c.release
		summary := "compressed summary"
		return &llm.ChatResponse{
			Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &summary}}},
			Usage:   &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
		}, nil
	}

	c.mu.Lock()
	c.round++
	r := c.round
	c.mu.Unlock()

	if r == 1 {
		content := strings.Repeat("word ", 650)
		return &llm.ChatResponse{
			Choices: []llm.Choice{{
				Message: llm.ResponseMessage{
					Content: &content,
					ToolCalls: []llm.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "code_comment",
							Arguments: `{"comments":[{"path":"main.go","line":1,"content":"finding"}]}`,
						},
					}},
				},
			}},
			Usage: &llm.UsageInfo{PromptTokens: 100, CompletionTokens: 50},
		}, nil
	}

	// Round 2+: wait until the background compression goroutine has reached
	// the mock (closed c.started) before returning task_done. This eliminates
	// the race where the main loop finishes before compression starts,
	// ensuring WaitBackground is the only thing holding Run open.
	<-c.started

	doneContent := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &doneContent,
				ToolCalls: []llm.ToolCall{{
					ID:   "call_done",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "task_done",
						Arguments: `{"state":"DONE"}`,
					},
				}},
			},
		}},
		Usage: &llm.UsageInfo{PromptTokens: 20, CompletionTokens: 10},
	}, nil
}

func TestScanAgent_WaitBackground_NoLeakOnRun(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, repo, "main.go", []byte("package main\n\nfunc main() {}\n"))
	gitCommit(t, repo, "init")

	tpl := makeTemplateWithFullScan()
	tpl.MaxTokens = 1000
	tpl.MemoryCompressionTask = template.LlmConversation{
		Messages: []template.ChatMessage{
			{Role: "system", Content: "compress {{context}}"},
		},
	}

	client := newBlockingScanCompressionClient()
	sess := session.New(repo, "main", "test-model", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})

	a := NewAgent(Args{
		RepoDir:          repo,
		Template:         tpl,
		LLMClient:        client,
		Model:            "test-model",
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		MainToolDefs: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "code_comment"}},
			{Type: "function", Function: llm.FunctionDef{Name: "task_done"}},
		},
		SkipPlan:       true,
		SkipDedup:      true,
		SkipSummary:    true,
		MaxConcurrency: 1,
		Session:        sess,
	})

	type runResult struct {
		comments []model.LlmComment
		err      error
	}
	done := make(chan runResult, 1)

	go func() {
		comments, err := a.Run(context.Background())
		done <- runResult{comments: comments, err: err}
	}()

	// Wait until background compression starts and blocks
	select {
	case <-client.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for background compression to start")
	}

	// Verify that a.Run has NOT returned yet (held by a.runner.WaitBackground()).
	// Use a timeout rather than default: the main goroutine's entire post-loop
	// cleanup (RunMainTask return → dispatchSubtasks → Finalize) completes in
	// under 1ms on any machine, so 200ms is a generous upper bound. If Run
	// returns within this window, WaitBackground is not holding it.
	select {
	case res := <-done:
		t.Fatalf("a.Run returned prematurely before background compression was released: err=%v", res.err)
	case <-time.After(200 * time.Millisecond):
	}

	// Release compression and wait for Run to finish
	close(client.release)

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("Run: %v", res.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a.Run to finish")
	}

	// Verify session JSONL file:
	// 1. memory_compression_task llm_request is recorded
	// 2. session_end is the final record
	path, err := session.SessionFilePath(repo, sess.SessionID)
	if err != nil {
		t.Fatalf("SessionFilePath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty session JSONL")
	}

	var hasCompressionRequest bool
	for _, l := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatalf("unmarshal line %q: %v", l, err)
		}
		if rec["type"] == "llm_request" && rec["taskType"] == string(session.MemoryCompressionTask) {
			hasCompressionRequest = true
		}
	}

	if !hasCompressionRequest {
		t.Errorf("session JSONL missing memory_compression_task llm_request; contents:\n%s", string(data))
	}

	var lastRec map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &lastRec); err != nil {
		t.Fatalf("unmarshal last line %q: %v", lines[len(lines)-1], err)
	}
	if lastRec["type"] != "session_end" {
		t.Errorf("last session JSONL record type = %q, want session_end", lastRec["type"])
	}
}
