// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/intent"
	acp "github.com/coder/acp-go-sdk"
)

func TestRejectedGuidancePreservesTerminalFailures(t *testing.T) {
	a := NewAgent("ocr", &captureRunner{})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := a.sendRejection(cancelled, "s", "Use /review.")
	if err != nil || resp.StopReason != acp.StopReasonCancelled || resp.Meta != nil {
		t.Fatalf("cancellation overwritten: %+v, %v", resp, err)
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	resp, err = a.sendRejection(expired, "s", "Use /review.")
	ocr, _ := resp.Meta["ocr"].(map[string]any)
	if err != nil || ocr["kind"] != "timed_out" {
		t.Fatalf("timeout overwritten: %+v, %v", resp, err)
	}
	sink := &discoveryRecorder{err: context.Canceled}
	a.SetAgentConnection(sink)
	resp, err = a.sendRejection(context.Background(), "s", "Use /review.")
	if err != nil || resp.StopReason != acp.StopReasonCancelled || resp.Meta != nil {
		t.Fatalf("transport cancellation overwritten: %+v, %v", resp, err)
	}
	sink.err = errors.New("disconnected")
	if _, err = a.sendRejection(context.Background(), "s", "Use /review."); !errors.Is(err, sink.err) {
		t.Fatalf("send error lost: %v", err)
	}
}

type noticeRecorder struct {
	text    string
	expired bool
}

func (r *noticeRecorder) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	r.expired = ctx.Err() != nil
	if n.Update.AgentMessageChunk != nil {
		r.text += n.Update.AgentMessageChunk.Content.Text.Text
	}
	return nil
}

func TestTimeoutHasVisibleNotice(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{}, intent.NewParser(waitingParser{make(chan struct{})}, nil))
	a.TurnTimeout = 10 * time.Millisecond
	recorder := &noticeRecorder{}
	a.SetAgentConnection(recorder)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	_, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("review please")}})
	if err != nil {
		t.Fatal(err)
	}
	if recorder.expired || !strings.Contains(recorder.text, "timed out") || !strings.Contains(recorder.text, "Retry") {
		t.Fatalf("notice: %+v", recorder)
	}
}

func TestSessionRejectsUnknownCommit(t *testing.T) {
	a := NewAgent("ocr", fakeRunner{})
	recorder := &noticeRecorder{}
	a.SetAgentConnection(recorder)
	s, _ := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	r, err := a.Prompt(context.Background(), acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review --commit missing-ref")}})
	if err != nil {
		t.Fatal(err)
	}
	if r.StopReason != acp.StopReasonEndTurn || !strings.Contains(recorder.text, "could not resolve that ref") || a.sessions[s.SessionId].state.Pending() == nil {
		t.Fatalf("invalid repository ref accepted: %+v", r)
	}
}
