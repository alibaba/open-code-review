// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
	Provider     string
	Protocol     string            // canonical protocol name (see protocol.go); resolver normalizes aliases
	AuthHeader   string            // Anthropic auth header: "x-api-key" or "authorization"
	Source       string            // human-readable config source label
	ExtraBody    map[string]any    // vendor-specific request body fields
	ExtraHeaders map[string]string // extra HTTP headers for the LLM request
	// Timeout is the per-request HTTP timeout; 0 means use the client default (5 min).
	// Only config file (llm/provider sections) and OCR_LLM_TIMEOUT env var can set this.
	// tryCCEnv and tryShellRC always leave it at 0 since those sources have no timeout
	// knob; users can still override via OCR_LLM_TIMEOUT.
	Timeout    time.Duration
	RetryCodes []int // additional HTTP status codes that trigger exponential-backoff retry

	// AmbientAuth marks an endpoint that carries no token and needs no base
	// URL, because the transport supplies both — AWS SigV4 signing derives the
	// host from the region and the credentials from the environment's own
	// chain. Completeness checks must treat an empty URL and Token as valid for
	// these; requiring either would reject a correctly configured endpoint.
	AmbientAuth bool

	// AWSProfile and AWSRegion override the ambient AWS chain for SigV4
	// providers. Empty means "let the AWS SDK decide".
	AWSProfile string
	AWSRegion  string
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
	// envOCRLLMTimeout is a global override parsed at the top of
	// ResolveEndpointWithOptions and applied by finalizeResolvedEndpoint to
	// whichever strategy resolves, rather than inside tryOCREnv like other
	// OCR_LLM_* vars. This lets it override timeout for all resolution paths
	// (OCR env, config file, provider config, Claude Code env, shell RC).
	envOCRLLMTimeout   = "OCR_LLM_TIMEOUT"
	envOCRUseAnthropic = "OCR_USE_ANTHROPIC"
)

// Environment variable names from Claude Code configuration.
const (
	envCCBaseURL = "ANTHROPIC_BASE_URL"
	envCCToken   = "ANTHROPIC_AUTH_TOKEN"
	envCCModel   = "ANTHROPIC_MODEL"
)

// ResolveOptions controls per-run endpoint resolution overrides.
type ResolveOptions struct {
	Provider string
	Model    string
}

// ResolveEndpoint resolves an endpoint without per-run overrides.
func ResolveEndpoint(configPath string) (ResolvedEndpoint, error) {
	return ResolveEndpointWithOptions(configPath, ResolveOptions{})
}

// ResolveEndpointWithModelOverride is a compatibility wrapper around
// ResolveEndpointWithOptions for callers that only override the model.
func ResolveEndpointWithModelOverride(configPath, modelOverride string) (ResolvedEndpoint, error) {
	return ResolveEndpointWithOptions(configPath, ResolveOptions{Model: modelOverride})
}

// ResolveEndpointWithOptions resolves an endpoint by first honoring an explicit
// provider selection. Otherwise it processes config, environment, then shell RC
// strategies in priority order, finalizing the first complete endpoint.
func ResolveEndpointWithOptions(configPath string, opts ResolveOptions) (ResolvedEndpoint, error) {
	opts.Provider = strings.TrimSpace(opts.Provider)
	opts.Model = strings.TrimSpace(opts.Model)

	// The global env overrides are parsed before any strategy runs, even though
	// they are applied to the endpoint afterwards. Parsing them inside
	// finalizeResolvedEndpoint would let a typo'd OCR_LLM_TIMEOUT ("30s") or an
	// unparseable OCR_LLM_EXTRA_HEADERS abort resolution *after* api_key_cmd
	// already prompted 1Password/pinentry/Touch ID for a credential that then
	// gets discarded.
	env, err := parseEnvOverrides()
	if err != nil {
		return ResolvedEndpoint{}, err
	}

	if opts.Provider != "" {
		ep, ok, err := tryOCRConfig(configPath, opts)
		if err != nil {
			return ResolvedEndpoint{}, fmt.Errorf("resolve OCR config file: %w", err)
		}
		if !ok {
			section := "custom_providers"
			if _, isPreset := LookupProvider(opts.Provider); isPreset {
				section = "providers"
			}
			return ResolvedEndpoint{}, fmt.Errorf("resolve OCR config file: provider %q is not configured in %s section because the config file does not exist", opts.Provider, section)
		}
		return finalizeResolvedEndpoint("OCR config file", ep, env), nil
	}

	strategies := []struct {
		name string
		fn   func() (ResolvedEndpoint, bool, error)
	}{
		{"OCR config file", func() (ResolvedEndpoint, bool, error) { return tryOCRConfig(configPath, opts) }},
		{"OCR environment", func() (ResolvedEndpoint, bool, error) { return tryOCREnv(opts.Model) }},
		{"Claude Code environment", func() (ResolvedEndpoint, bool, error) { return tryCCEnv(opts.Model) }},
		{"Shell rc file", func() (ResolvedEndpoint, bool, error) { return tryShellRC(opts.Model) }},
	}

	for _, strategy := range strategies {
		ep, ok, err := strategy.fn()
		if err != nil {
			return ResolvedEndpoint{}, fmt.Errorf("resolve %s: %w", strategy.name, err)
		}
		// An ambient-auth endpoint is complete without a URL or token: the
		// transport supplies both. Everything else still needs all three.
		complete := ep.Model != "" && (ep.AmbientAuth || (ep.URL != "" && ep.Token != ""))
		if ok && complete {
			return finalizeResolvedEndpoint(strategy.name, ep, env), nil
		}
	}

	return ResolvedEndpoint{}, fmt.Errorf("no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL, ~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/ANTHROPIC_MODEL must be set")
}

