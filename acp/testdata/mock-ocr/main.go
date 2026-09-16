// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// mock-ocr is a test double for the OCR CLI.
// It simulates various OCR behaviors for testing ocr-acp without real LLM calls.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

var (
	scenario     = flag.String("scenario", "success-review", "Test scenario to simulate")
	delay        = flag.Duration("delay", 0, "Delay before producing output")
	childPIDFile = flag.String("child-pid-file", "", "Write the spawned child PID for process-tree tests")
)

// The structs below mirror the real OCR CLI JSON envelope (cmd/opencodereview
// jsonOutput and model.LlmComment), NOT the adapter's DTO. Keeping the test
// double shaped like the real output is what lets the contract tests catch a
// DTO that drifted from the CLI. Field names therefore match the CLI tags
// (content/start_line/end_line), and thinking is present even though the
// adapter deliberately does not model it.
type runResult struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Summary   *summary  `json:"summary,omitempty"`
	Comments  []comment `json:"comments"`
	SessionID string    `json:"session_id,omitempty"`
	Manifest  *manifest `json:"manifest,omitempty"`
}

type summary struct {
	FilesReviewed int64  `json:"files_reviewed"`
	Comments      int64  `json:"comments"`
	TotalTokens   int64  `json:"total_tokens,omitempty"`
	Elapsed       string `json:"elapsed,omitempty"`
}

type comment struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	SuggestionCode string `json:"suggestion_code,omitempty"`
	ExistingCode   string `json:"existing_code,omitempty"`
	StartLine      int    `json:"start_line"`
	EndLine        int    `json:"end_line"`
	Category       string `json:"category,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Thinking       string `json:"thinking,omitempty"`
}

type manifest struct {
	SchemaVersion string      `json:"schema_version,omitempty"`
	Operation     string      `json:"operation,omitempty"`
	TerminalState string      `json:"terminal_state,omitempty"`
	RunFailure    *runFailure `json:"run_failure,omitempty"`
	Coverage      *coverage   `json:"coverage,omitempty"`
}

type runFailure struct {
	Classification string `json:"classification,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type coverage struct {
	Failed []coverageItem `json:"failed,omitempty"`
}

