// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestScanPathSelectionE2E(t *testing.T) {
	if os.Getenv("OCR_TEST_SCAN_PATH_SELECTION") == "1" {
		rootCmd.SetArgs(os.Args[3:])
		os.Exit(run())
	}
	freshOCRHome(t)
	repo := initTestGitRepo(t)
	gitCommitFile(t, repo, "one.go", "package example\n", "add one.go")
	gitCommitFile(t, repo, "two.go", "package example\n", "add two.go")

	tests := []struct {
		name      string
		args      []string
		wantPaths []string
		invalid   bool
	}{
		{name: "omitted", wantPaths: []string{"one.go", "two.go"}},
		{name: "valid", args: []string{"--path", "one.go"}, wantPaths: []string{"one.go"}},
		{name: "mixed", args: []string{"--path", ",one.go,"}, wantPaths: []string{"one.go"}},
		{name: "empty", args: []string{"--path", ""}, invalid: true},
		{name: "whitespace", args: []string{"--path", "   "}, invalid: true},
		{name: "separators", args: []string{"--path", " , , "}, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"-test.run=^TestScanPathSelectionE2E$", "--", "scan", "--repo", repo, "--preview", "--format", "json"}
			args = append(args, tt.args...)
			const sentinel = "keep\n"
			var outputPath string
			if tt.invalid {
				outputPath = filepath.Join(t.TempDir(), "preview.json")
				if err := os.WriteFile(outputPath, []byte(sentinel), 0o600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--output", outputPath)
			}
			cmd := exec.CommandContext(t.Context(), os.Args[0], args...)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "OCR_TEST_SCAN_PATH_SELECTION=1", "OCR_NO_UPDATE=1", "OCR_ENABLE_TELEMETRY=0")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if tt.invalid {
				if _, ok := errors.AsType[*exec.ExitError](err); !ok {
					t.Errorf("scan %q error = %v, want nonzero exit status", tt.args, err)
				}
				if !strings.Contains(stderr.String(), "--path requires at least one file or directory") {
					t.Errorf("scan %q stderr = %q, want path validation error", tt.args, stderr.String())
				}
				data, err := os.ReadFile(outputPath)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != sentinel {
					t.Errorf("scan %q output file = %q, want %q", tt.args, data, sentinel)
				}
				return
			}
			if err != nil {
				t.Fatalf("scan %q: %v\n%s", tt.args, err, stderr.String())
			}
			var preview model.Preview
			if err := json.Unmarshal(out, &preview); err != nil {
				t.Fatalf("decode scan preview: %v\n%s", err, out)
			}
			var paths []string
			for _, entry := range preview.Entries {
				if entry.WillReview {
					paths = append(paths, entry.Path)
				}
			}
			slices.Sort(paths)
			if !slices.Equal(paths, tt.wantPaths) {
				t.Errorf("scan %q selected %v, want %v", tt.args, paths, tt.wantPaths)
			}
		})
	}
}

