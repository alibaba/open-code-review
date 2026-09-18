// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLLMTest_CodingPlan(t *testing.T) {
	for _, provider := range []string{"dashscope-codingplan", "dashscope-codingplan-intl"} {
		t.Run(provider, func(t *testing.T) {
			setTestHome(t, t.TempDir())
			t.Setenv("OCR_LLM_TIMEOUT", "")
			t.Setenv("OCR_LLM_EXTRA_HEADERS", "")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-sp-test-key" {
					t.Errorf("unexpected connectivity request: %s", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				if body["model"] != "qwen3.7-plus" {
					t.Errorf("model = %v", body["model"])
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"test","object":"chat.completion","model":"qwen3.7-plus","choices":[{"index":0,"message":{"role":"assistant","content":"Connected."},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			data, err := json.Marshal(map[string]any{
				"provider": provider,
				"providers": map[string]any{provider: map[string]any{
					"api_key": "sk-sp-test-key", "model": "qwen3.7-plus", "url": server.URL + "/v1",
				}},
			})
			if err != nil {
				t.Fatalf("marshal config: %v", err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			t.Setenv("OCR_CONFIG_PATH", path)
			output := captureStdout(t, func() {
				if err := runLLMTest(); err != nil {
					t.Errorf("runLLMTest: %v", err)
				}
			})
			for _, want := range []string{"Source: provider:" + provider, "URL:    " + server.URL + "/v1", "Model:  qwen3.7-plus", "Connection test successful"} {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q: %s", want, output)
				}
			}
		})
	}
}
