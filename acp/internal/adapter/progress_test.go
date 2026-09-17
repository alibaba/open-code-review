// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

type progressRunner func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome)

func (r progressRunner) Run(ctx context.Context, req orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	return r(ctx, req)
}

type progressSink func(context.Context, acp.SessionNotification) error

func (s progressSink) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	return s(ctx, n)
}

func TestProgressLogPreservesLinesAndBounds(t *testing.T) {
	var log progressLog
	log.append("")
	log.append("[ocr] first\r\n")
	log.append("[ocr] second")
	if log.text != "[ocr] first\n[ocr] second\n" {
		t.Fatalf("lines joined: %q", log.text)
	}
	data, _ := json.Marshal(log.content())
	if !strings.Contains(string(data), `[ocr] first\n[ocr] second\n`) {
		t.Fatalf("logs not rendered as code: %s", data)
	}
	log.append(strings.Repeat("\u20ac", progressLogLimit))
	if len(log.text) > progressLogLimit || !utf8.ValidString(log.text) || !log.truncated {
		t.Fatal("log tail is not bounded valid UTF-8")
	}
	log.append("invalid\xff")
	data, _ = json.Marshal(log.content())
	if !strings.Contains(string(data), "truncated") || !strings.Contains(log.text, "invalid?") {
		t.Fatalf("missing truncation or encoding repair: %s", data)
	}
}

func TestExecutionCommandQuotesArguments(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"ocr", "ocr"}, {"--format=json", "--format=json"}, {"", "''"},
		{"two words", "'two words'"}, {"a'b", "'a'\"'\"'b'"},
		{"$(echo unsafe);*", "'$(echo unsafe);*'"}, {"a\nb", "'a\nb'"},
	} {
		if got := shellArgument(tc.value); got != tc.want {
			t.Errorf("quote %q = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestProgressTitleIsSingleLineBoundedAndEscaped(t *testing.T) {
	if got := progressTitle("OCR review", "", 0); got != "OCR review · Running · 0s" {
		t.Fatalf("unexpected empty title: %q", got)
	}
	if got := progressTitle("OCR scan", "previous line\n[ocr] reading [file](url)", 2*time.Second); got != "OCR scan · "+markdownLabel("reading [file](url)")+" · 2s" {
		t.Fatalf("latest activity not escaped: %q", got)
	}
	for _, message := range []string{
		"[ocr] a\tb\r c\u0085d\u202ee\u2028f\u2029",
		strings.Repeat("\u754c", 200), strings.Repeat("*", 200), "invalid\xff",
	} {
		title := progressTitle("OCR review", message, time.Minute)
		if !utf8.ValidString(title) || strings.ContainsAny(title, "\r\n\t\u0085\u202e\u2028\u2029") || len(title) > 370 {
			t.Fatalf("unsafe or unbounded title: %q", title)
		}
	}
}

func TestExecutionDetailsSurviveUpdatesWithoutCommandInChat(t *testing.T) {
	runner := &captureRunner{}
	a := NewAgent("/tools/my ocr", runner)
	var notices []acp.SessionNotification
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		notices = append(notices, n)
		return nil
	}))
	s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review --effort low")}})
	if err != nil || len(runner.requests) != 1 {
		t.Fatalf("runner: %v, %v", runner.requests, err)
	}
	if notices[0].Update.ToolCall == nil {
		t.Fatal("first update must open execution details")
	}
	var initial string
	var snapshots int
	for _, n := range notices {
		if chat := n.Update.AgentMessageChunk; chat != nil && strings.Contains(chat.Content.Text.Text, "Command:") {
			t.Fatal("command leaked into final report")
		}
		var content []acp.ToolCallContent
		if start := n.Update.ToolCall; start != nil {
			if !strings.HasPrefix(start.Title, "OCR review · Running · ") || start.Kind != acp.ToolKindOther {
				t.Fatalf("unexpected title/kind: %+v", start)
			}
			content = start.Content
		}
		if update := n.Update.ToolCallUpdate; update != nil {
			if update.Title == nil || !strings.HasPrefix(*update.Title, "OCR review · Completed · ") {
				t.Fatalf("unexpected final title: %+v", update.Title)
			}
			content = update.Content
		}
		if len(content) == 0 {
			continue
		}
		got, _ := json.Marshal(content[0])
		if initial == "" {
			initial = string(got)
		} else if string(got) != initial {
			t.Fatal("execution details changed")
		}
		snapshots++
	}
	cwd := runner.requests[0].CWD
	encodedCWD, err := json.Marshal(cwd)
	if err != nil {
		t.Fatal(err)
	}
	cwdInJSON := string(encodedCWD[1 : len(encodedCWD)-1])
	for _, want := range []string{"Command:", "Working directory:", "'/tools/my ocr'", "--effort low", cwdInJSON} {
		if !strings.Contains(initial, want) {
			t.Fatalf("missing %q in %s", want, initial)
		}
	}
	if snapshots < 2 {
		t.Fatal("missing execution snapshots")
	}
}