// envOverrides holds the global OCR_LLM_* overrides that apply to whichever
// strategy resolves the endpoint. Parsed once, up front — see the call site in
// ResolveEndpointWithOptions for why the timing matters.
type envOverrides struct {
	timeout    time.Duration
	hasTimeout bool
	headers    map[string]string
}

func parseEnvOverrides() (envOverrides, error) {
	var env envOverrides
	var err error
	env.timeout, env.hasTimeout, err = parseTimeoutEnv()
	if err != nil {
		return envOverrides{}, err
	}
	if raw := os.Getenv(envOCRLLMExtraHeaders); raw != "" {
		env.headers, err = ParseExtraHeaders(raw)
		if err != nil {
			return envOverrides{}, fmt.Errorf("%s: %w", envOCRLLMExtraHeaders, err)
		}
	}
	return env, nil
}

// finalizeResolvedEndpoint stamps the source label, strips the model suffix and
// applies the global env overrides, which win over config-file values.
func finalizeResolvedEndpoint(source string, ep ResolvedEndpoint, env envOverrides) ResolvedEndpoint {
	if ep.Source == "" {
		ep.Source = source
	}
	ep.Model = stripModelSuffix(ep.Model)
	if env.hasTimeout {
		ep.Timeout = env.timeout
	}
	if env.headers != nil {
		if ep.ExtraHeaders == nil {
			ep.ExtraHeaders = env.headers
		} else {
			for key, value := range env.headers {
				ep.ExtraHeaders[key] = value
			}
		}
	}
	return ep
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
	AuthTokenCmd string            `json:"auth_token_cmd,omitempty"` // shell command whose stdout is the auth token; used when auth_token is empty
	Protocol     string            `json:"protocol,omitempty"`       // anthropic|openai|openai-responses; takes priority over use_anthropic
	UseAnthropic *bool             `json:"use_anthropic,omitempty"`  // pointer to distinguish unset from false; legacy fallback when protocol is empty
	TimeoutSec   int               `json:"timeout_sec,omitempty"`    // per-request HTTP timeout in seconds
	ExtraBody    map[string]any    `json:"extra_body,omitempty"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
	RetryCodes   []int             `json:"retry_codes,omitempty"`
}

// providerEntryConfig represents a single provider entry in config.json.
type providerEntryConfig struct {
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

	// AWSProfile and AWSRegion apply to ambient-auth providers that sign with
	// SigV4 (currently bedrock). Both are optional: without them the standard
	// AWS chain decides, same as any other AWS tool. Setting them in config
	// makes a review run reproducible without exporting AWS_PROFILE first.
	AWSProfile string `json:"aws_profile,omitempty"`
	AWSRegion  string `json:"aws_region,omitempty"`
}

type configFile struct {
	Provider        string                         `json:"provider,omitempty"`
	Model           string                         `json:"model,omitempty"`
	Providers       map[string]providerEntryConfig `json:"providers,omitempty"`
	CustomProviders map[string]providerEntryConfig `json:"custom_providers,omitempty"`
	Llm             llmFileConfig                  `json:"llm,omitempty"`
}

// tryOCRConfig reads the OCR config file.
func tryOCRConfig(path string, opts ResolveOptions) (ResolvedEndpoint, bool, error) {
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

	if opts.Provider != "" {
		if opts.Provider != cfg.Provider {
			cfg.Model = ""
		}
		cfg.Provider = opts.Provider
	}
	if cfg.Provider != "" {
		return tryProviderConfig(cfg, opts.Model)
	}

	return tryLegacyLlmConfig(cfg, opts.Model)
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

	// Pick the credential source here, but run api_key_cmd only just before
	// returning (see below): a config typo must not trigger a secret-manager
	// prompt before the cheap validation below has had a chance to fail.
	// A whitespace-only api_key is a typo, not a credential: treat it as unset so
	// it cannot silently shadow a working api_key_cmd (which otherwise resolves to
	// a 401 with the command never running). A key with real content is used
	// verbatim -- unlike command stdout, which has a mechanical trailing newline
	// to strip, a static value has no artifact that trimming must undo.
	apiKey := entry.APIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = ""
	}
	// Same rule for the command: `sh -c "   "` exits 0 with no output, so a
	// whitespace-only api_key_cmd would suppress the env fallback and then fail
	// with "produced empty output". Treating it as unset keeps the typo from
	// being more disruptive than the equivalent typo in api_key.
	apiKeyCmd := entry.APIKeyCmd
	if strings.TrimSpace(apiKeyCmd) == "" {
		apiKeyCmd = ""
	}
	switch {
	case apiKey != "":
		// Static api_key always wins. Warn (don't error) if a command is also set,
		// so a config that keeps api_key_cmd as a deliberate fallback still works.
		if apiKeyCmd != "" {
			fmt.Fprintf(os.Stderr, "[ocr] WARNING: provider %q has both api_key and api_key_cmd set; using the static api_key\n", cfg.Provider)
		}
	case apiKeyCmd == "" && isPreset && preset.EnvVar != "":
		// Env var is the last resort: only when neither api_key nor api_key_cmd
		// is set, and only for preset providers (custom ones have no fallback).
		// Same whitespace rule as the static key above, so `export
		// ANTHROPIC_API_KEY="  "` reports "no api_key configured" instead of
		// sending `Authorization: Bearer  ` and getting an opaque 401.
		if v := os.Getenv(preset.EnvVar); strings.TrimSpace(v) != "" {
			apiKey = v
		}
	}
	// No credential at all is still an error here, before any other validation:
	// only the command's *execution* is deferred, not the emptiness check. An
	// ambient-auth provider (AWS SigV4, for instance) is the exception: it has no
	// key to configure, since credentials come from the environment's own chain
	// and the request is signed rather than bearing a token.
	if apiKey == "" && apiKeyCmd == "" && !(isPreset && preset.AmbientAuth) {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q has no api_key or api_key_cmd configured and no environment variable fallback found", cfg.Provider)
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

	retryCodes, _, err := sanitizeRetryCodes(entry.RetryCodes)
	if err != nil {
		return ResolvedEndpoint{}, false, fmt.Errorf("provider %q: %w", cfg.Provider, err)
	}

	if protocol == ProtocolAnthropic {
		url = ensureMessagesSuffix(url)
	}

	// Single api_key_cmd resolution site for both preset and custom providers,
	// as late as possible: everything above can fail without running the
	// command. Both are empty only for an ambient-auth provider, which the
	// emptiness check above lets through and which has no command to run; for
	// everyone else a failing command is a hard error.
	if apiKey == "" && apiKeyCmd != "" {
		resolved, err := resolveKeyCmd(apiKeyCmd, fmt.Sprintf("api_key_cmd for provider %q", cfg.Provider))
		if err != nil {
			return ResolvedEndpoint{}, false, err
		}
		apiKey = resolved
	}

	return ResolvedEndpoint{
		URL:          url,
		Token:        apiKey,
		Model:        model,
		Provider:     cfg.Provider,
		Protocol:     protocol,
		AuthHeader:   authHeader,
		Source:       "provider:" + cfg.Provider,
		ExtraBody:    extraBody,
		ExtraHeaders: extraHeaders,
		Timeout:      timeout,
		RetryCodes:   retryCodes,
		AmbientAuth:  isPreset && preset.AmbientAuth,
		AWSProfile:   entry.AWSProfile,
		AWSRegion:    entry.AWSRegion,
	}, true, nil
}

