// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/llmloop"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const (
	// A zero max duration disables the whole-review timer. Per-file rounds,
	// provider limits, idle watchdog, and explicit cancellation remain active.
	mcpReviewMaxDuration    time.Duration = 0
	mcpReviewMinIdle                      = 15 * time.Minute
	mcpReviewToolName                     = "ocr_review"
	mcpReviewCancelToolName               = "ocr_review_cancel"
	mcpReviewWaitToolName                 = "ocr_review_wait"
	mcpProgressEventName                  = "ocr_progress"
)

type ocrReviewInput struct {
	Commit     string   `json:"commit,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Background string   `json:"background,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
	Resume     string   `json:"resume,omitempty"`
}

type mcpReviewRunner func(context.Context, reviewOptions, io.Writer, io.Writer, llmloop.ProgressFunc, reviewStageFunc, *reviewWatchdog) error

type mcpReviewExecution struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	result *mcpsdk.CallToolResult
}

const (
	mcpErrorDeadlineExceeded = "deadline_exceeded"
	mcpErrorCancelled        = "cancelled"
	mcpErrorRunner           = "runner_error"
	mcpErrorInvalidResult    = "invalid_result"
	mcpErrorIntegration      = "integration_error"
	mcpErrorPersistence      = "persistence_error"
)

type reviewDiagnosticSnapshot struct {
	Stage          string
	Path           string
	LastProgressAt string
}

type reviewDiagnostics struct {
	mu           sync.Mutex
	stage        string
	path         string
	lastProgress time.Time
}

func (d *reviewDiagnostics) SetStage(stage, path string) {
	d.mu.Lock()
	d.stage = stage
	d.path = path
	d.mu.Unlock()
}

func (d *reviewDiagnostics) Progress(event llmloop.ProgressEvent) {
	d.mu.Lock()
	d.stage = event.Phase
	d.path = event.Path
	d.lastProgress = time.Now()
	d.mu.Unlock()
}

func (d *reviewDiagnostics) Snapshot() reviewDiagnosticSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	snapshot := reviewDiagnosticSnapshot{
		Stage: d.stage,
		Path:  d.path,
	}
	if !d.lastProgress.IsZero() {
		snapshot.LastProgressAt = d.lastProgress.UTC().Format(time.RFC3339Nano)
	}
	return snapshot
}

type ocrMCPServer struct {
	repoDir      string
	run          mcpReviewRunner
	serverCtx    context.Context
	maxDuration  time.Duration
	idleDuration time.Duration
	mu           sync.Mutex
	active       *mcpReviewExecution
	stderr       sync.Mutex
}

type reviewWatchdog struct {
	ctx           context.Context
	cancel        context.CancelFunc
	activity      chan struct{}
	requestEvents chan watchdogRequestEvent
	done          chan struct{}
	stopOnce      sync.Once
	causeMu       sync.Mutex
	cause         string
	inFlight      atomic.Int64
}

type watchdogRequestEvent struct {
	begin bool
	ack   chan struct{}
}

type watchdogLLMClient struct {
	inner    llm.LLMClient
	watchdog *reviewWatchdog
}

func (c watchdogLLMClient) CompletionsWithCtx(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.watchdog.BeginLLMRequest()
	defer c.watchdog.EndLLMRequest()
	return c.inner.CompletionsWithCtx(ctx, req)
}

func newReviewWatchdog(parent context.Context, maxDuration, idleDuration time.Duration) *reviewWatchdog {
	if maxDuration <= 0 {
		maxDuration = mcpReviewMaxDuration
	}
	if idleDuration <= 0 {
		idleDuration = mcpReviewMinIdle
	}
	ctx, cancel := context.WithCancel(parent)
	w := &reviewWatchdog{
		ctx:           ctx,
		cancel:        cancel,
		activity:      make(chan struct{}, 1),
		requestEvents: make(chan watchdogRequestEvent),
		done:          make(chan struct{}),
	}
	go w.run(maxDuration, idleDuration)
	return w
}

