// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

type discoveryOutput struct {
	bytes.Buffer
	calls  int
	failAt int
	short  bool
	closed bool
}

func (w *discoveryOutput) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		if w.short {
			return 0, nil
		}
		return 0, errors.New("disconnected")
	}
	return w.Buffer.Write(p)
}
func (w *discoveryOutput) Close() error { w.closed = true; return nil }

func TestDiscoveryWriterFailureAndDuplicateBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		failAt                   int
		short, closing, shutdown bool
	}{
		{name: "normal"}, {name: "response error", failAt: 1}, {name: "response short", failAt: 1, short: true},
		{name: "notification error", failAt: 2}, {name: "notification short", failAt: 2, short: true},
		{name: "closing", closing: true}, {name: "shutdown", shutdown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAgent("unused", &captureRunner{})
			s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			a.sessions[s.SessionId].closing = tc.closing
			a.closed = tc.shutdown
			out := &discoveryOutput{failAt: tc.failAt, short: tc.short}
			w := &discoveryWriter{output: &timedWriter{output: out, timeout: time.Second}, agent: a}
			frame, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": s})
			if err != nil {
				t.Fatal(err)
			}
			_, err = w.Write(append(frame, '\n'))
			if tc.failAt != 0 {
				if err == nil || !out.closed {
					t.Fatalf("failure not aborting: %v closed=%v", err, out.closed)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if tc.closing || tc.shutdown {
				want = 1
			}
			if out.calls != want {
				t.Fatalf("writes=%d want=%d", out.calls, want)
			}
			if _, err = w.Write(frame); err != nil {
				t.Fatal(err)
			}
			if out.calls != want+1 {
				t.Fatal("duplicate discovery")
			}
			for _, ignored := range []string{`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"missing"}}`, `{"jsonrpc":"2.0","id":3,"error":{"code":-1}}`, `not json`} {
				if _, err = w.Write([]byte(ignored)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestWireCommandsFollowSessionResponse(t *testing.T) {
	input, clientWrite := io.Pipe()
	clientRead, output := io.Pipe()
	defer input.Close()
	defer clientWrite.Close()
	defer clientRead.Close()
	defer output.Close()
	runner := &captureRunner{}
	a := NewAgent("unused", runner)
	conn := NewConnection(a, output, input)
	defer func() { clientWrite.Close(); <-conn.Done() }()
	encoder, decoder := json.NewEncoder(clientWrite), json.NewDecoder(clientRead)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": 1}}); err != nil {
			t.Error(err)
			return
		}
		var initialized map[string]any
		if err := decoder.Decode(&initialized); err != nil {
			t.Error(err)
			return
		}
		seen := map[acp.SessionId]bool{}
		for i := 1; i <= 3; i++ {
			if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": i, "method": "session/new", "params": acp.NewSessionRequest{Cwd: t.TempDir(), McpServers: []acp.McpServer{}}}); err != nil {
				t.Error(err)
				return
			}
			var response struct {
				ID     int                    `json:"id"`
				Result acp.NewSessionResponse `json:"result"`
				Method string                 `json:"method"`
			}
			if err := decoder.Decode(&response); err != nil {
				t.Error(err)
				return
			}
			if response.Method != "" || response.ID != i || response.Result.SessionId == "" {
				t.Errorf("notification preceded registration: %+v", response)
				return
			}
			id := response.Result.SessionId
			if seen[id] {
				t.Error("duplicate session")
				return
			}
			seen[id] = true
			var notification struct {
				Method string                  `json:"method"`
				Params acp.SessionNotification `json:"params"`
			}
			if err := decoder.Decode(&notification); err != nil {
				t.Error(err)
				return
			}
			commands := notification.Params.Update.AvailableCommandsUpdate
			if notification.Method != "session/update" || notification.Params.SessionId != id || commands == nil {
				t.Errorf("incorrect notification: %+v", notification)
				return
			}
			if len(commands.AvailableCommands) != 2 {
				t.Error("unexpected command count")
				return
			}
			for index, name := range []string{"review", "scan"} {
				command := commands.AvailableCommands[index]
				if command.Name != name || command.Description == "" || command.Input == nil || command.Input.Unstructured == nil || command.Input.Unstructured.Hint == "" {
					t.Errorf("invalid command: %+v", command)
					return
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("session discovery blocked")
	}
	if len(runner.requests) != 0 {
		t.Fatal("discovery started OCR")
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
