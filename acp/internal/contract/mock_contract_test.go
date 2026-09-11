// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// These tests run the mock OCR binary and decode its real stdout into the
// contract types. The mock mirrors the real OCR CLI JSON envelope, so a DTO
// that drifts from the CLI fails here. Assertions cover the field values that
// matter (content/start_line/end_line/summary/manifest), not just counts.
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

func TestMockSuccessReviewDecodesEveryField(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "success-review")
	if code != 0 {
		t.Fatalf("mock-ocr -scenario success-review exited %d, want 0", code)
	}

	result := decodeReview(t, stdout)
	if result.Status != contract.StatusComplete {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusComplete)
	}
	if result.Summary == nil {
		t.Fatal("Summary = nil, want the decoded summary object")
	}
	if result.Summary.FilesReviewed != 1 || result.Summary.Comments != 2 || result.Summary.TotalTokens != 18746 {
		t.Errorf("Summary = %+v, want files_reviewed=1 comments=2 total_tokens=18746", *result.Summary)
	}
	if len(result.Comments) != 2 {
		t.Fatalf("len(Comments) = %d, want 2", len(result.Comments))
	}

	first := result.Comments[0]
	if first.Path != "internal/agent/agent.go" || first.Content != "Potential nil dereference on the error path." {
		t.Errorf("first comment = %+v", first)
	}
	if first.StartLine != 42 || first.EndLine != 42 {
		t.Errorf("first comment lines = %d-%d, want 42-42", first.StartLine, first.EndLine)
	}
	if first.Category != "bug" || first.Severity != "warning" {
		t.Errorf("first comment category/severity = %q/%q", first.Category, first.Severity)
	}

	second := result.Comments[1]
	if second.StartLine != 0 || second.EndLine != 0 {
		t.Errorf("file-level comment lines = %d-%d, want 0-0", second.StartLine, second.EndLine)
	}
	if second.SuggestionCode != "+return err" || second.ExistingCode != "return nil" {
		t.Errorf("second comment suggestion/existing = %q/%q", second.SuggestionCode, second.ExistingCode)
	}

	if result.Manifest == nil || result.Manifest.TerminalState != "complete" {
		t.Errorf("Manifest = %+v, want terminal_state=complete", result.Manifest)
	}
}

func TestMockPartialCarriesCoverageFailure(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "partial")
	if code != 0 {
		t.Fatalf("mock-ocr -scenario partial exited %d, want 0", code)
	}
	result := decodeReview(t, stdout)
	if result.Status != contract.StatusPartial {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusPartial)
	}
	if result.Manifest == nil || result.Manifest.Coverage == nil || len(result.Manifest.Coverage.Failed) != 1 {
		t.Fatalf("Manifest = %+v, want one failed coverage item", result.Manifest)
	}
	if got := result.Manifest.Coverage.Failed[0].Classification; got != "timeout" {
		t.Errorf("coverage failure classification = %q, want timeout", got)
	}
}

func TestMockEmptyCommentsEmitsArray(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "empty-comments")
	if code != 0 {
		t.Fatalf("mock-ocr -scenario empty-comments exited %d, want 0", code)
	}
	if !strings.Contains(stdout, `"comments":[]`) {
		t.Errorf("stdout should contain an empty comments array, got %q", stdout)
	}
	result := decodeReview(t, stdout)
	if result.Status != contract.StatusComplete || len(result.Comments) != 0 {
		t.Errorf("result = %+v, want complete with no comments", result)
	}
}

func TestMockStderrPollutionDoesNotLeak(t *testing.T) {
	stdout, code := runMock(t, "-scenario", "stderr-pollution")
	if code != 0 {
		t.Fatalf("mock-ocr -scenario stderr-pollution exited %d, want 0", code)
	}
	result := decodeReview(t, stdout)
	if result.Status != contract.StatusComplete || len(result.Comments) != 1 {
		t.Errorf("result = %+v, want complete with 1 comment", result)
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
	if result.Status != contract.StatusSuccess {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusSuccess)
	}
	if result.Summary == nil || result.Summary.FilesReviewed != 2 {
		t.Errorf("Summary = %+v, want files_reviewed=2", result.Summary)
	}
	if len(result.Comments) != 1 {
		t.Fatalf("len(Comments) = %d, want 1", len(result.Comments))
	}
	if c := result.Comments[0]; c.Path != "cmd/main.go" || c.Content != "Unused import." || c.StartLine != 10 || c.Severity != "info" {
		t.Errorf("comment = %+v", c)
	}
}

// TestMockDefaultScenarioSucceeds pins the default -scenario value: running
// the binary bare is how a developer first tries it, so it must not error.
func TestMockDefaultScenarioSucceeds(t *testing.T) {
	stdout, code := runMock(t)
	if code != 0 {
		t.Fatalf("mock-ocr with no arguments exited %d, want 0", code)
	}
	if result := decodeReview(t, stdout); result.Status != contract.StatusComplete {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusComplete)
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
	if result.Status != contract.StatusFailed {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusFailed)
	}
	if result.Comments != nil {
		t.Errorf("Comments = %v, want nil (null in JSON)", result.Comments)
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

// TestMockCancelReportsCancelledManifest drives the cancellation path: wait for
// the READY marker on stderr, send SIGINT, then check the real cancellation
// contract. The real CLI has no dedicated cancel exit code, so the adapter
// must rely on the manifest classifications rather than the exit status.
func TestMockCancelReportsCancelledManifest(t *testing.T) {
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
	// Cancellation exits non-zero in the real CLI; the adapter must not treat
	// that alone as failure, which is why the manifest is asserted below.
	if waitErr == nil {
		t.Error("cancelled mock exited 0, want non-zero (mirrors the real CLI)")
	}

	result := decodeReview(t, string(out))
	if result.Status != contract.StatusFailed {
		t.Errorf("Status after cancel = %q, want %q", result.Status, contract.StatusFailed)
	}
	if result.Manifest == nil || result.Manifest.RunFailure == nil {
		t.Fatalf("Manifest = %+v, want a run_failure", result.Manifest)
	}
	if got := result.Manifest.RunFailure.Classification; got != "cancelled" {
		t.Errorf("run_failure.classification = %q, want cancelled", got)
	}
	if result.Manifest.Coverage == nil || len(result.Manifest.Coverage.Failed) != 1 {
		t.Fatalf("coverage = %+v, want one failed item", result.Manifest.Coverage)
	}
	if got := result.Manifest.Coverage.Failed[0].Classification; got != "cancelled" {
		t.Errorf("coverage.failed[0].classification = %q, want cancelled", got)
	}
}
