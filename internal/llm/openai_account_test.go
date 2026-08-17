// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenAIAuthorizationURL(t *testing.T) {
	redirectURI := "http://localhost:1455/auth/callback"
	authURL := OpenAIAuthorizationURL(redirectURI, "challenge", "state")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"client_id":                 OpenAIOAuthClientID,
		"redirect_uri":              redirectURI,
		"code_challenge":            "challenge",
		"code_challenge_method":     "S256",
		"state":                     "state",
		"originator":                OpenAIAccountOriginator,
		"codex_cli_simplified_flow": "true",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
}

func TestParseOpenAIModelCatalog(t *testing.T) {
	data := []byte(`{
		"data": {
			"models": [
				{"slug":"gpt-account","context_window":131072,"supported_reasoning_levels":["low",{"effort":"high"}]},
				{"id":"gpt-basic","context_length":8192,"supported_reasoning_efforts":[{"value":"medium"}]}
			]
		}
	}`)
	catalog, err := parseOpenAIModelCatalog(data)
	if err != nil {
		t.Fatalf("parseOpenAIModelCatalog: %v", err)
	}
	if want := []string{"gpt-account", "gpt-basic"}; !reflect.DeepEqual(catalog.Models, want) {
		t.Errorf("Models = %v, want %v", catalog.Models, want)
	}
	if got := catalog.ContextWindows["gpt-account"]; got != 131072 {
		t.Errorf("context window = %d, want 131072", got)
	}
	if want := []string{"low", "high"}; !reflect.DeepEqual(catalog.ReasoningEfforts["gpt-account"], want) {
		t.Errorf("reasoning efforts = %v, want %v", catalog.ReasoningEfforts["gpt-account"], want)
	}
}

func TestParseOpenAIAccountCredentials_CodexShape(t *testing.T) {
	data := []byte(`{"tokens":{"access_token":"access","refresh_token":"refresh","id_token":"id","account_id":"account","expires_at":123}}`)
	credentials, err := parseOpenAIAccountCredentials(data)
	if err != nil {
		t.Fatalf("parseOpenAIAccountCredentials: %v", err)
	}
	if credentials.AccessToken != "access" || credentials.RefreshToken != "refresh" || credentials.AccountID != "account" || credentials.ExpiresAt != 123 {
		t.Fatalf("unexpected credentials: %+v", credentials)
	}
}

func TestNormalizeOpenAIAccountCredentialsFromJWT(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":123,"https://api.openai.com/auth.chatgpt_account_id":"account"}`))
	credentials := normalizeOpenAIAccountCredentials(OpenAIAccountCredentials{AccessToken: "header." + payload + ".signature"})
	if credentials.AccountID != "account" {
		t.Errorf("AccountID = %q, want account", credentials.AccountID)
	}
	if credentials.ExpiresAt != 123000 {
		t.Errorf("ExpiresAt = %d, want 123000", credentials.ExpiresAt)
	}
}

func TestOpenAIAccountSettingsNormalization(t *testing.T) {
	if got, err := NormalizeOpenAIReasoningEffort(" HIGH "); err != nil || got != "high" {
		t.Errorf("reasoning normalization = %q, %v", got, err)
	}
	if got, err := NormalizeOpenAIServiceTier("fast"); err != nil || got != "priority" {
		t.Errorf("service tier normalization = %q, %v", got, err)
	}
	if _, err := NormalizeOpenAIReasoningEffort("turbo"); err == nil {
		t.Error("invalid reasoning effort was accepted")
	}
}

func TestSaveAndLoadOpenAIAccountCredentials(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "openai-auth.json")
	cachePath := filepath.Join(t.TempDir(), "openai-models.json")
	t.Setenv(openAIAuthFileEnv, authPath)
	t.Setenv(openAIModelCacheFileEnv, cachePath)

	credentials := OpenAIAccountCredentials{AccessToken: "access", RefreshToken: "refresh", AccountID: "account"}
	if err := SaveOpenAIAccountCredentials(credentials); err != nil {
		t.Fatalf("SaveOpenAIAccountCredentials: %v", err)
	}
	loaded, err := LoadOpenAIAccountCredentials()
	if err != nil {
		t.Fatalf("LoadOpenAIAccountCredentials: %v", err)
	}
	if loaded.AccessToken != credentials.AccessToken || loaded.AccountID != credentials.AccountID {
		t.Fatalf("loaded credentials = %+v", loaded)
	}
	mode := os.FileMode(0)
	if info, statErr := os.Stat(authPath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if mode != 0o600 {
		t.Errorf("auth file mode = %o, want 600", mode)
	}

	catalog := OpenAIModelCatalog{Models: []string{"gpt-account"}, ReasoningEfforts: map[string][]string{"gpt-account": {"high"}}}
	if err := SaveOpenAIModelCatalog(catalog); err != nil {
		t.Fatalf("SaveOpenAIModelCatalog: %v", err)
	}
	loadedCatalog, err := LoadOpenAIModelCatalog()
	if err != nil {
		t.Fatalf("LoadOpenAIModelCatalog: %v", err)
	}
	if !reflect.DeepEqual(loadedCatalog.Models, catalog.Models) {
		t.Errorf("loaded catalog = %+v", loadedCatalog)
	}

	data, _ := os.ReadFile(authPath)
	var shape map[string]any
	if err := json.Unmarshal(data, &shape); err != nil || shape["tokens"] == nil {
		t.Errorf("saved auth shape = %s", data)
	}
}
