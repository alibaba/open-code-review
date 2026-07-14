package llm

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ResolvedEndpoint holds the resolved LLM endpoint configuration.
type ResolvedEndpoint struct {
	URL          string
	Token        string
	Model        string
	Protocol     string            // canonical protocol name (see protocol.go); resolver normalizes aliases
	AuthHeader   string            // Anthropic auth header: "x-api-key" or "authorization"
	Source       string            // human-readable config source label
	ExtraBody    map[string]any    // vendor-specific request body fields
	ExtraHeaders map[string]string // extra HTTP headers for the LLM request
	// Timeout is the per-request HTTP timeout; 0 means use the client default (5 min).
	// Only config file (llm/provider sections) and OCR_LLM_TIMEOUT env var can set this.
	// tryCCEnv and tryShellRC always leave it at 0 since those sources have no timeout
	// knob; users can still override via OCR_LLM_TIMEOUT.
	Timeout time.Duration
}

// Environment variable names for OCR-specific configuration.
const (
	envOCRLLMURL          = "OCR_LLM_URL"
	envOCRLLMToken        = "OCR_LLM_TOKEN"
	envOCRLLMModel        = "OCR_LLM_MODEL"
	envOCRLLMAuthHeader   = "OCR_LLM_AUTH_HEADER"
	envOCRLLMExtraHeaders = "OCR_LLM_EXTRA_HEADERS"
	// envOCRLLMProtocol overrides the resolved protocol (anthropic |
	// openai | openai-responses). Takes priority
	// over OCR_USE_ANTHROPIC when set.
	envOCRLLMProtocol = "OCR_LLM_PROTOCOL"
	// envOCRLLMTimeout is a global override applied in ResolveEndpointWithModelOverride
	// after any strategy resolves, rather than inside tryOCREnv like other OCR_LLM_* vars.
	// This lets it override timeout for all resolution paths (OCR env, config file,
	// provider config, Claude Code env, shell RC).
	envOCRLLMTimeout   = "OCR_LLM_TIMEOUT"
	envOCRUseAnthropic = "OCR_USE_ANTHROPIC"
)

// Environment variable names from Claude Code configuration.
const (
	envCCBaseURL = "ANTHROPIC_BASE_URL"
	envCCToken   = "ANTHROPIC_AUTH_TOKEN"
	envCCModel   = "ANTHROPIC_MODEL"
)

// ResolveEndpoint reads from 4 strategy sources in priority order.
// Each strategy requires all three fields (URL, Token, Model) to be non-empty.
// Returns the first valid strategy's result.
func ResolveEndpoint(configPath string) (ResolvedEndpoint, error) {
	return ResolveEndpointWithModelOverride(configPath, "")
}

// ResolveEndpointWithModelOverride resolves an endpoint like ResolveEndpoint,
// but uses modelOverride as the request model when it is non-empty. The override
// can also supply the otherwise required model for a configured endpoint.
func ResolveEndpointWithModelOverride(configPath, modelOverride string) (ResolvedEndpoint, error) {
	modelOverride = strings.TrimSpace(modelOverride)

	strategies := []struct {
		name string
		fn   func() (ResolvedEndpoint, bool, error)
	}{
		{"OCR config file", func() (ResolvedEndpoint, bool, error) { return tryOCRConfig(configPath, modelOverride) }},
		{"OCR environment", func() (ResolvedEndpoint, bool, error) { return tryOCREnv(modelOverride) }},
		{"Claude Code environment", func() (ResolvedEndpoint, bool, error) { return tryCCEnv(modelOverride) }},
		{"Shell rc file", func() (ResolvedEndpoint, bool, error) { return tryShellRC(modelOverride) }},
	}

	for _, s := range strategies {
		ep, ok, err := s.fn()
		if err != nil {
			return ResolvedEndpoint{}, fmt.Errorf("resolve %s: %w", s.name, err)
		}
		if ok && ep.URL != "" && ep.Token != "" && ep.Model != "" {
			if ep.Source == "" {
				ep.Source = s.name
			}
			ep.Model = stripModelSuffix(ep.Model)
			// OCR_LLM_TIMEOUT is a global override: applies regardless of
			// which strategy resolved the endpoint, and takes precedence
			// over config-file values when set.
			envTimeout, ok, err := parseTimeoutEnv()
			if err != nil {
				return ResolvedEndpoint{}, fmt.Errorf("resolve %s: %w", s.name, err)
			}
			if ok {
				ep.Timeout = envTimeout
			}
			// OCR_LLM_EXTRA_HEADERS is a global override: merges into
			// extra headers regardless of which strategy resolved the
			// endpoint. Env values take precedence over config-file values.
			if raw := os.Getenv(envOCRLLMExtraHeaders); raw != "" {
				envHeaders, err := ParseExtraHeaders(raw)
				if err != nil {
					return ResolvedEndpoint{}, fmt.Errorf("resolve %s: %w", s.name, err)
				}
				if ep.ExtraHeaders == nil {
					ep.ExtraHeaders = envHeaders
				} else {
					for k, v := range envHeaders {
						ep.ExtraHeaders[k] = v
					}
				}
			}
			return ep, nil
		}
	}

	return ResolvedEndpoint{}, fmt.Errorf("no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL, ~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/ANTHROPIC_MODEL must be set")
}