func TestExecuteScanRejectsFileReadDiff(t *testing.T) {
	freshOCRHome(t)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		for _, def := range req.Tools {
			if def.Function.Name == tool.FileReadDiff.Name() {
				t.Error("scan exposed file_read_diff to the model")
			}
		}

		response := `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
		switch requests.Add(1) {
		case 1:
			response = `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"unexpected-diff","type":"function","function":{"name":"file_read_diff","arguments":"{\"path_array\":[\"main.go\"]}"}}]},"finish_reason":"tool_calls"}]}`
		case 2:
			rejected := false
			for _, msg := range req.Messages {
				if msg.Role == "tool" && msg.ToolCallID == "unexpected-diff" {
					rejected = true
					if msg.Content != tool.NotAvailableMsg {
						t.Errorf("unexpected diff result: %v", msg.Content)
					}
				}
			}
			if !rejected {
				t.Error("scan did not return a result for the unexpected diff call")
			}
		default:
			t.Error("unexpected extra LLM request")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(response)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/chat/completions")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "test-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")
	t.Setenv("OCR_USE_ANTHROPIC", "false")

	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseScanFlags([]string{
		"--repo", repoDir, "--path", "main.go", "--concurrency", "1",
		"--no-plan", "--no-dedup", "--no-summary", "--format", "json", "--audience", "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := executeScan(opts); err != nil {
			t.Errorf("executeScan: %v", err)
		}
	})
	if got := requests.Load(); got != 2 {
		t.Fatalf("LLM requests = %d, want 2; output: %s", got, output)
	}
}

func TestSplitPaths(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "internal/agent", []string{"internal/agent"}},
		{"multiple", "a.go,b.go,c.go", []string{"a.go", "b.go", "c.go"}},
		{"trims whitespace", "  a.go ,  b.go  ", []string{"a.go", "b.go"}},
		{"drops empty segments", "a.go,,b.go,", []string{"a.go", "b.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPaths(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitPaths(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseScanFlags_BareCommandScansWholeRepo(t *testing.T) {
	opts, err := parseScanFlags([]string{}) // no flags
	if err != nil {
		t.Fatalf("bare `ocr scan` should not error, got: %v", err)
	}
	if opts.paths != "" {
		t.Errorf("default paths should be empty (= whole repo), got %q", opts.paths)
	}
}

func TestParseScanFlags_RejectsInvalidAudience(t *testing.T) {
	_, err := parseScanFlags([]string{"--audience", "robot"})
	if err == nil {
		t.Fatal("expected error for invalid --audience")
	}
	if !strings.Contains(err.Error(), "invalid --audience") {
		t.Errorf("error message = %q; want invalid --audience", err.Error())
	}
}

func TestParseScanFlags_RejectsNegativeMaxTools(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-tools", "-1"})
	if err == nil {
		t.Fatal("expected error for negative --max-tools")
	}
	if !strings.Contains(err.Error(), "--max-tools") {
		t.Errorf("error message = %q; want it to mention --max-tools", err.Error())
	}
}

func TestParseScanFlags_RejectsNegativeMaxGitProcs(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-git-procs", "-3"})
	if err == nil {
		t.Fatal("expected error for negative --max-git-procs")
	}
	if !strings.Contains(err.Error(), "--max-git-procs") {
		t.Errorf("error message = %q; want it to mention --max-git-procs", err.Error())
	}
}

func TestParseScanFlags_DefaultsValid(t *testing.T) {
	opts, err := parseScanFlags([]string{}) // bare command
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.paths != "" {
		t.Errorf("opts = %+v; want paths empty (whole repo)", opts)
	}
	if opts.audience != "human" {
		t.Errorf("default audience = %q, want \"human\"", opts.audience)
	}
	if opts.outputFormat != "text" {
		t.Errorf("default outputFormat = %q, want \"text\"", opts.outputFormat)
	}
	if opts.concurrency != 8 {
		t.Errorf("default concurrency = %d, want 8", opts.concurrency)
	}
}

func TestParseScanFlags_PathNarrowsScope(t *testing.T) {
	opts, err := parseScanFlags([]string{"--path", "internal/agent,internal/diff"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := splitPaths(opts.paths); !reflect.DeepEqual(got, []string{"internal/agent", "internal/diff"}) {
		t.Errorf("splitPaths(opts.paths) = %v", got)
	}
}

func TestParseScanFlags_HelpFlag(t *testing.T) {
	// Cobra handles -h automatically; just ensure it does not error.
	_, err := parseScanFlags([]string{"-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseScanFlags_RejectsNegativeMaxTokensBudget(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-tokens-budget", "-100"})
	if err == nil {
		t.Fatal("expected error for negative --max-tokens-budget")
	}
	if !strings.Contains(err.Error(), "--max-tokens-budget") {
		t.Errorf("error message = %q; want it to mention --max-tokens-budget", err.Error())
	}
}

func TestParseScanFlags_RejectsNegativeMaxTokens(t *testing.T) {
	_, err := parseScanFlags([]string{"--max-tokens", "-100"})
	if err == nil {
		t.Fatal("expected error for negative --max-tokens")
	}
	if !strings.Contains(err.Error(), "--max-tokens") {
		t.Errorf("error message = %q; want it to mention --max-tokens", err.Error())
	}
}

func TestParseScanFlags_BooleanFlags(t *testing.T) {
	opts, err := parseScanFlags([]string{"--no-plan", "--no-dedup", "--no-summary", "--preview"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.noPlan {
		t.Error("noPlan should be true")
	}
	if !opts.noDedup {
		t.Error("noDedup should be true")
	}
	if !opts.noSummary {
		t.Error("noSummary should be true")
	}
	if !opts.preview {
		t.Error("preview should be true")
	}
}

func TestParseScanFlags_ModelOverride(t *testing.T) {
	opts, err := parseScanFlags([]string{"--model", "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.model != "claude-opus-4-6" {
		t.Errorf("model = %q, want claude-opus-4-6", opts.model)
	}
}

func TestParseScanFlags_ProviderAndModelOverrides(t *testing.T) {
	opts, err := parseScanFlags([]string{"--provider", "my-gateway", "--model", "llama-3-8b"})
	if err != nil {
		t.Fatalf("parseScanFlags: %v", err)
	}
	if opts.provider != "my-gateway" || opts.model != "llama-3-8b" {
		t.Fatalf("provider=%q model=%q", opts.provider, opts.model)
	}
}

func TestParseScanFlags_Resume(t *testing.T) {
	opts, err := parseScanFlags([]string{"--resume", "session-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.resume != "session-123" {
		t.Errorf("resume = %q, want session-123", opts.resume)
	}
}

func TestParseScanFlags_PreviewWithResume(t *testing.T) {
	_, err := parseScanFlags([]string{"--preview", "--resume", "session-123"})
	if err == nil {
		t.Fatal("expected error for --preview with --resume")
	}
	if !strings.Contains(err.Error(), "--preview and --resume") {
		t.Errorf("error = %q; want preview/resume conflict", err.Error())
	}
}

func TestParseScanFlags_AllStringFlags(t *testing.T) {
	opts, err := parseScanFlags([]string{
		"--rule", "/tmp/rule.json",
		"--repo", "/tmp/repo",
		"--exclude", "*.md,*.txt",
		"--batch", "by-language",
		"--background", "test context",
		"--audience", "agent",
		"-f", "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.rulePath != "/tmp/rule.json" {
		t.Errorf("rulePath = %q", opts.rulePath)
	}
	if opts.repoDir != "/tmp/repo" {
		t.Errorf("repoDir = %q", opts.repoDir)
	}
	if opts.excludes != "*.md,*.txt" {
		t.Errorf("excludes = %q", opts.excludes)
	}
	if opts.batch != "by-language" {
		t.Errorf("batch = %q", opts.batch)
	}
	if opts.background != "test context" {
		t.Errorf("background = %q", opts.background)
	}
	if opts.audience != "agent" {
		t.Errorf("audience = %q", opts.audience)
	}
	if opts.outputFormat != "json" {
		t.Errorf("outputFormat = %q", opts.outputFormat)
	}
}

func TestParseScanFlags_IntFlags(t *testing.T) {
	opts, err := parseScanFlags([]string{
		"--concurrency", "16",
		"--timeout", "20",
		"--max-tools", "50",
		"--max-git-procs", "32",
		"--max-tokens", "200000",
		"--max-tokens-budget", "100000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.concurrency != 16 {
		t.Errorf("concurrency = %d", opts.concurrency)
	}
	if opts.concurrentTaskTimeout != 20 {
		t.Errorf("concurrentTaskTimeout = %d", opts.concurrentTaskTimeout)
	}
	if opts.maxTools != 50 {
		t.Errorf("maxTools = %d", opts.maxTools)
	}
	if opts.maxGitProcs != 32 {
		t.Errorf("maxGitProcs = %d", opts.maxGitProcs)
	}
	if opts.maxTokens != 200000 {
		t.Errorf("maxTokens = %d", opts.maxTokens)
	}
	if opts.maxTokensBudget != 100000 {
		t.Errorf("maxTokensBudget = %d", opts.maxTokensBudget)
	}
}
