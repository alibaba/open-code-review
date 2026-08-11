// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOCRMCPServerListsReviewTool(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, out, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
		_, _ = io.WriteString(out, `{"status":"success"}`)
		return nil
	})
	defer stop()

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	var reviewTool *mcpsdk.Tool
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.Name == mcpReviewToolName {
			reviewTool = tool
		}
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "ocr_review,ocr_review_cancel,ocr_review_wait" {
		t.Fatalf("tools = %#v, want ocr_review,ocr_review_cancel,ocr_review_wait", names)
	}
	if reviewTool == nil {
		t.Fatal("ocr_review tool is missing")
	}

	schema, ok := reviewTool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema type = %T", reviewTool.InputSchema)
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

func TestOCRMCPReviewDetachesFromCallerAndCanBeCancelled(t *testing.T) {
	started := make(chan context.Context, 1)
	cs, stop := connectTestOCRServer(t, func(ctx context.Context, _ reviewOptions, _ io.Writer, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
		started <- ctx
		<-ctx.Done()
		return ctx.Err()
	})
	defer stop()

	callCtx, cancelCall := context.WithCancel(context.Background())
	firstDone := make(chan *mcpsdk.CallToolResult, 1)
	go func() {
		result, err := cs.CallTool(callCtx, &mcpsdk.CallToolParams{Name: mcpReviewToolName})
		if err != nil {
			firstDone <- &mcpsdk.CallToolResult{IsError: true, Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}}}
			return
		}
		firstDone <- result
	}()

	reviewCtx := <-started
	cancelCall()
	select {
	case <-reviewCtx.Done():
		t.Fatal("caller cancellation canceled the detached review")
	case <-time.After(30 * time.Millisecond):
	}

	cancelResult, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewCancelToolName})
	if err != nil {
		t.Fatalf("cancel CallTool: %v", err)
	}
	if cancelResult.IsError || toolText(cancelResult) != `{"status":"cancelling"}` {
		t.Fatalf("cancel result = %#v, text = %q", cancelResult, toolText(cancelResult))
	}

	select {
	case <-reviewCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("explicit cancellation did not stop the review")
	}

	wait, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewWaitToolName})
	if err != nil {
		t.Fatalf("wait CallTool: %v", err)
	}
	if !wait.IsError || !strings.Contains(toolText(wait), `"error_type":"cancelled"`) {
		t.Fatalf("wait result = %#v, text = %q", wait, toolText(wait))
	}
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("ocr_review did not finish after explicit cancellation")
	}
}

func TestOCRMCPReviewWaitsAfterCallerCancellation(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	cs, stop := connectTestOCRServer(t, func(ctx context.Context, _ reviewOptions, out io.Writer, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
		started <- ctx
		select {
		case <-release:
			_, _ = io.WriteString(out, `{"status":"success"}`)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	defer stop()

	callCtx, cancelCall := context.WithCancel(context.Background())
	callDone := make(chan error, 1)
	go func() {
		_, err := cs.CallTool(callCtx, &mcpsdk.CallToolParams{Name: mcpReviewToolName})
		callDone <- err
	}()

	<-started
	cancelCall()
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled MCP call did not return")
	}

	close(release)
	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewWaitToolName})
	if err != nil {
		t.Fatalf("wait CallTool: %v", err)
	}
	if result.IsError || toolText(result) != `{"status":"success"}` {
		t.Fatalf("wait result = %#v, text = %q", result, toolText(result))
	}
}

func TestOCRMCPReviewCancelRequiresReview(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, _ io.Writer, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
		t.Fatal("runner must not run")
		return nil
	})
	defer stop()

	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewCancelToolName})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError || !strings.Contains(toolText(result), "no OCR review") {
		t.Fatalf("result = %#v, text = %q", result, toolText(result))
	}
}

func TestOCRMCPReviewWaitRequiresReview(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, _ io.Writer, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
		t.Fatal("runner must not run")
		return nil
	})
	defer stop()

	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewWaitToolName})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError || !strings.Contains(toolText(result), "no OCR review") {
		t.Fatalf("result = %#v, text = %q", result, toolText(result))
	}
}

func TestOCRMCPReviewReturnsToolErrorWithSession(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, out, diagnostic io.Writer, progress llmloop.ProgressFunc, stage reviewStageFunc, _ *reviewWatchdog) error {
		stage("agent_run", "pkg/a.go")
		progress(llmloop.ProgressEvent{Phase: "llm_response", Path: "pkg/a.go"})
		_, _ = io.WriteString(out, `{"status":"partial","session_id":"session-123","manifest":{"coverage":{"completed":[{"path":"pkg/a.go"}]}}}`)
		_, _ = io.WriteString(diagnostic, "[ocr] Session: session-123 (retry with: --resume session-123)\n")
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
	for _, want := range []string{
		`"error_type":"runner_error"`,
		`"stage":"llm_response"`,
		`"path":"pkg/a.go"`,
		`"last_progress_at":`,
		`"partial_result":`,
		`"coverage":`,
		`"session_id":"session-123"`,
		`"resumable":true`,
		`"diagnostics":`,
		"context canceled",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tool error missing %q: %s", want, text)
		}
	}

	wait, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewWaitToolName})
	if err != nil {
		t.Fatalf("wait CallTool: %v", err)
	}
	if !wait.IsError || toolText(wait) != text {
		t.Fatalf("wait result = %#v, text = %q; want original result", wait, toolText(wait))
	}
}