type coverageItem struct {
	Path           string `json:"path,omitempty"`
	Classification string `json:"classification,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

func emit(result runResult) {
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "mock-ocr: encoding result: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "review" || os.Args[1] == "scan") {
		if err := flag.CommandLine.Parse(os.Args[2:]); err != nil {
			os.Exit(2)
		}
	} else {
		flag.Parse()
	}

	// Set up signal handling for cancellation tests.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *delay > 0 {
		select {
		case <-time.After(*delay):
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "mock-ocr: received signal during delay")
			os.Exit(130)
		}
	}

	switch *scenario {
	case "success-review":
		successReview()
	case "success-scan":
		successScan()
	case "partial":
		partialReview()
	case "empty-comments":
		emptyComments()
	case "stderr-pollution":
		stderrPollution()
	case "invalid-json":
		invalidJSON()
	case "non-zero-exit":
		nonZeroExit()
	case "block-for-cancel":
		blockForCancel(sigCh)
	case "spawn-child":
		spawnChild(sigCh)
	case "child-block":
		childBlock(sigCh)
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %s\n", *scenario)
		os.Exit(1)
	}
}

func successReview() {
	fmt.Fprintln(os.Stderr, "[ocr] 1 file(s) changed, reviewing 1")
	time.Sleep(50 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "[ocr] code_comment (0s)")

	emit(runResult{
		Status:    "complete",
		Message:   "Review complete: 2 finding(s) across 1 selected item(s).",
		Summary:   &summary{FilesReviewed: 1, Comments: 2, TotalTokens: 18746, Elapsed: "7s"},
		SessionID: "843e2022-8130-416f-bb95-ef76ab43ef9d",
		Comments: []comment{
			{
				Path:      "internal/agent/agent.go",
				Content:   "Potential nil dereference on the error path.",
				StartLine: 42,
				EndLine:   42,
				Category:  "bug",
				Severity:  "warning",
				Thinking:  "internal model reasoning that must not reach the user",
			},
			{
				Path:           "internal/agent/agent.go",
				Content:        "Consider adding error handling here.",
				SuggestionCode: "+return err",
				ExistingCode:   "return nil",
				Category:       "maintainability",
				Severity:       "info",
			},
		},
		Manifest: &manifest{
			SchemaVersion: "ocr.run-manifest/v1",
			Operation:     "review",
			TerminalState: "complete",
		},
	})
}

func successScan() {
	fmt.Fprintln(os.Stderr, "[ocr] scanning files...")

	emit(runResult{
		Status:  "success",
		Message: "Scan complete: 1 finding(s).",
		Summary: &summary{FilesReviewed: 2, Comments: 1, Elapsed: "3s"},
		Comments: []comment{
			{Path: "cmd/main.go", Content: "Unused import.", StartLine: 10, EndLine: 10, Category: "style", Severity: "info"},
		},
	})
}

func partialReview() {
	fmt.Fprintln(os.Stderr, "[ocr] review interrupted")

	emit(runResult{
		Status:  "partial",
		Message: "Review incomplete: 1 of 2 item(s) completed.",
		Summary: &summary{FilesReviewed: 1, Comments: 1, Elapsed: "2s"},
		Comments: []comment{
			{Path: "file1.go", Content: "Issue found before timeout.", StartLine: 5, EndLine: 5, Category: "bug", Severity: "warning"},
		},
		Manifest: &manifest{
			SchemaVersion: "ocr.run-manifest/v1",
			Operation:     "review",
			TerminalState: "partial",
			Coverage: &coverage{Failed: []coverageItem{
				{Path: "file2.go", Classification: "timeout", Reason: "per-file timeout"},
			}},
		},
	})
}

func emptyComments() {
	emit(runResult{
		Status:  "complete",
		Message: "Review complete: 0 finding(s).",
		Summary: &summary{FilesReviewed: 1, Comments: 0, Elapsed: "1s"},
		// The normal empty-result path emits [] (an empty array), unlike the
		// failure path which emits null. Both must decode.
		Comments: []comment{},
		Manifest: &manifest{SchemaVersion: "ocr.run-manifest/v1", Operation: "review", TerminalState: "complete"},
	})
}

func stderrPollution() {
	fmt.Fprintln(os.Stderr, "[ocr] diagnostic message")
	// A second JSON document on stderr, as emitFailureUsage does in the real
	// CLI. It must never be mistaken for the stdout result.
	fmt.Fprintln(os.Stderr, `{"status":"failed","comments":null}`)
	fmt.Fprintln(os.Stderr, "[ocr] more noise")

	emit(runResult{
		Status:   "complete",
		Message:  "Review complete: 1 finding(s).",
		Summary:  &summary{FilesReviewed: 1, Comments: 1, Elapsed: "1s"},
		Comments: []comment{{Path: "test.go", Content: "Test finding.", StartLine: 1, EndLine: 1, Category: "bug", Severity: "info"}},
	})
}

func invalidJSON() {
	fmt.Fprintln(os.Stderr, "[ocr] generating result...")
	fmt.Fprintln(os.Stdout, "{invalid json here")
	os.Exit(1)
}

func nonZeroExit() {
	emit(runResult{
		Status:   "failed",
		Message:  "Review failed: unable to reach the model.",
		Comments: nil, // the failure path emits null
		Manifest: &manifest{
			SchemaVersion: "ocr.run-manifest/v1",
			Operation:     "review",
			TerminalState: "failed",
			RunFailure:    &runFailure{Classification: "failed", Reason: "review failed"},
		},
	})
	os.Exit(1)
}

func blockForCancel(sigCh chan os.Signal) {
	fmt.Fprintln(os.Stderr, "[ocr] long-running operation...")
	fmt.Fprintln(os.Stderr, "READY") // Signal that we're ready to be cancelled

	<-sigCh
	fmt.Fprintln(os.Stderr, "mock-ocr: received cancellation signal")

	// Real review cancellation writes a failed document carrying cancelled
	// classifications and exits non-zero. The adapter must combine its own
	// cancel record with these fields, never the exit code alone.
	emit(runResult{
		Status:  "failed",
		Message: "Review failed (cancelled): 1 finding(s); 1 of 1 item(s) failed.",
		Summary: &summary{FilesReviewed: 1, Comments: 1, Elapsed: "5s"},
		Comments: []comment{
			{Path: "file.go", Content: "Found before cancel.", StartLine: 10, EndLine: 10, Category: "bug", Severity: "info"},
		},
		Manifest: &manifest{
			SchemaVersion: "ocr.run-manifest/v1",
			Operation:     "review",
			TerminalState: "failed",
			RunFailure:    &runFailure{Classification: "cancelled", Reason: "review was cancelled"},
			Coverage: &coverage{Failed: []coverageItem{
				{Path: "file.go", Classification: "cancelled", Reason: "file review was cancelled"},
			}},
		},
	})
	os.Exit(1)
}

func spawnChild(sigCh chan os.Signal) {
	fmt.Fprintln(os.Stderr, "[ocr] running with child process...")
	child := exec.Command(os.Args[0], "-scenario", "child-block")
	child.Stdout = os.Stdout
	childStderr, err := child.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock-ocr: child stderr pipe: %v\n", err)
		os.Exit(1)
	}
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "mock-ocr: start child: %v\n", err)
		os.Exit(1)
	}
	if *childPIDFile != "" {
		if err := os.WriteFile(*childPIDFile, []byte(fmt.Sprintf("%d\n", child.Process.Pid)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "mock-ocr: write child PID: %v\n", err)
			_ = child.Process.Kill()
			os.Exit(1)
		}
	}
	scanner := bufio.NewScanner(childStderr)
	if !scanner.Scan() || scanner.Text() != "CHILD_READY" {
		fmt.Fprintf(os.Stderr, "mock-ocr: child did not become ready: %v\n", scanner.Err())
		_ = child.Process.Kill()
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "READY")
	<-sigCh
	_ = child.Wait()

	emit(runResult{
		Status:   "failed",
		Message:  "Review cancelled with child process.",
		Comments: []comment{},
	})
	os.Exit(1)
}

func childBlock(sigCh chan os.Signal) {
	fmt.Fprintln(os.Stderr, "CHILD_READY")
	<-sigCh
	os.Exit(130)
}