func (w *reviewWatchdog) Context() context.Context { return w.ctx }

func (w *reviewWatchdog) Activity() {
	select {
	case w.activity <- struct{}{}:
	default:
	}
}

func (w *reviewWatchdog) BeginLLMRequest() {
	w.inFlight.Add(1)
	w.requestLLM(true)
}

func (w *reviewWatchdog) EndLLMRequest() {
	w.inFlight.Add(-1)
	w.requestLLM(false)
}

func (w *reviewWatchdog) requestLLM(begin bool) {
	event := watchdogRequestEvent{begin: begin, ack: make(chan struct{})}
	select {
	case w.requestEvents <- event:
		select {
		case <-event.ack:
		case <-w.ctx.Done():
		}
	case <-w.ctx.Done():
	}
}

func (w *reviewWatchdog) Cause() string {
	w.causeMu.Lock()
	defer w.causeMu.Unlock()
	return w.cause
}

func (w *reviewWatchdog) setCause(cause string) {
	w.causeMu.Lock()
	w.cause = cause
	w.causeMu.Unlock()
}

func (w *reviewWatchdog) Stop() {
	w.stopOnce.Do(func() {
		close(w.done)
		w.cancel()
	})
}

func (w *reviewWatchdog) run(maxDuration, idleDuration time.Duration) {
	idleTimer := time.NewTimer(idleDuration)
	var maxTimer *time.Timer
	var maxTimerC <-chan time.Time
	if maxDuration > 0 {
		maxTimer = time.NewTimer(maxDuration)
		maxTimerC = maxTimer.C
	}
	defer stopTimer(idleTimer)
	if maxTimer != nil {
		defer stopTimer(maxTimer)
	}

	for {
		select {
		case <-w.activity:
			if w.inFlight.Load() == 0 {
				resetTimer(idleTimer, idleDuration)
			}
		case event := <-w.requestEvents:
			if event.begin {
				stopTimer(idleTimer)
			} else if w.inFlight.Load() == 0 {
				resetTimer(idleTimer, idleDuration)
			}
			close(event.ack)
		case <-idleTimer.C:
			// BeginLLMRequest increments the counter before publishing its
			// event. Check the counter here so a ready request cannot lose
			// the race to the timer's select case.
			if w.inFlight.Load() > 0 {
				resetTimer(idleTimer, idleDuration)
				continue
			}
			w.setCause(fmt.Sprintf("MCP idle timeout after %s without OCR activity", idleDuration))
			w.cancel()
			return
		case <-maxTimerC:
			w.setCause(fmt.Sprintf("MCP maximum duration exceeded (%s)", maxDuration))
			w.cancel()
			return
		case <-w.done:
			return
		case <-w.ctx.Done():
			return
		}
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	stopTimer(timer)
	timer.Reset(duration)
}

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the MCP server",
}

var mcpServeCmd = &cobra.Command{
	Use:  "serve",
	Args: cobra.NoArgs,
	RunE: runMCPServe,
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPServe(cmd *cobra.Command, _ []string) error {
	repoDir, _, err := resolveWorkingDir("", true)
	if err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}
	server := newOCRMCPProtocolServerWithContext(repoDir, nil, cmd.Context(), mcpReviewMaxDuration, mcpReviewMinIdle)
	return server.Run(cmd.Context(), &mcpsdk.StdioTransport{})
}

func newOCRMCPProtocolServer(repoDir string, run mcpReviewRunner) *mcpsdk.Server {
	return newOCRMCPProtocolServerWithDurations(repoDir, run, mcpReviewMaxDuration, mcpReviewMinIdle)
}

func newOCRMCPProtocolServerWithDurations(repoDir string, run mcpReviewRunner, maxDuration, idleDuration time.Duration) *mcpsdk.Server {
	return newOCRMCPProtocolServerWithContext(repoDir, run, context.Background(), maxDuration, idleDuration)
}