// parseTimeoutEnv reads and validates the OCR_LLM_TIMEOUT environment variable.
// Returns the parsed duration and true if set, or 0 and false if unset/empty.
// Returns an error for invalid values (non-integer, negative, overflow) to give
// the user clear feedback instead of silently falling back to the default.
func parseTimeoutEnv() (time.Duration, bool, error) {
	raw := strings.TrimSpace(os.Getenv(envOCRLLMTimeout))
	if raw == "" {
		return 0, false, nil
	}
	sec, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("OCR_LLM_TIMEOUT must be an integer (seconds): %w", err)
	}
	d, err := validateTimeoutSec(sec)
	if err != nil {
		return 0, false, fmt.Errorf("OCR_LLM_TIMEOUT: %w", err)
	}
	return d, true, nil
}

// validateTimeoutSec converts a config-file timeout (in seconds) to time.Duration.
// Returns 0 for zero input (use default). Rejects negative values and overflow.
func validateTimeoutSec(sec int) (time.Duration, error) {
	if sec == 0 {
		return 0, nil
	}
	if sec < 0 {
		return 0, fmt.Errorf("timeout_sec must be non-negative, got %d", sec)
	}
	// Guard against overflow: time.Duration is int64 nanoseconds.
	maxSec := int64(math.MaxInt64 / int64(time.Second))
	if int64(sec) > maxSec {
		return 0, fmt.Errorf("timeout_sec %d overflows time.Duration (max %d)", sec, maxSec)
	}
	return time.Duration(sec) * time.Second, nil
}

// tryOCREnv reads OCR-specific environment variables.
func tryOCREnv(modelOverride string) (ResolvedEndpoint, bool, error) {
	url := os.Getenv(envOCRLLMURL)
	token := os.Getenv(envOCRLLMToken)
	model := os.Getenv(envOCRLLMModel)
	if modelOverride != "" {
		model = modelOverride
	}
	if url == "" || token == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	// OCR_LLM_PROTOCOL (normalized) wins over OCR_USE_ANTHROPIC when set.
	protocol := ""
	if raw := strings.TrimSpace(os.Getenv(envOCRLLMProtocol)); raw != "" {
		protocol = NormalizeProtocol(raw)
		if err := ValidateProtocol(protocol); err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("OCR environment: %w", err)
		}
	}
	if protocol == "" {
		useAnthropic := true // default true
		if v := os.Getenv(envOCRUseAnthropic); v != "" {
			lower := strings.ToLower(v)
			useAnthropic = lower == "true" || lower == "1" || lower == "yes"
		}
		if useAnthropic {
			protocol = ProtocolAnthropic
		} else {
			protocol = ProtocolOpenAIChatCompletions
		}
	}

	var authHeader string
	if protocol == ProtocolAnthropic {
		var err error
		authHeader, err = NormalizeAuthHeader(os.Getenv(envOCRLLMAuthHeader))
		if err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("OCR environment: %w", err)
		}
		if authHeader == "" {
			authHeader = defaultAuthHeader(protocol)
		}
	}

	return ResolvedEndpoint{URL: url, Token: token, Model: model, Protocol: protocol, AuthHeader: authHeader, Source: "OCR environment"}, true, nil
}

// llmFileConfig represents the llm section in config.json.
type llmFileConfig struct {
	URL          string            `json:"url,omitempty"`
	AuthToken    string            `json:"auth_token,omitempty"`
	AuthHeader   string            `json:"auth_header,omitempty"`
	Model        string            `json:"model,omitempty"`
	Protocol     string            `json:"protocol,omitempty"`      // anthropic|openai|openai-responses; takes priority over use_anthropic
	UseAnthropic *bool             `json:"use_anthropic,omitempty"` // pointer to distinguish unset from false; legacy fallback when protocol is empty
	TimeoutSec   int               `json:"timeout_sec,omitempty"`   // per-request HTTP timeout in seconds
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
}

