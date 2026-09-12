// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package llmresolve resolves the parsing LLM endpoint from the adapter's own
// minimal configuration, and speaks the two protocols the adapter supports.
//
// It deliberately does not read the OCR config file, the OCR_LLM_* or
// ANTHROPIC_* environment, any shell rc file, or run api_key_cmd. The parsing
// LLM is configured independently from the review LLM, so an OCR upgrade never
// forces the adapter to copy or drift with OCR's provider system.
package llmresolve

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Canonical protocol names, matching the OCR values.
const (
	ProtocolAnthropic        = "anthropic"
	ProtocolOpenAI           = "openai"
	ProtocolOpenAIResponses  = "openai-responses"
	ProtocolAnthropicBedrock = "anthropic-bedrock"
)

// Parsing configuration environment variables. The matching startup flags
// (--parser-provider, --parser-model, --parser-base-url) win over these; the
// API key has no flag and is environment-only.
const (
	EnvProvider = "OCR_ACP_PARSER_PROVIDER"
	EnvModel    = "OCR_ACP_PARSER_MODEL"
	EnvBaseURL  = "OCR_ACP_PARSER_BASE_URL"
	EnvAPIKey   = "OCR_ACP_PARSER_API_KEY"
)

// Endpoint is a resolved LLM endpoint. Token is never written to logs; use
// Summary for the reproducible evidence line.
type Endpoint struct {
	URL          string
	Token        string
	Model        string
	Provider     string
	Protocol     string
	AuthHeader   string
	Source       string
	Timeout      time.Duration
	ExtraHeaders map[string]string
	ExtraBody    map[string]any
}

// Options carries the parsing startup flags and injects the environment for
// tests. APIKey has no corresponding flag: the real CLI only ever sets it from
// OCR_ACP_PARSER_API_KEY, and this field exists so tests and a future secret
// source can supply it directly.
type Options struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
	Getenv   func(string) string
}

// defaultBaseURLs are the only two endpoints the adapter will guess. Any other
// provider must set --parser-base-url / OCR_ACP_PARSER_BASE_URL explicitly.
var defaultBaseURLs = map[string]string{
	ProtocolAnthropic: "https://api.anthropic.com",
	ProtocolOpenAI:    "https://api.openai.com/v1",
}

// Resolve builds the parsing endpoint from the startup flags and the
// OCR_ACP_PARSER_* environment variables, with the flags taking precedence.
func Resolve(opts Options) (Endpoint, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	proto, err := normalizeProtocol(firstNonEmpty(opts.Provider, getenv(EnvProvider)))
	if err != nil {
		return Endpoint{}, err
	}

	model := strings.TrimSpace(firstNonEmpty(opts.Model, getenv(EnvModel)))
	if model == "" {
		return Endpoint{}, fmt.Errorf("no parsing model configured: set --parser-model or %s", EnvModel)
	}

	baseURL := strings.TrimSpace(firstNonEmpty(opts.BaseURL, getenv(EnvBaseURL)))
	if baseURL == "" {
		baseURL = defaultBaseURLs[proto]
	}
	if err := validateBaseURL(baseURL); err != nil {
		return Endpoint{}, err
	}

	token := strings.TrimSpace(firstNonEmpty(opts.APIKey, getenv(EnvAPIKey)))
	if token == "" {
		return Endpoint{}, fmt.Errorf("no parsing API key configured: set %s", EnvAPIKey)
	}

	source := "environment"
	if strings.TrimSpace(opts.Provider) != "" || strings.TrimSpace(opts.Model) != "" || strings.TrimSpace(opts.BaseURL) != "" {
		source = "startup flags"
	}

	return Endpoint{
		URL:        baseURL,
		Token:      token,
		Model:      model,
		Provider:   proto,
		Protocol:   proto,
		AuthHeader: defaultAuthHeader(proto),
		Source:     source,
	}, nil
}

func normalizeProtocol(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ProtocolOpenAI:
		return ProtocolOpenAI, nil
	case ProtocolAnthropic:
		return ProtocolAnthropic, nil
	case "":
		return "", fmt.Errorf("no parsing provider configured: set --parser-provider or %s to %q or %q", EnvProvider, ProtocolAnthropic, ProtocolOpenAI)
	case ProtocolOpenAIResponses, ProtocolAnthropicBedrock:
		return "", fmt.Errorf("the parsing client does not support protocol %q yet; use %q or %q", raw, ProtocolAnthropic, ProtocolOpenAI)
	default:
		return "", fmt.Errorf("unsupported parsing provider %q; only %q and %q are supported", raw, ProtocolAnthropic, ProtocolOpenAI)
	}
}

func defaultAuthHeader(proto string) string {
	if proto == ProtocolAnthropic {
		return "x-api-key"
	}
	return ""
}

func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parsing base URL %q is not a valid URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("parsing base URL %q must start with http:// or https://", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("parsing base URL %q has no host", raw)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Summary returns a reproducible one-line description of the endpoint that
// never contains the token.
func (e Endpoint) Summary() string {
	return fmt.Sprintf("parsing LLM: source=%s protocol=%s model=%s url=%s", e.Source, e.Protocol, e.Model, redactURL(e.URL))
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
