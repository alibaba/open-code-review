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
	"time"

	"github.com/alibaba/open-code-review/internal/llmloop"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const (
	mcpReviewMaxDuration = 4 * time.Hour
	mcpReviewMinIdle     = 15 * time.Minute
	mcpReviewIdleGrace   = 5 * time.Minute
	mcpReviewToolName    = "ocr_review"
	mcpProgressEventName = "ocr_progress"
)

type ocrReviewInput struct {
	Commit     string   `json:"commit,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Background string   `json:"background,omitempty"`
	Exclude    []string `json:"exclude,omitempty"`
	Resume     string   `json:"resume,omitempty"`
}

type mcpReviewRunner func(context.Context, reviewOptions, io.Writer, llmloop.ProgressFunc, *reviewWatchdog) error

type ocrMCPServer struct {
	repoDir string
	run     mcpReviewRunner
	mu      sync.Mutex
	stderr  sync.Mutex
}

type reviewWatchdog struct {
	ctx      context.Context
	cancel   context.CancelFunc
	activity chan struct{}
	update   chan time.Duration
	done     chan struct{}
	stopOnce sync.Once
	causeMu  sync.Mutex
	cause    string
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
		ctx:      ctx,
		cancel:   cancel,
		activity: make(chan struct{}, 1),
		update:   make(chan time.Duration, 1),
		done:     make(chan struct{}),
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

func (w *reviewWatchdog) SetLLMTimeout(timeout time.Duration) {
	idle := mcpReviewMinIdle
	if timeout > 0 && timeout+mcpReviewIdleGrace > idle {
		idle = timeout + mcpReviewIdleGrace
	}
	select {
	case w.update <- idle:
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
	maxTimer := time.NewTimer(maxDuration)
	defer stopTimer(idleTimer)
	defer stopTimer(maxTimer)

	for {
		select {
		case <-w.activity:
			resetTimer(idleTimer, idleDuration)
		case idleDuration = <-w.update:
			resetTimer(idleTimer, idleDuration)
		case <-idleTimer.C:
			w.setCause(fmt.Sprintf("MCP idle timeout after %s without OCR activity", idleDuration))
			w.cancel()
			return
		case <-maxTimer.C:
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
	server := newOCRMCPProtocolServer(repoDir, nil)
	return server.Run(cmd.Context(), &mcpsdk.StdioTransport{})
}

func newOCRMCPProtocolServer(repoDir string, run mcpReviewRunner) *mcpsdk.Server {
	state := &ocrMCPServer{
		repoDir: repoDir,
		run:     run,
	}
	if state.run == nil {
		state.run = func(ctx context.Context, opts reviewOptions, output io.Writer, progress llmloop.ProgressFunc, watchdog *reviewWatchdog) error {
			return executeReviewContext(ctx, opts, output, io.Discard, progress, watchdog)
		}
	}

	server := mcpsdk.NewServer(
		&mcpsdk.Implementation{Name: "open-code-review", Version: Version},
		&mcpsdk.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
	)
	server.AddTool(&mcpsdk.Tool{
		Name:        mcpReviewToolName,
		Description: "Run one blocking OpenCodeReview review for this MCP server's current Git worktree. The call returns only after the review is complete, failed, cancelled, or timed out. Use commit or from+to for resumable reviews; omit both to review current workspace changes.",
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
	return server
}

func (s *ocrMCPServer) handleReview(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	input, err := decodeOCRReviewInput(req)
	if err != nil {
		return mcpToolError(err, nil), nil
	}
	if err := validateOCRReviewInput(input); err != nil {
		return mcpToolError(err, nil), nil
	}
	if !s.mu.TryLock() {
		return mcpToolError(errors.New("ocr_review is already running for this worktree"), nil), nil
	}
	defer s.mu.Unlock()

	watchdog := newReviewWatchdog(ctx, mcpReviewMaxDuration, mcpReviewMinIdle)
	defer watchdog.Stop()
	progress := func(event llmloop.ProgressEvent) {
		watchdog.Activity()
		s.writeProgress(event)
	}
	var output bytes.Buffer
	opts := input.reviewOptions(s.repoDir)
	err = s.run(watchdog.Context(), opts, &output, progress, watchdog)
	if cause := watchdog.Cause(); cause != "" {
		if err == nil {
			err = errors.New(cause)
		} else {
			err = fmt.Errorf("%w (%s)", err, cause)
		}
	}
	if err == nil && ctx.Err() != nil {
		err = fmt.Errorf("review cancelled: %w", ctx.Err())
	}
	if err != nil {
		return mcpToolError(err, output.Bytes()), nil
	}
	raw, err := singleJSONObject(output.Bytes())
	if err != nil {
		return mcpToolError(fmt.Errorf("ocr review returned invalid JSON: %w", err), output.Bytes()), nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(raw)}},
	}, nil
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

func mcpToolError(err error, raw []byte) *mcpsdk.CallToolResult {
	diagnostic := map[string]any{"error": err.Error()}
	if len(bytes.TrimSpace(raw)) > 0 {
		var result map[string]json.RawMessage
		if json.Unmarshal(raw, &result) == nil && result != nil {
			diagnostic["result"] = json.RawMessage(bytes.TrimSpace(raw))
			if sessionID, ok := result["session_id"]; ok {
				var id string
				if json.Unmarshal(sessionID, &id) == nil && id != "" {
					diagnostic["session_id"] = id
				}
			}
		} else {
			diagnostic["output"] = string(raw)
		}
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
