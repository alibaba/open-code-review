package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llmloop"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOCRMCPServerListsOnlyOCRReview(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, out io.Writer, _ llmloop.ProgressFunc, _ *reviewWatchdog) error {
		_, _ = io.WriteString(out, `{"status":"success"}`)
		return nil
	})
	defer stop()

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "ocr_review" {
		t.Fatalf("tools = %#v, want only ocr_review", result.Tools)
	}

	schema, ok := result.Tools[0].InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema type = %T", result.Tools[0].InputSchema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v", schema["properties"])
	}
	for _, name := range []string{"commit", "from", "to", "background", "exclude", "resume"} {
		if _, ok := properties[name]; !ok {
			t.Errorf("schema is missing %q", name)
		}
	}
	if _, ok := properties["repo"]; ok {
		t.Error("schema must not expose repo")
	}
}

func TestOCRMCPReviewReturnsToolErrorWithSession(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, out io.Writer, _ llmloop.ProgressFunc, _ *reviewWatchdog) error {
		_, _ = io.WriteString(out, `{"status":"failed","session_id":"session-123"}`)
		return errors.New("review failed: context canceled")
	})
	defer stop()

	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "ocr_review",
		Arguments: map[string]any{"commit": "abc123"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
	text := toolText(result)
	if !strings.Contains(text, "session-123") || !strings.Contains(text, "context canceled") {
		t.Fatalf("tool error = %q", text)
	}
}

func TestOCRMCPReviewRejectsUnknownInput(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, _ io.Writer, _ llmloop.ProgressFunc, _ *reviewWatchdog) error {
		t.Fatal("runner must not run for invalid input")
		return nil
	})
	defer stop()

	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "ocr_review",
		Arguments: map[string]any{"repo": "/tmp/other-worktree"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError || !strings.Contains(toolText(result), "unknown field") {
		t.Fatalf("result = %#v, text = %q", result, toolText(result))
	}
}

func TestOCRMCPReviewRejectsConcurrentCall(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, out io.Writer, _ llmloop.ProgressFunc, _ *reviewWatchdog) error {
		close(started)
		<-release
		_, _ = io.WriteString(out, `{"status":"success"}`)
		return nil
	})
	defer stop()

	firstDone := make(chan *mcpsdk.CallToolResult, 1)
	go func() {
		result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "ocr_review"})
		if err != nil {
			firstDone <- &mcpsdk.CallToolResult{IsError: true, Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}}}
			return
		}
		firstDone <- result
	}()
	<-started

	second, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "ocr_review"})
	if err != nil {
		t.Fatalf("second CallTool: %v", err)
	}
	if !second.IsError || !strings.Contains(toolText(second), "already running") {
		t.Fatalf("second result = %#v, text = %q", second, toolText(second))
	}

	close(release)
	first := <-firstDone
	if first.IsError {
		t.Fatalf("first result = %#v", first)
	}
}

func TestReviewWatchdogResetsOnActivity(t *testing.T) {
	w := newReviewWatchdog(context.Background(), 500*time.Millisecond, 35*time.Millisecond)
	defer w.Stop()

	for range 4 {
		time.Sleep(15 * time.Millisecond)
		w.Activity()
	}
	select {
	case <-w.Context().Done():
		t.Fatal("watchdog canceled while activity continued")
	default:
	}

	select {
	case <-w.Context().Done():
		if !strings.Contains(w.Cause(), "idle timeout") {
			t.Fatalf("cause = %q", w.Cause())
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchdog did not cancel after idle period")
	}
}

func connectTestOCRServer(t *testing.T, run mcpReviewRunner) (*mcpsdk.ClientSession, func()) {
	t.Helper()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	server := newOCRMCPProtocolServer("/repo", run)
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(context.Background(), serverTransport) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "v1"}, nil)
	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return cs, func() {
		_ = cs.Close()
		select {
		case <-serverDone:
		case <-time.After(time.Second):
			t.Error("MCP server did not stop")
		}
	}
}

func toolText(result *mcpsdk.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if text, ok := result.Content[0].(*mcpsdk.TextContent); ok {
		return text.Text
	}
	return ""
}
