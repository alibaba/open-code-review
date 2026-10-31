// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package presentation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

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
	a := Progress{Binary: "ocr", Runner: runner}
	seenActivity, seenDiagnostic := false, false
	a.Notify = notifySink(progressSink(func(_ context.Context, n acp.SessionNotification) error {
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
	if _, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"scan"}}); err != nil {
		t.Fatal(err)
	}
	if !seenActivity || !seenDiagnostic {
		t.Fatal("activity or diagnostic details lost")
	}
}

func TestProgressStartFailureDoesNotRunOCR(t *testing.T) {
	r := &captureRunner{}
	a := Progress{Binary: "ocr", Runner: r}
	want := errors.New("output unavailable")
	attempts := 0
	a.Notify = notifySink(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		attempts++
		if n.Update.ToolCall == nil {
			t.Error("expected execution entry")
		}
		return want
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
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
	a := Progress{Binary: "ocr", Runner: runner}
	seen := false
	var final acp.SessionNotification
	a.Notify = notifySink(progressSink(func(_ context.Context, n acp.SessionNotification) error {
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
	_, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
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
	a := Progress{Binary: "ocr", Runner: runner}
	want := errors.New("write failed")
	a.Notify = notifySink(progressSink(func(_ context.Context, n acp.SessionNotification) error {
		if n.Update.ToolCallUpdate != nil {
			return want
		}
		return nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
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

func TestProgressTerminalStates(t *testing.T) {
	for _, kind := range []orchestrator.OutcomeKind{orchestrator.OutcomeFailed, orchestrator.OutcomeCancelled, orchestrator.OutcomeTimedOut} {
		t.Run(string(kind), func(t *testing.T) {
			a := Progress{Binary: "ocr", Runner: errorRunner{kind: kind}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var final acp.SessionNotification
			a.Notify = notifySink(progressSink(func(ctx context.Context, n acp.SessionNotification) error {
				if n.Update.ToolCallUpdate != nil {
					if ctx.Err() != nil {
						t.Fatal("terminal update uses cancelled context")
					}
					final = n
				}
				return nil
			}))
			_, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
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
			a := Progress{Binary: "ocr", Runner: r}
			var final acp.SessionNotification
			a.Notify = notifySink(progressSink(func(ctx context.Context, n acp.SessionNotification) error {
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
				_, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}})
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
	a := Progress{Binary: "ocr", Runner: runner}
	a.Notify = notifySink(progressSink(func(_ context.Context, n acp.SessionNotification) error {
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
	if _, err := a.Collect(ctx, cancel, "s", orchestrator.Request{Args: []string{"review"}}); err != nil {
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
			a := Progress{Binary: "ocr", Runner: runner}
			var title string
			a.Notify = notifySink(progressSink(func(_ context.Context, n acp.SessionNotification) error {
				if u := n.Update.ToolCallUpdate; u != nil && u.Title != nil {
					title = *u.Title
				}
				return nil
			}))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := a.Collect(ctx, cancel, "test", orchestrator.Request{Args: []string{"review"}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(title, "OCR review · "+tc.want+" · ") {
				t.Fatalf("misleading final title: %s", title)
			}
		})
	}
}
