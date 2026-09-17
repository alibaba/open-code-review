// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	acp "github.com/coder/acp-go-sdk"
	"io"
	"testing"
	"time"
)

func TestUnreadOutputClosesConnection(t *testing.T) {
	input, peerInput := io.Pipe()
	peerOutput, output := io.Pipe()
	defer input.Close()
	defer peerInput.Close()
	defer peerOutput.Close()
	defer output.Close()
	a := NewAgent("ocr", fakeRunner{})
	c := NewConnection(a, output, input)
	done := make(chan error, 1)
	go func() { _, err := a.reportTimeout(context.Background(), "s-1"); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("notice failure replaced timeout response: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("write did not unblock")
	}
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("transport remained open")
	}
}

func TestTimeoutMetadataSurvivesNoticeFailure(t *testing.T) {
	for _, failure := range []error{errors.New("notice failed"), context.DeadlineExceeded, context.Canceled} {
		a := NewAgent("ocr", fakeRunner{})
		a.SetAgentConnection(failingOutput{err: failure})
		resp, err := a.reportTimeout(context.Background(), "s-1")
		meta, _ := resp.Meta["ocr"].(map[string]any)
		if err != nil || resp.StopReason != acp.StopReasonEndTurn || meta["kind"] != "timed_out" || meta["retryable"] != true {
			t.Fatalf("timeout overwritten by %v: %+v, %v", failure, resp, err)
		}
	}
}

type failingOutput struct {
	cancel context.CancelFunc
	err    error
}

func (f failingOutput) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	if n.Update.AgentMessageChunk == nil {
		return nil
	}
	if f.cancel != nil {
		f.cancel()
	}
	return f.err
}

func TestFinalResultSendFailure(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		a := NewAgent("ocr", fakeRunner{})
		ctx, cancel := context.WithCancel(context.Background())
		failure := errors.New("output failed")
		sink := failingOutput{err: failure}
		if cancelled {
			sink.cancel = cancel
			sink.err = context.Canceled
		}
		s, _ := a.NewSession(ctx, acp.NewSessionRequest{Cwd: testGitDir(t)})
		a.SetAgentConnection(sink)
		r, err := a.Prompt(ctx, acp.PromptRequest{SessionId: s.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
		cancel()
		if cancelled {
			if err != nil || r.StopReason != acp.StopReasonCancelled {
				t.Fatalf("cancelled send: %+v %v", r, err)
			}
		} else if !errors.Is(err, failure) {
			t.Fatalf("send failure swallowed: %v", err)
		}
	}
}
