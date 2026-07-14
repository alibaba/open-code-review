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

// resolveConfigPath returns OCR_CONFIG_PATH when set, otherwise the default user config path.
// Intentionally used only by read-only commands (e.g. ocr llm test). Write paths such as
// config set and review keep defaultConfigPath() so a leaked OCR_CONFIG_PATH cannot redirect writes.
func resolveConfigPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("OCR_CONFIG_PATH")); p != "" {
		return p, nil
	}
	return defaultConfigPath()
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
	case "unset":
		return runConfigUnset(action.key)
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

func runConfigUnset(key string) error {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Errorf("unset supports custom_providers.<name> and mcp_servers.<name>")
	}

	configPath, err := defaultConfigPath()
	if err != nil {
		return err
	}

	switch parts[0] {
	case "custom_providers":
		return unsetCustomProvider(configPath, parts[1])
	case "mcp_servers":
		return unsetMCPServer(configPath, parts[1])
	default:
		return fmt.Errorf("unset supports custom_providers.<name> and mcp_servers.<name>")
	}
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
	URL          string            `json:"url,omitempty"`
	Protocol     string            `json:"protocol,omitempty"`
	Model        string            `json:"model,omitempty"`
	Models       []string          `json:"models,omitempty"`
	AuthHeader   string            `json:"auth_header,omitempty"`
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
}

// MCPServerConfig holds configuration for a single MCP server (stdio transport).
type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	Tools   []string `json:"tools,omitempty"`
	Setup   string   `json:"setup,omitempty"`
}

// Config represents the user-level configuration file (~/.opencodereview/config.json).
type Config struct {
	Provider        string                     `json:"provider,omitempty"`
	Model           string                     `json:"model,omitempty"`
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
	AuthHeader   string            `json:"auth_header,omitempty"`
	Model        string            `json:"model,omitempty"`
	Protocol     string            `json:"protocol,omitempty"`      // canonical protocol name; takes priority over UseAnthropic
	UseAnthropic *bool             `json:"use_anthropic,omitempty"` // nil = default true; false = OpenAI protocol (legacy fallback)
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
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
	default:
		return fmt.Errorf("unknown config key: %s\nSupported keys: provider, model, providers.<name>.<field>, custom_providers.<name>.<field>, mcp_servers.<name>.<field>, llm.url, llm.auth_token, llm.auth_header, llm.model, llm.protocol, llm.use_anthropic, llm.extra_body, llm.extra_headers, language, telemetry.enabled, telemetry.exporter, telemetry.otlp_endpoint, telemetry.content_logging\nProvider fields: api_key, url, protocol, model, models, auth_header, extra_body, extra_headers\nProtocol values: anthropic, openai, openai-responses\nMCP server fields: command, args, env, tools, setup", key)
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
		normalized := llm.NormalizeProtocol(value)
		if err := llm.ValidateProtocol(normalized); err != nil {
			return err
		}
		entry.Protocol = normalized
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
	default:
		return fmt.Errorf("unknown provider field %q: supported fields are api_key, url, protocol, model, models, auth_header, extra_body, extra_headers", field)
	}
	return nil
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
	return setCustomProviderField(cfg, parts[1], parts[2], key, value)
}

func setCustomProviderField(cfg *Config, name, field, key, value string) error {
	if cfg.CustomProviders == nil {
		cfg.CustomProviders = make(map[string]ProviderEntry)
	}
	entry := cfg.CustomProviders[name]
	if err := applyProviderField(&entry, field, key, value); err != nil {
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
		return fmt.Errorf("unknown MCP server field %q: supported fields are command, args, env, tools, setup", field)
	}

	cfg.MCPServers[name] = entry
	return nil
}

func (c *Config) ensureTelemetry() {
	if c.Telemetry == nil {
		c.Telemetry = &TelemetryConfig{}
	}
}