func newOCRMCPProtocolServerWithContext(repoDir string, run mcpReviewRunner, serverCtx context.Context, maxDuration, idleDuration time.Duration) *mcpsdk.Server {
	if serverCtx == nil {
		serverCtx = context.Background()
	}
	state := &ocrMCPServer{
		repoDir:      repoDir,
		run:          run,
		serverCtx:    serverCtx,
		maxDuration:  maxDuration,
		idleDuration: idleDuration,
	}
	if state.run == nil {
		state.run = func(ctx context.Context, opts reviewOptions, output, diagnostic io.Writer, progress llmloop.ProgressFunc, stage reviewStageFunc, watchdog *reviewWatchdog) error {
			return executeReviewContextWithStage(ctx, opts, output, diagnostic, progress, watchdog, stage)
		}
	}

	server := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "open-code-review", Version: Version},
		&mcpsdk.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
	)
	server.AddTool(&mcpsdk.Tool{
		Name:        mcpReviewToolName,
		Description: "Run one blocking OpenCodeReview review for this MCP server's current Git worktree. If the caller disconnects, the server keeps the review running; use ocr_review_wait to retrieve its terminal result. Use ocr_review_cancel for explicit cancellation. Use commit or from+to for resumable reviews; omit both to review current workspace changes.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"commit":     map[string]any{"type": "string", "description": "Commit ref to review against its first parent."},
				"from":       map[string]any{"type": "string", "description": "Base commit or branch for a range review; must be paired with to."},
				"to":         map[string]any{"type": "string", "description": "Head commit or branch for a range review; must be paired with from."},
				"background": map[string]any{"type": "string", "description": "Business or requirement context for the review."},
				"exclude":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Glob patterns to exclude from review."},
				"resume":     map[string]any{"type": "string", "description": "Previous commit/range review session ID to resume."},
			},
		},
	}, state.handleReview)
	server.AddTool(&mcpsdk.Tool{
		Name:        mcpReviewCancelToolName,
		Description: "Cancel the currently running OpenCodeReview call for this worktree. Completed checkpoints remain available for a later resumable commit or range review.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		},
	}, state.handleReviewCancel)
	server.AddTool(&mcpsdk.Tool{
		Name:        mcpReviewWaitToolName,
		Description: "Wait for the current or most recent OpenCodeReview call in this worktree and return its terminal result. Use this after caller disconnection or after ocr_review reports that a review is already running; it does not start or poll a review.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		},
	}, state.handleReviewWait)
	return server
}

