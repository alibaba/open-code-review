// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

// tuiReviewCommand is a seam for exercising the real subprocess transport with
// the Go test executable. Production always executes this OCR binary directly.
var tuiReviewCommand = exec.Command

func runTUIReview(ctx context.Context, opts reviewOptions, progress func(string)) (jsonOutput, error) {
	var result jsonOutput
	data, runErr := runTUIProcess(ctx, opts, progress)
	if len(data) == 0 {
		if runErr != nil {
			return result, runErr
		}
		return result, errors.New("review process produced no result")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, errors.Join(runErr, fmt.Errorf("decode review result: %w", err))
	}
	// A nonzero exit may still carry a complete manifest and partial findings.
	return result, runErr
}

func runTUIPreview(ctx context.Context, opts reviewOptions) (model.Preview, error) {
	opts.preview = true
	var result model.Preview
	data, runErr := runTUIProcess(ctx, opts, nil)
	if len(data) == 0 {
		if runErr != nil {
			return result, runErr
		}
		return result, errors.New("review process produced no result")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, errors.Join(runErr, fmt.Errorf("decode preview result: %w", err))
	}
	return result, runErr
}

func tuiReviewArgs(opts reviewOptions, address string) []string {
	args := []string{"review", "--format", "json", "--audience", "human", "--color", "never",
		"--tui-control-address", address}
	for _, flag := range []struct{ name, value string }{
		{"repo", opts.repoDir}, {"from", opts.from}, {"to", opts.to}, {"commit", opts.commit},
		{"resume", opts.resume}, {"tools", opts.toolConfigPath}, {"rule", opts.rulePath},
		{"exclude", opts.excludes}, {"background", opts.background}, {"background-file", opts.backgroundFile},
		{"provider", opts.provider}, {"model", opts.model}, {"effort", opts.effort},
	} {
		if flag.value != "" {
			args = append(args, "--"+flag.name, flag.value)
		}
	}
	for _, flag := range []struct {
		name  string
		value int
	}{
		{"concurrency", opts.concurrency}, {"timeout", opts.concurrentTaskTimeout},
		{"max-tools", opts.maxTools}, {"max-git-procs", opts.maxGitProcs},
		{"max-tokens", opts.maxTokens}, {"max-tokens-budget", opts.maxTokensBudget},
	} {
		args = append(args, "--"+flag.name, strconv.Itoa(flag.value))
	}
	if opts.noFilter {
		args = append(args, "--no-filter")
	}
	if opts.preview {
		args = append(args, "--preview")
	}
	return args
}

func runTUIProcess(ctx context.Context, opts reviewOptions, progress func(string)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find OCR executable: %w", err)
	}
	control, err := newTUIControl(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		control.close()
		<-control.done
	}()

	report, err := os.CreateTemp("", "ocr-tui-result-*.json")
	if err != nil {
		return nil, fmt.Errorf("create TUI result file: %w", err)
	}
	reportPath := report.Name()
	defer func() { _ = os.Remove(reportPath) }()
	if err := report.Close(); err != nil {
		return nil, fmt.Errorf("close TUI result file: %w", err)
	}
	args := append(tuiReviewArgs(opts, control.listener.Addr().String()), "--output", reportPath)
	cmd := tuiReviewCommand(executable, args...)
	// Keep the authentication capability out of command-line process listings.
	cmd.Env = append(cmd.Environ(), tuiControlTokenEnv+"="+control.token)
	logs := &tuiProgressWriter{progress: progress}
	// The result is read from its private file. Human JSON progress and warnings
	// use stderr; stdout can contain console telemetry or other structured data.
	cmd.Stdout = io.Discard
	cmd.Stderr = &stripAnsiWriter{dst: logs}
	// The TUI owns terminal input. Credential commands must be noninteractive;
	// os/exec supplies EOF rather than letting helpers race Bubble Tea's reader.
	cmd.Stdin = nil
	// Helpers or MCP grandchildren may retain an inherited stderr handle after
	// OCR has exited. Bound pipe draining after that exit so the UI can finish.
	cmd.WaitDelay = tuiControlTimeout
	// A child context is cancelled by closing the private connection. Do not use
	// CommandContext's default Kill: that bypasses report and session finalization.
	err = cmd.Run()
	logs.flush()
	if err != nil {
		err = fmt.Errorf("review process: %w", err)
		if tail := logs.tail(); tail != "" {
			err = fmt.Errorf("%w\n%s", err, tail)
		}
	}
	data, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		readErr = fmt.Errorf("read TUI result file: %w", readErr)
	}
	return data, errors.Join(err, ctx.Err(), readErr)
}

