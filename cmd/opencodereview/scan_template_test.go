// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestParseScanFlags_PromptOverride(t *testing.T) {
	opts, err := parseScanFlags([]string{"--scan-template", "bounded.json", "--no-plan"})
	if err != nil || opts.scanTemplatePath != "bounded.json" || !opts.noPlan {
		t.Fatalf("prompt flag not forwarded: opts=%+v, err=%v", opts, err)
	}
}

// Capture real HTTP requests to verify the CLI loads, validates, and renders
// the override before calling the model, with the same tools and output budget.
func TestExecuteScan_PromptOverrideRequests(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := initTestGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "handler.go"), []byte("package handler\nfunc Handle() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rulePath := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(rulePath, []byte("{\"rules\":[{\"path\":\"**\",\"rule\":\"RULE_MARKER\"}]}"), 0o600); err != nil {
		t.Fatal(err)
	}
	type request struct {
		System    json.RawMessage `json:"system"`
		Messages  json.RawMessage `json:"messages"`
		Tools     json.RawMessage `json:"tools"`
		MaxTokens int             `json:"max_tokens"`
	}
	var mu sync.Mutex
	var requests []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var got request
		if err := json.Unmarshal(body, &got); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, got)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[{\"type\":\"tool_use\",\"id\":\"done\",\"name\":\"task_done\",\"input\":{}}],\"stop_reason\":\"tool_use\",\"usage\":{\"input_tokens\":100,\"output_tokens\":10}}")
	}))
	t.Cleanup(server.Close)
	t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "claude-test")
	t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
	t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
	t.Setenv("OCR_LLM_TIMEOUT", "5")
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "scan", "bounded-template.json"))
	if err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(t.TempDir(), "bounded.json")
	if err := os.WriteFile(templatePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := scanOptions{
		repoDir: repo, paths: "handler.go", rulePath: rulePath, background: "BACKGROUND_MARKER: Handle only",
		outputFormat: "json", audience: "agent", concurrency: 1, noPlan: true, noSummary: true, noDedup: true,
		maxTokens: 100000,
	}
	run := func(opts scanOptions) {
		t.Helper()
		captureStdout(t, func() {
			if err := executeScan(opts); err != nil {
				t.Errorf("executeScan: %v", err)
			}
		})
	}
	run(opts)
	opts.scanTemplatePath = templatePath
	opts.noPlan = false // PLAN_TASK null must disable the default whole-file planner.
	run(opts)
	mu.Lock()
	captured := append([]request(nil), requests...)
	mu.Unlock()
	if len(captured) != 2 {
		t.Fatalf("default and custom scans must each issue one main request, got %d", len(captured))
	}
	control, custom := captured[0], captured[1]
	if !strings.Contains(string(control.System), "ENTIRE existing source file") ||
		!strings.Contains(string(control.Messages), "review the entire file as usual") {
		t.Fatal("default control did not retain its original contract")
	}
	if !strings.Contains(string(custom.System), "Review only the function selected") {
		t.Fatal("custom system prompt missing from actual request")
	}
	for _, want := range []string{"handler.go", "func Handle()", "RULE_MARKER", "BACKGROUND_MARKER", "Keep the selected review scope"} {
		if !strings.Contains(string(custom.Messages), want) {
			t.Errorf("custom user request missing %q: %s", want, custom.Messages)
		}
	}
	for _, unwanted := range []string{"ENTIRE existing source file", "review the entire file as usual", "{{"} {
		if strings.Contains(string(custom.System)+string(custom.Messages), unwanted) {
			t.Errorf("custom request contains %q", unwanted)
		}
	}
	if custom.MaxTokens != control.MaxTokens || custom.MaxTokens != 16384 || !reflect.DeepEqual(custom.Tools, control.Tools) {
		t.Fatal("prompt override changed the output budget or exposed tool definitions")
	}

	// Invalid templates must make no HTTP requests, including in preview mode.
	for _, preview := range []bool{false, true} {
		if err := os.WriteFile(templatePath, append(data[:len(data)-2], []byte(", \"MAX_TOKENS\": 1}")...), 0o600); err != nil {
			t.Fatal(err)
		}
		opts.preview = preview
		if err := executeScan(opts); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("invalid override did not fail before execution: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("invalid configuration called the model: %d requests", len(requests))
	}
}
