package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llmloop"
	mcpclient "github.com/alibaba/open-code-review/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var stdioTestRunCount atomic.Int32

func TestMain(m *testing.M) {
	if os.Getenv("OCR_STDIO_TEST_SERVER") == "1" {
		server := newOCRMCPProtocolServerWithDurations("/repo", stdioTestRunner, 80*time.Millisecond, 500*time.Millisecond)
		if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

func TestOCRMCPStdioIntegration(t *testing.T) {
	t.Run("server deadline returns before host timeout", func(t *testing.T) {
		client := startStdioTestClient(t, "deadline", "")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		result, err := client.CallTool(ctx, mcpReviewToolName, nil)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		for _, want := range []string{
			`"error_type":"deadline_exceeded"`,
			`"stage":"agent_run"`,
			`"path":"internal/mcp.go"`,
			`"partial_result":`,
			`"coverage":`,
			`"session_id":"stdio-session"`,
			`"resumable":true`,
		} {
			if !strings.Contains(result, want) {
				t.Fatalf("result missing %q: %s", want, result)
			}
		}
	})

	t.Run("host cancellation detaches and explicit cancellation reaches runner", func(t *testing.T) {
		marker := t.TempDir() + "/review"
		client := startStdioTestClient(t, "cancel", marker)

		ctx, cancel := context.WithCancel(context.Background())
		callDone := make(chan error, 1)
		go func() {
			_, err := client.CallTool(ctx, mcpReviewToolName, nil)
			callDone <- err
		}()
		waitForFile(t, marker+".started")
		cancel()

		select {
		case <-callDone:
		case <-time.After(time.Second):
			t.Fatal("cancelled MCP call did not return")
		}

		cancelResult, err := client.CallTool(context.Background(), mcpReviewCancelToolName, nil)
		if err != nil {
			t.Fatalf("cancel tool: %v", err)
		}
		if cancelResult != `{"status":"cancelling"}` {
			t.Fatalf("cancel result = %s", cancelResult)
		}
		waitForFile(t, marker+".cancelled")
		waitResult, err := client.CallTool(context.Background(), mcpReviewWaitToolName, nil)
		if err != nil {
			t.Fatalf("wait after cancellation: %v", err)
		}
		if !strings.Contains(waitResult, `"error_type":"cancelled"`) {
			t.Fatalf("wait result after cancellation = %s", waitResult)
		}

		result, err := client.CallTool(context.Background(), mcpReviewToolName, nil)
		if err != nil {
			t.Fatalf("second CallTool after cancellation: %v", err)
		}
		if result != `{"status":"success"}` {
			t.Fatalf("second result = %s", result)
		}
	})
}

func startStdioTestClient(t *testing.T, mode, marker string) *mcpclient.Client {
	t.Helper()
	env := []string{
		"OCR_STDIO_TEST_SERVER=1",
		"OCR_STDIO_TEST_MODE=" + mode,
	}
	if marker != "" {
		env = append(env, "OCR_STDIO_TEST_MARKER="+marker)
	}
	client, err := mcpclient.NewClient(context.Background(), "stdio-test", os.Args[0], nil, env, "", Version)
	if err != nil {
		t.Fatalf("start stdio MCP client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	hasReview, hasCancel, hasWait := false, false, false
	for _, tool := range client.Tools() {
		switch tool.Name {
		case mcpReviewToolName:
			hasReview = true
		case mcpReviewCancelToolName:
			hasCancel = true
		case mcpReviewWaitToolName:
			hasWait = true
		}
	}
	if len(client.Tools()) != 3 || !hasReview || !hasCancel || !hasWait {
		t.Fatalf("tools = %#v", client.Tools())
	}
	return client
}

func stdioTestRunner(ctx context.Context, _ reviewOptions, out, _ io.Writer, _ llmloop.ProgressFunc, stage reviewStageFunc, _ *reviewWatchdog) error {
	run := stdioTestRunCount.Add(1)
	marker := os.Getenv("OCR_STDIO_TEST_MARKER")
	if marker != "" {
		_ = os.WriteFile(marker+".started", nil, 0o600)
	}
	stage("agent_run", "internal/mcp.go")
	if run == 1 {
		_, _ = io.WriteString(out, `{"status":"partial","session_id":"stdio-session","manifest":{"coverage":{"completed":[{"path":"internal/mcp.go"}]}}}`)
		<-ctx.Done()
		if marker != "" {
			_ = os.WriteFile(marker+".cancelled", []byte(ctx.Err().Error()), 0o600)
		}
		return ctx.Err()
	}
	_, _ = io.WriteString(out, `{"status":"success"}`)
	return nil
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", path)
		}
	}
}
