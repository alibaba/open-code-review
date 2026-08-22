// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration settings",
	Long: `Configuration management.

Examples:
  # Provider setup (interactive)
  ocr config provider
  ocr config model

  # Provider setup (non-interactive)
  ocr config set provider anthropic
  ocr config set model claude-opus-4-6
  ocr config set providers.anthropic.api_key "$ANTHROPIC_API_KEY"

  # Custom provider
  ocr config set provider my-gateway
  ocr config set custom_providers.my-gateway.url https://gateway.internal.com/v1
  ocr config set custom_providers.my-gateway.protocol openai`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var configSetCmd = &cobra.Command{
	Use:     "set <key> <value>",
	Short:   "Set a configuration value",
	Example: "  ocr config set llm.model claude-opus-4-6\n  ocr config set provider anthropic",
	Args:    exactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigSet(args[0], args[1])
	},
}

var configUnsetCmd = &cobra.Command{
	Use:     "unset <key>",
	Short:   "Remove a configuration value",
	Long:    "Remove a provider, custom_providers.<name>, or mcp_servers.<name>.",
	Example: "  ocr config unset provider\n  ocr config unset custom_providers.my-provider\n  ocr config unset mcp_servers.github",
	Args:    exactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigUnset(args[0])
	},
}

var configProviderCmd = &cobra.Command{
	Use:   "provider",
	Short: "Interactive provider setup",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigProvider()
	},
}

var configModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Interactive model selection",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigModel()
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(configProviderCmd)
	configCmd.AddCommand(configModelCmd)
}

// Default config file location: ~/.opencodereview/config.json
func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "config.json"), nil
}

// resolveConfigPath returns OCR_CONFIG_PATH when set, otherwise the default user config path.
// Intentionally used only by read-only commands (e.g. ocr llm test). Write paths such as
// config set and review keep defaultConfigPath() so a leaked OCR_CONFIG_PATH cannot redirect writes.
func resolveConfigPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("OCR_CONFIG_PATH")); p != "" {
		return p, nil
	}
	return defaultConfigPath()
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
	if shouldMaskConfigValue(key) {
		displayValue = maskKey(value)
	}
	fmt.Printf("Set %s = %s\n", key, displayValue)
	if warning := legacyLLMShadowWarning(cfg.Provider, key); warning != "" {
		fmt.Fprint(os.Stderr, warning)
	}
	return nil
}

// shouldMaskConfigValue reports whether the echoed value of a config key holds a
// secret and must be masked. Matching on the normalized suffix covers both
// snake_case and Go field spellings of api_key/auth_token at any path depth,
// while the *_cmd variants stay unmasked: a command line is not a secret.
func shouldMaskConfigValue(key string) bool {
	normalizedKey := strings.ToLower(strings.ReplaceAll(key, "_", ""))
	return strings.HasSuffix(normalizedKey, "apikey") || strings.HasSuffix(normalizedKey, "authtoken")
}

func runConfigUnset(key string) error {
	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	if key == "provider" {
		return unsetActiveProvider(configPath)
	}
	if key == "max_tokens" {
		return unsetMaxTokens(configPath)
	}

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Errorf("unset supports provider, max_tokens, custom_providers.<name>, and mcp_servers.<name>")
	}

	switch parts[0] {
	case "custom_providers":
		return unsetCustomProvider(configPath, parts[1])
	case "mcp_servers":
		return unsetMCPServer(configPath, parts[1])
	default:
		return fmt.Errorf("unset supports provider, max_tokens, custom_providers.<name>, and mcp_servers.<name>")
	}
}

func unsetMaxTokens(configPath string) error {
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.MaxTokens = 0
	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Println("Cleared max_tokens; using the embedded template default.")
	return nil
}

func unsetActiveProvider(configPath string) error {
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.Provider = ""
	cfg.Model = ""
	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Println("Cleared active provider and model.")
	return nil
}

