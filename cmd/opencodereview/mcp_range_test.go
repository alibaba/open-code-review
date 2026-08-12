// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llmloop"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func writeTestBaseState(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := runGitCmdStdout(dir, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		t.Fatalf("resolve test base: %v", err)
	}
	sha := strings.TrimSpace(string(out))
	if err := os.MkdirAll(filepath.Join(dir, ".scratch"), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "base_sha: " + sha + "\nsource: user\nsummary: test task\n"
	if err := os.WriteFile(filepath.Join(dir, ".scratch", "base"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return sha
}

func TestPrepareMCPRangeDefaultsToHEADAndResolvesSHAs(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "change.go", "package change\n", "change")
	writeTestBaseState(t, dir, "HEAD~1")
	prepared, empty, err := prepareMCPRange(dir, ocrReviewInput{From: "HEAD~1"})
	if err != nil {
		t.Fatalf("prepareMCPRange: %v", err)
	}
	if empty || len(prepared.From) != 40 || len(prepared.To) != 40 {
		t.Fatalf("prepared=%+v empty=%v", prepared, empty)
	}
}

func TestPrepareMCPRangeSkipsEmptyRange(t *testing.T) {
	dir := initTestGitRepo(t)
	writeTestBaseState(t, dir, "HEAD")
	prepared, empty, err := prepareMCPRange(dir, ocrReviewInput{From: "HEAD", To: "HEAD"})
	if err != nil {
		t.Fatalf("prepareMCPRange: %v", err)
	}
	if !empty || prepared.From == "" || prepared.To == "" {
		t.Fatalf("prepared=%+v empty=%v", prepared, empty)
	}
	var result map[string]any
	if err := json.Unmarshal(emptyRangeReviewResult(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "skipped" {
		t.Fatalf("empty result = %v", result)
	}
}

func TestPrepareMCPRangeRejectsNonAncestor(t *testing.T) {
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "change.go", "package change\n", "change")
	writeTestBaseState(t, dir, "HEAD")
	_, _, err := prepareMCPRange(dir, ocrReviewInput{From: "HEAD", To: "HEAD~1"})
	if err == nil {
		t.Fatal("expected non-ancestor range to fail")
	}
}

func TestMCPReviewOptionsAlwaysExcludeScratch(t *testing.T) {
	opts := (ocrReviewInput{Exclude: []string{"vendor/**"}}).reviewOptions(t.TempDir())
	if opts.excludes != ".scratch/**,vendor/**" {
		t.Fatalf("excludes = %q", opts.excludes)
	}
}

func TestLoadBaseStateAndFindingStateUseScratch(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".scratch"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".scratch", "base"), []byte("base_sha: "+testBaseSHA+"\nsource: user\nsummary: task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBaseState(dir); err != nil {
		t.Fatal(err)
	}
}

func TestMCPRangeAnnotatesCompletedFindings(t *testing.T) {
	dir := initTestGitRepo(t)
	baseSHA := writeTestBaseState(t, dir, "HEAD")
	gitCommitFile(t, dir, "change.go", "package change\n", "change")
	cs, stop := connectTestOCRServerAt(t, dir, func(_ context.Context, opts reviewOptions, out, _ io.Writer, _ llmloop.ProgressFunc, _ reviewStageFunc, _ *reviewWatchdog) error {
		if opts.from != baseSHA || len(opts.to) != 40 || opts.excludes != ".scratch/**" {
			t.Errorf("review opts = %+v", opts)
		}
		_, _ = io.WriteString(out, `{"status":"complete","session_id":"range-review-1","comments":[{"path":"change.go","start_line":1,"end_line":1,"category":"bug","severity":"high","content":"same issue"}]}`)
		return nil
	})
	defer stop()

	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      mcpReviewToolName,
		Arguments: map[string]any{"from": baseSHA},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("result = %s", toolText(result))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(toolText(result)), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	comment := payload["comments"].([]any)[0].(map[string]any)
	if comment["finding_id"] == nil || comment["consecutive_review_count"] != float64(1) || comment["automation_status"] != findingStatusActive {
		t.Fatalf("annotated comment = %+v", comment)
	}
}
