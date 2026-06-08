package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}

// Config represents the user-level configuration file (~/.opencodereview/config.json).
type Config struct {
	Llm       LlmConfig        `json:"llm,omitempty"`
	Language  string           `json:"language,omitempty"`  // Output language, defaults to Chinese when empty
	Telemetry *TelemetryConfig `json:"telemetry,omitempty"` // Telemetry/observability settings
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
	switch key {
	case "llm.url", "llm.URL":
		cfg.Llm.URL = value
	case "llm.auth_token", "llm.AuthToken":
		cfg.Llm.AuthToken = value
	case "llm.auth_header", "llm.AuthHeader":
		cfg.Llm.AuthHeader = normalizeConfigAuthHeader(value)
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
		return fmt.Errorf("unknown config key: %s\nSupported keys: llm.url, llm.auth_token, llm.auth_header, llm.model, llm.use_anthropic, llm.extra_body, language, telemetry.enabled, telemetry.exporter, telemetry.otlp_endpoint, telemetry.content_logging", key)
	}
	return nil
}

func normalizeConfigAuthHeader(header string) string {
	header = strings.TrimSpace(header)
	switch strings.ToLower(header) {
	case "x-api-key":
		return "x-api-key"
	case "authorization", "bearer":
		return "authorization"
	default:
		return header
	}
}

func (c *Config) ensureTelemetry() {
	if c.Telemetry == nil {
		c.Telemetry = &TelemetryConfig{}
	}
}