// tryLegacyLlmConfig resolves an endpoint from the legacy llm config block.
func tryLegacyLlmConfig(cfg configFile, modelOverride string) (ResolvedEndpoint, bool, error) {
	model := cfg.Llm.Model
	if modelOverride != "" {
		model = modelOverride
	}
	// Fall through to later strategies when the legacy block is incomplete. This
	// includes the case where neither auth_token nor auth_token_cmd is set — and,
	// critically, an incomplete block (e.g. missing url) never runs auth_token_cmd.
	// "Incomplete" is judged after modelOverride is applied above, so a block
	// missing only `model` is complete under --model and does run the command;
	// that is the documented contract of ResolveEndpointWithModelOverride.
	// Whitespace-only auth_token is treated as unset, same as api_key above, so it
	// cannot shadow a working auth_token_cmd; same rule for the command itself.
	token := cfg.Llm.AuthToken
	if strings.TrimSpace(token) == "" {
		token = ""
	}
	tokenCmd := cfg.Llm.AuthTokenCmd
	if strings.TrimSpace(tokenCmd) == "" {
		tokenCmd = ""
	}
	if cfg.Llm.URL == "" || model == "" || (token == "" && tokenCmd == "") {
		return ResolvedEndpoint{}, false, nil
	}
	// Static auth_token always wins; warn if a command is also set. The command
	// itself runs only just before returning, after the validation below.
	if token != "" && tokenCmd != "" {
		fmt.Fprintln(os.Stderr, "[ocr] WARNING: llm config has both auth_token and auth_token_cmd set; using the static auth_token")
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

	retryCodes, _, err := sanitizeRetryCodes(cfg.Llm.RetryCodes)
	if err != nil {
		return ResolvedEndpoint{}, false, fmt.Errorf("OCR config file: %w", err)
	}

	// Runs last, after every cheap validation above: token is empty here only for
	// an otherwise-complete block whose auth_token_cmd is set (guaranteed by the
	// incompleteness check above), so a failing command is a hard error and an
	// incomplete or invalid block never prompts for a credential.
	if token == "" {
		resolved, err := resolveKeyCmd(tokenCmd, "auth_token_cmd for llm config")
		if err != nil {
			return ResolvedEndpoint{}, false, err
		}
		token = resolved
	}

	return ResolvedEndpoint{
		URL:          cfg.Llm.URL,
		Token:        token,
		Model:        model,
		Protocol:     protocol,
		AuthHeader:   authHeader,
		Source:       "OCR config file",
		ExtraBody:    cfg.Llm.ExtraBody,
		ExtraHeaders: cfg.Llm.ExtraHeaders,
		Timeout:      timeout,
		RetryCodes:   retryCodes,
	}, true, nil
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
	// Already ends with the full messages path — don't modify.
	if strings.HasSuffix(u, "/v1/messages") {
		return rawURL
	}
	// Ends with /v1 (e.g. "https://api.anthropic.com/v1") — only append /messages.
	if strings.HasSuffix(u, "/v1") {
		return u + "/messages"
	}
	// URL has /v1/ in the middle (e.g. /v1/anthropic) — already versioned, leave it alone.
	if strings.Contains(u, "/v1/") {
		return rawURL
	}
	return u + "/v1/messages"
}

// sanitizeRetryCodes filters out SDK-default codes (408, 409, 429) and returns
// the remaining valid codes, any warnings for filtered codes, and an error if
// any code is outside the 4xx range.
func sanitizeRetryCodes(codes []int) (filtered []int, warnings []string, err error) {
	for _, code := range codes {
		if code < 400 || code > 499 {
			return nil, nil, fmt.Errorf("invalid retry code %d: must be a 4xx status code (5xx codes are already retried by default)", code)
		}
		if code == 408 || code == 409 || code == 429 {
			warnings = append(warnings, fmt.Sprintf("retry code %d is unnecessary (SDK already retries it) and will be ignored", code))
			continue
		}
		filtered = append(filtered, code)
	}
	return filtered, warnings, nil
}

// ParseRetryCodes parses a comma-separated list of HTTP status codes that
// should trigger retry. Non-4xx codes return an error; SDK-default codes
// (408, 409, 429) are filtered out and reported via warnings; the
// remaining codes are returned. An empty input returns nil.
func ParseRetryCodes(raw string) ([]int, []string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[int]bool, len(parts))
	codes := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		code, err := strconv.Atoi(p)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid retry code %q: must be an integer", p)
		}
		if !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	filtered, warnings, err := sanitizeRetryCodes(codes)
	if err != nil {
		return nil, nil, err
	}
	return filtered, warnings, nil
}
