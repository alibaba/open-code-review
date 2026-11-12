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
	"github.com/alibaba/open-code-review/internal/session"
)

func TestComparisonToolsReachModelAndManifest(test *testing.T) {
	freshOCRHome(test)
	repo, err := resolveRepoDir(initTestGitRepo(test))
	if err != nil {
		test.Fatal(err)
	}
	commitFile(test, repo, "helper.go", "package example\nconst Helper = 1\n", "old helper")
	commitFile(test, repo, "version.go", "package example\nconst Value = \"old-marker\"\n", "old")
	base := revParse(test, repo, "HEAD")
	commitFile(test, repo, "helper.go", "package example\nconst Helper = 2\n", "change helper")
	advancedBase := revParse(test, repo, "HEAD")
	commitFile(test, repo, "version.go", "package example\nconst Value = \"new-marker\"\n", "new")
	head := revParse(test, repo, "HEAD")
	var mutex sync.Mutex
	called := false
	seen := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Tools    []llm.ToolDef `json:"tools"`
			Messages []struct {
				Role       string          `json:"role"`
				Name       string          `json:"name"`
				ToolCallID string          `json:"tool_call_id"`
				Content    json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			test.Error(err)
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		for _, message := range body.Messages {
			if message.Role != "tool" {
				continue
			}
			var content string
			if err := json.Unmarshal(message.Content, &content); err != nil {
				test.Error(err)
				continue
			}
			if strings.HasPrefix(content, "SIDE: old\nREVISION: "+base+"\n") {
				seen[message.ToolCallID] = true
			} else if strings.HasPrefix(message.ToolCallID, "old-") {
				test.Errorf("old-side tool result lost its source: %s", content)
			}
		}
		for _, definition := range body.Tools {
			name := definition.Function.Name
			if name == "file_read" || name == "file_find" || name == "code_search" {
				properties := definition.Function.Parameters["properties"].(map[string]any)
				if _, exists := properties["side"]; !exists {
					test.Errorf("model did not receive side for %s", name)
				}
			}
		}
		response := `{"id":"done","object":"chat.completion","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"done","type":"function","function":{"name":"task_done","arguments":"{\"state\":\"DONE\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
		if len(body.Tools) == 0 {
			response = `{"id":"plan","object":"chat.completion","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Inspect the changed file."}}]}`
		} else if !called {
			called = true
			response = `{"id":"read","object":"chat.completion","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"old-read","type":"function","function":{"name":"file_read","arguments":"{\"file_path\":\"version.go\",\"side\":\"old\"}"}},{"id":"old-search","type":"function","function":{"name":"code_search","arguments":"{\"search_text\":\"old-marker\",\"side\":\"old\"}"}},{"id":"old-find","type":"function","function":{"name":"file_find","arguments":"{\"query_name\":\"version.go\",\"side\":\"old\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
		}
		writer.Header().Set("Content-Type", "application/json")
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
	output := filepath.Join(test.TempDir(), "result.json")
	opts, err := parseReviewFlags([]string{"--repo", repo, "--from", base, "--to", "HEAD", "--exclude", "helper.go", "--format", "json", "--audience", "agent", "--output", output})
	if err != nil {
		test.Fatal(err)
	}
	if err := executeReview(opts); err != nil {
		test.Fatal(err)
	}
	mutex.Lock()
	if len(seen) != 3 {
		test.Errorf("expected all old-side results, got %v", seen)
	}
	mutex.Unlock()
	raw, err := os.ReadFile(output)
	if err != nil {
		test.Fatal(err)
	}
	var result jsonOutput
	if err := json.Unmarshal(raw, &result); err != nil {
		test.Fatal(err)
	}
	if result.Manifest == nil || result.Manifest.Input.ResolvedBase != base || result.Manifest.Input.ResolvedHead != head {
		test.Fatalf("manifest lost comparison endpoints: %+v", result.Manifest)
	}
	if strings.Contains(string(raw), "code_access_profile") {
		test.Fatal("manifest persisted a code access profile")
	}
	state, err := loadReviewResumeState(repo, reviewOptions{resume: result.SessionID, from: base, to: "HEAD"})
	if err != nil {
		test.Fatal(err)
	}
	if state.Manifest == nil || state.Manifest.Input.ResolvedBase != base || state.Manifest.Input.ResolvedHead != head {
		test.Fatalf("persisted endpoints did not survive resume loading: %+v", state.Manifest)
	}

	test.Run("equivalent refs preserve reuse", func(test *testing.T) {
		resumed := opts
		resumed.resume = result.SessionID
		resumed.from = "HEAD~2"
		resumed.to = head
		resumed.outputPath = filepath.Join(test.TempDir(), "resumed.json")
		if err := executeReview(resumed); err != nil {
			test.Fatal(err)
		}
		raw, err := os.ReadFile(resumed.outputPath)
		if err != nil {
			test.Fatal(err)
		}
		var result jsonOutput
		if err := json.Unmarshal(raw, &result); err != nil {
			test.Fatal(err)
		}
		if result.Manifest == nil || len(result.Manifest.Coverage.Reused) != 1 || result.Manifest.Input.ResolvedBase != base || result.Manifest.Input.ResolvedHead != head {
			test.Fatalf("equivalent refs lost the comparison or reuse: %+v", result.Manifest)
		}
	})

	test.Run("legacy current-only session uses comparison tools", func(test *testing.T) {
		parent := session.New(repo, "feature", "test-model", session.SessionOptions{
			ReviewMode: session.ReviewModeRange, DiffFrom: base, DiffTo: head, Operation: session.OperationReview,
		})
		builder := parent.Manifest()
		builder.SetRepository(result.Manifest.Repository)
		builder.SetInput(result.Manifest.Input)
		builder.SetExecution(result.Manifest.Execution)
		for _, item := range result.Manifest.Coverage.Selected {
			if err := builder.RegisterSelected(item); err != nil {
				test.Fatal(err)
			}
			if err := builder.MarkFailed(item.ItemID, session.FailureProvider, "interrupted request"); err != nil {
				test.Fatal(err)
			}
		}
		manifest, err := builder.Finalize(0)
		if err != nil {
			test.Fatal(err)
		}
		parent.SetFinalManifest(&manifest)
		if err := parent.Finalize(); err != nil {
			test.Fatal(err)
		}
		path, err := session.SessionFilePath(repo, parent.SessionID)
		if err != nil {
			test.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			test.Fatal(err)
		}
		legacy := strings.Replace(string(raw), `"execution":{`, `"execution":{"code_access_profile":"current-only/v1",`, 1)
		if legacy == string(raw) {
			test.Fatal("legacy session fixture has no execution metadata")
		}
		if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
			test.Fatal(err)
		}
		mutex.Lock()
		called = false
		seen = make(map[string]bool)
		mutex.Unlock()
		resumed := opts
		resumed.resume = parent.SessionID
		resumed.outputPath = filepath.Join(test.TempDir(), "legacy-resumed.json")
		if err := executeReview(resumed); err != nil {
			test.Fatal(err)
		}
		mutex.Lock()
		if len(seen) != 3 {
			test.Errorf("legacy resume did not execute all old-side tools: %v", seen)
		}
		mutex.Unlock()
		raw, err = os.ReadFile(resumed.outputPath)
		if err != nil {
			test.Fatal(err)
		}
		var result jsonOutput
		if err := json.Unmarshal(raw, &result); err != nil {
			test.Fatal(err)
		}
		if result.Manifest == nil || len(result.Manifest.Coverage.Completed) != 1 || len(result.Manifest.Coverage.Reused) != 0 {
			test.Fatalf("legacy resume did not complete the pending file: %+v", result.Manifest)
		}
		if strings.Contains(string(raw), "code_access_profile") {
			test.Fatal("legacy resume preserved a code access profile")
		}
	})

	commitFile(test, repo, "helper.go", "package example\nconst Helper = 3\n", "advance head")
	advancedHead := revParse(test, repo, "HEAD")
	for _, scenario := range []struct {
		name string
		from string
		to   string
	}{
		{name: "changed base with identical selected patch", from: advancedBase, to: head},
		{name: "changed head with identical selected patch", from: base, to: advancedHead},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			resumed := opts
			resumed.resume = result.SessionID
			resumed.from = scenario.from
			resumed.to = scenario.to
			resumed.outputPath = filepath.Join(test.TempDir(), "rejected.json")
			before := countSessionFiles(test, repo)
			if err := executeReview(resumed); err == nil || !strings.Contains(err.Error(), "comparison endpoints changed") {
				test.Errorf("expected endpoint rejection despite an identical selected patch, got %v", err)
			}
			if after := countSessionFiles(test, repo); after != before {
				test.Errorf("rejected resume created %d session files", after-before)
			}
		})
	}
}

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
				base, err := loadToolDefs(want)
				if err != nil {
					test.Fatal(err)
				}
				baseJSON, _ := json.Marshal(base)
				requestJSON, _ := json.Marshal(definitions)
				if string(baseJSON) != string(requestJSON) {
					test.Error("current-only command changed the model-visible tool definitions")
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
