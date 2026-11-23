// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
)

const validScanTemplateFileJSON = `{
  "MAIN_TASK": {
    "tools": ["task_done", "code_comment"],
    "messages": [
      {"role": "system", "content": "bounded scope system {{system_rule}}"},
      {"role": "user", "content": "file {{current_file_path}}:\n{{file_content}}\n{{plan_guidance}}"}
    ]
  },
  "NO_PLAN_GUIDANCE": "(bounded scope; no pre-scan plan)",
  "MAX_TOKENS": 58888,
  "MAX_TOOL_REQUEST_TIMES": 60
}`

func writeScanTemplateFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan-template.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubLLMCountingRequests(t *testing.T, requests *atomic.Int64, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if response == "" {
			http.Error(w, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(response)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
}

func setStubLLMEnv(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/chat/completions")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "test-model")
	t.Setenv("OCR_LLM_PROTOCOL", "openai")
	t.Setenv("OCR_USE_ANTHROPIC", "false")
}

func TestParseScanFlagsAcceptsScanTemplate(t *testing.T) {
	opts, err := parseScanFlags([]string{"--scan-template", "custom.json"})
	if err != nil {
		t.Fatalf("parseScanFlags: %v", err)
	}
	if opts.scanTemplate != "custom.json" {
		t.Errorf("scanTemplate = %q, want custom.json", opts.scanTemplate)
	}
}

func TestExecuteScanTemplateInvalidFailsClosed(t *testing.T) {
	withUnknownTool := strings.Replace(validScanTemplateFileJSON, `"code_comment"`, `"no_such_tool"`, 1)
	withUnknownToken := strings.Replace(validScanTemplateFileJSON, `{{plan_guidance}}`, `{{frobnicate}}`, 1)
	for _, testCase := range []struct {
		name    string
		content string
		wantErr string
	}{
		{"malformed JSON", `{oops`, "load scan template"},
		{"unknown tool name", withUnknownTool, "scan MAIN_TASK.tools"},
		{"unknown token", withUnknownToken, "invalid scan template"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			freshOCRHome(t)
			var requests atomic.Int64
			server := stubLLMCountingRequests(t, &requests, "")
			defer server.Close()
			setStubLLMEnv(t, server)

			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			tplPath := writeScanTemplateFile(t, testCase.content)
			opts, err := parseScanFlags([]string{
				"--repo", repo, "--scan-template", tplPath,
				"--no-plan", "--no-dedup", "--no-summary",
			})
			if err != nil {
				t.Fatal(err)
			}
			var scanErr error
			captureStdout(t, func() { scanErr = executeScan(opts) })
			if scanErr == nil || !strings.Contains(scanErr.Error(), testCase.wantErr) {
				t.Errorf("executeScan error = %v, want substring %q", scanErr, testCase.wantErr)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("LLM requests = %d, want zero for an invalid template", got)
			}
		})
	}
}

func TestExecuteScanPreviewRejectsInvalidTemplate(t *testing.T) {
	freshOCRHome(t)
	tplPath := writeScanTemplateFile(t, `{oops`)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseScanFlags([]string{
		"--repo", repo, "--preview", "--scan-template", tplPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	var scanErr error
	output := captureStdout(t, func() { scanErr = executeScan(opts) })
	if scanErr == nil || !strings.Contains(scanErr.Error(), "load scan template") {
		t.Errorf("preview executeScan error = %v, want template load failure", scanErr)
	}
	if strings.Contains(output, "main.go") {
		t.Errorf("preview enumerated files despite invalid template: %s", output)
	}
}

func TestApplyScanScalarOverrides(t *testing.T) {
	newTpl := func(tools int, strategy string, budget int64) *template.ScanTemplate {
		return &template.ScanTemplate{
			MaxToolRequestTimes: tools,
			BatchStrategy:       strategy,
			MaxTokensBudget:     budget,
		}
	}
	for _, testCase := range []struct {
		name       string
		tpl        *template.ScanTemplate
		opts       scanOptions
		wantTools  int
		wantBatch  string
		wantBudget int64
	}{
		{"flag unset keeps file value", newTpl(5, "by-language", 100), scanOptions{}, 5, "by-language", 100},
		{"flag raises file value", newTpl(5, "by-language", 100), scanOptions{maxTools: 60}, 60, "by-language", 100},
		{"flag cannot lower file value", newTpl(60, "by-language", 100), scanOptions{maxTools: 5}, 60, "by-language", 100},
		{"batch override passthrough", newTpl(5, "by-language", 100), scanOptions{batch: "none"}, 5, "none", 100},
		{"budget override passthrough", newTpl(5, "by-language", 100), scanOptions{maxTokensBudget: 200}, 5, "by-language", 200},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gotBudget := applyScanScalarOverrides(testCase.tpl, testCase.opts)
			if testCase.tpl.MaxToolRequestTimes != testCase.wantTools {
				t.Errorf("MaxToolRequestTimes = %d, want %d", testCase.tpl.MaxToolRequestTimes, testCase.wantTools)
			}
			if testCase.tpl.BatchStrategy != testCase.wantBatch {
				t.Errorf("BatchStrategy = %q, want %q", testCase.tpl.BatchStrategy, testCase.wantBatch)
			}
			if gotBudget != testCase.wantBudget {
				t.Errorf("budget = %d, want %d", gotBudget, testCase.wantBudget)
			}
		})
	}
}

func TestExecuteScanTemplatePrecedenceBehavioral(t *testing.T) {
	tinyBudget := strings.Replace(validScanTemplateFileJSON, `"MAX_TOKENS": 58888`, `"MAX_TOKENS": 100`, 1)
	taskDone := `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"done","type":"function","function":{"name":"task_done","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
	for _, testCase := range []struct {
		name       string
		extraArgs  []string
		wantErr    string
		wantZeroLL bool
	}{
		{"tiny template budget alone skips the file", nil, "all 1 file scan(s) failed", true},
		{"large CLI ceiling reviews the file", []string{"--max-tokens", "100000"}, "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			freshOCRHome(t)
			var requests atomic.Int64
			server := stubLLMCountingRequests(t, &requests, taskDone)
			defer server.Close()
			setStubLLMEnv(t, server)

			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			tplPath := writeScanTemplateFile(t, tinyBudget)
			args := []string{
				"--repo", repo, "--path", "main.go", "--concurrency", "1",
				"--scan-template", tplPath,
				"--no-plan", "--no-dedup", "--no-summary",
				"--format", "json", "--audience", "agent",
			}
			args = append(args, testCase.extraArgs...)
			opts, err := parseScanFlags(args)
			if err != nil {
				t.Fatal(err)
			}
			var scanErr error
			captureStdout(t, func() { scanErr = executeScan(opts) })
			if testCase.wantErr == "" {
				if scanErr != nil {
					t.Errorf("executeScan error = %v, want success", scanErr)
				}
			} else if scanErr == nil || !strings.Contains(scanErr.Error(), testCase.wantErr) {
				t.Errorf("executeScan error = %v, want substring %q", scanErr, testCase.wantErr)
			}
			if testCase.wantZeroLL && requests.Load() != 0 {
				t.Errorf("LLM requests = %d, want zero for a skipped file", requests.Load())
			}
			if !testCase.wantZeroLL && requests.Load() == 0 {
				t.Error("LLM requests = 0, want at least one for a reviewed file")
			}
		})
	}
}