func legacyLLMShadowWarning(provider, key string) string {
	if provider == "" || !strings.HasPrefix(key, "llm.") {
		return ""
	}
	section := "custom_providers"
	if _, isPreset := llm.LookupProvider(provider); isPreset {
		section = "providers"
	}
	return fmt.Sprintf("[ocr] WARNING: provider %q is active and takes precedence over llm.* settings.\n"+
		"[ocr] Use 'ocr config set %s.%s.<field> <value>' to configure the active provider,\n"+
		"[ocr] or run 'ocr config unset provider' to disable provider-based config.\n", provider, section, provider)
}

func unsetCustomProvider(configPath, name string) error {
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	wasActive, err := deleteCustomProvider(cfg, name)
	if err != nil {
		return err
	}

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Deleted custom provider %q.\n", name)
	if wasActive {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: active provider was deleted; 'provider' and 'model' have been cleared.\n")
		fmt.Fprintf(os.Stderr, "[ocr] Run 'ocr config provider' to select a new provider.\n")
	}
	return nil
}

func unsetMCPServer(configPath, name string) error {
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.MCPServers == nil {
		return fmt.Errorf("MCP server %q not found", name)
	}
	if _, exists := cfg.MCPServers[name]; !exists {
		return fmt.Errorf("MCP server %q not found", name)
	}

	delete(cfg.MCPServers, name)
	if len(cfg.MCPServers) == 0 {
		cfg.MCPServers = nil
	}

	if err := saveConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Printf("Deleted MCP server %q.\n", name)
	return nil
}

// deleteCustomProvider removes a custom provider from cfg in memory.
// Returns true if the deleted provider was the active one.
func deleteCustomProvider(cfg *Config, name string) (bool, error) {
	if cfg.CustomProviders == nil {
		return false, fmt.Errorf("custom provider %q not found", name)
	}
	if _, exists := cfg.CustomProviders[name]; !exists {
		return false, fmt.Errorf("custom provider %q not found", name)
	}

	wasActive := cfg.Provider == name
	delete(cfg.CustomProviders, name)
	if len(cfg.CustomProviders) == 0 {
		cfg.CustomProviders = nil
	}

	if wasActive {
		cfg.Provider = ""
		cfg.Model = ""
	}

	return wasActive, nil
}

// ProviderEntry holds per-provider configuration in the providers map.
type ProviderEntry struct {
	APIKey       string            `json:"api_key,omitempty"`
	APIKeyCmd    string            `json:"api_key_cmd,omitempty"` // shell command whose stdout is the api key; used when api_key is empty
	URL          string            `json:"url,omitempty"`
	Protocol     string            `json:"protocol,omitempty"`
	Model        string            `json:"model,omitempty"`
	Models       []string          `json:"models,omitempty"`
	AuthHeader   string            `json:"auth_header,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"` // per-request HTTP timeout in seconds
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
	RetryCodes   []int             `json:"retry_codes,omitempty"`

	// AWSProfile and AWSRegion pin the credentials and region for providers that
	// authenticate from the AWS chain (bedrock). Both are optional — without
	// them the standard chain decides, as with any other AWS tool. They must
	// exist here as well as in the resolver's own view of the file: config is
	// unmarshalled into this struct and marshalled back on every write, so a
	// field missing from it is silently dropped from a hand-written config the
	// first time any config command runs.
	AWSProfile string `json:"aws_profile,omitempty"`
	AWSRegion  string `json:"aws_region,omitempty"`
}

// MCPServerConfig holds configuration for a single MCP server.
// Type "stdio" (default) uses a subprocess; type "remote" uses Streamable HTTP.
type MCPServerConfig struct {
	Type    string            `json:"type,omitempty"` // "stdio" (default) or "remote"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []string          `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Tools   []string          `json:"tools,omitempty"`
	Setup   string            `json:"setup,omitempty"`
}

