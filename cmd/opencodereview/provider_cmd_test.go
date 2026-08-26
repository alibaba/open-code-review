// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateLLMConnectionTest keeps the "Testing connection..." step that ends
// every apply*Config call away from the developer's own machine. Without it
// resolveConfigPath() falls back to ~/.opencodereview/config.json and `go test`
// resolves a real endpoint: with providers.<name>.api_key_cmd configured that
// runs the credential helper and blocks on a pinentry/Touch ID prompt for up to
// the 60s credential timeout, and with a static key it fires a real request.
//
// The path points at a file that does not exist, so resolution fails fast the
// way it already does on a machine with no config. HOME is redirected into an
// empty temp dir as well, so the shell-rc strategy has nothing to read either.
func isolateLLMConnectionTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("OCR_CONFIG_PATH", filepath.Join(dir, "no-such-config.json"))
	// Both, because os.UserHomeDir reads USERPROFILE on Windows and never falls
	// back to HOME -- setting HOME alone would leave the shell-rc strategy reading
	// the real profile.
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	for _, k := range []string{
		"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL",
		"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL",
	} {
		t.Setenv(k, "")
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"empty", "", "(not set)"},
		{"short", "abcd", "***"},
		{"exactly 8", "12345678", "***"},
		{"normal", "sk-ant-secret-key-1234", "sk-a***1234"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskKey(tc.key)
			if got != tc.want {
				t.Errorf("maskKey(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")

	cfg := &Config{
		Provider: "anthropic",
		Model:    "claude-opus-4-6",
		Language: "English",
	}

	if err := saveConfig(path, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows reports 0666 regardless of the mode passed to OpenFile, so only the
	// unix arms can assert the 0600 the config file is written with.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %o, want 600", perm)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.Provider != "anthropic" {
		t.Errorf("Provider = %q", loaded.Provider)
	}
	if loaded.Language != "English" {
		t.Errorf("Language = %q", loaded.Language)
	}
}

func TestApplyProviderDeletions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := &Config{
		Provider: "keep",
		CustomProviders: map[string]ProviderEntry{
			"del1": {URL: "https://a.example.com"},
			"del2": {URL: "https://b.example.com"},
			"keep": {URL: "https://c.example.com"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	clearedActive, err := applyProviderDeletions(configPath, cfg, []string{"del1", "del2"})
	if err != nil {
		t.Fatalf("applyProviderDeletions: %v", err)
	}
	if clearedActive {
		t.Error("should not have cleared active provider")
	}
	if _, exists := cfg.CustomProviders["del1"]; exists {
		t.Error("del1 should have been deleted")
	}
	if _, exists := cfg.CustomProviders["del2"]; exists {
		t.Error("del2 should have been deleted")
	}
	if _, exists := cfg.CustomProviders["keep"]; !exists {
		t.Error("keep should still exist")
	}
}

func TestApplyProviderDeletions_ActiveCleared(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := &Config{
		Provider: "active-one",
		CustomProviders: map[string]ProviderEntry{
			"active-one": {URL: "https://x.example.com"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	clearedActive, err := applyProviderDeletions(configPath, cfg, []string{"active-one"})
	if err != nil {
		t.Fatalf("applyProviderDeletions: %v", err)
	}
	if !clearedActive {
		t.Error("should have cleared active provider")
	}
}

func TestApplyProviderDeletions_SkipsNotFound(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := &Config{
		CustomProviders: map[string]ProviderEntry{
			"exists": {URL: "https://a.example.com"},
		},
	}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	_, err := applyProviderDeletions(configPath, cfg, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("applyProviderDeletions should not fail, got: %v", err)
	}
}

func TestRemoveModels(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		remove   []string
		want     []string
	}{
		{"remove one", []string{"a", "b", "c"}, []string{"b"}, []string{"a", "c"}},
		{"remove none", []string{"a", "b"}, []string{"x"}, []string{"a", "b"}},
		{"remove all", []string{"a", "b"}, []string{"a", "b"}, []string{}},
		{"empty existing", nil, []string{"a"}, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := removeModels(tc.existing, tc.remove)
			if len(got) != len(tc.want) {
				t.Fatalf("removeModels() = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestApplyManualConfig_MissingURL(t *testing.T) {
	err := applyManualConfig("", &Config{}, providerTUIResult{url: "", model: "m"})
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestApplyManualConfig_MissingModel(t *testing.T) {
	err := applyManualConfig("", &Config{}, providerTUIResult{url: "https://example.com", model: ""})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestApplyCustomProviderConfig_MissingProvider(t *testing.T) {
	err := applyCustomProviderConfig("", &Config{}, providerTUIResult{provider: "", model: "m"})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestApplyCustomProviderConfig_MissingModel(t *testing.T) {
	err := applyCustomProviderConfig("", &Config{}, providerTUIResult{provider: "p", model: ""})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestApplyOfficialProviderConfig_MissingFields(t *testing.T) {
	err := applyOfficialProviderConfig("", &Config{}, providerTUIResult{provider: "", model: ""})
	if err == nil {
		t.Fatal("expected error for missing provider/model")
	}
}

func TestApplyOfficialProviderConfig_EmptyKeyClearsSavedAPIKey(t *testing.T) {
	isolateLLMConnectionTest(t)
	t.Setenv("DEEPSEEK_API_KEY", "sk-from-env")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {
				APIKey: "old-saved-key",
				Model:  "deepseek-v4-flash",
			},
		},
	}

	err := applyOfficialProviderConfig(configPath, cfg, providerTUIResult{
		provider: "deepseek",
		model:    "deepseek-v4-flash",
		apiKey:   "",
	})
	if err != nil {
		t.Fatalf("applyOfficialProviderConfig: %v", err)
	}
	if got := cfg.Providers["deepseek"].APIKey; got != "" {
		t.Errorf("in-memory APIKey = %q, want empty", got)
	}
	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := diskCfg.Providers["deepseek"].APIKey; got != "" {
		t.Errorf("persisted APIKey = %q, want empty", got)
	}
}

// A provider configured with only api_key_cmd must survive a trip through the
// TUI: picking a model returns an empty apiKey, which must not be mistaken for
// "no credential" and abandon the save.
func TestApplyOfficialProviderConfig_APIKeyCmdSatisfiesRequirement(t *testing.T) {
	isolateLLMConnectionTest(t)
	t.Setenv("DEEPSEEK_API_KEY", "")
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{
		Providers: map[string]ProviderEntry{
			"deepseek": {APIKeyCmd: "op read op://dev/deepseek/api-key"},
		},
	}

	err := applyOfficialProviderConfig(configPath, cfg, providerTUIResult{
		provider: "deepseek",
		model:    "deepseek-v4-flash",
		apiKey:   "",
	})
	if err != nil {
		t.Fatalf("api_key_cmd should satisfy the API key requirement: %v", err)
	}
	diskCfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if diskCfg.Provider != "deepseek" || diskCfg.Model != "deepseek-v4-flash" {
		t.Errorf("save was abandoned: provider=%q model=%q", diskCfg.Provider, diskCfg.Model)
	}
	if got := diskCfg.Providers["deepseek"].APIKeyCmd; got != "op read op://dev/deepseek/api-key" {
		t.Errorf("persisted api_key_cmd = %q, want it preserved", got)
	}
}

func TestApplyCustomProviderConfig_EmptyKeyClearsSavedAPIKey(t *testing.T) {
	isolateLLMConnectionTest(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "aaa",
		Model:    "test",
		CustomProviders: map[string]ProviderEntry{
			"aaa": {
				URL:      "https://example.com/v1",
				Protocol: "openai",
				APIKey:   "old-saved-key",
				Model:    "test",
				Models:   []string{"test"},
			},
		},
	}

	err := applyCustomProviderConfig(configPath, cfg, providerTUIResult{
		provider: "aaa",
		model:    "test",
		models:   []string{"test"},
		apiKey:   "",
		isCustom: true,
		url:      "https://example.com/v1",
		protocol: "openai",
	})
	if err != nil {
		t.Fatalf("applyCustomProviderConfig: %v", err)
	}
	if got := cfg.CustomProviders["aaa"].APIKey; got != "" {
		t.Errorf("APIKey = %q, want empty", got)
	}
}

func TestProviderTUIResult_ResolvedModel(t *testing.T) {
	r := providerTUIResult{
		provider: "baidu-qianfan",
		model:    "glm-5",
	}
	if got := r.resolvedModel(); got != "glm-5" {
		t.Errorf("resolvedModel() = %q, want glm-5", got)
	}

	r = providerTUIResult{
		provider: "baidu-qianfan",
		sessionModelPick: map[string]string{
			"baidu-qianfan": "glm-5",
		},
	}
	if got := r.resolvedModel(); got != "glm-5" {
		t.Errorf("resolvedModel() from session pick = %q, want glm-5", got)
	}

	r = providerTUIResult{provider: "baidu-qianfan"}
	if got := r.resolvedModel(); got != "" {
		t.Errorf("resolvedModel() = %q, want empty", got)
	}
}

func TestApplyOfficialProviderConfig_UsesSessionModelPick(t *testing.T) {
	isolateLLMConnectionTest(t)
	t.Setenv("QIANFAN_API_KEY", "sk-from-env")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Provider: "deepseek",
		Model:    "deepseek-v4-flash",
		Providers: map[string]ProviderEntry{
			"deepseek": {Model: "deepseek-v4-flash"},
		},
	}

	err := applyOfficialProviderConfig(configPath, cfg, providerTUIResult{
		provider: "baidu-qianfan",
		apiKey:   "",
		sessionModelPick: map[string]string{
			"baidu-qianfan": "glm-5",
		},
	})
	if err != nil {
		t.Fatalf("applyOfficialProviderConfig: %v", err)
	}
	if cfg.Provider != "baidu-qianfan" {
		t.Errorf("Provider = %q, want baidu-qianfan", cfg.Provider)
	}
	if cfg.Model != "glm-5" {
		t.Errorf("Model = %q, want glm-5", cfg.Model)
	}
}

func TestPrintWizardCancelled(t *testing.T) {
	tests := []struct {
		name           string
		savedInSession bool
		scope          string
		want           string
	}{
		{
			name:           "no changes",
			savedInSession: false,
			scope:          "Configuration changes",
			want:           "Cancelled.\n",
		},
		{
			name:           "provider wizard kept changes",
			savedInSession: true,
			scope:          "Configuration changes",
			want:           "Cancelled. (Configuration changes made during this session were kept.)\n",
		},
		{
			name:           "model wizard kept changes",
			savedInSession: true,
			scope:          "Model list changes",
			want:           "Cancelled. (Model list changes made during this session were kept.)\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := os.Stdout
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout = w
			printWizardCancelled(tc.savedInSession, tc.scope)
			_ = w.Close()
			os.Stdout = old
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("output = %q, want %q", string(got), tc.want)
			}
		})
	}
}

// TestApplyOfficialProviderConfig_PreservesURLWhenWizardOmitsURL verifies that
// the URL configured through `ocr config set` survives a later provider wizard
// confirmation, whose official flow no longer edits Base URL.
func TestApplyOfficialProviderConfig_PreservesURLWhenWizardOmitsURL(t *testing.T) {
	t.Setenv("LITELLM_API_KEY", "sk-litellm")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	wantURL := "https://old-gateway.internal:9000/v1"
	cfg := &Config{
		Providers: map[string]ProviderEntry{
			"litellm": {URL: wantURL},
		},
	}

	err := applyOfficialProviderConfig(configPath, cfg, providerTUIResult{
		provider: "litellm",
		model:    "openai/gpt-5.4",
		apiKey:   "sk-litellm",
	})
	if err != nil {
		t.Fatalf("applyOfficialProviderConfig: %v", err)
	}
	if got := cfg.Providers["litellm"].URL; got != wantURL {
		t.Errorf("persisted URL = %q, want existing override %q", got, wantURL)
	}
}

func TestApplyOfficialProviderConfig_IgnoresURLFromResult(t *testing.T) {
	t.Setenv("LITELLM_API_KEY", "sk-litellm")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	wantURL := "https://configured-gateway.internal:9000/v1"
	cfg := &Config{
		Providers: map[string]ProviderEntry{
			"litellm": {URL: wantURL},
		},
	}

	err := applyOfficialProviderConfig(configPath, cfg, providerTUIResult{
		provider: "litellm",
		model:    "openai/gpt-5.4",
		apiKey:   "sk-litellm",
		url:      "https://stale-result.internal:8000/v1",
	})
	if err != nil {
		t.Fatalf("applyOfficialProviderConfig: %v", err)
	}
	if got := cfg.Providers["litellm"].URL; got != wantURL {
		t.Errorf("persisted URL = %q, want existing URL %q", got, wantURL)
	}
}
