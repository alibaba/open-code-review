// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
)

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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok-123")
	t.Setenv("OCR_LLM_MODEL", "test-model")

	tpl := loadTestTemplate(t)
	rt, err := loadLLMRuntime(tpl, "", llm.ResolveOptions{})
	if err != nil {
		t.Fatalf("loadLLMRuntime error: %v", err)
	}
	if rt.Model != "test-model" {
		t.Errorf("model = %q, want test-model", rt.Model)
	}
	if rt.Client == nil {
		t.Error("expected non-nil client")
	}
	if rt.Collector == nil {
		t.Error("expected non-nil collector")
	}
	if len(rt.MainToolDefs) == 0 {
		t.Error("expected main tool defs")
	}
	if rt.RuntimeConfig.EndpointHost != "api.example.test" {
		t.Errorf("endpoint host = %q, want api.example.test", rt.RuntimeConfig.EndpointHost)
	}
}

// TestLoadLLMRuntime_BadToolConfig covers the toolsconfig.Load failure branch.
func TestLoadLLMRuntime_BadToolConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tpl := loadTestTemplate(t)
	_, err := loadLLMRuntime(tpl, filepath.Join(t.TempDir(), "no-such-tools.json"), llm.ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "load tools") {
		t.Fatalf("err = %v, want load-tools failure", err)
	}
}

// TestLoadLLMRuntime_UnresolvableEndpoint covers the ResolveEndpointWithOptions
// failure branch: no config file and no env vars means no endpoint resolves.
func TestLoadLLMRuntime_UnresolvableEndpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Clear any inherited resolution sources.
	t.Setenv("OCR_LLM_URL", "")
	t.Setenv("OCR_LLM_TOKEN", "")
	t.Setenv("OCR_LLM_MODEL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_MODEL", "")

	tpl := loadTestTemplate(t)
	_, err := loadLLMRuntime(tpl, "", llm.ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "resolve LLM endpoint") {
		t.Fatalf("err = %v, want resolve-endpoint failure", err)
	}
}

// TestLoadLLMRuntime_BadAppConfig covers the LoadAppConfig parse-failure branch
// by writing an invalid config.json at the default HOME-based path.
func TestLoadLLMRuntime_BadAppConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".opencodereview")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tpl := loadTestTemplate(t)
	_, err := loadLLMRuntime(tpl, "", llm.ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "load app config") {
		t.Fatalf("err = %v, want load-app-config failure", err)
	}
}