// Config represents the user-level configuration file (~/.opencodereview/config.json).
type Config struct {
	Provider        string                     `json:"provider,omitempty"`
	Model           string                     `json:"model,omitempty"`
	MaxTokens       int                        `json:"max_tokens,omitempty"`
	Providers       map[string]ProviderEntry   `json:"providers,omitempty"`
	CustomProviders map[string]ProviderEntry   `json:"custom_providers,omitempty"`
	Llm             LlmConfig                  `json:"llm,omitempty"`
	Language        string                     `json:"language,omitempty"`
	Telemetry       *TelemetryConfig           `json:"telemetry,omitempty"`
	MCPServers      map[string]MCPServerConfig `json:"mcp_servers,omitempty"`
}

type LlmConfig struct {
	URL          string            `json:"url,omitempty"`
	AuthToken    string            `json:"auth_token,omitempty"`
	AuthTokenCmd string            `json:"auth_token_cmd,omitempty"` // shell command whose stdout is the auth token; used when auth_token is empty
	AuthHeader   string            `json:"auth_header,omitempty"`
	Model        string            `json:"model,omitempty"`
	Protocol     string            `json:"protocol,omitempty"`      // canonical protocol name; takes priority over UseAnthropic
	UseAnthropic *bool             `json:"use_anthropic,omitempty"` // nil = default true; false = OpenAI protocol (legacy fallback)
	TimeoutSec   int               `json:"timeout_sec,omitempty"`   // per-request HTTP timeout in seconds
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
	RetryCodes   []int             `json:"retry_codes,omitempty"`
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

// supportedConfigKeys is the single source of truth for the top-level config
// keys accepted by setConfigValue. The unknown-key error message is generated
// from this list so the two cannot drift apart when a new key is added.
var supportedConfigKeys = []string{
	"provider",
	"model",
	"max_tokens",
	"providers.<name>.<field>",
	"custom_providers.<name>.<field>",
	"mcp_servers.<name>.<field>",
	"llm.url",
	"llm.auth_token",
	"llm.auth_token_cmd",
	"llm.auth_header",
	"llm.model",
	"llm.protocol",
	"llm.use_anthropic",
	"llm.extra_body",
	"llm.extra_headers",
	"llm.retry_codes",
	"language",
	"telemetry.enabled",
	"telemetry.exporter",
	"telemetry.otlp_endpoint",
	"telemetry.content_logging",
}

func setConfigValue(cfg *Config, key, value string) error {
	// Handle providers.<name>.<field> paths.
	if strings.HasPrefix(key, "providers.") {
		return setProviderValue(cfg, key, value)
	}
	if strings.HasPrefix(key, "custom_providers.") {
		return setCustomProviderValue(cfg, key, value)
	}
	if strings.HasPrefix(key, "mcp_servers.") {
		return setMCPServerValue(cfg, key, value)
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
	case "max_tokens":
		maxTokens, err := strconv.Atoi(value)
		if err != nil || maxTokens <= 0 {
			return fmt.Errorf("invalid max_tokens %q: must be a positive integer", value)
		}
		cfg.MaxTokens = maxTokens
	case "llm.url", "llm.URL":
		cfg.Llm.URL = value
	case "llm.auth_token", "llm.AuthToken":
		cfg.Llm.AuthToken = value
	case "llm.auth_token_cmd", "llm.AuthTokenCmd":
		cfg.Llm.AuthTokenCmd = value
	case "llm.auth_header", "llm.AuthHeader":
		normalized, err := llm.NormalizeAuthHeader(value)
		if err != nil {
			return err
		}
		cfg.Llm.AuthHeader = normalized
	case "llm.extra_headers", "llm.ExtraHeaders":
		parsed, err := llm.ParseExtraHeaders(value)
		if err != nil {
			return err
		}
		cfg.Llm.ExtraHeaders = parsed
	case "llm.model", "llm.Model":
		cfg.Llm.Model = value
	case "llm.protocol", "llm.Protocol":
		normalized := llm.NormalizeProtocol(value)
		if err := llm.ValidateProtocol(normalized); err != nil {
			return err
		}
		// The llm block is a single url + token endpoint. Bedrock needs neither
		// and has nowhere here to put a region or a profile, so it is refused at
		// the point of setting rather than accepted and ignored at resolve time.
		if normalized == llm.ProtocolAnthropicBedrock {
			return fmt.Errorf("llm.protocol cannot be %q: bedrock derives its host from aws_region and signs with the AWS credential chain, so it has no use for llm.url or llm.auth_token; run `ocr config set provider bedrock` instead", normalized)
		}
		cfg.Llm.Protocol = normalized
		// Mirror use_anthropic so older binaries that predate llm.protocol
		// still pick the right protocol family: anthropic -> true, the OpenAI
		// family (including openai-responses) -> false.
		if normalized == llm.ProtocolAnthropic {
			t := true
			cfg.Llm.UseAnthropic = &t
		} else {
			f := false
			cfg.Llm.UseAnthropic = &f
		}
	case "llm.use_anthropic", "llm.UseAnthropic":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean for llm.use_anthropic: %w", err)
		}
		cfg.Llm.UseAnthropic = &b
		// Mirror protocol for backward compatibility. true always selects
		// anthropic. false only mirrors to the legacy openai default when
		// protocol is unset or already a legacy value (anthropic/openai);
		// openai-responses is preserved to avoid a silent downgrade.
		if b {
			cfg.Llm.Protocol = llm.ProtocolAnthropic
		} else if cfg.Llm.Protocol == "" || cfg.Llm.Protocol == llm.ProtocolAnthropic || cfg.Llm.Protocol == llm.ProtocolOpenAIChatCompletions {
			cfg.Llm.Protocol = llm.ProtocolOpenAIChatCompletions
		}
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
	case "llm.retry_codes", "llm.RetryCodes":
		codes, warnings, err := llm.ParseRetryCodes(value)
		if err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: %s\n", w)
		}
		cfg.Llm.RetryCodes = codes
	default:
		return fmt.Errorf("unknown config key: %s\nSupported keys: %s\nProvider fields: api_key, api_key_cmd, url, protocol, model, models, auth_header, extra_body, extra_headers, retry_codes, aws_region, aws_profile\nProtocol values: anthropic, anthropic-bedrock, openai, openai-responses\nMCP server fields: type, command, args, env, url, headers, tools, setup", key, strings.Join(supportedConfigKeys, ", "))
	}
	return nil
}

