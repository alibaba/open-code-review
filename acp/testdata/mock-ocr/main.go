// SPDX-License-Identifier: Apache-2.0

// mock-ocr is a test double for the OCR CLI.
// It simulates various OCR behaviors for testing ocr-acp without real LLM calls.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	scenario = flag.String("scenario", "success-review", "Test scenario to simulate")
	delay    = flag.Duration("delay", 0, "Delay before producing output")
)

type reviewResult struct {
	Status   string    `json:"status"`
	Summary  string    `json:"summary"`
	Comments []comment `json:"comments"`
	Thinking string    `json:"thinking,omitempty"`
}

type scanResult struct {
	Status   string    `json:"status"`
	Comments []comment `json:"comments"`
}

type comment struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Severity string `json:"severity,omitempty"`
}

func main() {
	flag.Parse()

	// Set up signal handling for cancellation tests
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Simulate delay if requested
	if *delay > 0 {
		select {
		case <-time.After(*delay):
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "mock-ocr: received signal during delay")
			os.Exit(130) // Standard SIGINT exit code
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
		spawnChild()
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %s\n", *scenario)
		os.Exit(1)
	}
}

func successReview() {
	fmt.Fprintln(os.Stderr, "[progress] Analyzing files...")
	time.Sleep(100 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "[progress] Running review...")

	result := reviewResult{
		Status:  "completed",
		Summary: "Found 2 issues in 1 file",
		Comments: []comment{
			{Path: "internal/agent/agent.go", Line: 42, Message: "Potential nil dereference", Severity: "warning"},
			{Path: "internal/agent/agent.go", Line: 0, Message: "Consider adding error handling", Severity: "info"},
		},
		Thinking: "This is internal reasoning not shown to user",
	}

	json.NewEncoder(os.Stdout).Encode(result)
}

func successScan() {
	fmt.Fprintln(os.Stderr, "[progress] Scanning files...")

	result := scanResult{
		Status: "completed",
		Comments: []comment{
			{Path: "cmd/main.go", Line: 10, Message: "Unused import", Severity: "info"},
		},
	}

	json.NewEncoder(os.Stdout).Encode(result)
}

func partialReview() {
	fmt.Fprintln(os.Stderr, "[progress] Review interrupted")

	result := reviewResult{
		Status:  "partial",
		Summary: "Review incomplete due to timeout",
		Comments: []comment{
			{Path: "file1.go", Line: 5, Message: "Issue found before timeout", Severity: "warning"},
		},
	}

	json.NewEncoder(os.Stdout).Encode(result)
}

func emptyComments() {
	result := reviewResult{
		Status:   "completed",
		Summary:  "No issues found",
		Comments: nil,
	}

	json.NewEncoder(os.Stdout).Encode(result)
}

func stderrPollution() {
	fmt.Fprintln(os.Stderr, "Some diagnostic message")
	fmt.Fprintln(os.Stderr, "[progress] Working...")
	fmt.Fprintln(os.Stderr, "More noise")

	result := reviewResult{
		Status:  "completed",
		Summary: "Test with stderr pollution",
		Comments: []comment{
			{Path: "test.go", Line: 1, Message: "Test finding", Severity: "info"},
		},
	}

	json.NewEncoder(os.Stdout).Encode(result)
}

func invalidJSON() {
	fmt.Fprintln(os.Stderr, "[progress] Generating result...")
	fmt.Fprintln(os.Stdout, "{invalid json here")
	os.Exit(1)
}

func nonZeroExit() {
	result := reviewResult{
		Status:  "failed",
		Summary: "Review failed due to error",
	}

	json.NewEncoder(os.Stdout).Encode(result)
	os.Exit(1)
}

func blockForCancel(sigCh chan os.Signal) {
	fmt.Fprintln(os.Stderr, "[progress] Long-running operation...")
	fmt.Fprintln(os.Stderr, "READY") // Signal that we're ready to be cancelled

	<-sigCh
	fmt.Fprintln(os.Stderr, "mock-ocr: received cancellation signal")

	// Simulate graceful shutdown with partial results
	result := reviewResult{
		Status:  "partial",
		Summary: "Review cancelled by user",
		Comments: []comment{
			{Path: "file.go", Line: 10, Message: "Found before cancel", Severity: "info"},
		},
	}

	json.NewEncoder(os.Stdout).Encode(result)
	os.Exit(0)
}

func spawnChild() {
	// Simulate spawning a child process that ignores signals
	// For now, just simulate the scenario without actual fork
	fmt.Fprintln(os.Stderr, "[progress] Running with child process...")
	time.Sleep(200 * time.Millisecond)

	result := reviewResult{
		Status:   "completed",
		Summary:  "Test with child process",
		Comments: []comment{},
	}

	json.NewEncoder(os.Stdout).Encode(result)
}
