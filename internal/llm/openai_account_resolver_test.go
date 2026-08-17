// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEndpoint_OpenAIAccountProvider(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "openai-auth.json")
	cachePath := filepath.Join(t.TempDir(), "openai-models.json")
	t.Setenv(openAIAuthFileEnv, authPath)
	t.Setenv(openAIModelCacheFileEnv, cachePath)
	if err := SaveOpenAIAccountCredentials(OpenAIAccountCredentials{AccessToken: "account-token", AccountID: "account-id"}); err != nil {
		t.Fatalf("save credentials: %v", err)
	}

	cfg := configFile{
		Provider: OpenAIAccountProviderName,
		Model:    "gpt-5.4",
		Providers: map[string]providerEntryConfig{
			OpenAIAccountProviderName: {
				Model:           "gpt-5.4",
				ReasoningEffort: "high",
				ServiceTier:     "fast",
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ep, err := ResolveEndpoint(configPath)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Provider != OpenAIAccountProviderName || ep.Protocol != ProtocolOpenAIResponses {
		t.Errorf("provider/protocol = %q/%q", ep.Provider, ep.Protocol)
	}
	if ep.URL != OpenAIAccountResponsesURL || ep.Token != "account-token" || ep.Model != "gpt-5.4" {
		t.Errorf("endpoint = %+v", ep)
	}
	if ep.OpenAIAccount == nil || ep.OpenAIAccount.AccountID != "account-id" {
		t.Errorf("account credentials = %+v", ep.OpenAIAccount)
	}
	if ep.ReasoningEffort != "high" || ep.ServiceTier != "priority" {
		t.Errorf("account options = %q/%q", ep.ReasoningEffort, ep.ServiceTier)
	}
}