func applyProviderField(providerName string, entry *ProviderEntry, field, key, value string) error {
	switch field {
	case "api_key":
		entry.APIKey = value
	case "api_key_cmd":
		entry.APIKeyCmd = value
	case "url":
		trimmedURL := strings.TrimSpace(value)
		if trimmedURL != "" {
			if err := validateBaseURL(trimmedURL); err != nil {
				return fmt.Errorf("invalid URL for %s: %w", key, err)
			}
		}
		entry.URL = trimmedURL
	case "protocol":
		normalized := llm.NormalizeProtocol(value)
		if err := llm.ValidateProtocol(normalized); err != nil {
			return err
		}
		entry.Protocol = normalized
		// Switching away from bedrock leaves aws_region/aws_profile as dead
		// config that reads as applied but nothing reads it — clear both, the
		// same way the TUI drops url/api_key/auth_header when switching onto
		// bedrock (see cpAmbientProtocol in provider_tui.go).
		if normalized != llm.ProtocolAnthropicBedrock && (entry.AWSRegion != "" || entry.AWSProfile != "") {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: clearing aws_region/aws_profile on %q: protocol %q does not use the AWS credential chain\n", providerName, normalized)
			entry.AWSRegion = ""
			entry.AWSProfile = ""
		}
	case "model":
		entry.Model = value
	case "models":
		models, err := parseModelListValue(value)
		if err != nil {
			return fmt.Errorf("invalid model list for %s: %w", key, err)
		}
		entry.Models = models
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
	case "extra_headers":
		parsed, err := llm.ParseExtraHeaders(value)
		if err != nil {
			return fmt.Errorf("invalid extra headers for %s: %w", key, err)
		}
		entry.ExtraHeaders = parsed
	case "retry_codes":
		codes, warnings, err := llm.ParseRetryCodes(value)
		if err != nil {
			return fmt.Errorf("invalid retry codes for %s: %w", key, err)
		}
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: %s\n", w)
		}
		entry.RetryCodes = codes
	case "aws_region", "aws_profile":
		normalized, err := normalizeAWSSetting(field, key, value)
		if err != nil {
			return err
		}
		if !providerAcceptsAWSSettings(providerName, entry) {
			return fmt.Errorf("%s does not apply to provider %q: aws_region and aws_profile are only used by providers that authenticate from the AWS credential chain (protocol %s)", field, providerName, llm.ProtocolAnthropicBedrock)
		}
		if field == "aws_region" {
			entry.AWSRegion = normalized
		} else {
			entry.AWSProfile = normalized
		}
	default:
		return fmt.Errorf("unknown provider field %q: supported fields are api_key, api_key_cmd, url, protocol, model, models, auth_header, extra_body, extra_headers, retry_codes, aws_region, aws_profile", field)
	}
	return nil
}