func TestDiagnosticsDoNotReplaceProgressActivity(t *testing.T) {
	runner := progressRunner(func(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
		events := make(chan orchestrator.Event, 2)
		events <- orchestrator.Event{Kind: orchestrator.EventProgress, Message: "[ocr] Reviewing files"}
		events <- orchestrator.Event{Kind: orchestrator.EventDiagnostic, Message: "TraceID: diagnostic-only"}
		close(events)
		outcomes := make(chan orchestrator.Outcome, 1)
		outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted}
		close(outcomes)
		return events, outcomes
	})
	a := NewAgent("ocr", runner)
	seenActivity, seenDiagnostic := false, false
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		if update := n.Update.ToolCallUpdate; update != nil {
			if update.Title != nil && strings.Contains(*update.Title, "diagnostic-only") {
				t.Fatal("diagnostic became activity")
			}
			if update.Title != nil && strings.Contains(*update.Title, "Reviewing files") {
				seenActivity = true
			}
			data, _ := json.Marshal(update.Content)
			seenDiagnostic = seenDiagnostic || strings.Contains(string(data), "diagnostic-only")
		}
		return nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"scan"}}); err != nil {
		t.Fatal(err)
	}
	if !seenActivity || !seenDiagnostic {
		t.Fatal("activity or diagnostic details lost")
	}
}

func TestProgressUsesOneCollapsibleToolAndKeepsLogsOutOfChat(t *testing.T) {
	for _, command := range []string{"/review", "/scan"} {
		t.Run(command, func(t *testing.T) {
			runner := progressRunner(func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
				events := make(chan orchestrator.Event, 2)
				events <- orchestrator.Event{Message: "first"}
				events <- orchestrator.Event{Message: "second"}
				close(events)
				outcomes := make(chan orchestrator.Outcome, 1)
				outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted, Diagnostics: "first\nsecond\n", Warnings: []orchestrator.Event{{Message: "warning", Truncated: true}}, Result: &orchestrator.Result{Review: &contract.ReviewResult{Message: "summary"}}}
				close(outcomes)
				return events, outcomes
			})
			a := NewAgent("ocr", runner)
			var notices []acp.SessionNotification
			a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
				notices = append(notices, n)
				return nil
			}))
			s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: testGitDir(t)})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock(command)}})
			if err != nil || resp.StopReason != acp.StopReasonEndTurn {
				t.Fatalf("%+v %v", resp, err)
			}
			start := notices[0].Update.ToolCall
			finish := notices[len(notices)-2].Update.ToolCallUpdate
			if start == nil || finish == nil || start.ToolCallId != finish.ToolCallId || start.Status != acp.ToolCallStatusInProgress || finish.Status == nil || *finish.Status != acp.ToolCallStatusCompleted {
				t.Fatalf("incorrect lifecycle: %+v", notices)
			}
			data, _ := json.Marshal(finish)
			if !strings.Contains(string(data), `first\nsecond\n`) || !strings.Contains(string(data), "warning") || !strings.Contains(string(data), "truncated") {
				t.Fatalf("missing logs: %s", data)
			}
			for _, notice := range notices[:len(notices)-1] {
				if notice.Update.AgentMessageChunk != nil {
					t.Fatal("raw progress leaked into the conversation")
				}
			}
			chat := notices[len(notices)-1].Update.AgentMessageChunk
			if chat == nil || !strings.Contains(chat.Content.Text.Text, "summary") || strings.Contains(chat.Content.Text.Text, "first") || strings.Contains(chat.Content.Text.Text, "second") {
				t.Fatalf("logs leaked to chat: %+v", chat)
			}
		})
	}
}

func TestProgressStartFailureDoesNotRunOCR(t *testing.T) {
	r := &captureRunner{}
	a := NewAgent("ocr", r)
	want := errors.New("output unavailable")
	attempts := 0
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		attempts++
		if n.Update.ToolCall == nil {
			t.Error("expected execution entry")
		}
		return want
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
	if !errors.Is(err, want) || len(r.requests) != 0 || attempts != 1 {
		t.Fatalf("OCR started after output failure: %v", err)
	}
}

