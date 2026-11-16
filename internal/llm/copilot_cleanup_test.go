// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

func TestCopilotCleanupRuntime(t *testing.T) {
	tracePath := os.Getenv("OCR_COPILOT_CLEANUP_STUB")
	if tracePath == "" {
		t.Skip("subprocess fixture")
	}
	// The parent can reclaim this port only after the runtime has exited.
	lease, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	defer lease.Close()
	trace, err := os.Create(tracePath)
	if err != nil {
		os.Exit(2)
	}
	defer trace.Close()
	fmt.Fprintln(trace, lease.Addr())
	reader := textproto.NewReader(bufio.NewReader(os.Stdin))
	for {
		headers, err := reader.ReadMIMEHeader()
		if err == io.EOF {
			os.Exit(0)
		}
		if err != nil {
			os.Exit(2)
		}
		length, err := strconv.Atoi(headers.Get("Content-Length"))
		if err != nil || length < 0 {
			os.Exit(2)
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(reader.R, data); err != nil {
			os.Exit(2)
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if json.Unmarshal(data, &request) != nil {
			os.Exit(2)
		}
		fmt.Fprintln(trace, request.Method)
		if request.Method == os.Getenv("OCR_COPILOT_CLEANUP_BLOCK") {
			// Release the parent test if a regression leaves this request stuck.
			time.AfterFunc(12*time.Second, func() { os.Exit(2) })
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		result := map[string]any{"success": true}
		switch request.Method {
		case "connect", "ping":
			result["protocolVersion"] = copilot.GetSDKProtocolVersion()
		case "session.create":
			result["sessionId"] = request.Params["sessionId"]
		case "session.eventLog.registerInterest":
			result["id"] = "interest-1"
		}
		response, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		if _, err := fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(response), response); err != nil {
			os.Exit(2)
		}
	}
}

func TestCopilotCleanupDeadline(t *testing.T) {
	graceful := []string{"session.abort", "session.detach", "sessions.bulkDelete", "runtime.shutdown"}
	for _, blocked := range append([]string{""}, graceful...) {
		t.Run("blocked="+blocked, func(t *testing.T) {
			t.Parallel()
			tracePath := filepath.Join(t.TempDir(), "trace")
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			client := copilot.NewClient(&copilot.ClientOptions{
				Mode: copilot.ModeEmpty, BaseDirectory: dir,
				Connection: copilot.StdioConnection{
					Path: binary, Args: []string{"-test.run=^TestCopilotCleanupRuntime$", "--"},
					Env: append(os.Environ(), "OCR_COPILOT_CLEANUP_STUB="+tracePath, "OCR_COPILOT_CLEANUP_BLOCK="+blocked),
				},
			})
			defer client.ForceStop()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			if err := client.Start(ctx); err != nil {
				t.Fatal(err)
			}
			session, err := client.CreateSession(ctx, &copilot.SessionConfig{AvailableTools: []string{}})
			if err != nil {
				t.Fatal(err)
			}
			owner := &CopilotClient{}
			owner.sessions.Store("cleanup", &copilotConversation{client: client, session: session, stateDir: dir})
			start := time.Now()
			owner.CloseSession("cleanup")
			if elapsed := time.Since(start); elapsed > 7*time.Second {
				t.Errorf("cleanup took %s, exceeding five seconds plus termination margin", elapsed)
			}
			trace, err := os.ReadFile(tracePath)
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Fields(string(trace))
			if len(fields) < 2 {
				t.Fatalf("incomplete runtime trace: %q", trace)
			}
			// ForceStop may return while the OS is finishing process termination.
			deadline := time.Now().Add(2 * time.Second)
			for {
				lease, err := net.Listen("tcp4", fields[0])
				if err == nil {
					lease.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("runtime did not release its listener: %v", err)
				}
				time.Sleep(10 * time.Millisecond)
			}
			methods := fields[1:]
			if blocked == "" {
				first := slices.Index(methods, graceful[0])
				if first < 0 || !slices.Equal(methods[first:], graceful) {
					t.Errorf("unexpected cleanup order: %v", methods)
				}
			} else if !slices.Contains(methods, blocked) {
				t.Errorf("runtime never received %s: %v", blocked, methods)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("state directory remains: %v", err)
			}
			if _, kept := owner.sessions.Load("cleanup"); kept {
				t.Error("owner retained the session")
			}
		})
	}
}
