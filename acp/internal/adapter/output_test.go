// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	acp "github.com/coder/acp-go-sdk"
	"io"
	"strings"
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
		if err == nil {
			t.Fatal("unread output succeeded")
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

type failingOutput struct {
	cancel context.CancelFunc
	err    error
}

func (f failingOutput) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	if n.Update.AgentMessageChunk == nil || strings.HasPrefix(n.Update.AgentMessageChunk.Content.Text.Text, "Command:\n\n") {
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