func TestProgressDetailsArriveBeforeRunnerCompletes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	visible := make(chan struct{})
	runner := progressRunner(func(ctx context.Context, _ orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
		events := make(chan orchestrator.Event, 1)
		outcomes := make(chan orchestrator.Outcome, 1)
		go func() {
			defer close(outcomes)
			events <- orchestrator.Event{Kind: orchestrator.EventProgress, Message: "READY"}
			select {
			case <-visible:
			case <-ctx.Done():
			}
			events <- orchestrator.Event{Message: strings.Repeat("\u20ac", progressLogLimit)}
			close(events)
			outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted}
		}()
		return events, outcomes
	})
	a := NewAgent("ocr", runner)
	seen := false
	var final acp.SessionNotification
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		if chat := n.Update.AgentMessageChunk; chat != nil {
			t.Error("raw logs leaked into chat")
		}
		if update := n.Update.ToolCallUpdate; update != nil {
			data, _ := json.Marshal(update.Content)
			if update.Status == nil && strings.Contains(string(data), "READY") && !seen {
				if update.Title == nil || !strings.Contains(*update.Title, "READY") {
					t.Error("collapsed title does not show current activity")
				}
				seen = true
				close(visible)
			}
			final = n
		}
		return nil
	}))
	_, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
	if err != nil || ctx.Err() != nil || !seen {
		t.Fatalf("live progress stalled: %v / %v / seen=%v", err, ctx.Err(), seen)
	}
	update := final.Update.ToolCallUpdate
	if update == nil || len(update.Content) != 2 {
		t.Fatalf("missing final command and details: %+v", update)
	}
	data, _ := json.Marshal(update.Content[1])
	if len(data) > progressLogLimit+512 || !strings.Contains(string(data), "truncated") {
		t.Fatalf("final details are not a bounded tail: %d bytes", len(data))
	}
}

func TestProgressOutputFailureWaitsForCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelled, release := make(chan struct{}), make(chan struct{})
	runner := progressRunner(func(ctx context.Context, _ orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
		events := make(chan orchestrator.Event, 1)
		events <- orchestrator.Event{Kind: orchestrator.EventProgress, Message: "READY"}
		outcomes := make(chan orchestrator.Outcome, 1)
		go func() {
			<-ctx.Done()
			close(cancelled)
			<-release
			close(events)
			outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCancelled}
			close(outcomes)
		}()
		return events, outcomes
	})
	a := NewAgent("ocr", runner)
	want := errors.New("write failed")
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		if n.Update.ToolCallUpdate != nil {
			return want
		}
		return nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
		done <- err
	}()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("output failure did not cancel OCR")
	}
	select {
	case <-done:
		t.Fatal("returned before cleanup")
	default:
	}
	close(release)
	if err := <-done; !errors.Is(err, want) {
		t.Fatalf("lost write error: %v", err)
	}
}

func TestPromptProgressFailurePreservesErrorAndFinalizesTool(t *testing.T) {
	for _, terminalFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "transient", true: "persistent"}[terminalFails], func(t *testing.T) {
			cleaned := false
			runner := progressRunner(func(ctx context.Context, _ orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
				events := make(chan orchestrator.Event, 1)
				events <- orchestrator.Event{Kind: orchestrator.EventProgress, Message: "READY"}
				outcomes := make(chan orchestrator.Outcome, 1)
				go func() {
					<-ctx.Done()
					cleaned = true
					close(events)
					outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCancelled}
					close(outcomes)
				}()
				return events, outcomes
			})
			a := NewAgent("ocr", runner)
			first, last := errors.New("progress write failed"), errors.New("terminal write failed")
			var id acp.ToolCallId
			terminalCount := 0
			a.SetAgentConnection(progressSink(func(ctx context.Context, n acp.SessionNotification) error {
				if start := n.Update.ToolCall; start != nil {
					id = start.ToolCallId
				}
				if update := n.Update.ToolCallUpdate; update != nil {
					if update.Status == nil {
						return first
					}
					terminalCount++
					if !cleaned || ctx.Err() != nil || update.ToolCallId != id || *update.Status != acp.ToolCallStatusFailed {
						t.Error("invalid terminal update or premature cleanup")
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Error("terminal update has no deadline")
					}
					if terminalFails {
						return last
					}
				}
				return nil
			}))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s, err := a.NewSession(ctx, acp.NewSessionRequest{Cwd: testGitDir(t)})
			if err != nil {
				t.Fatal(err)
			}
			resp, err := a.Prompt(ctx, acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
			if !errors.Is(err, first) || resp.StopReason == acp.StopReasonCancelled {
				t.Errorf("write failure became cancellation: %+v %v", resp, err)
			}
			if terminalCount != 1 {
				t.Errorf("want one terminal attempt, got %d", terminalCount)
			}
		})
	}
}

