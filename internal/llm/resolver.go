package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ResolvedEndpoint holds the resolved LLM endpoint configuration.
type ResolvedEndpoint struct {
	URL        string
	Token      string
	Model      string
	Protocol   string         // "anthropic" or "openai"
	AuthHeader string         // Anthropic auth header: "x-api-key" or "authorization"
	Source     string         // human-readable config source label
	ExtraBody  map[string]any // vendor-specific request body fields
}

// Environment variable names for OCR-specific configuration.
const (
	envOCRLLMURL        = "OCR_LLM_URL"
	envOCRLLMToken      = "OCR_LLM_TOKEN"
	envOCRLLMModel      = "OCR_LLM_MODEL"
	envOCRLLMAuthHeader = "OCR_LLM_AUTH_HEADER"
	envOCRUseAnthropic  = "OCR_USE_ANTHROPIC"
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
	strategies := []struct {
		name string
		fn   func() (ResolvedEndpoint, bool, error)
	}{
		{"OCR config file", func() (ResolvedEndpoint, bool, error) { return tryOCRConfig(configPath) }},
		{"OCR environment", tryOCREnv},
		{"Claude Code environment", tryCCEnv},
		{"Shell rc file", tryShellRC},
	}

	for _, s := range strategies {
		ep, ok, err := s.fn()
		if err != nil {
			return ResolvedEndpoint{}, fmt.Errorf("resolve %s: %w", s.name, err)
		}
		if ok && ep.URL != "" && ep.Token != "" && ep.Model != "" {
			ep.Source = s.name
			ep.Model = stripModelSuffix(ep.Model)
			return ep, nil
		}
	}

	return ResolvedEndpoint{}, fmt.Errorf("no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL, ~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/ANTHROPIC_MODEL must be set")
}

// tryOCREnv reads OCR-specific environment variables.
func tryOCREnv() (ResolvedEndpoint, bool, error) {
	url := os.Getenv(envOCRLLMURL)
	token := os.Getenv(envOCRLLMToken)
	model := os.Getenv(envOCRLLMModel)
	if url == "" || token == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	useAnthropic := true // default true
	if v := os.Getenv(envOCRUseAnthropic); v != "" {
		lower := strings.ToLower(v)
		useAnthropic = lower == "true" || lower == "1" || lower == "yes"
	}

	protocol := "anthropic"
	if !useAnthropic {
		protocol = "openai"
	}

	var authHeader string
	if protocol == "anthropic" {
		authHeader = normalizeAuthHeader(os.Getenv(envOCRLLMAuthHeader))
		if authHeader == "" {
			authHeader = defaultAuthHeader(protocol)
		}
	}

	return ResolvedEndpoint{URL: url, Token: token, Model: model, Protocol: protocol, AuthHeader: authHeader, Source: "OCR environment"}, true, nil
}

// llmFileConfig represents the llm section in config.json.
type llmFileConfig struct {
	URL          string         `json:"url,omitempty"`
	AuthToken    string         `json:"auth_token,omitempty"`
	AuthHeader   string         `json:"auth_header,omitempty"`
	Model        string         `json:"model,omitempty"`
	UseAnthropic *bool          `json:"use_anthropic,omitempty"` // pointer to distinguish unset from false
	ExtraBody    map[string]any `json:"extra_body,omitempty"`
}

type configFile struct {
	Llm llmFileConfig `json:"llm,omitempty"`
}

// tryOCRConfig reads the OCR config file.
func tryOCRConfig(path string) (ResolvedEndpoint, bool, error) {
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

	if cfg.Llm.URL == "" || cfg.Llm.AuthToken == "" || cfg.Llm.Model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	useAnthropic := true // default true
	if cfg.Llm.UseAnthropic != nil {
		useAnthropic = *cfg.Llm.UseAnthropic
	}

	protocol := "anthropic"
	if !useAnthropic {
		protocol = "openai"
	}

	var authHeader string
	if protocol == "anthropic" {
		authHeader = normalizeAuthHeader(cfg.Llm.AuthHeader)
		if authHeader == "" {
			authHeader = defaultAuthHeader(protocol)
		}
	}

	return ResolvedEndpoint{URL: cfg.Llm.URL, Token: cfg.Llm.AuthToken, Model: cfg.Llm.Model, Protocol: protocol, AuthHeader: authHeader, Source: "OCR config file", ExtraBody: cfg.Llm.ExtraBody}, true, nil
}

// tryCCEnv reads Claude Code environment variables.
func tryCCEnv() (ResolvedEndpoint, bool, error) {
	baseURL := os.Getenv(envCCBaseURL)
	token := os.Getenv(envCCToken)
	model := os.Getenv(envCCModel)
	if baseURL == "" || token == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	url := ensureMessagesSuffix(baseURL)

	// Claude Code environment tokens are OAuth/Bearer-style credentials.
	return ResolvedEndpoint{URL: url, Token: token, Model: model, Protocol: "anthropic", AuthHeader: "authorization", Source: "Claude Code environment"}, true, nil
}

// tryShellRC parses ~/.zshrc and ~/.bashrc for ANTHROPIC_* exports.
func tryShellRC() (ResolvedEndpoint, bool, error) {
	files := shellRCFiles()
	for _, f := range files {
		ep, ok, err := parseShellRC(f)
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

func parseShellRC(path string) (ResolvedEndpoint, bool, error) {
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

	if baseURL == "" || token == "" || model == "" {
		return ResolvedEndpoint{}, false, nil
	}

	url := ensureMessagesSuffix(baseURL)

	// Claude Code shell rc tokens are OAuth/Bearer-style credentials.
	return ResolvedEndpoint{URL: url, Token: token, Model: model, Protocol: "anthropic", AuthHeader: "authorization", Source: "Shell rc file"}, true, nil
}

func defaultAuthHeader(protocol string) string {
	// auth_header is Anthropic-only; OpenAI-compatible clients keep API key auth.
	if protocol == "anthropic" {
		return "authorization"
	}
	return ""
}

func normalizeAuthHeader(header string) string {
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

// ensureMessagesSuffix appends /v1/messages to base URLs that lack a versioned path.
func ensureMessagesSuffix(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if strings.Contains(u, "/v1/") {
		// Already has versioned path — don't modify.
		return rawURL
	}
	return u + "/v1/messages"
}
