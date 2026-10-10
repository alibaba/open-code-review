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
