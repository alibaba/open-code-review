package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/open-code-review/open-code-review/internal/llm"
)

func runConfigProvider() error {
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	m := newProviderTUI(cfg)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	final := finalModel.(providerTUIModel)

	if len(final.deletedProviders) > 0 {
		clearedActive, err := applyProviderDeletions(configPath, cfg, final.deletedProviders)
		if err != nil {
			return err
		}
		if clearedActive && !final.confirmed {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: active provider was deleted; 'provider' and 'model' have been cleared.\n")
			fmt.Fprintf(os.Stderr, "[ocr] Run 'ocr config provider' to select a new provider.\n")
		}
	}

	if !final.confirmed {
		if len(final.deletedProviders) > 0 {
			return nil
		}
		fmt.Println("Cancelled.")
		return nil
	}

	result := final.result()

	if result.isManual {
		return applyManualConfig(configPath, cfg, result)
	}

	if result.isCustom {
		return applyCustomProviderConfig(configPath, cfg, result)
	}

	return applyOfficialProviderConfig(configPath, cfg, result)
}

func applyProviderDeletions(configPath string, cfg *Config, names []string) (bool, error) {
	clearedActive := false
	for _, name := range names {
		wasActive, err := deleteCustomProvider(cfg, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ocr] skip delete %q: %v\n", name, err)
			continue
		}
		if wasActive {
			clearedActive = true
		}
		fmt.Printf("Deleted custom provider %q.\n", name)
	}
	if err := saveConfig(configPath, cfg); err != nil {
		return false, err
	}
	return clearedActive, nil
}

func applyManualConfig(configPath string, cfg *Config, result providerTUIResult) error {
	if result.url == "" {
		return fmt.Errorf("URL is required for manual configuration")
	}
	if result.model == "" {
		return fmt.Errorf("model is required for manual configuration")
	}

	cfg.Provider = ""
	cfg.Model = ""
	cfg.Llm.URL = result.url
	cfg.Llm.Model = result.model
	cfg.Llm.AuthToken = result.apiKey

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Println("\nManual configuration saved.")
	fmt.Printf("URL: %s\n", result.url)
	fmt.Printf("Model: %s\n", result.model)

	fmt.Println("\nTesting connection...")
	if err := runLLMTest(); err != nil {
		fmt.Fprintf(os.Stderr, "Connection test failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Configuration has been saved. Fix the issue and run 'ocr llm test' to re-verify.")
		return nil
	}

	return nil
}

func applyCustomProviderConfig(configPath string, cfg *Config, result providerTUIResult) error {
	if result.provider == "" {
		return fmt.Errorf("provider name is required")
	}
	if result.model == "" {
		return fmt.Errorf("model is required")
	}

	if cfg.CustomProviders == nil {
		cfg.CustomProviders = make(map[string]ProviderEntry)
	}

	entry := cfg.CustomProviders[result.provider]
	entry.Model = result.model
	if len(result.models) > 0 {
		entry.Models = mergeModelLists([]string{result.model}, result.models)
	}
	if result.url != "" {
		entry.URL = result.url
	}
	if result.protocol != "" {
		entry.Protocol = result.protocol
	}
	if result.authHeader != "" {
		entry.AuthHeader = result.authHeader
	}
	if result.apiKey != "" {
		entry.APIKey = result.apiKey
	}
	cfg.CustomProviders[result.provider] = entry

	if cfg.Provider != result.provider {
		cfg.Model = ""
	}
	cfg.Provider = result.provider

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("\nProvider set to: %s (custom)\n", result.provider)
	fmt.Printf("Model: %s\n", result.model)

	fmt.Println("\nTesting connection...")
	if err := runLLMTest(); err != nil {
		fmt.Fprintf(os.Stderr, "Connection test failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Provider configuration has been saved. Fix the issue and run 'ocr llm test' to re-verify.")
		return nil
	}

	fmt.Println("\nTip: run 'ocr config model' to switch model later.")
	return nil
}

func applyOfficialProviderConfig(configPath string, cfg *Config, result providerTUIResult) error {
	if result.provider == "" || result.model == "" {
		return fmt.Errorf("provider and model are required")
	}

	preset, isPreset := llm.LookupProvider(result.provider)

	if result.apiKey == "" {
		if isPreset && preset.EnvVar != "" {
			if os.Getenv(preset.EnvVar) == "" {
				return fmt.Errorf("API key is required for provider %s (configure it or set $%s)", result.provider, preset.EnvVar)
			}
		} else {
			return fmt.Errorf("API key is required for provider %s", result.provider)
		}
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderEntry)
	}

	entry := cfg.Providers[result.provider]
	entry.Model = result.model
	if len(result.models) > 0 {
		entry.Models = mergeModelLists(entry.Models, result.models)
	}
	if result.apiKey != "" {
		entry.APIKey = result.apiKey
	}
	cfg.Providers[result.provider] = entry

	if cfg.Provider != result.provider {
		cfg.Model = ""
	}
	cfg.Provider = result.provider

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("\nProvider set to: %s\n", result.provider)
	fmt.Printf("Model: %s\n", result.model)

	fmt.Println("\nTesting connection...")
	if err := runLLMTest(); err != nil {
		fmt.Fprintf(os.Stderr, "Connection test failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "Provider configuration has been saved. Fix the issue and run 'ocr llm test' to re-verify.")
		return nil
	}

	fmt.Println("\nTip: run 'ocr config model' to switch model later.")
	return nil
}

func runConfigModel() error {
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Provider == "" {
		return fmt.Errorf("no provider configured. Run 'ocr config provider' first")
	}

	currentModel := ""
	provider := llm.Provider{Name: cfg.Provider, DisplayName: cfg.Provider}
	isCustom := false
	if preset, isPreset := llm.LookupProvider(cfg.Provider); isPreset {
		provider = preset
		if entry, ok := cfg.Providers[cfg.Provider]; ok {
			currentModel = entry.Model
			provider.Models = mergeModelLists(provider.Models, entry.Models)
		}
	} else {
		isCustom = true
		entry, ok := cfg.CustomProviders[cfg.Provider]
		if !ok {
			return fmt.Errorf("provider %q is not configured in custom_providers", cfg.Provider)
		}
		currentModel = entry.Model
		provider.DisplayName = cfg.Provider + " (custom)"
		provider.Protocol = entry.Protocol
		provider.BaseURL = entry.URL
		provider.Models = mergeModelLists(entry.Models)
	}
	if currentModel == "" {
		currentModel = cfg.Model
	}

	m := newModelTUI(provider, currentModel)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	final := finalModel.(modelTUIModel)
	if final.cancelled {
		fmt.Println("Cancelled.")
		return nil
	}

	selectedModel := final.selectedModel()
	if selectedModel == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	if isCustom {
		if cfg.CustomProviders == nil {
			cfg.CustomProviders = make(map[string]ProviderEntry)
		}
		entry := cfg.CustomProviders[cfg.Provider]
		entry.Model = selectedModel
		entry.Models = mergeModelLists([]string{selectedModel}, entry.Models)
		cfg.CustomProviders[cfg.Provider] = entry
	} else {
		if cfg.Providers == nil {
			cfg.Providers = make(map[string]ProviderEntry)
		}
		entry := cfg.Providers[cfg.Provider]
		entry.Model = selectedModel
		if !modelListContains(provider.Models, selectedModel) {
			entry.Models = mergeModelLists([]string{selectedModel}, entry.Models)
		}
		cfg.Providers[cfg.Provider] = entry
	}

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("\nModel set to: %s\n", selectedModel)
	return nil
}

func saveConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}

func maskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}
