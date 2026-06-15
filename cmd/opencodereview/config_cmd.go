package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/open-code-review/open-code-review/internal/llm"
)

// Default config file location: ~/.opencodereview/config.json
func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "config.json"), nil
}

func runConfig(args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return nil
	}

	switch args[0] {
	case "provider":
		if len(args) != 1 {
			return fmt.Errorf("config provider does not accept arguments; use 'ocr config set provider <name>' for non-interactive setup")
		}
		return runConfigProvider()
	case "model":
		if len(args) != 1 {
			return fmt.Errorf("config model does not accept arguments; use 'ocr config set model <name>' for non-interactive setup")
		}
		return runConfigModel()
	}

	action, err := parseConfigArgs(args)
	if err != nil {
		return err
	}

	switch action.subCmd {
	case "set":
		return runConfigSet(action.key, action.value)
	default:
		return fmt.Errorf("unknown config sub-command: %s", action.subCmd)
	}
}

func runConfigSet(key, value string) error {
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := setConfigValue(cfg, key, value); err != nil {
		return err
	}

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	displayValue := value
	normalizedKey := strings.ToLower(strings.ReplaceAll(key, "_", ""))
	if strings.HasSuffix(normalizedKey, "apikey") || strings.HasSuffix(normalizedKey, "authtoken") {
		displayValue = maskKey(value)
	}
	fmt.Printf("Set %s = %s\n", key, displayValue)
	return nil
}

// ProviderEntry holds per-provider configuration in the providers map.
type ProviderEntry struct {
	APIKey     string         `json:"api_key,omitempty"`
	URL        string         `json:"url,omitempty"`
	Protocol   string         `json:"protocol,omitempty"`
	Model      string         `json:"model,omitempty"`
	AuthHeader string         `json:"auth_header,omitempty"`
	ExtraBody  map[string]any `json:"extra_body,omitempty"`
}

// Config represents the user-level configuration file (~/.opencodereview/config.json).
type Config struct {
	Provider        string                   `json:"provider,omitempty"`
	Model           string                   `json:"model,omitempty"`
	Providers       map[string]ProviderEntry `json:"providers,omitempty"`
	CustomProviders map[string]ProviderEntry `json:"custom_providers,omitempty"`
	Llm             LlmConfig                `json:"llm,omitempty"`
	Language        string                   `json:"language,omitempty"`
	Telemetry       *TelemetryConfig         `json:"telemetry,omitempty"`
}

type LlmConfig struct {
	URL          string         `json:"url,omitempty"`
	AuthToken    string         `json:"auth_token,omitempty"`
	AuthHeader   string         `json:"auth_header,omitempty"`
	Model        string         `json:"model,omitempty"`
	UseAnthropic *bool          `json:"use_anthropic,omitempty"` // nil = default true; false = OpenAI protocol
	ExtraBody    map[string]any `json:"extra_body,omitempty"`
}

