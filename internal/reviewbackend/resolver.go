package reviewbackend

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/open-code-review/open-code-review/internal/llm"
)

// CursorConfig holds resolved Cursor Agent SDK settings.
type CursorConfig struct {
	APIKey string
	Model  string
	Source string
}

// ResolvedBackend is the outcome of configuration resolution.
type ResolvedBackend struct {
	Kind     Kind
	Endpoint llm.ResolvedEndpoint
	Cursor   CursorConfig
}

// ResolveBackend reads OCR config and returns the appropriate backend kind.
func ResolveBackend(configPath string) (ResolvedBackend, error) {
	data, cfg, err := readConfigBytes(configPath)
	if err != nil {
		return ResolvedBackend{}, err
	}
	if cfg != nil && strings.EqualFold(cfg.Provider, "cursor") {
		return resolveCursorProvider(cfg)
	}

	if len(data) > 0 {
		ep, ok, err := llm.TryOCRConfigBytes(data)
		if err != nil {
			return ResolvedBackend{}, err
		}
		if ok {
			return ResolvedBackend{Kind: KindChatCompletions, Endpoint: ep}, nil
		}
	}

	ep, err := llm.ResolveEndpoint(configPath)
	if err != nil {
		return ResolvedBackend{}, err
	}
	return ResolvedBackend{Kind: KindChatCompletions, Endpoint: ep}, nil
}

type providerEntry struct {
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model,omitempty"`
}

type configFile struct {
	Provider  string                   `json:"provider,omitempty"`
	Model     string                   `json:"model,omitempty"`
	Providers map[string]providerEntry `json:"providers,omitempty"`
}

func readConfigBytes(path string) ([]byte, *configFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	return data, &cfg, nil
}

func resolveCursorProvider(cfg *configFile) (ResolvedBackend, error) {
	entry, ok := cfg.Providers["cursor"]
	if !ok {
		return ResolvedBackend{}, fmt.Errorf("provider %q is set but not configured in providers section", cfg.Provider)
	}

	apiKey := entry.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("CURSOR_API_KEY")
	}
	if apiKey == "" {
		return ResolvedBackend{}, fmt.Errorf("provider %q has no api_key configured and CURSOR_API_KEY is not set", cfg.Provider)
	}

	model := cfg.Model
	if entry.Model != "" {
		model = entry.Model
	}
	if model == "" {
		return ResolvedBackend{}, fmt.Errorf("provider %q has no model configured; run 'ocr config model' to select one", cfg.Provider)
	}

	return ResolvedBackend{
		Kind: KindCursorAgent,
		Cursor: CursorConfig{
			APIKey: apiKey,
			Model:  model,
			Source: "provider:cursor",
		},
	}, nil
}