// providerAcceptsAWSSettings reports whether aws_region / aws_profile mean
// anything for this provider. Storing them anywhere else would be dead config
// that reads as applied, so it is rejected instead.
//
// The entry's own protocol decides whenever it sets one: a preset's protocol can
// be overridden per entry (see tryProviderConfig), so `protocol: openai` on the
// bedrock preset would otherwise still accept AWS settings that nothing reads.
// Only when the entry is silent does the preset's own AmbientAuth flag answer.
func providerAcceptsAWSSettings(providerName string, entry *ProviderEntry) bool {
	if entry.Protocol != "" {
		return llm.NormalizeProtocol(entry.Protocol) == llm.ProtocolAnthropicBedrock
	}
	preset, isPreset := llm.LookupProvider(providerName)
	return isPreset && preset.AmbientAuth
}

// normalizeAWSSetting trims the value and rejects the shapes AWS itself will
// not accept. Region names are deliberately not checked against a fixed list:
// AWS adds regions faster than any embedded list stays correct, and a wrong one
// already surfaces at request time.
func normalizeAWSSetting(field, key, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil // clearing the field hands the decision back to the AWS chain
	}
	if strings.ContainsAny(trimmed, " \t\n") {
		return "", fmt.Errorf("invalid %s for %s: %q contains whitespace", field, key, value)
	}
	return trimmed, nil
}

func parseModelListValue(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if strings.HasPrefix(value, "[") {
		var models []string
		if err := json.Unmarshal([]byte(value), &models); err == nil {
			return normalizeModelList(models), nil
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	}

	return normalizeModelList(strings.Split(value, ",")), nil
}

func activeModelForProvider(cfg *Config, providerName string, entry ProviderEntry) string {
	if entry.Model != "" {
		return entry.Model
	}
	if cfg != nil && cfg.Provider == providerName && cfg.Model != "" {
		return cfg.Model
	}
	return ""
}

func normalizeModelList(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func mergeModelLists(lists ...[]string) []string {
	var merged []string
	for _, list := range lists {
		merged = append(merged, list...)
	}
	return normalizeModelList(merged)
}

// ensureModelInList appends model to the end when missing; never reorders existing entries.
func ensureModelInList(models []string, model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return models
	}
	if llm.ModelListContains(models, model) {
		return models
	}
	out := append([]string(nil), models...)
	return append(out, model)
}

func setProviderValue(cfg *Config, key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("invalid provider key %q: expected providers.<name>.<field>", key)
	}
	if _, isPreset := llm.LookupProvider(parts[1]); !isPreset {
		return setCustomProviderField(cfg, parts[1], parts[2], key, value)
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderEntry)
	}
	entry := cfg.Providers[parts[1]]
	if err := applyProviderField(parts[1], &entry, parts[2], key, value); err != nil {
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
	return setCustomProviderField(cfg, parts[1], parts[2], key, value)
}