func TestMCPPersistenceFailureIsNotResumable(t *testing.T) {
	result := mcpToolErrorWithDetails(
		errors.New("finalize session: write session_end"),
		[]byte(`{"status":"partial","session_id":"session-123","manifest":{"coverage":{"completed":[{"path":"pkg/a.go"}]}}}`),
		mcpErrorPersistence,
		reviewDiagnosticSnapshot{},
		nil,
	)
	text := toolText(result)
	if !strings.Contains(text, `"error_type":"persistence_error"`) {
		t.Fatalf("error type = %s", text)
	}
	if !strings.Contains(text, `"resumable":false`) {
		t.Fatalf("persistence failure must not be resumable: %s", text)
	}
}

func TestOCRMCPReviewRejectsUnknownInput(t *testing.T) {
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, _ io.Writer, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
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
	cs, stop := connectTestOCRServer(t, func(_ context.Context, _ reviewOptions, out, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
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

	waitDone := make(chan *mcpsdk.CallToolResult, 1)
	go func() {
		result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: mcpReviewWaitToolName})
		if err != nil {
			t.Errorf("wait CallTool: %v", err)
			waitDone <- nil
			return
		}
		waitDone <- result
	}()
	select {
	case <-waitDone:
		t.Fatal("ocr_review_wait returned before the review finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	first := <-firstDone
	if first.IsError {
		t.Fatalf("first result = %#v", first)
	}
	wait := <-waitDone
	if wait == nil || wait.IsError || toolText(wait) != `{"status":"success"}` {
		t.Fatalf("wait result = %#v, text = %q", wait, toolText(wait))
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

func TestReviewWatchdogZeroMaxDurationUsesIdleOnly(t *testing.T) {
	w := newReviewWatchdog(context.Background(), 0, 100*time.Millisecond)
	defer w.Stop()

	time.Sleep(30 * time.Millisecond)
	select {
	case <-w.Context().Done():
		t.Fatalf("watchdog canceled with zero max duration: %s", w.Cause())
	default:
	}
}

func TestReviewWatchdogTriggersWithoutActivity(t *testing.T) {
	w := newReviewWatchdog(context.Background(), 0, 25*time.Millisecond)
	defer w.Stop()

	select {
	case <-w.Context().Done():
		if !strings.Contains(w.Cause(), "idle timeout") {
			t.Fatalf("cause = %q", w.Cause())
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchdog did not cancel without activity")
	}
}

type blockingLLMClient struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingLLMClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	close(c.started)
	select {
	case <-c.release:
		return &llm.ChatResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestLLMRequestKeepsWatchdogPaused(t *testing.T) {
	w := newReviewWatchdog(context.Background(), 0, 25*time.Millisecond)
	defer w.Stop()

	inner := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	client := watchdogLLMClient{inner: inner, watchdog: w}
	done := make(chan error, 1)
	go func() {
		_, err := client.CompletionsWithCtx(w.Context(), llm.ChatRequest{})
		done <- err
	}()

	select {
	case <-inner.started:
	case <-time.After(time.Second):
		t.Fatal("LLM request did not start")
	}

	select {
	case <-w.Context().Done():
		t.Fatalf("watchdog canceled during active LLM request: %s", w.Cause())
	case <-time.After(100 * time.Millisecond):
	}

	close(inner.release)
	if err := <-done; err != nil {
		t.Fatalf("LLM request: %v", err)
	}
}

func TestConcurrentLLMRequestsKeepWatchdogPausedUntilLastReturn(t *testing.T) {
	w := newReviewWatchdog(context.Background(), 0, 25*time.Millisecond)
	defer w.Stop()

	first := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	second := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	firstClient := watchdogLLMClient{inner: first, watchdog: w}
	secondClient := watchdogLLMClient{inner: second, watchdog: w}
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := firstClient.CompletionsWithCtx(w.Context(), llm.ChatRequest{})
		firstDone <- err
	}()
	go func() {
		_, err := secondClient.CompletionsWithCtx(w.Context(), llm.ChatRequest{})
		secondDone <- err
	}()

	for _, started := range []<-chan struct{}{first.started, second.started} {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("LLM request did not start")
		}
	}

	close(first.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first LLM request: %v", err)
	}

	select {
	case <-w.Context().Done():
		t.Fatalf("watchdog canceled while second LLM request was active: %s", w.Cause())
	case <-time.After(100 * time.Millisecond):
	}

	close(second.release)
	if err := <-secondDone; err != nil {
		t.Fatalf("second LLM request: %v", err)
	}

	select {
	case <-w.Context().Done():
		if !strings.Contains(w.Cause(), "idle timeout") {
			t.Fatalf("cause = %q", w.Cause())
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("watchdog did not resume after the last LLM request returned")
	}
}

func TestLLMRequestPreservesPerRequestTimeout(t *testing.T) {
	w := newReviewWatchdog(context.Background(), 0, 200*time.Millisecond)
	defer w.Stop()

	inner := &blockingLLMClient{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	client := watchdogLLMClient{inner: inner, watchdog: w}
	requestCtx, cancel := context.WithTimeout(w.Context(), 25*time.Millisecond)
	defer cancel()

	_, err := client.CompletionsWithCtx(requestCtx, llm.ChatRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("LLM error = %v, want context deadline exceeded", err)
	}
	select {
	case <-w.Context().Done():
		t.Fatalf("watchdog canceled with cause %q", w.Cause())
	default:
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