func (s *ocrMCPServer) handleReview(ctx context.Context, req *mcpsdk.CallToolRequest) (result *mcpsdk.CallToolResult, err error) {
	input, err := decodeOCRReviewInput(req)
	if err != nil {
		return mcpToolErrorWithDetails(err, nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
	}
	if err := validateOCRReviewInput(input); err != nil {
		return mcpToolErrorWithDetails(err, nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
	}
	execution, ok := s.beginReview()
	if !ok {
		return mcpToolErrorWithDetails(errors.New("ocr_review is already running for this worktree; call ocr_review_wait to await its result"), nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
	}
	defer func() { s.finishReview(execution, result) }()

	reviewCtx := execution.ctx
	watchdog := newReviewWatchdog(reviewCtx, s.maxDuration, s.idleDuration)
	defer watchdog.Stop()

	var diagnostics reviewDiagnostics
	progress := func(event llmloop.ProgressEvent) {
		diagnostics.Progress(event)
		watchdog.Activity()
		s.writeProgress(event)
	}
	stage := diagnostics.SetStage
	var output bytes.Buffer
	var diagnosticOutput bytes.Buffer
	opts := input.reviewOptions(s.repoDir)
	err = s.run(watchdog.Context(), opts, &output, &diagnosticOutput, progress, stage, watchdog)
	if cause := watchdog.Cause(); cause != "" {
		if err == nil {
			err = errors.New(cause)
		} else {
			err = fmt.Errorf("%w (%s)", err, cause)
		}
	}
	if err == nil && reviewCtx.Err() != nil {
		err = fmt.Errorf("review cancelled: %w", reviewCtx.Err())
	}
	if err != nil {
		return mcpToolErrorWithDetails(err, output.Bytes(), classifyMCPReviewError(err, reviewCtx, watchdog), diagnostics.Snapshot(), diagnosticOutput.Bytes()), nil
	}
	raw, err := singleJSONObject(output.Bytes())
	if err != nil {
		return mcpToolErrorWithDetails(fmt.Errorf("ocr review returned invalid JSON: %w", err), output.Bytes(), mcpErrorInvalidResult, diagnostics.Snapshot(), diagnosticOutput.Bytes()), nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(raw)}},
	}, nil
}

func (s *ocrMCPServer) handleReviewWait(ctx context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	s.mu.Lock()
	execution := s.active
	s.mu.Unlock()
	if execution == nil {
		return mcpToolErrorWithDetails(errors.New("no OCR review is running or has completed for this worktree"), nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
	}

	select {
	case <-execution.done:
		if execution.result == nil {
			return mcpToolErrorWithDetails(errors.New("OCR review finished without a terminal result"), nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
		}
		return execution.result, nil
	case <-ctx.Done():
		return mcpToolErrorWithDetails(fmt.Errorf("ocr_review_wait cancelled: %w", ctx.Err()), nil, mcpErrorCancelled, reviewDiagnosticSnapshot{}, nil), nil
	}
}

func (s *ocrMCPServer) handleReviewCancel(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	s.mu.Lock()
	execution := s.active
	s.mu.Unlock()
	if execution == nil {
		return mcpToolErrorWithDetails(errors.New("no OCR review is running for this worktree"), nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
	}
	select {
	case <-execution.done:
		return mcpToolErrorWithDetails(errors.New("no OCR review is running for this worktree"), nil, mcpErrorIntegration, reviewDiagnosticSnapshot{}, nil), nil
	default:
	}
	execution.cancel()
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: `{"status":"cancelling"}`}},
	}, nil
}

func (s *ocrMCPServer) beginReview() (*mcpReviewExecution, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		select {
		case <-s.active.done:
		default:
			return nil, false
		}
	}
	ctx, cancel := context.WithCancel(s.serverCtx)
	execution := &mcpReviewExecution{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	s.active = execution
	return execution, true
}

func (s *ocrMCPServer) finishReview(execution *mcpReviewExecution, result *mcpsdk.CallToolResult) {
	s.mu.Lock()
	execution.result = result
	close(execution.done)
	s.mu.Unlock()
	execution.cancel()
}

func (s *ocrMCPServer) writeProgress(event llmloop.ProgressEvent) {
	event.Event = mcpProgressEventName
	s.stderr.Lock()
	defer s.stderr.Unlock()
	data, err := json.Marshal(event)
	if err == nil {
		_, _ = fmt.Fprintln(os.Stderr, string(data))
	}
}

func decodeOCRReviewInput(req *mcpsdk.CallToolRequest) (ocrReviewInput, error) {
	var raw []byte
	if req != nil && req.Params != nil {
		raw = req.Params.Arguments
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input ocrReviewInput
	if err := decoder.Decode(&input); err != nil {
		return ocrReviewInput{}, fmt.Errorf("invalid ocr_review input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ocrReviewInput{}, errors.New("invalid ocr_review input: multiple JSON values")
		}
		return ocrReviewInput{}, fmt.Errorf("invalid ocr_review input: %w", err)
	}
	return input, nil
}

func validateOCRReviewInput(input ocrReviewInput) error {
	if input.Commit != "" && (input.From != "" || input.To != "") {
		return errors.New("ocr_review input cannot combine commit with from/to")
	}
	if (input.From == "") != (input.To == "") {
		return errors.New("ocr_review input requires both from and to for a range review")
	}
	if input.Resume != "" && input.Commit == "" && input.From == "" {
		return errors.New("ocr_review resume requires commit or from/to; workspace resume is unsupported")
	}
	for _, pattern := range input.Exclude {
		if strings.TrimSpace(pattern) == "" {
			return errors.New("ocr_review input exclude patterns must not be empty")
		}
	}
	return nil
}

func (input ocrReviewInput) reviewOptions(repoDir string) reviewOptions {
	return reviewOptions{
		repoDir:      repoDir,
		from:         input.From,
		to:           input.To,
		commit:       input.Commit,
		resume:       input.Resume,
		excludes:     strings.Join(input.Exclude, ","),
		background:   input.Background,
		outputFormat: "json",
		audience:     "agent",
	}
}

func singleJSONObject(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New("empty output")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("result is not a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return raw, nil
}

func classifyMCPReviewError(err error, reviewCtx context.Context, watchdog *reviewWatchdog) string {
	message := err.Error()
	if strings.Contains(message, "persist") || strings.Contains(message, "session_end") || strings.Contains(message, "finalize session") {
		return mcpErrorPersistence
	}
	if watchdog.Cause() != "" || errors.Is(err, context.DeadlineExceeded) {
		return mcpErrorDeadlineExceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(reviewCtx.Err(), context.Canceled) {
		return mcpErrorCancelled
	}
	return mcpErrorRunner
}

func mcpToolErrorWithDetails(err error, raw []byte, errorType string, snapshot reviewDiagnosticSnapshot, diagnosticOutput []byte) *mcpsdk.CallToolResult {
	diagnostic := map[string]any{
		"error":            err.Error(),
		"error_type":       errorType,
		"stage":            snapshot.Stage,
		"path":             snapshot.Path,
		"last_progress_at": snapshot.LastProgressAt,
		"partial_result":   nil,
		"coverage":         nil,
		"session_id":       "",
		"resumable":        false,
	}
	if result, ok := resultObject(raw); ok {
		diagnostic["partial_result"] = json.RawMessage(bytes.TrimSpace(raw))
		coverage := resultCoverage(result)
		diagnostic["coverage"] = coverage
		if id := resultSessionID(result); id != "" {
			diagnostic["session_id"] = id
		}
		diagnostic["resumable"] = coverageIsResumable(coverage)
		if errorType == mcpErrorPersistence {
			diagnostic["resumable"] = false
		}
	} else if len(bytes.TrimSpace(raw)) > 0 {
		diagnostic["output"] = string(raw)
	}
	if diagnostic["session_id"] == "" {
		if id := diagnosticSessionID(diagnosticOutput); id != "" {
			diagnostic["session_id"] = id
		}
	}
	if len(bytes.TrimSpace(diagnosticOutput)) > 0 {
		diagnostic["diagnostics"] = string(bytes.TrimSpace(diagnosticOutput))
	}
	data, marshalErr := json.Marshal(diagnostic)
	if marshalErr != nil {
		data = []byte(fmt.Sprintf(`{"error":%q}`, marshalErr.Error()))
	}
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}
}

func resultObject(raw []byte) (map[string]json.RawMessage, bool) {
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil || result == nil {
		return nil, false
	}
	return result, true
}

func resultSessionID(result map[string]json.RawMessage) string {
	var id string
	if json.Unmarshal(result["session_id"], &id) == nil {
		return id
	}
	return ""
}

func resultCoverage(result map[string]json.RawMessage) json.RawMessage {
	if coverage, ok := result["coverage"]; ok {
		return coverage
	}
	var manifest map[string]json.RawMessage
	if json.Unmarshal(result["manifest"], &manifest) == nil {
		if coverage, ok := manifest["coverage"]; ok {
			return coverage
		}
	}
	return nil
}

func coverageIsResumable(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var coverage map[string][]json.RawMessage
	if json.Unmarshal(raw, &coverage) != nil {
		return false
	}
	return len(coverage["completed"]) > 0 || len(coverage["reused"]) > 0
}

func diagnosticSessionID(raw []byte) string {
	const marker = "[ocr] Session:"
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, marker)))
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}
