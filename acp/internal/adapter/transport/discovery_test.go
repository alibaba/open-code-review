// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package transport

import (
	"bytes"
	"encoding/json"
	"errors"
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
			id := acp.SessionId("s-1")
			claimed := false
			claim := func(sessionID acp.SessionId) bool {
				if sessionID != id || tc.closing || tc.shutdown || claimed {
					return false
				}
				claimed = true
				return true
			}
			out := &discoveryOutput{failAt: tc.failAt, short: tc.short}
			w := &discoveryWriter{output: &timedWriter{output: out, timeout: time.Second}, claim: claim}
			frame, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": acp.NewSessionResponse{SessionId: id}})
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
