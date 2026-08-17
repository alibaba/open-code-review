// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// OpenAIAccountProviderName is the built-in provider name for ChatGPT/Codex
	// account authentication. It is separate from the API-key based "openai"
	// provider.
	OpenAIAccountProviderName = "openai-account"

	OpenAIOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	OpenAIOAuthAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	OpenAIOAuthTokenURL     = "https://auth.openai.com/oauth/token"
	OpenAIOAuthCallbackPath = "/auth/callback"
	OpenAIOAuthScopes       = "openid profile email offline_access api.connectors.read api.connectors.invoke"

	OpenAIAccountResponsesURL = "https://chatgpt.com/backend-api/codex/responses"
	OpenAIAccountModelsURL    = "https://chatgpt.com/backend-api/codex/models?client_version=1.0.0"
	OpenAIAccountOriginator   = "codex_cli_rs"
	OpenAIAccountDefaultPort  = 1455
)

const (
	openAIAuthFileEnv       = "OCR_OPENAI_AUTH_FILE"
	openAIModelCacheFileEnv = "OCR_OPENAI_MODEL_CACHE_FILE"
)

// OpenAIAccountCredentials contains the OAuth credentials required by the
// ChatGPT/Codex Responses endpoint.
type OpenAIAccountCredentials struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	SourcePath   string `json:"-"`
}

// OpenAIModelCatalog is the account model list returned by the Codex backend.
// ReasoningEfforts and ContextWindows are keyed by model slug.
type OpenAIModelCatalog struct {
	Models           []string            `json:"models"`
	ReasoningEfforts map[string][]string `json:"reasoning_efforts,omitempty"`
	ContextWindows   map[string]int      `json:"context_windows,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at,omitempty"`
}

// OpenAIAccountHTTPError preserves the status code returned by an account API.
// The client uses it to refresh OAuth credentials after a 401 or 403.
type OpenAIAccountHTTPError struct {
	StatusCode int
	Body       string
}

func (e *OpenAIAccountHTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("OpenAI account request failed with HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("OpenAI account request failed with HTTP %d: %s", e.StatusCode, e.Body)
}

type openAIAuthFile struct {
	Tokens       OpenAIAccountCredentials `json:"tokens,omitempty"`
	AccessToken  string                   `json:"access_token,omitempty"`
	RefreshToken string                   `json:"refresh_token,omitempty"`
	IDToken      string                   `json:"id_token,omitempty"`
	AccountID    string                   `json:"account_id,omitempty"`
	ExpiresAt    int64                    `json:"expires_at,omitempty"`
}

var (
	openAIAuthHTTPClient         = &http.Client{Timeout: 30 * time.Second}
	openAIModelCatalogHTTPClient = &http.Client{Timeout: 30 * time.Second}
	openAIAuthMu                 sync.Mutex
)

// OpenAIAccountAuthPath returns the path OCR uses for its account credentials.
func OpenAIAccountAuthPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv(openAIAuthFileEnv)); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "openai-auth.json"), nil
}

// OpenAIAccountModelCachePath returns the path OCR uses for the model catalog.
func OpenAIAccountModelCachePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv(openAIModelCacheFileEnv)); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "openai-models.json"), nil
}

// LoadOpenAIAccountCredentials loads OCR credentials and, when OCR has no
// account file yet, accepts the official Codex CLI auth file as a read-only
// fallback. OCR never writes to the Codex CLI file.
func LoadOpenAIAccountCredentials() (OpenAIAccountCredentials, error) {
	path, err := OpenAIAccountAuthPath()
	if err != nil {
		return OpenAIAccountCredentials{}, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		credentials, parseErr := parseOpenAIAccountCredentials(data)
		if parseErr != nil {
			return OpenAIAccountCredentials{}, fmt.Errorf("parse OpenAI account credentials %s: %w", path, parseErr)
		}
		credentials.SourcePath = path
		return normalizeOpenAIAccountCredentials(credentials), nil
	}
	if !os.IsNotExist(err) {
		return OpenAIAccountCredentials{}, fmt.Errorf("read OpenAI account credentials %s: %w", path, err)
	}

	if os.Getenv(openAIAuthFileEnv) != "" {
		return OpenAIAccountCredentials{}, fmt.Errorf("OpenAI account credentials not found at %s; run 'ocr login --provider openai'", path)
	}

	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return OpenAIAccountCredentials{}, fmt.Errorf("OpenAI account credentials not found at %s; run 'ocr login --provider openai'", path)
	}
	codexPath := filepath.Join(home, ".codex", "auth.json")
	data, err = os.ReadFile(codexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return OpenAIAccountCredentials{}, fmt.Errorf("OpenAI account credentials not found; run 'ocr login --provider openai'")
		}
		return OpenAIAccountCredentials{}, fmt.Errorf("read Codex credentials %s: %w", codexPath, err)
	}
	credentials, err := parseOpenAIAccountCredentials(data)
	if err != nil {
		return OpenAIAccountCredentials{}, fmt.Errorf("parse Codex credentials %s: %w", codexPath, err)
	}
	credentials.SourcePath = codexPath
	return normalizeOpenAIAccountCredentials(credentials), nil
}

