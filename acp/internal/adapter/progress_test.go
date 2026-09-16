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
	if got := progressTitle("  "); got != "OCR progress" {
		t.Fatalf("unexpected empty title: %q", got)
	}
	if got := progressTitle("previous line\n[ocr] reading [file](url)"); got != "OCR progress · "+markdownLabel("reading [file](url)") {
		t.Fatalf("latest activity not escaped: %q", got)
	}
	for _, message := range []string{
		"[ocr] a\tb\r c\u0085d\u202ee\u2028f\u2029",
		strings.Repeat("\u754c", 200),
		strings.Repeat("*", 200),
		"invalid\xff",
	} {
		title := progressTitle(message)
		if !utf8.ValidString(title) || strings.ContainsAny(title, "\r\n\t\u0085\u202e\u2028\u2029") || len(title) > 350 {
			t.Fatalf("unsafe or unbounded title: %q", title)
		}
	}
}

func TestExecutionCommandPrecedesToolAndDirectorySurvivesUpdates(t *testing.T) {
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
	wantCommand := executionCommand("/tools/my ocr", runner.requests[0])
	if first := notices[0].Update.AgentMessageChunk; first == nil || first.Content.Text.Text != wantCommand {
		t.Fatalf("command not visible before tool: %+v", notices[0])
	}
	var directory strings.Builder
	directory.WriteString("Working directory:\n\n")
	writeCodeBlock(&directory, "", runner.requests[0].CWD)
	want, _ := json.Marshal(acp.ToolContent(acp.TextBlock(directory.String())))
	wantTitle := "OCR progress"
	var snapshots int
	for _, n := range notices[1:] {
		if chat := n.Update.AgentMessageChunk; chat != nil && strings.Contains(chat.Content.Text.Text, "Command:") {
			t.Fatal("command was repeated")
		}
		var content []acp.ToolCallContent
		if start := n.Update.ToolCall; start != nil {
			if start.Title != wantTitle || start.Kind != acp.ToolKindOther {
				t.Fatalf("start title is not actual command: %q", start.Title)
			}
			content = start.Content
		}
		if update := n.Update.ToolCallUpdate; update != nil {
			if update.Title == nil || *update.Title != "OCR progress · completed" {
				t.Fatalf("command changed at completion: %q", *update.Title)
			}
			content = update.Content
		}
		if content == nil {
			continue
		}
		got, _ := json.Marshal(content[0])
		if string(got) != string(want) {
			t.Fatalf("working directory changed: %s; want %s", got, want)
		}
		snapshots++
	}
	if snapshots < 2 {
		t.Fatal("working directory missing from start or final update")
	}
	if !strings.Contains(wantCommand, "--effort low") || !strings.Contains(wantCommand, "'/tools/my ocr'") || strings.Contains(wantCommand, "Working directory:") || strings.Contains(string(want), "Command:") {
		t.Fatalf("command and directory are not separated: %s / %s", wantCommand, want)
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
			start := notices[1].Update.ToolCall
			finish := notices[len(notices)-2].Update.ToolCallUpdate
			if start == nil || finish == nil || start.ToolCallId != finish.ToolCallId || start.Status != acp.ToolCallStatusInProgress || finish.Status == nil || *finish.Status != acp.ToolCallStatusCompleted {
				t.Fatalf("incorrect lifecycle: %+v", notices)
			}
			data, _ := json.Marshal(finish)
			if !strings.Contains(string(data), `first\nsecond\n`) || !strings.Contains(string(data), "warning") || !strings.Contains(string(data), "truncated") {
				t.Fatalf("missing logs: %s", data)
			}
			for _, notice := range notices[1 : len(notices)-1] {
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
	for _, failure := range []string{"command", "tool start"} {
		t.Run(failure, func(t *testing.T) {
			r := &captureRunner{}
			a := NewAgent("ocr", r)
			want := errors.New("output unavailable")
			attempts := 0
			a.SetAgentConnection(progressSink(func(_ context.Context, n acp.SessionNotification) error {
				attempts++
				if failure == "command" || n.Update.ToolCall != nil {
					return want
				}
				return nil
			}))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := a.collectProgress(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
			if !errors.Is(err, want) || len(r.requests) != 0 {
				t.Fatalf("OCR started after output failure: %v", err)
			}
			if failure == "command" && attempts != 1 {
				t.Fatal("tool notification followed failed command output")
			}
		})
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
			events <- orchestrator.Event{Message: "READY"}
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
		if chat := n.Update.AgentMessageChunk; chat != nil && !strings.HasPrefix(chat.Content.Text.Text, "Command:\n\n") {
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
		events <- orchestrator.Event{Message: "READY"}
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
				events <- orchestrator.Event{Message: "READY"}
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
			if update == nil || update.Status == nil || *update.Status != acp.ToolCallStatusFailed || *update.Title != "OCR progress · "+string(kind) || !strings.Contains(string(data), string(kind)) {
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
			if update == nil || *update.Status != acp.ToolCallStatusFailed || *update.Title != "OCR progress · "+want || !strings.Contains(string(data), want) {
				t.Fatalf("incorrect terminal state: %+v", update)
			}
		})
	}
}
