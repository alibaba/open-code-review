// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func useTUIProcessHelper(t *testing.T, mode string) {
	t.Helper()
	old := tuiReviewCommand
	tuiReviewCommand = func(executable string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=^TestTUIReviewProcessHelper$", "--"}, args...)
		cmd := exec.Command(executable, helperArgs...)
		cmd.Env = append(os.Environ(), "OCR_TUI_TEST_CHILD="+mode)
		return cmd
	}
	t.Cleanup(func() { tuiReviewCommand = old })
}

func TestTUIReviewProcessHelper(t *testing.T) {
	mode := os.Getenv("OCR_TUI_TEST_CHILD")
	if mode == "" {
		return
	}
	var address, reportPath string
	for i := 0; i+1 < len(os.Args); i++ {
		switch os.Args[i] {
		case "--tui-control-address":
			address = os.Args[i+1]
		case "--output":
			reportPath = os.Args[i+1]
		}
	}
	ctx, stop, err := tuiChildContext(context.Background(), address, os.Getenv(tuiControlTokenEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer stop()
	report, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer func() { _ = report.Close() }()
	if mode == "preview" {
		_ = json.NewEncoder(report).Encode(model.Preview{
			TotalFiles: 1, ReviewableCount: 1,
			Entries: []model.PreviewEntry{{Path: "sample.go", WillReview: true}},
		})
		os.Exit(0)
	}
	if mode == "invalid" {
		fmt.Fprintln(report, "invalid JSON")
		os.Exit(0)
	}
	if mode == "empty" {
		os.Exit(0)
	}
	if mode == "failure" {
		fmt.Fprintln(os.Stderr, "credential helper needs interactive input")
		os.Exit(1)
	}
	if mode == "stdin" {
		var b [1]byte
		if _, err := os.Stdin.Read(b[:]); !errors.Is(err, io.EOF) {
			fmt.Fprintf(os.Stderr, "expected noninteractive EOF, got %v\n", err)
			os.Exit(2)
		}
	}
	fmt.Fprint(os.Stderr, "\x1b[32mworker started\x1b[0m\n")
	if mode == "extra-stdout" || mode == "cancel" {
		fmt.Fprintln(os.Stdout, `{"telemetry":"console export"}`)
	}
	result := jsonOutput{
		Status: "success", SessionID: "saved-session",
		Comments: []model.LlmComment{{Path: "sample.go", Content: "kept finding"}},
	}
	if mode == "extra-stdout" {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	}
	if mode == "cancel" {
		<-ctx.Done()
		result.Status = "failed"
		result.Manifest = &session.RunManifest{
			TerminalState: session.StateFailed,
			RunFailure:    &session.RunFailure{Classification: session.RunFailureCancelled},
		}
		_ = json.NewEncoder(report).Encode(result)
		_ = json.NewEncoder(os.Stdout).Encode(result)
		fmt.Fprintln(os.Stderr, "review cancelled after saving partial findings")
		os.Exit(1)
	}
	_ = json.NewEncoder(report).Encode(result)
	os.Exit(0)
}

func TestTUIResultFileSeparatesTelemetryStdout(t *testing.T) {
	useTUIProcessHelper(t, "extra-stdout")
	factory := tuiReviewCommand
	var reportPath string
	tuiReviewCommand = func(executable string, args ...string) *exec.Cmd {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--output" {
				reportPath = args[i+1]
			}
		}
		return factory(executable, args...)
	}
	var logs []string
	result, err := runTUIReview(context.Background(), reviewOptions{}, func(line string) { logs = append(logs, line) })
	if err != nil || result.SessionID != "saved-session" {
		t.Fatalf("telemetry corrupted result: %+v, %v", result, err)
	}
	if !reflect.DeepEqual(logs, []string{"worker started"}) {
		t.Fatalf("structured stdout polluted progress: %q", logs)
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatalf("temporary report was not removed: %v", err)
	}
}

func TestTUIReviewSubprocess(t *testing.T) {
	useTUIProcessHelper(t, "success")
	var logs []string
	result, err := runTUIReview(context.Background(), reviewOptions{}, func(line string) { logs = append(logs, line) })
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "saved-session" || len(result.Comments) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(logs, []string{"worker started"}) {
		t.Fatalf("progress is not sanitized: %q", logs)
	}
}

func TestTUIReviewCancellationKeepsResult(t *testing.T) {
	useTUIProcessHelper(t, "cancel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	type response struct {
		result jsonOutput
		err    error
	}
	done := make(chan response, 1)
	go func() {
		result, err := runTUIReview(ctx, reviewOptions{}, func(string) {
			select {
			case started <- struct{}{}:
			default:
			}
		})
		done <- response{result, err}
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("review subprocess did not start")
	}
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("cancellation cause lost: %v", got.err)
		}
		if !strings.Contains(got.err.Error(), "review cancelled after saving partial findings") {
			t.Fatalf("cancellation stderr diagnostic lost: %v", got.err)
		}
		if strings.Contains(got.err.Error(), "console export") || strings.Contains(got.err.Error(), `"comments"`) {
			t.Fatalf("structured stdout polluted the cancellation error: %v", got.err)
		}
		if got.result.SessionID != "saved-session" || len(got.result.Comments) != 1 || got.result.Manifest == nil {
			t.Fatalf("partial result lost: %+v", got.result)
		}
		if got.result.Manifest.RunFailure.Classification != session.RunFailureCancelled {
			t.Fatalf("wrong cancellation manifest: %+v", got.result.Manifest)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("review subprocess did not shut down gracefully")
	}
}