// providerEntryConfig represents a single provider entry in config.json.
type providerEntryConfig struct {
	APIKey       string            `json:"api_key,omitempty"`
	URL          string            `json:"url,omitempty"`
	Protocol     string            `json:"protocol,omitempty"`
	Model        string            `json:"model,omitempty"`
	Models       []string          `json:"models,omitempty"`
	AuthHeader   string            `json:"auth_header,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"` // per-request HTTP timeout in seconds
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
}

type configFile struct {
	Provider        string                         `json:"provider,omitempty"`
	Model           string                         `json:"model,omitempty"`
	Providers       map[string]providerEntryConfig `json:"providers,omitempty"`
	CustomProviders map[string]providerEntryConfig `json:"custom_providers,omitempty"`
	Llm             llmFileConfig                  `json:"llm,omitempty"`
}

// tryOCRConfig reads the OCR config file.
func tryOCRConfig(path, modelOverride string) (ResolvedEndpoint, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedEndpoint{}, false, nil
		}
		return ResolvedEndpoint{}, false, err
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ResolvedEndpoint{}, false, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Provider != "" {
		return tryProviderConfig(cfg, modelOverride)
	}

	return tryLegacyLlmConfig(cfg, modelOverride)
}

// tryProviderConfig resolves an endpoint from the provider-based configuration.
func tryProviderConfig(cfg configFile, modelOverride string) (ResolvedEndpoint, bool, error) {
	preset, isPreset := LookupProvider(cfg.Provider)

	var entry providerEntryConfig
	var ok bool
	if isPreset {
		entry, ok = cfg.Providers[cfg.Provider]
	} else {
		entry, ok = cfg.CustomProviders[cfg.Provider]
	}
	if !ok {
		section := "providers"
		if !isPreset {
			section = "custom_providers"
		}
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q is set but not configured in %s section", cfg.Provider, section)
	}

	apiKey := entry.APIKey
	if apiKey == "" {
		if isPreset && preset.EnvVar != "" {
			apiKey = os.Getenv(preset.EnvVar)
		}
	}
	if apiKey == "" {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q has no api_key configured and no environment variable fallback found", cfg.Provider)
	}

	var url, protocol, authHeader, model string
	var extraBody map[string]any

	if isPreset {
		url = preset.BaseURL
		protocol = NormalizeProtocol(preset.Protocol)
		if err := ValidateProtocol(protocol); err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("provider %q: %w", cfg.Provider, err)
		}
		authHeader = preset.AuthHeader
		if entry.URL != "" {
			url = entry.URL
		}
		if entry.Protocol != "" {
			normalized := NormalizeProtocol(entry.Protocol)
			if err := ValidateProtocol(normalized); err != nil {
				return ResolvedEndpoint{}, false, fmt.Errorf("provider %q: %w", cfg.Provider, err)
			}
			protocol = normalized
		}
	} else {
		// Custom provider: url and protocol are required; model can come from cfg.Model.
		if entry.URL == "" || entry.Protocol == "" {
			return ResolvedEndpoint{}, false, fmt.Errorf("custom provider %q requires url and protocol fields", cfg.Provider)
		}
		normalized := NormalizeProtocol(entry.Protocol)
		if err := ValidateProtocol(normalized); err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("custom provider %q: %w", cfg.Provider, err)
		}
		url = entry.URL
		protocol = normalized
	}

	if cfg.Model != "" {
		model = cfg.Model
	}
	if entry.Model != "" {
		model = entry.Model
	}

	// Build available model list for validation.
	var availableModels []string
	if isPreset {
		availableModels = append(availableModels, preset.Models...)
	}
	availableModels = append(availableModels, entry.Models...)

	// Apply model override with validation.
	if modelOverride != "" {
		if len(availableModels) > 0 {
			if !ModelListContains(availableModels, modelOverride) {
				return ResolvedEndpoint{}, false, fmt.Errorf(
					"model %q is not available for provider %q; available models: %s",
					modelOverride,
					cfg.Provider,
					strings.Join(availableModels, ", "),
				)
			}
		}
		model = modelOverride
	}

	if model == "" {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q has no model configured; run 'ocr config model' to select one or pass --model", cfg.Provider)
	}

	if protocol == ProtocolAnthropic {
		var err error
		ah := "authorization"
		if isPreset && authHeader != "" {
			ah = authHeader
		}
		if entry.AuthHeader != "" {
			ah = entry.AuthHeader
		}
		authHeader, err = NormalizeAuthHeader(ah)
		if err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("provider %q: %w", cfg.Provider, err)
		}
		if authHeader == "" {
			authHeader = defaultAuthHeader(protocol)
		}
	} else {
		authHeader = ""
	}

	extraBody = entry.ExtraBody
	extraHeaders := entry.ExtraHeaders

	timeout, err := validateTimeoutSec(entry.TimeoutSec)
	if err != nil {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q: %w", cfg.Provider, err)
	}

	if protocol == ProtocolAnthropic {
		url = ensureMessagesSuffix(url)
	}

	return ResolvedEndpoint{
		URL:          url,
		Token:        apiKey,
		Model:        model,
		Protocol:     protocol,
		AuthHeader:   authHeader,
		Source:       "provider:" + cfg.Provider,
		ExtraBody:    extraBody,
		ExtraHeaders: extraHeaders,
		Timeout:      timeout,
	}, true, nil
}

