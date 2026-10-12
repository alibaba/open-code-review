// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	runAsServerEnv         = "_OCR_MCP_TEST_SERVER"
	runAsHangingServerEnv  = "_OCR_MCP_TEST_HANG_SERVER"
	runAsExit3ServerEnv    = "_OCR_MCP_TEST_EXIT3_SERVER"
	runAsSlowExitServerEnv = "_OCR_MCP_TEST_SLOW_EXIT_SERVER"
)

func TestMain(m *testing.M) {
	if raceDetectorEnabled {
		// A race-instrumented child pays ~1s of runtime overhead between
		// seeing stdin EOF and exiting (measured: 12ms plain vs ~1s raced).
		// Tests that need a responsive child to beat its own shutdown budget
		// widen that budget accordingly; tests that only rely on signal death
		// keep the production value and pin it themselves.
		subprocessTerminateDuration = 2500 * time.Millisecond
	}
	switch {
	case os.Getenv(runAsServerEnv) != "":
		runTestMCPServer()
	case os.Getenv(runAsHangingServerEnv) != "":
		runHangingMCPServer()
	case os.Getenv(runAsExit3ServerEnv) != "":
		runExit3MCPServer()
	case os.Getenv(runAsSlowExitServerEnv) != "":
		runSlowExitMCPServer()
	default:
		os.Exit(m.Run())
	}
}

// newTestMCPServer returns a working MCP server with a single echo tool.
func newTestMCPServer() *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "v0.0.1"},
		nil,
	)
	server.AddTool(
		&mcp.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		},
		func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + args.Message}},
			}, nil
		},
	)
	return server
}

func runTestMCPServer() {
	if err := newTestMCPServer().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
	// Exit explicitly with code 0 as soon as stdin EOF ends the server loop,
	// rather than unwinding through the test runtime, whose teardown adds a
	// slow and variable tail under the race detector.
	os.Exit(0)
}

// runHangingMCPServer serves MCP normally but never exits: it ignores SIGTERM
// and blocks forever once stdin closes, so only the SDK's SIGKILL escalation
// can reclaim it. This is the unresponsive-server case from issue #1141.
func runHangingMCPServer() {
	signal.Ignore(syscall.SIGTERM)
	go func() {
		_ = newTestMCPServer().Run(context.Background(), &mcp.StdioTransport{})
	}()
	for {
		// Sleeping keeps a timer pending, which keeps the runtime's deadlock
		// detector quiet after the server goroutine returns on stdin EOF.
		time.Sleep(time.Hour)
	}
}

// runExit3MCPServer behaves like the normal server but exits with status 3
// once stdin closes, so Close deterministically reports a child error.
func runExit3MCPServer() {
	_ = newTestMCPServer().Run(context.Background(), &mcp.StdioTransport{})
	os.Exit(3)
}

// runSlowExitMCPServer models a cooperative but slow server: it serves MCP
// until stdin closes (the parent's Close), then takes a while to wind down
// its own state before exiting cleanly. It ignores SIGTERM so its wind-down
// completes, exiting on its own before the SDK's SIGKILL escalation.
func runSlowExitMCPServer() {
	signal.Ignore(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = newTestMCPServer().Run(context.Background(), &mcp.StdioTransport{})
	}()
	<-done                              // stdin EOF: the parent's Close started
	time.Sleep(1200 * time.Millisecond) // cooperative wind-down
	os.Exit(0)
}

func TestNewClient_Stdio(t *testing.T) {
	requireExec(t)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	c, err := NewClient(ctx, "test-srv", exe, nil, []string{runAsServerEnv + "=1"}, "", "v0.0.1")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	if c.Name() != "test-srv" {
		t.Errorf("Name() = %q, want %q", c.Name(), "test-srv")
	}

	found := false
	for _, tool := range c.Tools() {
		if tool.Name == "echo" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected echo tool in Tools()")
	}

	result, err := c.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result != "echo: hello" {
		t.Errorf("CallTool result = %q, want %q", result, "echo: hello")
	}

	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNewClient_Stdio_BadCommand(t *testing.T) {
	requireExec(t)

	ctx := context.Background()
	_, err := NewClient(ctx, "bad", "/nonexistent/mcp-server-binary", nil, nil, "", "v0.0.1")
	if err == nil {
		t.Fatal("expected error for bad command, got nil")
	}
}

func requireExec(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		t.Skip("unsupported OS for subprocess test")
	}
}