func setCustomProviderField(cfg *Config, name, field, key, value string) error {
	if cfg.CustomProviders == nil {
		cfg.CustomProviders = make(map[string]ProviderEntry)
	}
	entry := cfg.CustomProviders[name]
	if err := applyProviderField(name, &entry, field, key, value); err != nil {
		return err
	}
	cfg.CustomProviders[name] = entry
	return nil
}

func setMCPServerValue(cfg *Config, key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return fmt.Errorf("invalid MCP server key %q: expected mcp_servers.<name>.<field>", key)
	}
	name, field := parts[1], parts[2]

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]MCPServerConfig)
	}
	entry := cfg.MCPServers[name]

	switch field {
	case "type":
		if value != "stdio" && value != "remote" {
			return fmt.Errorf("invalid MCP server type %q: must be \"stdio\" or \"remote\"", value)
		}
		entry.Type = value
	case "command":
		if value == "" {
			return fmt.Errorf("MCP server command cannot be empty")
		}
		entry.Command = value
	case "args":
		var args []string
		if err := json.Unmarshal([]byte(value), &args); err != nil {
			return fmt.Errorf("invalid JSON array for %s: %w", key, err)
		}
		entry.Args = args
	case "env":
		var env []string
		if err := json.Unmarshal([]byte(value), &env); err != nil {
			return fmt.Errorf("invalid JSON array for %s: %w", key, err)
		}
		for _, e := range env {
			idx := strings.Index(e, "=")
			if idx <= 0 {
				return fmt.Errorf("invalid env entry %q: must be in KEY=VALUE format", e)
			}
		}
		entry.Env = env
	case "url":
		if value == "" {
			return fmt.Errorf("MCP server URL cannot be empty")
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid MCP server URL %q: %w", value, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("MCP server URL must use http or https scheme, got %q", parsed.Scheme)
		}
		if parsed.Host == "" {
			return fmt.Errorf("MCP server URL %q must include a host", value)
		}
		entry.URL = value
	case "headers":
		parsed, err := parseMCPHeaders(value)
		if err != nil {
			return fmt.Errorf("invalid headers for %s: %w", key, err)
		}
		entry.Headers = parsed
	case "tools":
		var tools []string
		if err := json.Unmarshal([]byte(value), &tools); err != nil {
			return fmt.Errorf("invalid JSON array for %s: %w", key, err)
		}
		seen := make(map[string]struct{}, len(tools))
		filtered := make([]string, 0, len(tools))
		for _, t := range tools {
			if t == "" {
				return fmt.Errorf("tool names in %s must not be empty", key)
			}
			if _, dup := seen[t]; dup {
				continue
			}
			seen[t] = struct{}{}
			filtered = append(filtered, t)
		}
		entry.Tools = filtered
	case "setup":
		entry.Setup = value
	default:
		return fmt.Errorf("unknown MCP server field %q: supported fields are type, command, args, env, url, headers, tools, setup", field)
	}

	cfg.MCPServers[name] = entry
	return nil
}

// parseMCPHeaders parses a JSON object of header key-value pairs.
// Example: {"Authorization": "Bearer $TOKEN", "X-Custom": "value"}
func parseMCPHeaders(value string) (map[string]string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		return nil, fmt.Errorf("expected JSON object: %w", err)
	}
	for k, v := range m {
		if k == "" {
			return nil, fmt.Errorf("header name must not be empty")
		}
		if v == "" {
			return nil, fmt.Errorf("header value for %q must not be empty", k)
		}
	}
	return m, nil
}

func (c *Config) ensureTelemetry() {
	if c.Telemetry == nil {
		c.Telemetry = &TelemetryConfig{}
	}
}
