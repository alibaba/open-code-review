// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
)

func TestCommandsUseTaskToolBindings(test *testing.T) {
	for _, mode := range []string{"review", "scan"} {
		test.Run(mode, func(test *testing.T) {
			freshOCRHome(test)
			var mutex sync.Mutex
			var requests [][]llm.ToolDef
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var body struct {
					Tools []llm.ToolDef `json:"tools"`
				}
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					test.Error(err)
					http.Error(writer, "invalid request", http.StatusBadRequest)
					return
				}
				mutex.Lock()
				requests = append(requests, body.Tools)
				mutex.Unlock()
				writer.Header().Set("Content-Type", "application/json")
				response := `{"id":"response","object":"chat.completion","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"done","type":"function","function":{"name":"task_done","arguments":"{\"state\":\"DONE\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
				if len(body.Tools) == 0 {
					response = `{"id":"plan","object":"chat.completion","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Summary: inspect the file\n\nIssues\n\n(none)"}}]}`
				}
				if _, err := writer.Write([]byte(response)); err != nil {
					test.Error(err)
				}
			}))
			defer server.Close()
			test.Setenv("OCR_LLM_URL", server.URL+"/v1/chat/completions")
			test.Setenv("OCR_LLM_TOKEN", "test-token")
			test.Setenv("OCR_LLM_MODEL", "test-model")
			test.Setenv("OCR_LLM_PROTOCOL", "openai")
			test.Setenv("OCR_USE_ANTHROPIC", "false")
			repo := initTestGitRepo(test)
			if err := os.WriteFile(filepath.Join(repo, "sample.go"), []byte("package sample\n"), 0o600); err != nil {
				test.Fatal(err)
			}
			arguments := []string{"--repo", repo, "--format", "json", "--audience", "agent", "--output", filepath.Join(test.TempDir(), "result.json")}
			var runError error
			if mode == "review" {
				options, err := parseReviewFlags(arguments)
				if err != nil {
					test.Fatal(err)
				}
				runError = executeReview(options)
			} else {
				options, err := parseScanFlags(append(arguments, "--path", "sample.go", "--no-plan", "--no-dedup", "--no-summary"))
				if err != nil {
					test.Fatal(err)
				}
				runError = executeScan(options)
			}
			if runError != nil {
				test.Fatal(runError)
			}
			want := []string{"task_done", "code_comment", "code_search", "file_read", "file_read_diff", "file_find"}
			if mode == "scan" {
				want = []string{"task_done", "code_comment", "code_search", "file_read", "file_find"}
			}
			mutex.Lock()
			defer mutex.Unlock()
			toolRequests := 0
			for _, definitions := range requests {
				if len(definitions) == 0 {
					continue
				}
				toolRequests++
				var names []string
				for _, definition := range definitions {
					names = append(names, definition.Function.Name)
				}
				if !reflect.DeepEqual(names, want) {
					test.Errorf("request tools = %v, want %v", names, want)
				}
			}
			if toolRequests == 0 {
				test.Fatal("no main-task requests captured")
			}
		})
	}
}

// loadTestTemplate returns a validated default template for runtime tests.
func loadTestTemplate(t *testing.T) *template.Template {
	t.Helper()
	tpl, err := template.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	return tpl
}

// TestLoadLLMRuntime_Success resolves an endpoint via OCR_LLM_* env vars (no
// config file on disk, so LoadAppConfig returns nil,nil) and asserts the
// runtime bundle is fully populated.
func TestLoadLLMRuntime_Success(t *testing.T) {
	setTestHome(t, t.TempDir())
	t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok-123")
	t.Setenv("OCR_LLM_MODEL", "test-model")

	tpl := loadTestTemplate(t)
	rt, err := loadLLMRuntime(tpl, llm.ResolveOptions{})
	if err != nil {
		t.Fatalf("loadLLMRuntime error: %v", err)
	}
	if rt.Model != "test-model" {
		t.Errorf("model = %q, want test-model", rt.Model)
	}
	if rt.Source != "OCR environment" {
		t.Errorf("source = %q, want OCR environment", rt.Source)
	}
	if rt.Client == nil {
		t.Error("expected non-nil client")
	}
	if rt.Collector == nil {
		t.Error("expected non-nil collector")
	}
	if rt.RuntimeConfig.EndpointHost != "api.example.test" {
		t.Errorf("endpoint host = %q, want api.example.test", rt.RuntimeConfig.EndpointHost)
	}
}

func TestLoadToolDefsFromTaskTemplates(test *testing.T) {
	reviewTemplate := loadTestTemplate(test)
	var repeated template.LlmConversation
	if err := json.Unmarshal([]byte(`{"tools":["file_find","task_done","file_find","file_read","task_done"],"tool_references":["file_find","code_search","file_find"]}`), &repeated); err != nil {
		test.Fatal(err)
	}
	scanTemplate, err := template.LoadScanDefault()
	if err != nil {
		test.Fatal(err)
	}
	for _, scenario := range []struct {
		name      string
		selection []string
		want      []string
	}{
		{"review main", reviewTemplate.MainTask.Tools, []string{"task_done", "code_comment", "code_search", "file_read", "file_read_diff", "file_find"}},
		{"review plan references", reviewTemplate.PlanTask.ToolReferences, []string{"code_search", "file_read", "file_read_diff", "file_find"}},
		{"review plan calls", reviewTemplate.PlanTask.Tools, nil},
		{"scan main", scanTemplate.MainTask.Tools, []string{"task_done", "code_comment", "code_search", "file_read", "file_find"}},
		{"scan plan calls", scanTemplate.PlanTask.Tools, nil},
		{"scan plan references", scanTemplate.PlanTask.ToolReferences, nil},
		{"explicit order", []string{"file_find", "task_done"}, []string{"file_find", "task_done"}},
		{"repeated tools", repeated.Tools, []string{"file_find", "task_done", "file_read"}},
		{"repeated references", repeated.ToolReferences, []string{"file_find", "code_search"}},
		{"empty selection", nil, nil},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			definitions, err := loadToolDefs(scenario.selection)
			if err != nil {
				test.Fatal(err)
			}
			var names []string
			for _, definition := range definitions {
				names = append(names, definition.Function.Name)
			}
			if !reflect.DeepEqual(names, scenario.want) {
				test.Errorf("tools = %v, want %v", names, scenario.want)
			}
		})
	}
	for _, invalid := range [][]string{{"unknown"}, {"task_done", "task_done", "unknown"}} {
		if _, err := loadToolDefs(invalid); err == nil {
			test.Errorf("expected error for %v", invalid)
		}
	}
}

// TestLoadLLMRuntime_UnresolvableEndpoint covers the ResolveEndpointWithOptions
// failure branch: no config file and no env vars means no endpoint resolves.
func TestLoadLLMRuntime_UnresolvableEndpoint(t *testing.T) {
	setTestHome(t, t.TempDir())
	// Clear any inherited resolution sources.
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	tpl := loadTestTemplate(t)
	_, err := loadLLMRuntime(tpl, llm.ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "resolve LLM endpoint") {
		t.Fatalf("err = %v, want resolve-endpoint failure", err)
	}
}

// TestLoadLLMRuntime_BadAppConfig covers the LoadAppConfig parse-failure branch
// by writing an invalid config.json at the default HOME-based path.
func TestLoadLLMRuntime_BadAppConfig(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	cfgDir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tpl := loadTestTemplate(t)
	_, err := loadLLMRuntime(tpl, llm.ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "load app config") {
		t.Fatalf("err = %v, want load-app-config failure", err)
	}
}