func TestProgressTerminalStates(t *testing.T) {
	for _, kind := range []orchestrator.OutcomeKind{orchestrator.OutcomeFailed, orchestrator.OutcomeCancelled, orchestrator.OutcomeTimedOut} {
		t.Run(string(kind), func(t *testing.T) {
			a := NewAgent("ocr", errorRunner{kind: kind})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var final acp.SessionNotification
			a.SetAgentConnection(progressSink(func(ctx context.Context, n acp.SessionNotification) error {
				if n.Update.ToolCallUpdate != nil {
					if ctx.Err() != nil {
						t.Fatal("terminal update uses cancelled context")
					}
					final = n
				}
				return nil
			}))
			_, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
			if err != nil {
				t.Fatal(err)
			}
			update := final.Update.ToolCallUpdate
			data, _ := json.Marshal(update)
			if update == nil || update.Status == nil || *update.Status != acp.ToolCallStatusFailed || !strings.HasPrefix(*update.Title, "OCR review · "+outcomeLabel(kind)+" · ") || !strings.Contains(string(data), string(kind)) {
				t.Fatalf("missing failed state: %+v", update)
			}
		})
	}
}

func TestProgressCancellationFinishesToolAfterCleanup(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[timeout], func(t *testing.T) {
			parent, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			if timeout {
				var expire context.CancelFunc
				ctx, expire = context.WithDeadline(parent, time.Now().Add(-time.Second))
				defer expire()
			}
			r := cleanupRunner{make(chan struct{}), make(chan struct{}), make(chan struct{})}
			a := NewAgent("ocr", r)
			var final acp.SessionNotification
			a.SetAgentConnection(progressSink(func(ctx context.Context, n acp.SessionNotification) error {
				if n.Update.ToolCallUpdate != nil {
					if ctx.Err() != nil {
						t.Error("terminal update context expired")
					}
					final = n
				}
				return nil
			}))
			done := make(chan error, 1)
			go func() {
				_, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
				done <- err
			}()
			<-r.ready
			cancel()
			<-r.cancelled
			select {
			case <-done:
				t.Fatal("returned before cleanup")
			default:
			}
			close(r.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			want := "cancelled"
			if timeout {
				want = "timed_out"
			}
			update := final.Update.ToolCallUpdate
			data, _ := json.Marshal(update)
			if update == nil || *update.Status != acp.ToolCallStatusFailed || !strings.HasPrefix(*update.Title, "OCR review · "+outcomeLabel(orchestrator.OutcomeKind(want))+" · ") || !strings.Contains(string(data), want) {
				t.Fatalf("incorrect terminal state: %+v", update)
			}
		})
	}
}

func TestExecutionStatsFromCLIJSON(t *testing.T) {
	for _, operation := range []string{"review", "scan"} {
		t.Run(operation, func(t *testing.T) {
			const payload = `{"summary":{"total_tokens":14,"input_tokens":10,"output_tokens":0,"cache_read_tokens":3,"cache_write_tokens":1},"tool_calls":{"total":3,"by_tool":{"read_file":2,"code_search":1}}}`
			result := &orchestrator.Result{}
			if operation == "review" {
				result.Review = &contract.ReviewResult{}
				if err := json.Unmarshal([]byte(payload), result.Review); err != nil {
					t.Fatal(err)
				}
			} else {
				result.Scan = &contract.ScanResult{}
				if err := json.Unmarshal([]byte(payload), result.Scan); err != nil {
					t.Fatal(err)
				}
			}
			got := executionStats(result)
			for _, want := range []string{"3 calls", "code\\_search × 1", "read\\_file × 2", "14 total", "10 input", "0 output", "3 cache read", "1 cache write"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q: %s", want, got)
				}
			}
			if strings.Index(got, "code") > strings.Index(got, "read") {
				t.Errorf("tool counts are not deterministic: %s", got)
			}
		})
	}
	for _, result := range []*orchestrator.Result{nil, {}, {Review: &contract.ReviewResult{Summary: &contract.Summary{}}}} {
		if got := executionStats(result); got != "" {
			t.Errorf("missing statistics invented: %s", got)
		}
	}
}

