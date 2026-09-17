// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package configfile is the single source of truth for the schema of the
// user-level app config (~/.opencodereview/config.json). The cmd layer
// unmarshals into these structs and marshals back on every write, and the
// llm resolver reads the same file, so both must agree on every field:
// a field missing here is silently dropped from a hand-written config the
// first time any config command runs.
package configfile

// Config represents the user-level configuration file (~/.opencodereview/config.json).
type Config struct {
	Provider        string                     `json:"provider,omitempty"`
	Model           string                     `json:"model,omitempty"`
	MaxTokens       int                        `json:"max_tokens,omitempty"`
	Effort          string                     `json:"effort,omitempty"`
	Providers       map[string]ProviderEntry   `json:"providers,omitempty"`
	CustomProviders map[string]ProviderEntry   `json:"custom_providers,omitempty"`
	Llm             LlmConfig                  `json:"llm,omitempty"`
	Language        string                     `json:"language,omitempty"`
	Telemetry       *TelemetryConfig           `json:"telemetry,omitempty"`
	MCPServers      map[string]MCPServerConfig `json:"mcp_servers,omitempty"`
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
	// them the standard chain decides, as with any other AWS tool. Setting them
	// in config makes a review run reproducible without exporting AWS_PROFILE first.
	AWSProfile string `json:"aws_profile,omitempty"`
	AWSRegion  string `json:"aws_region,omitempty"`
}

// LlmConfig represents the llm section in config.json.
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
