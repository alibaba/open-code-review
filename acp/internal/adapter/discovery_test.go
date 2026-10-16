// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func TestCommandDiscoveryClaimsLiveSessionOnce(t *testing.T) {
	for _, name := range []string{"live", "closing", "shutdown"} {
		t.Run(name, func(t *testing.T) {
			a := NewAgent("unused", &captureRunner{})
			s, err := a.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if a.claimCommandDiscovery("missing") {
				t.Fatal("unknown session claimed discovery")
			}
			a.sessions[s.SessionId].closing = name == "closing"
			a.closed = name == "shutdown"
			const attempts = 32
			claimed := make(chan bool, attempts)
			for range attempts {
				go func() { claimed <- a.claimCommandDiscovery(s.SessionId) }()
			}
			count := 0
			for range attempts {
				if <-claimed {
					count++
				}
			}
			want := 0
			if name == "live" {
				want = 1
			}
			if count != want {
				t.Fatalf("discovery claims = %d, want %d", count, want)
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