func TestFinalProgressIncludesStatsOnlyInDetails(t *testing.T) {
	runner := progressRunner(func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
		events := make(chan orchestrator.Event)
		close(events)
		outcomes := make(chan orchestrator.Outcome, 1)
		outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted, Result: &orchestrator.Result{Scan: &contract.ScanResult{Status: "success", ToolCalls: &contract.ToolCalls{Total: 2, ByTool: map[string]int64{"read_file": 2}}, Summary: &contract.Summary{TotalTokens: 8}}}}
		close(outcomes)
		return events, outcomes
	})
	a := NewAgent("ocr", runner)
	var final acp.SessionNotification
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		if n.Update.ToolCallUpdate != nil {
			final = n
		}
		if chat := n.Update.AgentMessageChunk; chat != nil && strings.Contains(chat.Content.Text.Text, "Tool usage:") {
			t.Error("details duplicated in report")
		}
		return nil
	}))
	s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/scan")}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(final)
	for _, want := range []string{"OCR scan · Completed", "2 calls", "8 total", "Working directory:", "Command:"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("final tool missing %q: %s", want, data)
		}
	}
}

func TestIdleProgressUpdatesElapsedWithoutRepeatingContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	visible := make(chan struct{})
	runner := progressRunner(func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
		events := make(chan orchestrator.Event)
		outcomes := make(chan orchestrator.Outcome, 1)
		go func() {
			select {
			case <-visible:
			case <-ctx.Done():
			}
			close(events)
			outcomes <- orchestrator.Outcome{Kind: orchestrator.OutcomeCompleted}
			close(outcomes)
		}()
		return events, outcomes
	})
	seen := false
	a := NewAgent("ocr", runner)
	a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		if update := n.Update.ToolCallUpdate; update != nil && update.Status == nil && !seen {
			if update.Title == nil || strings.HasSuffix(*update.Title, " · 0s") {
				t.Error("elapsed time did not advance")
			}
			if len(update.Content) != 0 {
				t.Error("unchanged log content was resent")
			}
			seen = true
			close(visible)
		}
		return nil
	}))
	if _, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}}); err != nil {
		t.Fatal(err)
	}
	if !seen || ctx.Err() != nil {
		t.Fatal("idle elapsed-time update never arrived")
	}
}

func TestTerminalProgressUsesResultStatus(t *testing.T) {
	for _, tc := range []struct {
		name, status, manifest, want string
		kind                         orchestrator.OutcomeKind
		scan                         bool
	}{
		{"partial", "partial", "partial", "Partial", orchestrator.OutcomeCompleted, false},
		{"manifest", "success", "partial", "Partial", orchestrator.OutcomeCompleted, false},
		{"warnings", "completed_with_warnings", "", "Completed with warnings", orchestrator.OutcomeCompleted, true},
		{"errors", "completed_with_errors", "", "Completed with errors", orchestrator.OutcomeCompleted, false},
		{"skipped", "skipped", "", "Skipped", orchestrator.OutcomeCompleted, false},
		{"cancelled", "partial", "partial", "Cancelled", orchestrator.OutcomeCancelled, false},
		{"failed", "partial", "partial", "Failed", orchestrator.OutcomeFailed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &orchestrator.Result{Review: &contract.ReviewResult{Status: tc.status, Manifest: &contract.Manifest{TerminalState: tc.manifest}}}
			if tc.scan {
				result = &orchestrator.Result{Scan: &contract.ScanResult{Status: tc.status}}
			}
			runner := progressRunner(func(context.Context, orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
				events := make(chan orchestrator.Event)
				close(events)
				outcomes := make(chan orchestrator.Outcome, 1)
				outcomes <- orchestrator.Outcome{Kind: tc.kind, Result: result}
				close(outcomes)
				return events, outcomes
			})
			a := NewAgent("ocr", runner)
			var title string
			a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
				if u := n.Update.ToolCallUpdate; u != nil && u.Title != nil {
					title = *u.Title
				}
				return nil
			}))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := a.collectProgress(ctx, cancel, "test", orchestrator.Request{Args: []string{"review"}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(title, "OCR review · "+tc.want+" · ") {
				t.Fatalf("misleading final title: %s", title)
			}
		})
	}
}
