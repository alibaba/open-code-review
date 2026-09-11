// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// These tests run the mock OCR binary and decode its real stdout into the
// contract types. They exist to stop the test double and the contract from
// drifting apart: a struct tag typo or a renamed JSON field on either side
// fails here instead of silently producing empty results at runtime.
package contract_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/contract"
)

// mockBin is the compiled test double, built once per test binary run.
var mockBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ocr-acp-mock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}

	mockBin = filepath.Join(dir, "mock-ocr")
	build := exec.Command("go", "build", "-o", mockBin, "../../testdata/mock-ocr")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building mock-ocr: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// runMock executes the test double, returning stdout and the exit code.
// stderr is discarded: these tests assert on the stdout contract only.
func runMock(t *testing.T, args ...string) (string, int) {
	t.Helper()

	cmd := exec.Command(mockBin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	err := cmd.Run()
	if err == nil {
		return stdout.String(), 0
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running mock-ocr %v: %v", args, err)
	}
	return stdout.String(), exitErr.ExitCode()
}

func decodeReview(t *testing.T, stdout string) contract.ReviewResult {
	t.Helper()
	var result contract.ReviewResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("mock stdout does not decode into contract.ReviewResult: %v\nstdout: %q", err, stdout)
	}
	return result
}

func TestMockReviewScenariosMatchContract(t *testing.T) {
	tests := []struct {
		scenario     string
		wantStatus   string
		wantComments int
	}{
		{"success-review", "completed", 2},
		{"partial", "partial", 1},
		{"empty-comments", "completed", 0},
		// stderr pollution must not leak into the stdout JSON document.
		{"stderr-pollution", "completed", 1},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			stdout, code := runMock(t, "-scenario", tt.scenario)
			if code != 0 {
				t.Fatalf("mock-ocr -scenario %s exited %d, want 0", tt.scenario, code)
			}

			result := decodeReview(t, stdout)
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if len(result.Comments) != tt.wantComments {
				t.Errorf("len(Comments) = %d, want %d", len(result.Comments), tt.wantComments)
			}
		})
	}
}

func TestMockScanScenarioMatchesContract(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "success-scan")
	if code != 0 {
		t.Fatalf("mock-ocr -scenario success-scan exited %d, want 0", code)
	}

	var result contract.ScanResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("mock stdout does not decode into contract.ScanResult: %v\nstdout: %q", err, stdout)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
	if len(result.Comments) != 1 {
		t.Errorf("len(Comments) = %d, want 1", len(result.Comments))
	}
}

// TestMockDefaultScenarioSucceeds pins the default -scenario value: running
// the binary bare is how a developer first tries it, so it must not error.
func TestMockDefaultScenarioSucceeds(t *testing.T) {
	stdout, code := runMock(t)
	if code != 0 {
		t.Fatalf("mock-ocr with no arguments exited %d, want 0", code)
	}

	result := decodeReview(t, stdout)
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
}

// TestMockNonZeroExitStillDecodes documents that a failed run reports its
// status through the JSON document, not only through the exit code.
func TestMockNonZeroExitStillDecodes(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "non-zero-exit")
	if code == 0 {
		t.Fatal("mock-ocr -scenario non-zero-exit exited 0, want non-zero")
	}

	result := decodeReview(t, stdout)
	if result.Status != "failed" {
		t.Errorf("Status = %q, want %q", result.Status, "failed")
	}
}

// TestMockInvalidJSONDoesNotDecode confirms the malformed-output scenario
// actually produces output the adapter must reject.
func TestMockInvalidJSONDoesNotDecode(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "invalid-json")
	if code == 0 {
		t.Fatal("mock-ocr -scenario invalid-json exited 0, want non-zero")
	}

	var result contract.ReviewResult
	if err := json.Unmarshal([]byte(stdout), &result); err == nil {
		t.Fatalf("malformed stdout decoded without error: %q", stdout)
	}
}

// TestMockCancelEmitsPartialResult drives the cancellation path: wait for the
// READY marker on stderr, send SIGINT, then check the contract the adapter
// will rely on when a review is cancelled.
func TestMockCancelEmitsPartialResult(t *testing.T) {
	cmd := exec.Command(mockBin, "-scenario", "block-for-cancel")

	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting mock-ocr: %v", err)
	}

	ready := make(chan struct{})
	var once sync.Once
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "READY") {
				once.Do(func() { close(ready) })
			}
		}
	}()

	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("mock-ocr never reported READY")
	}

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("sending SIGINT: %v", err)
	}

	// Drain stdout before Wait so the child cannot block on a full pipe.
	out, readErr := io.ReadAll(stdout)
	waitErr := cmd.Wait()

	if readErr != nil {
		t.Fatalf("reading stdout: %v", readErr)
	}
	if waitErr != nil {
		t.Fatalf("mock-ocr exited non-zero after cancellation: %v", waitErr)
	}

	result := decodeReview(t, string(out))
	if result.Status != "partial" {
		t.Errorf("Status after cancellation = %q, want %q", result.Status, "partial")
	}
}
