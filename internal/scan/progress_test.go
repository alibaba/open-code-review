// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/progress"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

// The scan's progress events are the dashboard's only view of a running scan.
// A publisher that nothing calls leaves the bar with no denominator, the work
// list empty, and the counters at zero, so each one is exercised here.

// captureProgress collects the events fn publishes.
func captureProgress(t *testing.T, fn func()) []progress.Event {
	t.Helper()
	var got []progress.Event
	restore := progress.SetSinkForTest(func(ev progress.Event) {
		got = append(got, ev)
	})
	defer restore()
	fn()
	return got
}

func TestPublishPlannedSetsTheCoverageDenominator(t *testing.T) {
	got := captureProgress(t, func() { publishPlanned(7, Estimate{InputTokens: 300, OutputTokens: 40}, "by-language", 5) })

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Kind != progress.KindPlanned {
		t.Errorf("kind = %v, want KindPlanned", got[0].Kind)
	}
	if got[0].Files != 7 {
		t.Errorf("files = %d, want 7", got[0].Files)
	}
	if got[0].Plan == nil {
		t.Fatal("the event carries no plan, so the dashboard has no estimate to show")
	}
	if got[0].Plan.InputTokens != 300 || got[0].Plan.OutputTokens != 40 {
		t.Errorf("plan tokens = %d in / %d out, want 300 in / 40 out",
			got[0].Plan.InputTokens, got[0].Plan.OutputTokens)
	}
	if got[0].Plan.Strategy != "by-language" || got[0].Plan.Groups != 5 {
		t.Errorf("plan = %q over %d groups, want by-language over 5",
			got[0].Plan.Strategy, got[0].Plan.Groups)
	}
}

func TestPublishItemLifecycle(t *testing.T) {
	it := model.ScanItem{Path: "a.go"}

	start := captureProgress(t, func() { publishItemStart(it, "2 of 5") })
	if len(start) != 1 {
		t.Fatalf("expected 1 event, got %d", len(start))
	}
	if start[0].Kind != progress.KindGroupStart {
		t.Errorf("kind = %v, want KindGroupStart", start[0].Kind)
	}
	if start[0].Group != "a.go" {
		t.Errorf("group = %q, want the item's path", start[0].Group)
	}
	if start[0].Plan == nil || start[0].Plan.Batch != "2 of 5" {
		t.Errorf("batch = %v, want \"2 of 5\"", start[0].Plan)
	}

	done := captureProgress(t, func() { publishItemDone(it, 3) })
	if len(done) != 1 {
		t.Fatalf("expected 1 event, got %d", len(done))
	}
	if done[0].Kind != progress.KindGroupDone {
		t.Errorf("kind = %v, want KindGroupDone", done[0].Kind)
	}
	if done[0].Findings != 3 {
		t.Errorf("findings = %d, want 3", done[0].Findings)
	}

	failed := captureProgress(t, func() { publishItemFailed(it, "boom") })
	if len(failed) != 1 {
		t.Fatalf("expected 1 event, got %d", len(failed))
	}
	if failed[0].Kind != progress.KindGroupFailed {
		t.Errorf("kind = %v, want KindGroupFailed", failed[0].Kind)
	}
	if failed[0].Err != "boom" {
		t.Errorf("err = %q, want the failure reason", failed[0].Err)
	}
}

func TestPublishBudgetReached(t *testing.T) {
	got := captureProgress(t, func() { publishBudgetReached() })

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Kind != progress.KindBudgetReached {
		t.Errorf("kind = %v, want KindBudgetReached", got[0].Kind)
	}
}

// The dashboard's log rows are the events' own text, and the text sink adds
// the "[ocr] " prefix itself. A scan line that carried its own prefix would
// show it twice.
func TestScanProgressTextHasNoPrefix(t *testing.T) {
	got := captureProgress(t, func() { progress.Noticef("a scan line\n") })

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if strings.HasPrefix(got[0].Text, "[ocr] ") {
		t.Errorf("scan line carries the text sink's prefix: %q", got[0].Text)
	}
}