func TestTUIWorkerFailureUsageKeepsReportAndHumanDiagnostics(t *testing.T) {
	for _, worker := range []bool{false, true} {
		t.Run(fmt.Sprintf("worker=%v", worker), func(t *testing.T) {
			isolateLLMConnectionTest(t)
			freshOCRHome(t)
			t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test-model")
			dir := initTestGitRepo(t)
			gitCommitFile(t, dir, "sample.go", "package sample\n", "add sample")
			opts := reviewOptions{
				repoDir: dir, commit: "HEAD", outputFormat: "json", audience: "human",
				outputPath: filepath.Join(t.TempDir(), "report.json"),
			}
			if worker {
				opts.tuiControlAddress = "127.0.0.1:1"
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			diagnostic := captureStderr(t, func() {
				err := executeReviewContext(ctx, opts)
				if !errors.Is(err, context.Canceled) {
					t.Errorf("cancellation cause lost: %v", err)
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			})
			data, err := os.ReadFile(opts.outputPath)
			if err != nil {
				t.Fatal(err)
			}
			var result jsonOutput
			if err := json.Unmarshal(data, &result); err != nil || result.Manifest == nil || result.SessionID == "" {
				t.Fatalf("failed run report lost: %+v, %v", result, err)
			}
			if !strings.Contains(diagnostic, "context canceled") {
				t.Fatalf("failure diagnostic lost: %q", diagnostic)
			}
			if worker {
				if !strings.Contains(diagnostic, "[ocr] usage on failure:") || strings.Contains(diagnostic, `"tool_calls"`) {
					t.Fatalf("worker stderr must contain human usage: %q", diagnostic)
				}
			} else if !strings.Contains(diagnostic, `"tool_calls"`) {
				t.Fatalf("CLI JSON failure usage changed: %q", diagnostic)
			}
		})
	}
}

func TestTUIRangeInputPreservesContextFailure(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%v", deadline), func(t *testing.T) {
			isolateLLMConnectionTest(t)
			freshOCRHome(t)
			t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test-model")
			dir := initTestGitRepo(t)
			gitCommitFile(t, dir, "sample.go", "package sample\n", "add sample")
			opts := reviewOptions{
				repoDir: dir, from: "HEAD", to: "HEAD", outputFormat: "json", audience: "human",
				outputPath: filepath.Join(t.TempDir(), "report.json"), tuiControlAddress: "127.0.0.1:1",
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			wantErr, wantClass := context.Canceled, session.RunFailureCancelled
			if deadline {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()
				wantErr, wantClass = context.DeadlineExceeded, session.RunFailureTimeout
			}
			captureStderr(t, func() {
				if err := executeReviewContext(ctx, opts); !errors.Is(err, wantErr) {
					t.Errorf("context cause lost: got %v, want %v", err, wantErr)
				}
			})
			data, err := os.ReadFile(opts.outputPath)
			if err != nil {
				t.Fatal(err)
			}
			var result jsonOutput
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.Manifest == nil || result.Manifest.RunFailure == nil || result.Manifest.RunFailure.Classification != wantClass {
				t.Fatalf("range input failure must be %s: %+v", wantClass, result.Manifest)
			}
		})
	}
}

func TestTUIPreviewSubprocess(t *testing.T) {
	useTUIProcessHelper(t, "preview")
	preview, err := runTUIPreview(context.Background(), reviewOptions{})
	if err != nil || preview.ReviewableCount != 1 || len(preview.Entries) != 1 {
		t.Fatalf("preview = %+v, error = %v", preview, err)
	}
}

func TestTUIEmptyResultReportsMissingOutput(t *testing.T) {
	useTUIProcessHelper(t, "empty")
	_, reviewErr := runTUIReview(context.Background(), reviewOptions{}, nil)
	_, previewErr := runTUIPreview(context.Background(), reviewOptions{})
	for _, err := range []error{reviewErr, previewErr} {
		if err == nil || !strings.Contains(err.Error(), "review process produced no result") {
			t.Fatalf("missing result diagnostic = %v", err)
		}
	}
}