// SaveOpenAIAccountCredentials writes OCR credentials with owner-only file
// permissions. It intentionally does not modify ~/.codex/auth.json.
func SaveOpenAIAccountCredentials(credentials OpenAIAccountCredentials) error {
	path, err := OpenAIAccountAuthPath()
	if err != nil {
		return err
	}
	if err := writePrivateJSON(path, openAIAuthFile{Tokens: normalizeOpenAIAccountCredentials(credentials)}); err != nil {
		return fmt.Errorf("save OpenAI account credentials: %w", err)
	}
	return nil
}

// GenerateOpenAIPKCE returns a verifier, its S256 challenge, and an OAuth
// state value suitable for the authorization URL.
func GenerateOpenAIPKCE() (verifier, challenge, state string, err error) {
	verifier, err = randomString(64)
	if err != nil {
		return "", "", "", fmt.Errorf("generate PKCE verifier: %w", err)
	}
	digest := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(digest[:])
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", "", fmt.Errorf("generate OAuth state: %w", err)
	}
	state = fmt.Sprintf("%x", stateBytes)
	return verifier, challenge, state, nil
}

// OpenAIAuthorizationURL creates the official OpenAI OAuth authorization URL.
func OpenAIAuthorizationURL(redirectURI, challenge, state string) string {
	query := url.Values{}
	query.Set("response_type", "code")
	query.Set("client_id", OpenAIOAuthClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", OpenAIOAuthScopes)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("state", state)
	query.Set("id_token_add_organizations", "true")
	query.Set("codex_cli_simplified_flow", "true")
	query.Set("originator", OpenAIAccountOriginator)
	query.Set("prompt", "login")
	return OpenAIOAuthAuthorizeURL + "?" + query.Encode()
}

// ExchangeOpenAICode exchanges an OAuth callback code for account credentials.
func ExchangeOpenAICode(ctx context.Context, code, verifier, redirectURI string) (OpenAIAccountCredentials, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", OpenAIOAuthClientID)
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", redirectURI)

	return exchangeOpenAIToken(ctx, form)
}

// RefreshOpenAIAccountCredentials refreshes an account access token.
func RefreshOpenAIAccountCredentials(ctx context.Context, credentials OpenAIAccountCredentials) (OpenAIAccountCredentials, error) {
	if strings.TrimSpace(credentials.RefreshToken) == "" {
		return OpenAIAccountCredentials{}, errors.New("OpenAI account has no refresh token; run 'ocr login --provider openai' again")
	}

	openAIAuthMu.Lock()
	defer openAIAuthMu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", OpenAIOAuthClientID)
	form.Set("refresh_token", credentials.RefreshToken)
	refreshed, err := exchangeOpenAIToken(ctx, form)
	if err != nil {
		return OpenAIAccountCredentials{}, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = credentials.RefreshToken
	}
	if refreshed.IDToken == "" {
		refreshed.IDToken = credentials.IDToken
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = credentials.AccountID
	}
	refreshed.SourcePath = credentials.SourcePath
	refreshed = normalizeOpenAIAccountCredentials(refreshed)
	if refreshed.SourcePath == "" || refreshed.SourcePath == mustOpenAIAccountAuthPath() || refreshed.SourcePath == defaultCodexAuthPath() {
		if err := SaveOpenAIAccountCredentials(refreshed); err != nil {
			return OpenAIAccountCredentials{}, err
		}
	}
	return refreshed, nil
}

func exchangeOpenAIToken(ctx context.Context, form url.Values) (OpenAIAccountCredentials, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OpenAIOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return OpenAIAccountCredentials{}, fmt.Errorf("create OpenAI OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := openAIAuthHTTPClient.Do(req)
	if err != nil {
		return OpenAIAccountCredentials{}, fmt.Errorf("request OpenAI OAuth token: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return OpenAIAccountCredentials{}, fmt.Errorf("read OpenAI OAuth token response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OpenAIAccountCredentials{}, &OpenAIAccountHTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"`
		ExpiresAt    int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return OpenAIAccountCredentials{}, fmt.Errorf("parse OpenAI OAuth token response: %w", err)
	}
	if token.AccessToken == "" {
		return OpenAIAccountCredentials{}, errors.New("OpenAI OAuth token response did not include access_token")
	}
	expiresAt := token.ExpiresAt
	if expiresAt > 0 && expiresAt < 1e12 {
		expiresAt *= 1000
	}
	if expiresAt == 0 && token.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli()
	}
	return normalizeOpenAIAccountCredentials(OpenAIAccountCredentials{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      token.IDToken,
		ExpiresAt:    expiresAt,
	}), nil
}

// EnsureOpenAIAccountCredentials refreshes credentials when they expire soon.
func EnsureOpenAIAccountCredentials(ctx context.Context, credentials OpenAIAccountCredentials) (OpenAIAccountCredentials, error) {
	if credentials.AccessToken == "" {
		return OpenAIAccountCredentials{}, errors.New("OpenAI account has no access token; run 'ocr login --provider openai'")
	}
	if credentials.ExpiresAt == 0 || credentials.ExpiresAt > time.Now().Add(5*time.Minute).UnixMilli() {
		return credentials, nil
	}
	return RefreshOpenAIAccountCredentials(ctx, credentials)
}

// FetchOpenAIAccountModelCatalog fetches models visible to the authenticated
// account from the Codex model endpoint.
func FetchOpenAIAccountModelCatalog(ctx context.Context, accessToken string) (OpenAIModelCatalog, error) {
	return fetchOpenAIAccountModelCatalog(ctx, accessToken, openAIModelCatalogHTTPClient, OpenAIAccountModelsURL)
}

// RefreshOpenAIAccountModelCatalog refreshes and persists the account model
// catalog. It refreshes the access token first when necessary.
func RefreshOpenAIAccountModelCatalog(ctx context.Context) (OpenAIAccountCredentials, OpenAIModelCatalog, error) {
	credentials, err := LoadOpenAIAccountCredentials()
	if err != nil {
		return OpenAIAccountCredentials{}, OpenAIModelCatalog{}, err
	}
	credentials, err = EnsureOpenAIAccountCredentials(ctx, credentials)
	if err != nil {
		return OpenAIAccountCredentials{}, OpenAIModelCatalog{}, err
	}
	catalog, err := FetchOpenAIAccountModelCatalog(ctx, credentials.AccessToken)
	if err != nil {
		var accountErr *OpenAIAccountHTTPError
		if errors.As(err, &accountErr) && (accountErr.StatusCode == http.StatusUnauthorized || accountErr.StatusCode == http.StatusForbidden) && credentials.RefreshToken != "" {
			credentials, err = RefreshOpenAIAccountCredentials(ctx, credentials)
			if err == nil {
				catalog, err = FetchOpenAIAccountModelCatalog(ctx, credentials.AccessToken)
			}
		}
	}
	if err != nil {
		return credentials, OpenAIModelCatalog{}, err
	}
	if err := SaveOpenAIModelCatalog(catalog); err != nil {
		return credentials, OpenAIModelCatalog{}, err
	}
	return credentials, catalog, nil
}

func fetchOpenAIAccountModelCatalog(ctx context.Context, accessToken string, client *http.Client, endpoint string) (OpenAIModelCatalog, error) {
	if strings.TrimSpace(accessToken) == "" {
		return OpenAIModelCatalog{}, errors.New("OpenAI account model request requires an access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return OpenAIModelCatalog{}, fmt.Errorf("create OpenAI model request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("originator", OpenAIAccountOriginator)
	resp, err := client.Do(req)
	if err != nil {
		return OpenAIModelCatalog{}, fmt.Errorf("request OpenAI account models: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		return OpenAIModelCatalog{}, fmt.Errorf("read OpenAI account models: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OpenAIModelCatalog{}, &OpenAIAccountHTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	catalog, err := parseOpenAIModelCatalog(body)
	if err != nil {
		return OpenAIModelCatalog{}, fmt.Errorf("parse OpenAI account models: %w", err)
	}
	catalog.UpdatedAt = time.Now().UTC()
	return catalog, nil
}

// LoadOpenAIModelCatalog loads the cached account model catalog.
func LoadOpenAIModelCatalog() (OpenAIModelCatalog, error) {
	path, err := OpenAIAccountModelCachePath()
	if err != nil {
		return OpenAIModelCatalog{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OpenAIModelCatalog{}, nil
		}
		return OpenAIModelCatalog{}, fmt.Errorf("read OpenAI model cache %s: %w", path, err)
	}
	var catalog OpenAIModelCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return OpenAIModelCatalog{}, fmt.Errorf("parse OpenAI model cache %s: %w", path, err)
	}
	return normalizeOpenAIModelCatalog(catalog), nil
}

// SaveOpenAIModelCatalog persists the account model catalog.
func SaveOpenAIModelCatalog(catalog OpenAIModelCatalog) error {
	path, err := OpenAIAccountModelCachePath()
	if err != nil {
		return err
	}
	return writePrivateJSON(path, normalizeOpenAIModelCatalog(catalog))
}

// OpenAIAccountModels returns cached models, or a small fallback list before
// the first successful catalog refresh.
func OpenAIAccountModels() []string {
	catalog, err := LoadOpenAIModelCatalog()
	if err == nil && len(catalog.Models) > 0 {
		return append([]string(nil), catalog.Models...)
	}
	return []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.5", "gpt-5.3-codex"}
}

// OpenAIAccountReasoningEfforts returns catalog-supported reasoning values for
// a model. The empty result means the backend did not publish model metadata.
func OpenAIAccountReasoningEfforts(model string) []string {
	catalog, err := LoadOpenAIModelCatalog()
	if err != nil {
		return nil
	}
	return append([]string(nil), catalog.ReasoningEfforts[strings.TrimSpace(model)]...)
}

// NormalizeOpenAIReasoningEffort validates the account reasoning setting.
func NormalizeOpenAIReasoningEffort(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported reasoning_effort %q; expected none, minimal, low, medium, high, xhigh, or max", value)
	}
}

// NormalizeOpenAIServiceTier validates the service tier setting. "fast" is
// the user-facing alias for the Responses API's "priority" tier.
func NormalizeOpenAIServiceTier(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "fast" || value == "fast_mode" {
		value = "priority"
	}
	switch value {
	case "", "auto", "default", "flex", "scale", "priority":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported service_tier %q; expected auto, default, flex, scale, priority, or fast", value)
	}
}

func parseOpenAIAccountCredentials(data []byte) (OpenAIAccountCredentials, error) {
	var file openAIAuthFile
	if err := json.Unmarshal(data, &file); err != nil {
		return OpenAIAccountCredentials{}, err
	}
	credentials := file.Tokens
	if credentials.AccessToken == "" {
		credentials = OpenAIAccountCredentials{
			AccessToken:  file.AccessToken,
			RefreshToken: file.RefreshToken,
			IDToken:      file.IDToken,
			AccountID:    file.AccountID,
			ExpiresAt:    file.ExpiresAt,
		}
	}
	if credentials.AccessToken == "" {
		var raw struct {
			OpenAIAccounts []OpenAIAccountCredentials `json:"openai_accounts"`
			Active         string                     `json:"active_openai_account"`
		}
		if err := json.Unmarshal(data, &raw); err == nil && len(raw.OpenAIAccounts) > 0 {
			credentials = raw.OpenAIAccounts[0]
			for _, candidate := range raw.OpenAIAccounts {
				if raw.Active != "" && (candidate.AccountID == raw.Active || candidate.AccessToken == raw.Active) {
					credentials = candidate
					break
				}
			}
		}
	}
	if credentials.AccessToken == "" {
		return OpenAIAccountCredentials{}, errors.New("credentials file did not include access_token")
	}
	return normalizeOpenAIAccountCredentials(credentials), nil
}

func normalizeOpenAIAccountCredentials(credentials OpenAIAccountCredentials) OpenAIAccountCredentials {
	credentials.AccessToken = strings.TrimSpace(credentials.AccessToken)
	credentials.RefreshToken = strings.TrimSpace(credentials.RefreshToken)
	credentials.IDToken = strings.TrimSpace(credentials.IDToken)
	credentials.AccountID = strings.TrimSpace(credentials.AccountID)
	if credentials.AccountID == "" {
		credentials.AccountID = jwtClaim(credentials.IDToken, "https://api.openai.com/auth.chatgpt_account_id")
	}
	if credentials.AccountID == "" {
		credentials.AccountID = jwtClaim(credentials.AccessToken, "https://api.openai.com/auth.chatgpt_account_id")
	}
	if credentials.ExpiresAt == 0 {
		if exp := jwtClaimInt64(credentials.IDToken, "exp"); exp > 0 {
			credentials.ExpiresAt = exp * 1000
		}
	}
	if credentials.ExpiresAt == 0 {
		if exp := jwtClaimInt64(credentials.AccessToken, "exp"); exp > 0 {
			credentials.ExpiresAt = exp * 1000
		}
	}
	return credentials
}

func jwtClaim(token, name string) string {
	claims := jwtClaims(token)
	value, _ := claims[name].(string)
	return value
}

func jwtClaimInt64(token, name string) int64 {
	claims := jwtClaims(token)
	value, ok := claims[name]
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case float64:
		return int64(number)
	case json.Number:
		parsed, _ := number.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(number, 10, 64)
		return parsed
	default:
		return 0
	}
}

func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

func parseOpenAIModelCatalog(data []byte) (OpenAIModelCatalog, error) {
	var root any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return OpenAIModelCatalog{}, err
	}

	catalog := OpenAIModelCatalog{
		ReasoningEfforts: make(map[string][]string),
		ContextWindows:   make(map[string]int),
	}
	for _, model := range catalogModelObjects(root) {
		object, ok := model.(map[string]any)
		if !ok {
			continue
		}
		name := firstString(object, "slug", "id", "model")
		if name == "" {
			continue
		}
		catalog.Models = appendUniqueString(catalog.Models, name)
		if contextWindow := firstInt(object, "context_window", "context_length"); contextWindow > 0 {
			catalog.ContextWindows[name] = contextWindow
		}
		efforts := firstArray(object, "supported_reasoning_levels", "supported_reasoning_efforts")
		for _, effort := range efforts {
			value := ""
			switch item := effort.(type) {
			case string:
				value = item
			case map[string]any:
				value = firstString(item, "reasoning_effort", "effort", "value")
			}
			value, err := NormalizeOpenAIReasoningEffort(value)
			if err == nil && value != "" {
				catalog.ReasoningEfforts[name] = appendUniqueString(catalog.ReasoningEfforts[name], value)
			}
		}
	}
	if len(catalog.Models) == 0 {
		return OpenAIModelCatalog{}, errors.New("model response did not contain any model slugs")
	}
	return normalizeOpenAIModelCatalog(catalog), nil
}

func catalogModelObjects(root any) []any {
	switch value := root.(type) {
	case []any:
		return value
	case map[string]any:
		for _, key := range []string{"models", "data"} {
			if nested, ok := value[key]; ok {
				if objects := catalogModelObjects(nested); len(objects) > 0 {
					return objects
				}
			}
		}
	}
	return nil
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstInt(object map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := object[key].(type) {
		case json.Number:
			parsed, _ := strconv.Atoi(string(value))
			if parsed > 0 {
				return parsed
			}
		case float64:
			if value > 0 {
				return int(value)
			}
		case int:
			if value > 0 {
				return value
			}
		}
	}
	return 0
}

func firstArray(object map[string]any, keys ...string) []any {
	for _, key := range keys {
		if value, ok := object[key].([]any); ok {
			return value
		}
	}
	return nil
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func normalizeOpenAIModelCatalog(catalog OpenAIModelCatalog) OpenAIModelCatalog {
	models := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		models = appendUniqueString(models, model)
	}
	catalog.Models = models
	if catalog.ReasoningEfforts == nil {
		catalog.ReasoningEfforts = make(map[string][]string)
	}
	for model, efforts := range catalog.ReasoningEfforts {
		var normalized []string
		for _, effort := range efforts {
			if value, err := NormalizeOpenAIReasoningEffort(effort); err == nil && value != "" {
				normalized = appendUniqueString(normalized, value)
			}
		}
		catalog.ReasoningEfforts[model] = normalized
	}
	if catalog.ContextWindows == nil {
		catalog.ContextWindows = make(map[string]int)
	}
	return catalog
}

func randomString(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i, value := range bytes {
		bytes[i] = alphabet[int(value)%len(alphabet)]
	}
	return string(bytes), nil
}

func writePrivateJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(dir, ".openai-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func mustOpenAIAccountAuthPath() string {
	path, _ := OpenAIAccountAuthPath()
	return path
}

func defaultCodexAuthPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "auth.json")
}
