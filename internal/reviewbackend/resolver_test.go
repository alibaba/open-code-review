package reviewbackend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir string, cfg any) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestResolveBackend_CursorProvider(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "")
	cfgPath := writeConfig(t, t.TempDir(), map[string]any{
		"provider": "cursor",
		"providers": map[string]any{
			"cursor": map[string]any{
				"api_key": "cursor-test-key",
				"model":   "composer-2.5",
			},
		},
	})

	resolved, err := ResolveBackend(cfgPath)
	if err != nil {
		t.Fatalf("ResolveBackend: %v", err)
	}
	if resolved.Kind != KindCursorAgent {
		t.Fatalf("Kind = %q, want %q", resolved.Kind, KindCursorAgent)
	}
	if resolved.Cursor.APIKey != "cursor-test-key" {
		t.Errorf("APIKey = %q, want cursor-test-key", resolved.Cursor.APIKey)
	}
	if resolved.Cursor.Model != "composer-2.5" {
		t.Errorf("Model = %q, want composer-2.5", resolved.Cursor.Model)
	}
	if resolved.Cursor.Source != "provider:cursor" {
		t.Errorf("Source = %q, want provider:cursor", resolved.Cursor.Source)
	}
	if resolved.Endpoint.URL != "" {
		t.Errorf("Endpoint.URL = %q, want empty for cursor", resolved.Endpoint.URL)
	}
}

func TestResolveBackend_CursorProviderKeyCaseInsensitive(t *testing.T) {
	cfgPath := writeConfig(t, t.TempDir(), map[string]any{
		"provider": "cursor",
		"providers": map[string]any{
			"Cursor": map[string]any{
				"api_key": "cursor-test-key",
				"model":   "composer-2.5",
			},
		},
	})

	resolved, err := ResolveBackend(cfgPath)
	if err != nil {
		t.Fatalf("ResolveBackend: %v", err)
	}
	if resolved.Cursor.APIKey != "cursor-test-key" {
		t.Errorf("APIKey = %q, want cursor-test-key", resolved.Cursor.APIKey)
	}
}

func TestResolveBackend_CursorEnvAPIKeyFallback(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "env-cursor-key")
	cfgPath := writeConfig(t, t.TempDir(), map[string]any{
		"provider": "cursor",
		"providers": map[string]any{
			"cursor": map[string]any{
				"model": "auto",
			},
		},
	})

	resolved, err := ResolveBackend(cfgPath)
	if err != nil {
		t.Fatalf("ResolveBackend: %v", err)
	}
	if resolved.Cursor.APIKey != "env-cursor-key" {
		t.Errorf("APIKey = %q, want env-cursor-key", resolved.Cursor.APIKey)
	}
}

func TestResolveBackend_CursorMissingAPIKey(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "")
	cfgPath := writeConfig(t, t.TempDir(), map[string]any{
		"provider": "cursor",
		"providers": map[string]any{
			"cursor": map[string]any{
				"model": "auto",
			},
		},
	})

	_, err := ResolveBackend(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing api key")
	}
}

func TestResolveBackend_CursorMissingModel(t *testing.T) {
	cfgPath := writeConfig(t, t.TempDir(), map[string]any{
		"provider": "cursor",
		"providers": map[string]any{
			"cursor": map[string]any{
				"api_key": "key",
			},
		},
	})

	_, err := ResolveBackend(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestResolveBackend_ChatCompletionsUnchanged(t *testing.T) {
	t.Setenv("OCR_LLM_URL", "https://api.example.com/v1/chat/completions")
	t.Setenv("OCR_LLM_TOKEN", "test-token")
	t.Setenv("OCR_LLM_MODEL", "gpt-4o")

	cfgPath := filepath.Join(t.TempDir(), "missing.json")
	resolved, err := ResolveBackend(cfgPath)
	if err != nil {
		t.Fatalf("ResolveBackend: %v", err)
	}
	if resolved.Kind != KindChatCompletions {
		t.Fatalf("Kind = %q, want %q", resolved.Kind, KindChatCompletions)
	}
	if resolved.Endpoint.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", resolved.Endpoint.Model)
	}
}