// The dashboard collapses a run of identically-reasoned skips into one row
// and counts them in the skipped counter. That only happens when the skip
// arrives as a skip event: a plain notice is just another line in the log.
func TestLogSelectionPublishesSkipsAsSkipEvents(t *testing.T) {
	a := &Agent{args: Args{Template: template.ScanTemplate{MaxTokens: 1000}}}
	decisions := []scanSelection{
		{item: model.ScanItem{Path: "a.go", IsBinary: true}, reason: model.ExcludeBinary},
		{item: model.ScanItem{Path: "b.go"}, reason: model.ExcludeSecret},
		{item: model.ScanItem{Path: "c.go"}, reason: model.ExcludeExtension},
	}

	got := captureProgress(t, func() { a.logSelection(decisions) })

	var skips []progress.Event
	for _, ev := range got {
		if ev.Kind == progress.KindSkipped {
			skips = append(skips, ev)
		}
	}
	if len(skips) != 3 {
		t.Fatalf("expected 3 skip events, got %d: %+v", len(skips), got)
	}
	want := []struct{ path, reason string }{
		{"a.go", "binary file"},
		{"b.go", "matches a built-in secret path"},
		{"c.go", "filtered by path/extension rules"},
	}
	for i, w := range want {
		if skips[i].Detail != w.path {
			t.Errorf("skip %d detail = %q, want %q", i, skips[i].Detail, w.path)
		}
		if skips[i].SkipReason != w.reason {
			t.Errorf("skip %d reason = %q, want %q", i, skips[i].SkipReason, w.reason)
		}
	}
}

// The skip event's own text is what the plain sink prints, so it has to be
// the line the scan has always printed — same bytes, no second prefix.
func TestSkippedEventTextMatchesThePrintedLine(t *testing.T) {
	a := &Agent{args: Args{Template: template.ScanTemplate{MaxTokens: 1000}}}
	decisions := []scanSelection{
		{item: model.ScanItem{Path: "a.go"}, reason: model.ExcludeExtension},
	}

	got := captureProgress(t, func() { a.logSelection(decisions) })

	var skip *progress.Event
	for i, ev := range got {
		if ev.Kind == progress.KindSkipped {
			skip = &got[i]
			break
		}
	}
	if skip == nil {
		t.Fatalf("expected a skip event, got %+v", got)
	}
	if want := "Skipping a.go — filtered by path/extension rules\n"; skip.Text != want {
		t.Errorf("skip text = %q, want %q", skip.Text, want)
	}
}

// The dashboard's findings counter is how many comments a file produced. The
// scan reads that count after the comment workers have drained, so the count
// is the file's real count rather than whatever had landed so far — the
// workers that add the comments run on their own goroutines.
func TestDispatchSubtasks_DoneEventCountsTheFindings(t *testing.T) {
	setTestHome(t, t.TempDir())
	item := model.ScanItem{Path: "a.go", Content: "package a\n", LineCount: 1}
	collector := tool.NewCommentCollector()
	reg := tool.NewRegistry()
	reg.Register(&tool.CodeCommentProvider{Collector: collector})
	pool := llmloop.NewCommentWorkerPool(2)

	client := &fakeScanClient{responses: []*llm.ChatResponse{{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "1", Type: "function", Function: llm.FunctionCall{
					Name:      "code_comment",
					Arguments: `{"comments":[{"path":"a.go","content":"missing a nil check"}]}`,
				}},
				{ID: "2", Type: "function", Function: llm.FunctionCall{Name: "task_done", Arguments: "{}"}},
			}},
			FinishReason: "tool_calls",
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 10, TotalTokens: 10},
	}}}

	sess := session.New(t.TempDir(), "main", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	// The session writer holds the file open until it is finalized, and
	// Windows cannot delete an open file: the temp directory's cleanup fails
	// without this.
	t.Cleanup(func() { _ = sess.Finalize() })

	a := NewAgent(Args{
		Template:          makeTemplateWithFullScan(),
		LLMClient:         client,
		Model:             "fake",
		CommentCollector:  collector,
		CommentWorkerPool: pool,
		Tools:             reg,
		MaxConcurrency:    1,
		SkipPlan:          true,
		SkipDedup:         true,
		SkipSummary:       true,
		Session:           sess,
	})
	a.items = []model.ScanItem{item}
	a.currentDate = "2026-06-26"

	// The count the event carries must be the count the collector holds by
	// the time the event is published: the file's comments are added by the
	// pool's workers, so reading before they drain would report zero.
	var published, held int
	restore := progress.SetSinkForTest(func(ev progress.Event) {
		if ev.Kind == progress.KindGroupDone {
			published = ev.Findings
			held = len(collector.CommentsForPath(ev.Group))
		}
	})
	defer restore()

	if _, err := a.dispatchSubtasks(context.Background()); err != nil {
		t.Fatalf("dispatchSubtasks: %v", err)
	}

	if published == 0 {
		t.Error("the done event must carry the file's findings, got 0")
	}
	if published != held {
		t.Errorf("done event carries %d findings, but the collector holds %d", published, held)
	}
}