// TelemetryConfig holds telemetry-specific settings.
type TelemetryConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`         // Master switch for telemetry
	Exporter     string `json:"exporter,omitempty"`        // "console" or "otlp"
	OTLPEndpoint string `json:"otlp_endpoint,omitempty"`   // OTLP collector address
	ContentLog   bool   `json:"content_logging,omitempty"` // Include prompt/response content
}

func loadOrCreateConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// LoadAppConfig loads config from path. Returns nil, nil if file does not exist.
func LoadAppConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read app config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse app config: %w", err)
	}
	return &cfg, nil
}

func setConfigValue(cfg *Config, key, value string) error {
	// Handle providers.<name>.<field> paths.
	if strings.HasPrefix(key, "providers.") {
		return setProviderValue(cfg, key, value)
	}
	if strings.HasPrefix(key, "custom_providers.") {
		return setCustomProviderValue(cfg, key, value)
	}

	switch key {
	case "provider":
		if cfg.Provider != value {
			cfg.Model = ""
		}
		cfg.Provider = value
		if _, isPreset := llm.LookupProvider(value); isPreset {
			if cfg.Providers == nil {
				cfg.Providers = make(map[string]ProviderEntry)
			}
			if _, exists := cfg.Providers[value]; !exists {
				cfg.Providers[value] = ProviderEntry{}
			}
		} else {
			if cfg.CustomProviders == nil {
				cfg.CustomProviders = make(map[string]ProviderEntry)
			}
			if _, exists := cfg.CustomProviders[value]; !exists {
				cfg.CustomProviders[value] = ProviderEntry{}
			}
		}
	case "model":
		if cfg.Provider != "" {
			if _, isPreset := llm.LookupProvider(cfg.Provider); isPreset {
				if cfg.Providers == nil {
					cfg.Providers = make(map[string]ProviderEntry)
				}
				entry := cfg.Providers[cfg.Provider]
				entry.Model = value
				cfg.Providers[cfg.Provider] = entry
			} else {
				if cfg.CustomProviders == nil {
					cfg.CustomProviders = make(map[string]ProviderEntry)
				}
				entry := cfg.CustomProviders[cfg.Provider]
				entry.Model = value
				cfg.CustomProviders[cfg.Provider] = entry
			}
		} else {
			cfg.Model = value
		}
	case "llm.url", "llm.URL":
		cfg.Llm.URL = value
	case "llm.auth_token", "llm.AuthToken":
		cfg.Llm.AuthToken = value
	case "llm.auth_header", "llm.AuthHeader":
		normalized, err := llm.NormalizeAuthHeader(value)
		if err != nil {
			return err
		}
		cfg.Llm.AuthHeader = normalized
	case "llm.model", "llm.Model":
		cfg.Llm.Model = value
	case "llm.use_anthropic", "llm.UseAnthropic":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for llm.use_anthropic: %w", err)
		}
		cfg.Llm.UseAnthropic = &b
	case "language", "Language":
		cfg.Language = value
	case "telemetry.enabled", "telemetry.Enabled":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for telemetry.enabled: %w", err)
		}
		cfg.ensureTelemetry()
		cfg.Telemetry.Enabled = b
	case "telemetry.exporter", "telemetry.Exporter":
		cfg.ensureTelemetry()
		cfg.Telemetry.Exporter = value
	case "telemetry.otlp_endpoint", "telemetry.OTLPEndpoint":
		cfg.ensureTelemetry()
		cfg.Telemetry.OTLPEndpoint = value
	case "telemetry.content_logging", "telemetry.ContentLog":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for telemetry.content_logging: %w", err)
		}
		cfg.ensureTelemetry()
		cfg.Telemetry.ContentLog = b
	case "llm.extra_body", "llm.ExtraBody":
		var m map[string]any
		if err := json.Unmarshal([]byte(value), &m); err != nil {
			return fmt.Errorf("invalid JSON for llm.extra_body: %w", err)
		}
		cfg.Llm.ExtraBody = m
	default:
		return fmt.Errorf("unknown config key: %s\nSupported keys: provider, model, providers.<name>.<field>, custom_providers.<name>.<field>, llm.url, llm.auth_token, llm.auth_header, llm.model, llm.use_anthropic, llm.extra_body, language, telemetry.enabled, telemetry.exporter, telemetry.otlp_endpoint, telemetry.content_logging", key)
	}
	return nil
}

func applyProviderField(entry *ProviderEntry, field, key, value string) error {
	switch field {
	case "api_key":
		entry.APIKey = value
	case "url":
		entry.URL = value
	case "protocol":
		if value != "anthropic" && value != "openai" {
			return fmt.Errorf("invalid protocol %q: must be \"anthropic\" or \"openai\"", value)
		}
		entry.Protocol = value
	case "model":
		entry.Model = value
	case "auth_header":
		normalized, err := llm.NormalizeAuthHeader(value)
		if err != nil {
			return err
		}
		entry.AuthHeader = normalized
	case "extra_body":
		var m map[string]any
		if err := json.Unmarshal([]byte(value), &m); err != nil {
			return fmt.Errorf("invalid JSON for %s: %w", key, err)
		}
		entry.ExtraBody = m
	default:
		return fmt.Errorf("unknown provider field %q: supported fields are api_key, url, protocol, model, auth_header, extra_body", field)
	}
	return nil
}

func setProviderValue(cfg *Config, key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("invalid provider key %q: expected providers.<name>.<field>", key)
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderEntry)
	}
	entry := cfg.Providers[parts[1]]
	if err := applyProviderField(&entry, parts[2], key, value); err != nil {
		return err
	}
	cfg.Providers[parts[1]] = entry
	return nil
}

func setCustomProviderValue(cfg *Config, key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("invalid custom provider key %q: expected custom_providers.<name>.<field>", key)
	}
	if cfg.CustomProviders == nil {
		cfg.CustomProviders = make(map[string]ProviderEntry)
	}
	entry := cfg.CustomProviders[parts[1]]
	if err := applyProviderField(&entry, parts[2], key, value); err != nil {
		return err
	}
	cfg.CustomProviders[parts[1]] = entry
	return nil
}

func (c *Config) ensureTelemetry() {
	if c.Telemetry == nil {
		c.Telemetry = &TelemetryConfig{}
	}
}