func TestTUIReviewSubprocessFailures(t *testing.T) {
	for _, mode := range []string{"invalid", "failure", "stdin"} {
		t.Run(mode, func(t *testing.T) {
			useTUIProcessHelper(t, mode)
			_, err := runTUIReview(context.Background(), reviewOptions{}, nil)
			switch mode {
			case "invalid":
				if err == nil || !strings.Contains(err.Error(), "decode review result") {
					t.Fatalf("invalid JSON error = %v", err)
				}
			case "failure":
				if err == nil || !strings.Contains(err.Error(), "credential helper needs interactive input") {
					t.Fatalf("child diagnostic lost: %v", err)
				}
			case "stdin":
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runTUIReview(ctx, reviewOptions{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("already-cancelled review returned %v", err)
	}
}

func TestTUIReviewArgsPreserveLiteralValues(t *testing.T) {
	opts := reviewOptions{
		repoDir: "repo with spaces", from: "main", to: "feature", resume: "previous",
		toolConfigPath: "tools.json", rulePath: "rule.json", excludes: "**/generated/*",
		background: "literal $(command); & text", backgroundFile: "context with spaces.md",
		provider: "custom", model: "custom-model", effort: "high", concurrency: 3,
		concurrentTaskTimeout: 7, maxTools: 60, maxGitProcs: 4, maxTokens: 1200,
		maxTokensBudget: 2300, noFilter: true, preview: true,
		outputPath: "must-not-steal-result.json", outputFormat: "text", audience: "agent",
	}
	args := tuiReviewArgs(opts, "127.0.0.1:1234")
	values := make(map[string]string)
	for i := 1; i < len(args); i++ {
		if args[i] == "--preview" || args[i] == "--no-filter" {
			values[args[i]] = "true"
			continue
		}
		values[args[i]] = args[i+1]
		i++
	}
	for flag, want := range map[string]string{
		"--repo": opts.repoDir, "--from": "main", "--to": "feature", "--resume": "previous",
		"--tools": "tools.json", "--rule": "rule.json", "--exclude": opts.excludes,
		"--background": opts.background, "--background-file": opts.backgroundFile,
		"--provider": "custom", "--model": "custom-model", "--effort": "high",
		"--concurrency": "3", "--timeout": "7", "--max-tools": "60", "--max-git-procs": "4",
		"--max-tokens": "1200", "--max-tokens-budget": "2300", "--no-filter": "true",
		"--preview": "true", "--format": "json", "--audience": "human", "--color": "never",
	} {
		if values[flag] != want {
			t.Errorf("%s = %q, want %q", flag, values[flag], want)
		}
	}
	if _, ok := values["--output"]; ok {
		t.Fatal("user output path must not replace the private result file")
	}
	if got := tuiReviewArgs(reviewOptions{commit: "HEAD"}, "address"); !strings.Contains(strings.Join(got, " "), "--commit HEAD") {
		t.Fatalf("commit scope missing: %q", got)
	}
}

func TestTUITelemetryInitializationUsesResolvedCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"tui"}, false},
		{[]string{"--color", "never", "tui", "--background", "changes"}, false},
		{[]string{"review", "--background", "tui"}, true},
		{[]string{"r", "--model", "tui"}, true},
		{[]string{"config", "model"}, true},
		{[]string{"unknown-command"}, true},
	} {
		if got := shouldInitializeTelemetry(tc.args); got != tc.want {
			t.Errorf("args %q: initialize telemetry = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestTUIControlAuthenticatesAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	control, err := newTUIControl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(control.close)
	address := control.listener.Addr().String()
	if _, _, err := tuiChildContext(context.Background(), address, strings.Repeat("00", 32)); err == nil {
		t.Fatal("invalid token was accepted")
	}
	child, stop, err := tuiChildContext(context.Background(), address, control.token)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	cancel()
	select {
	case <-child.Done():
	case <-time.After(time.Second):
		t.Fatal("connection closure did not cancel the child")
	}
	control.close()
	<-control.done
}

func TestTUIChildContextValidation(t *testing.T) {
	parent := context.Background()
	ctx, stop, err := tuiChildContext(parent, "", "")
	if err != nil || ctx != parent {
		t.Fatalf("normal CLI context changed: %v", err)
	}
	stop()
	for _, address := range []string{"not-an-address", "192.0.2.1:1234", "127.0.0.1:1234"} {
		if _, _, err := tuiChildContext(parent, address, "invalid"); err == nil {
			t.Fatalf("accepted invalid control %q", address)
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	if _, _, err := tuiChildContext(parent, address, strings.Repeat("00", 32)); err == nil {
		t.Fatal("closed cancellation server accepted a connection")
	}
}

func TestTUIProgressWriterBoundsOpaqueLogs(t *testing.T) {
	writer := &tuiProgressWriter{}
	ansiWriter := &stripAnsiWriter{dst: writer}
	_, _ = io.WriteString(ansiWriter, "\x1b[")
	_, _ = io.WriteString(ansiWriter, "31mhello\x1b[0m\x00\r\n\x1b]0;unsafe title\a")
	writer.flush()
	if got := writer.tail(); got != "hello" {
		t.Fatalf("unsafe terminal content retained: %q", got)
	}
	_, _ = io.WriteString(writer, strings.Repeat("x", 4096*20)+"\nlast line")
	writer.flush()
	if len(writer.lines) > 12 || len(writer.pending) != 0 || len(writer.tail()) > 12*4097 {
		t.Fatalf("unbounded stderr: %d lines, %d pending bytes", len(writer.lines), len(writer.pending))
	}
	if !strings.HasSuffix(writer.tail(), "last line") {
		t.Fatal("final diagnostic lost")
	}
}