const tuiControlTimeout = 5 * time.Second
const tuiControlTokenEnv = "OCR_TUI_CONTROL_TOKEN"

// tuiControl is a private authenticated connection whose closure requests a
// graceful child shutdown. It works on Windows without process signal support.
type tuiControl struct {
	listener net.Listener
	token    string
	done     chan struct{}
	stop     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	conn     net.Conn
}

func newTUIControl(ctx context.Context) (*tuiControl, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, fmt.Errorf("create TUI cancellation token: %w", err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("open TUI cancellation connection: %w", err)
	}
	c := &tuiControl{listener: listener, token: hex.EncodeToString(secret[:]), done: make(chan struct{}), stop: make(chan struct{})}
	go c.serve()
	go func() {
		select {
		case <-ctx.Done():
			c.close()
		case <-c.done:
		}
	}()
	return c, nil
}

func (c *tuiControl) serve() {
	defer close(c.done)
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		c.mu.Lock()
		select {
		case <-c.stop:
			c.mu.Unlock()
			_ = conn.Close()
			return
		default:
			c.conn = conn
		}
		c.mu.Unlock()
		_ = conn.SetDeadline(time.Now().Add(tuiControlTimeout))
		provided := make([]byte, len(c.token)+1)
		_, err = io.ReadFull(conn, provided)
		if err != nil || string(provided) != c.token+"\n" {
			_ = conn.Close()
			continue
		}
		if _, err := io.WriteString(conn, "ready\n"); err != nil {
			_ = conn.Close()
			continue
		}
		_ = conn.SetDeadline(time.Time{})
		_ = c.listener.Close()
		<-c.stop
		return
	}
}

func (c *tuiControl) close() {
	c.once.Do(func() {
		close(c.stop)
		_ = c.listener.Close()
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.mu.Unlock()
	})
}

func tuiChildContext(parent context.Context, address, token string) (context.Context, context.CancelFunc, error) {
	if address == "" && token == "" {
		return parent, func() {}, nil
	}
	host, _, err := net.SplitHostPort(address)
	secret, tokenErr := hex.DecodeString(token)
	if err != nil || host != "127.0.0.1" || tokenErr != nil || len(secret) != 32 {
		return nil, nil, fmt.Errorf("invalid internal TUI cancellation connection")
	}
	ctx, cancel := context.WithCancel(parent)
	conn, err := (&net.Dialer{Timeout: tuiControlTimeout}).DialContext(ctx, "tcp4", address)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("connect TUI cancellation connection: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(tuiControlTimeout))
	_, err = io.WriteString(conn, token+"\n")
	if err == nil {
		var ready [6]byte
		_, err = io.ReadFull(conn, ready[:])
		if err == nil && string(ready[:]) != "ready\n" {
			err = fmt.Errorf("unexpected cancellation handshake")
		}
	}
	if err != nil {
		_ = conn.Close()
		cancel()
		return nil, nil, fmt.Errorf("authenticate TUI cancellation connection: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		cancel()
	}()
	return ctx, func() {
		_ = conn.Close()
		cancel()
	}, nil
}

// tuiProgressWriter frames opaque stderr into bounded, sanitized display lines.
// It never infers review state from log wording; the JSON manifest carries it.
type tuiProgressWriter struct {
	mu       sync.Mutex
	pending  []byte
	lines    []string
	progress func(string)
}

func (w *tuiProgressWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			w.emit()
			continue
		}
		w.pending = append(w.pending, b)
		if len(w.pending) >= 4096 {
			w.emit()
		}
	}
	return len(p), nil
}

func (w *tuiProgressWriter) emit() {
	line := strings.TrimSpace(sanitizeTerminal(string(w.pending)))
	w.pending = w.pending[:0]
	if line == "" {
		return
	}
	w.lines = append(w.lines, line)
	if len(w.lines) > 12 {
		w.lines = w.lines[len(w.lines)-12:]
	}
	if w.progress != nil {
		w.progress(line)
	}
}

func (w *tuiProgressWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.emit()
}

func (w *tuiProgressWriter) tail() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.lines, "\n")
}
