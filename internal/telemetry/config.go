// Package telemetry provides OpenTelemetry-based observability for OpenCodeReview CLI.
// It supports console output (for personal use) and OTLP export (for system integration).
package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	defaultServiceName    = "open-code-review"
	defaultOTLPEndpoint   = ""
	defaultExporter       = "console"
	defaultContentLogging = false
)

// Config holds resolved telemetry configuration.
type Config struct {
	Enabled      bool   // Master switch; false when OCR_ENABLE_TELEMETRY is unset
	ServiceName  string // Service name in traces/metrics
	Exporter     string // "console" or "otlp"
	OTLPEndpoint string // OTLP collector address (grpc/http)
	OTLPProtocol string // "grpc", "http/protobuf", "http/json"
	ContentLog   bool   // Include prompt/response content in log events
}

// DefaultConfig returns a disabled default configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		ServiceName:  defaultServiceName,
		Exporter:     defaultExporter,
		OTLPEndpoint: defaultOTLPEndpoint,
		OTLPProtocol: "grpc",
		ContentLog:   defaultContentLogging,
	}
}

// resolveEnv reads environment variables to override defaults.
// Environment takes highest priority.
func resolveEnv(cfg *Config) {
	if os.Getenv("OCR_ENABLE_TELEMETRY") == "1" {
		cfg.Enabled = true
	}
	if v := os.Getenv("OTEL_SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.Exporter = "otlp"
		cfg.OTLPEndpoint = v
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); v != "" {
		cfg.OTLPProtocol = v
	}
	if os.Getenv("OCR_CONTENT_LOGGING") == "1" {
		cfg.ContentLog = true
	}
}

// telemetrySection matches the telemetry key in ~/.opencodereview/config.json.
type telemetrySection struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	Exporter     *string `json:"exporter,omitempty"`
	OTLPEndpoint *string `json:"otlp_endpoint,omitempty"`
	ContentLog   *bool   `json:"content_logging,omitempty"`
}

// LoadFromJSON merges telemetry settings from a JSON config file if present.
// This has lower priority than environment variables.
func LoadFromJSON(cfg *Config, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var root struct {
		Telemetry *telemetrySection `json:"telemetry"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil // malformed JSON — skip silently
	}
	if root.Telemetry == nil {
		return nil
	}
	t := root.Telemetry
	if t.Enabled != nil {
		cfg.Enabled = *t.Enabled
	}
	if t.Exporter != nil && cfg.Exporter == defaultExporter {
		cfg.Exporter = *t.Exporter
	}
	if t.OTLPEndpoint != nil && cfg.OTLPEndpoint == "" {
		cfg.OTLPEndpoint = *t.OTLPEndpoint
		if cfg.Exporter == defaultExporter {
			cfg.Exporter = "otlp"
		}
	}
	if t.ContentLog != nil {
		cfg.ContentLog = *t.ContentLog
	}
	return nil
}

// ResolveConfig builds the final Config by merging defaults, JSON config, and env vars.
// Precedence: defaults < config.json < environment variables.
func ResolveConfig(configPath string) Config {
	cfg := DefaultConfig()

	// Layer 1: JSON config file
	if configPath != "" {
		_ = LoadFromJSON(&cfg, configPath)
	}

	// Layer 2: Environment variables (highest priority)
	resolveEnv(&cfg)

	return cfg
}

// HomeConfigPath returns the default path to ~/.opencodereview/config.json.
func HomeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".opencodereview", "config.json")
}