// tryLegacyLlmConfig resolves an endpoint from the legacy llm config block.
func tryLegacyLlmConfig(cfg configFile, modelOverride string) (ResolvedEndpoint, bool, error) {
	model := cfg.Llm.Model
	if modelOverride != "" {
		model = modelOverride
	}
	if cfg.Llm.URL == "" || cfg.Llm.AuthToken == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	// llm.protocol (normalized) wins over use_anthropic when set.
	protocol := ""
	if raw := strings.TrimSpace(cfg.Llm.Protocol); raw != "" {
		protocol = NormalizeProtocol(raw)
		if err := ValidateProtocol(protocol); err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("OCR config file: %w", err)
		}
	}
	if protocol == "" {
		useAnthropic := true // default true
		if cfg.Llm.UseAnthropic != nil {
			useAnthropic = *cfg.Llm.UseAnthropic
		}
		if useAnthropic {
			protocol = ProtocolAnthropic
		} else {
			protocol = ProtocolOpenAIChatCompletions
		}
	}

	var authHeader string
	if protocol == ProtocolAnthropic {
		var err error
		authHeader, err = NormalizeAuthHeader(cfg.Llm.AuthHeader)
		if err != nil {
			return ResolvedEndpoint{}, false, fmt.Errorf("OCR config file: %w", err)
		}
		if authHeader == "" {
			authHeader = defaultAuthHeader(protocol)
		}
	}

	timeout, err := validateTimeoutSec(cfg.Llm.TimeoutSec)
	if err != nil {
		return ResolvedEndpoint{}, false, fmt.Errorf("OCR config file: %w", err)
	}

	return ResolvedEndpoint{URL: cfg.Llm.URL, Token: cfg.Llm.AuthToken, Model: model, Protocol: protocol, AuthHeader: authHeader, Source: "OCR config file", ExtraBody: cfg.Llm.ExtraBody, ExtraHeaders: cfg.Llm.ExtraHeaders, Timeout: timeout}, true, nil
}

// tryCCEnv reads Claude Code environment variables.
func tryCCEnv(modelOverride string) (ResolvedEndpoint, bool, error) {
	baseURL := os.Getenv(envCCBaseURL)
	token := os.Getenv(envCCToken)
	model := os.Getenv(envCCModel)
	if modelOverride != "" {
		model = modelOverride
	}
	if baseURL == "" || token == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	url := ensureMessagesSuffix(baseURL)

	// Claude Code environment tokens are OAuth/Bearer-style credentials.
	return ResolvedEndpoint{URL: url, Token: token, Model: model, Protocol: ProtocolAnthropic, AuthHeader: "authorization", Source: "Claude Code environment"}, true, nil
}

// tryShellRC parses ~/.zshrc and ~/.bashrc for ANTHROPIC_* exports.
func tryShellRC(modelOverride string) (ResolvedEndpoint, bool, error) {
	files := shellRCFiles()
	for _, f := range files {
		ep, ok, err := parseShellRC(f, modelOverride)
		if err != nil || ok {
			return ep, ok, err
		}
	}
	return ResolvedEndpoint{}, false, nil
}

func shellRCFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
	}
	var valid []string
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			valid = append(valid, f)
		}
	}
	return valid
}

