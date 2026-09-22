// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package configfile is the single source of truth for the schema of the
// user-level app config (~/.opencodereview/config.json). The cmd layer
// unmarshals into these structs and marshals back on every write, and the
// llm resolver reads the same file, so both must agree on every field:
// a field missing here is silently dropped from a hand-written config the
// first time any config command runs.
package configfile

import (
	"encoding/json"
	"reflect"
	"strings"
)

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

	unknownJSONFields map[string]json.RawMessage
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

	unknownJSONFields map[string]json.RawMessage
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

	unknownJSONFields map[string]json.RawMessage
}

// TelemetryConfig holds telemetry-specific settings.
type TelemetryConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`         // Master switch for telemetry
	Exporter     string `json:"exporter,omitempty"`        // "console" or "otlp"
	OTLPEndpoint string `json:"otlp_endpoint,omitempty"`   // OTLP collector address
	ContentLog   bool   `json:"content_logging,omitempty"` // Include prompt/response content

	unknownJSONFields map[string]json.RawMessage
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

	unknownJSONFields map[string]json.RawMessage
}

// jsonFieldNames returns the exported JSON tag names of a struct value,
// following pointers. Unexported fields and fields tagged "-" are skipped.
func jsonFieldNames(value any) []string {
	typeOf := reflect.TypeOf(value)
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}

	fields := make([]string, 0, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	return fields
}

// collectUnknownJSONFields keeps JSON keys with no matching struct field alive
// across a load/save cycle. Unexported: any struct-literal rebuild must copy it
// (see the exported accessors below) or the fields are dropped again.
func collectUnknownJSONFields(data []byte, knownFields []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}

	known := make(map[string]struct{}, len(knownFields))
	for _, field := range knownFields {
		known[field] = struct{}{}
	}
	for field := range fields {
		if _, ok := known[strings.ToLower(field)]; ok {
			delete(fields, field)
		}
	}
	return fields, nil
}

// mergeUnknownJSONFields re-merges previously collected unknown JSON fields
// into freshly marshalled data unless a known field now shadows them.
func mergeUnknownJSONFields(data []byte, unknown map[string]json.RawMessage) ([]byte, error) {
	if len(unknown) == 0 {
		return data, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(fields))
	for field := range fields {
		known[strings.ToLower(field)] = struct{}{}
	}
	for field, value := range unknown {
		if _, exists := known[strings.ToLower(field)]; !exists {
			fields[field] = value
		}
	}
	return json.Marshal(fields)
}

func cloneUnknownJSONFields(src map[string]json.RawMessage) map[string]json.RawMessage {
	if src == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(src))
	for key, value := range src {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

// UnknownJSONFields returns a deep copy of the JSON fields this struct did not
// recognize on load. May be nil. SetUnknownJSONFields applies a copy back.
func (c *Config) UnknownJSONFields() map[string]json.RawMessage {
	return cloneUnknownJSONFields(c.unknownJSONFields)
}
func (c *Config) SetUnknownJSONFields(fields map[string]json.RawMessage) {
	c.unknownJSONFields = cloneUnknownJSONFields(fields)
}
func (e *ProviderEntry) UnknownJSONFields() map[string]json.RawMessage {
	return cloneUnknownJSONFields(e.unknownJSONFields)
}
func (e *ProviderEntry) SetUnknownJSONFields(fields map[string]json.RawMessage) {
	e.unknownJSONFields = cloneUnknownJSONFields(fields)
}
func (l *LlmConfig) UnknownJSONFields() map[string]json.RawMessage {
	return cloneUnknownJSONFields(l.unknownJSONFields)
}
func (l *LlmConfig) SetUnknownJSONFields(fields map[string]json.RawMessage) {
	l.unknownJSONFields = cloneUnknownJSONFields(fields)
}
func (t *TelemetryConfig) UnknownJSONFields() map[string]json.RawMessage {
	return cloneUnknownJSONFields(t.unknownJSONFields)
}
func (t *TelemetryConfig) SetUnknownJSONFields(fields map[string]json.RawMessage) {
	t.unknownJSONFields = cloneUnknownJSONFields(fields)
}
func (m *MCPServerConfig) UnknownJSONFields() map[string]json.RawMessage {
	return cloneUnknownJSONFields(m.unknownJSONFields)
}
func (m *MCPServerConfig) SetUnknownJSONFields(fields map[string]json.RawMessage) {
	m.unknownJSONFields = cloneUnknownJSONFields(fields)
}

func (c *Config) UnmarshalJSON(data []byte) error {
	type configAlias Config
	var decoded configAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := collectUnknownJSONFields(data, jsonFieldNames(Config{}))
	if err != nil {
		return err
	}
	*c = Config(decoded)
	c.unknownJSONFields = unknown
	return nil
}

func (c Config) MarshalJSON() ([]byte, error) {
	type configAlias Config
	data, err := json.Marshal(configAlias(c))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(data, c.unknownJSONFields)
}

func (e *ProviderEntry) UnmarshalJSON(data []byte) error {
	type providerEntryAlias ProviderEntry
	var decoded providerEntryAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := collectUnknownJSONFields(data, jsonFieldNames(ProviderEntry{}))
	if err != nil {
		return err
	}
	*e = ProviderEntry(decoded)
	e.unknownJSONFields = unknown
	return nil
}

func (e ProviderEntry) MarshalJSON() ([]byte, error) {
	type providerEntryAlias ProviderEntry
	data, err := json.Marshal(providerEntryAlias(e))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(data, e.unknownJSONFields)
}

func (c *MCPServerConfig) UnmarshalJSON(data []byte) error {
	type mcpServerConfigAlias MCPServerConfig
	var decoded mcpServerConfigAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := collectUnknownJSONFields(data, jsonFieldNames(MCPServerConfig{}))
	if err != nil {
		return err
	}
	*c = MCPServerConfig(decoded)
	c.unknownJSONFields = unknown
	return nil
}

func (c MCPServerConfig) MarshalJSON() ([]byte, error) {
	type mcpServerConfigAlias MCPServerConfig
	data, err := json.Marshal(mcpServerConfigAlias(c))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(data, c.unknownJSONFields)
}

func (c *LlmConfig) UnmarshalJSON(data []byte) error {
	type llmConfigAlias LlmConfig
	var decoded llmConfigAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := collectUnknownJSONFields(data, jsonFieldNames(LlmConfig{}))
	if err != nil {
		return err
	}
	*c = LlmConfig(decoded)
	c.unknownJSONFields = unknown
	return nil
}

func (c LlmConfig) MarshalJSON() ([]byte, error) {
	type llmConfigAlias LlmConfig
	data, err := json.Marshal(llmConfigAlias(c))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(data, c.unknownJSONFields)
}

func (c *TelemetryConfig) UnmarshalJSON(data []byte) error {
	type telemetryConfigAlias TelemetryConfig
	var decoded telemetryConfigAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	unknown, err := collectUnknownJSONFields(data, jsonFieldNames(TelemetryConfig{}))
	if err != nil {
		return err
	}
	*c = TelemetryConfig(decoded)
	c.unknownJSONFields = unknown
	return nil
}

func (c TelemetryConfig) MarshalJSON() ([]byte, error) {
	type telemetryConfigAlias TelemetryConfig
	data, err := json.Marshal(telemetryConfigAlias(c))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(data, c.unknownJSONFields)
}
