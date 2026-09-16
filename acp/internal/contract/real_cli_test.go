// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// This file guards the contract against drift from the real OCR CLI.
//
// TestRealOCRReviewShapeDecodes and TestRealOCRScanShapeDecodes are
// deterministic fixtures taken from the phase 1 CLI Contract. They run
// everywhere and fail if the DTO stops matching the documented real output
// shape, including the summary object and the content/start_line fields.
//
// TestRealOCRBinaryScanMultiplePaths is opt-in: set OCR_BINARY to a real ocr
// binary (for example ../dist/opencodereview) to run it. It exercises the
// adapter's own argv against the real CLI, which is how the --path
// comma-joining rule is verified end to end.
package contract_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/contract"
)

// realReviewJSON is the shape documented in the CLI Contract (jsonOutput plus
// model.LlmComment), including fields the adapter does not model (llm,
// tool_calls, groups, retry_report, per-comment thinking) so that tolerance of
// unknown fields is exercised too. The inner quotes are avoided on purpose so
// the document stays a plain raw string.
const realReviewJSON = `{
  "status": "complete",
  "llm": {"model": "deepseek-flash"},
  "trace_id": "trace-1",
  "message": "Review complete: 1 finding(s) across 1 selected item(s).",
  "summary": {
    "files_reviewed": 1,
    "comments": 1,
    "total_tokens": 18746,
    "input_tokens": 17952,
    "output_tokens": 794,
    "cache_read_tokens": 14592,
    "elapsed": "7s"
  },
  "tool_calls": {"total": 1, "by_tool": {"code_comment": 1}},
  "comments": [
    {
      "path": "main.go",
      "content": "Type mismatch: assigning a string literal to an int variable does not compile.",
      "suggestion_code": "+var x int = 0",
      "existing_code": "+var x int = zero",
      "start_line": 3,
      "end_line": 3,
      "thinking": "long internal reasoning that must not surface",
      "category": "bug",
      "severity": "critical"
    }
  ],
  "groups": [{"label": "main.go", "files": ["main.go"]}],
  "session_id": "843e2022-8130-416f-bb95-ef76ab43ef9d",
  "manifest": {
    "schema_version": "ocr.run-manifest/v1",
    "run_id": "843e2022",
    "operation": "review",
    "terminal_state": "complete",
    "coverage": {"selected": [{"path": "main.go"}], "completed": [{"path": "main.go"}], "failed": []},
    "elapsed_ms": 7232
  },
  "retry_report": {"schema_version": "ocr.llm-retry-report/v1"}
}`

func TestRealOCRReviewShapeDecodes(t *testing.T) {
	var result contract.ReviewResult
	if err := json.Unmarshal([]byte(realReviewJSON), &result); err != nil {
		t.Fatalf("real review JSON does not decode: %v", err)
	}
	if result.Status != contract.StatusComplete {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusComplete)
	}
	if result.Message == "" {
		t.Error("Message is empty")
	}
	if result.Summary == nil || result.Summary.FilesReviewed != 1 || result.Summary.Comments != 1 {
		t.Fatalf("Summary = %+v, want files_reviewed=1 comments=1", result.Summary)
	}
	if len(result.Comments) != 1 {
		t.Fatalf("len(Comments) = %d, want 1", len(result.Comments))
	}
	c := result.Comments[0]
	if c.Path != "main.go" || c.Content == "" {
		t.Errorf("comment path/content = %q/%q", c.Path, c.Content)
	}
	if c.StartLine != 3 || c.EndLine != 3 || c.Category != "bug" || c.Severity != "critical" {
		t.Errorf("comment = %+v, want 3-3 bug/critical", c)
	}
	if c.SuggestionCode == "" || c.ExistingCode == "" {
		t.Errorf("suggestion/existing code not decoded: %+v", c)
	}
	if result.Manifest == nil || result.Manifest.TerminalState != "complete" {
		t.Errorf("Manifest = %+v, want terminal_state=complete", result.Manifest)
	}
}

const realScanJSON = `{"status":"success","message":"Scan complete.","summary":{"files_reviewed":2,"comments":0,"elapsed":"1s"},"comments":null}`

func TestRealOCRScanShapeDecodes(t *testing.T) {
	var result contract.ScanResult
	if err := json.Unmarshal([]byte(realScanJSON), &result); err != nil {
		t.Fatalf("real scan JSON does not decode: %v", err)
	}
	if result.Status != contract.StatusSuccess {
		t.Errorf("Status = %q, want %q", result.Status, contract.StatusSuccess)
	}
	if result.Comments != nil {
		t.Errorf("Comments = %v, want nil (null)", result.Comments)
	}
	if result.Summary == nil || result.Summary.FilesReviewed != 2 {
		t.Errorf("Summary = %+v, want files_reviewed=2", result.Summary)
	}
}

func TestRealOCRBinaryScanMultiplePaths(t *testing.T) {
	bin := os.Getenv("OCR_BINARY")
	if bin == "" {
		t.Skip("set OCR_BINARY to a real ocr binary to run this integration test")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("OCR_BINARY=%q is not usable: %v", bin, err)
	}

	root := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "a", "one.go"), "package a\n\nfunc A() {}\n")
	writeFile(t, filepath.Join(root, "b", "two.go"), "package b\n\nfunc B() {}\n")

	args, err := contract.BuildScanArgs(&contract.ScanIntent{Paths: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("BuildScanArgs: %v", err)
	}
	// The adapter runs the child with its cwd set to the scan root; --preview
	// keeps the call LLM-free.
	args = append(args, "--preview")

	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running %s %v: %v", bin, args, err)
	}

	var preview struct {
		Files []struct {
			Path       string `json:"path"`
			WillReview bool   `json:"will_review"`
		} `json:"files"`
	}
	if err := json.Unmarshal(out, &preview); err != nil {
		t.Fatalf("decoding preview: %v\n%s", err, out)
	}

	seen := map[string]bool{}
	for _, f := range preview.Files {
		seen[f.Path] = true
	}
	if !seen["a/one.go"] || !seen["b/two.go"] {
		t.Errorf("preview files = %v, want both a/one.go and b/two.go (repeated --path would drop one)", seen)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