var exportRe = regexp.MustCompile(`^export\s+(ANTHROPIC_\w+)\s*=\s*(?:"([^"]*)"|'([^']*)'|(.+))\s*$`)

var modelSuffixRe = regexp.MustCompile(`\[\d+m\]$`)

func stripModelSuffix(model string) string {
	return modelSuffixRe.ReplaceAllString(model, "")
}

func parseShellRC(path, modelOverride string) (ResolvedEndpoint, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ResolvedEndpoint{}, false, nil
	}

	var baseURL, token, model string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		matches := exportRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		key := matches[1]
		value := matches[2]
		if value == "" {
			value = matches[3]
		}
		if value == "" {
			value = matches[4]
		}
		value = strings.TrimSpace(value)

		switch key {
		case "ANTHROPIC_BASE_URL":
			baseURL = value
		case "ANTHROPIC_AUTH_TOKEN":
			token = value
		case "ANTHROPIC_MODEL":
			model = value
		}
	}
	if modelOverride != "" {
		model = modelOverride
	}

	if baseURL == "" || token == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	url := ensureMessagesSuffix(baseURL)

	// Claude Code shell rc tokens are OAuth/Bearer-style credentials.
	return ResolvedEndpoint{URL: url, Token: token, Model: model, Protocol: ProtocolAnthropic, AuthHeader: "authorization", Source: "Shell rc file"}, true, nil
}

func defaultAuthHeader(protocol string) string {
	// auth_header is Anthropic-only; OpenAI-compatible clients keep API key auth.
	if protocol == ProtocolAnthropic {
		return "authorization"
	}
	return ""
}

// ModelListContains reports whether target matches any entry in models (trimmed).
func ModelListContains(models []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, model := range models {
		if strings.TrimSpace(model) == target {
			return true
		}
	}
	return false
}

// NormalizeAuthHeader normalizes an auth header value to a canonical form.
// It returns an error for unrecognized values.
func NormalizeAuthHeader(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", nil
	}
	switch strings.ToLower(header) {
	case "x-api-key":
		return "x-api-key", nil
	case "authorization", "bearer":
		return "authorization", nil
	default:
		return "", fmt.Errorf("unsupported auth_header value %q; expected \"x-api-key\" or \"authorization\"", header)
	}
}

// reservedHeaders are HTTP headers that extra_headers must not override.
// They are managed by dedicated config fields (auth_header, auth_token) or set automatically by the SDK.
// Letting extra_headers clobber them would cause confusing auth/content-type failures with no clear error.
var reservedHeaders = map[string]bool{
	"authorization": true,
	"x-api-key":     true,
	"content-type":  true,
	"user-agent":    true,
}

// ParseExtraHeaders parses a string of comma-separated key=value pairs into a dictionary.
// Values may be double-quoted to include commas, e.g. X-Forwarded-For="1.2.3.4,5.6.7.8".
// Reserved header names (authorization, x-api-key, content-type, user-agent) are rejected
// to prevent accidental override of auth or content-type set by the SDK.
func ParseExtraHeaders(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}

	pairs, err := splitHeaderPairs(raw)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid extra header %q: expected key=value", pair)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid extra header %q: empty header name", pair)
		}
		if reservedHeaders[strings.ToLower(key)] {
			return nil, fmt.Errorf("extra header %q conflicts with a reserved header; use the dedicated config field instead", key)
		}
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		result[key] = value
	}
	return result, nil
}

// splitHeaderPairs splits a comma-separated string while respecting double-quoted segments.
// Commas inside quotes are part of the value, not pair separators.
func splitHeaderPairs(raw string) ([]string, error) {
	var pairs []string
	var sb strings.Builder
	inQuote := false
	for _, c := range raw {
		switch {
		case c == '"':
			inQuote = !inQuote
			sb.WriteRune(c)
		case c == ',' && !inQuote:
			pairs = append(pairs, sb.String())
			sb.Reset()
		default:
			sb.WriteRune(c)
		}
	}
	if sb.Len() > 0 || len(pairs) == 0 {
		pairs = append(pairs, sb.String())
	}
	if inQuote {
		return nil, fmt.Errorf("unclosed quote in extra headers")
	}
	return pairs, nil
}

// ensureMessagesSuffix appends /v1/messages to base URLs that lack a versioned path.
func ensureMessagesSuffix(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.Contains(u, "/v1/") {
		// Already has versioned path — don't modify.
		return rawURL
	}
	return u + "/v1/messages"
}
