// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/codexauth"
)

type resolverCodexStore struct {
	auth  *codexauth.CodexAuth
	loads int
	saves int
}

func (s *resolverCodexStore) Load() (*codexauth.CodexAuth, error) {
	s.loads++
	copy := *s.auth
	return &copy, nil
}

func (s *resolverCodexStore) Save(auth *codexauth.CodexAuth) error {
	copy := *auth
	s.auth = &copy
	s.saves++
	return nil
}

func (s *resolverCodexStore) Clear() error { return nil }

type resolverRoundTripFunc func(*http.Request) (*http.Response, error)

func (f resolverRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setCodexResolverAuthSeams(t *testing.T, store codexauth.CodexStore, client *codexauth.OAuthClient) {
	t.Helper()
	originalStore := codexAuthStore
	originalNewClient := newCodexOAuthClient
	codexAuthStore = store
	newCodexOAuthClient = func() *codexauth.OAuthClient { return client }
	t.Cleanup(func() {
		codexAuthStore = originalStore
		newCodexOAuthClient = originalNewClient
	})
}

func codexResolverConfig(t *testing.T) string {
	t.Helper()
	path, _ := writeResolverConfig(t, configFile{
		Provider: "codex",
		Providers: map[string]providerEntryConfig{
			"codex": {Model: "gpt-5.6-luna"},
		},
	})
	return path
}

func TestResolveEndpoint_CodexUsesStoredCredential(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	wantToken := "access-token-fixture"
	if err := codexauth.Save(&codexauth.CodexAuth{
		AccessToken:  wantToken,
		RefreshToken: "refresh-token-fixture",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save Codex auth: %v", err)
	}

	ep, err := ResolveEndpoint(codexResolverConfig(t))
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != wantToken {
		t.Errorf("Token = %q, want stored access token", ep.Token)
	}
	if !ep.RequiresStreaming {
		t.Error("RequiresStreaming = false, want true")
	}
	if !ep.RejectsSamplingParams {
		t.Error("RejectsSamplingParams = false, want true")
	}
	if !ep.DetailErrorEnvelope {
		t.Error("DetailErrorEnvelope = false, want true")
	}
	if ep.Protocol != ProtocolOpenAIResponses {
		t.Errorf("Protocol = %q, want %q", ep.Protocol, ProtocolOpenAIResponses)
	}
}

func TestResolveEndpoint_CodexMissingCredentialIsActionable(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())

	_, err := ResolveEndpoint(codexResolverConfig(t))
	if err == nil {
		t.Fatal("ResolveEndpoint returned nil error")
	}
	for _, want := range []string{"codex provider is selected but not signed in", "ocr auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestResolveEndpoint_CodexRefreshesStoredCredential(t *testing.T) {
	clearAllEnv(t)
	store := &resolverCodexStore{auth: &codexauth.CodexAuth{
		AccessToken: "expired-access", RefreshToken: "refresh-token", ExpiresAt: time.Now().Add(-time.Minute),
	}}
	transport := resolverRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Error("refresh request context has no deadline")
		} else if remaining := time.Until(deadline); remaining <= 0 || remaining > codexRefreshTimeout {
			t.Errorf("refresh request deadline remaining = %v", remaining)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":3600}`)),
			Request: req,
		}, nil
	})
	client := &codexauth.OAuthClient{
		Issuer: "https://auth.example", HTTPClient: &http.Client{Transport: transport},
	}
	setCodexResolverAuthSeams(t, store, client)

	ep, err := ResolveEndpoint(codexResolverConfig(t))
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != "refreshed-access" {
		t.Errorf("Token = %q, want refreshed-access", ep.Token)
	}
	if store.saves != 1 {
		t.Errorf("Save calls = %d, want 1", store.saves)
	}
}

func TestResolveEndpoint_CodexRefreshFailureIsActionable(t *testing.T) {
	clearAllEnv(t)
	store := &resolverCodexStore{auth: &codexauth.CodexAuth{
		AccessToken: "expired-access", RefreshToken: "refresh-token", ExpiresAt: time.Now().Add(-time.Minute),
	}}
	transport := resolverRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"unavailable"}`)),
			Request:    req,
		}, nil
	})
	client := &codexauth.OAuthClient{
		Issuer: "https://auth.example", HTTPClient: &http.Client{Transport: transport},
	}
	setCodexResolverAuthSeams(t, store, client)

	_, err := ResolveEndpoint(codexResolverConfig(t))
	if err == nil || !strings.Contains(err.Error(), "ocr auth login") {
		t.Fatalf("ResolveEndpoint error = %v, want ocr auth login guidance", err)
	}
}

func TestResolveEndpoint_CodexStaticKeyTakesPrecedence(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	path, _ := writeResolverConfig(t, configFile{
		Provider: "codex",
		Providers: map[string]providerEntryConfig{
			"codex": {APIKey: "configured-key", Model: "gpt-5.6-terra"},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.Token != "configured-key" {
		t.Errorf("Token = %q, want configured-key", ep.Token)
	}
}

func TestResolveEndpoint_CodexOverrideRequiresExplicitAPIKey(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry providerEntryConfig
	}{
		{name: "url", entry: providerEntryConfig{URL: "https://gateway.example/v1", Model: "gpt-5.6-luna"}},
		{name: "protocol", entry: providerEntryConfig{Protocol: ProtocolOpenAIChatCompletions, Model: "gpt-5.6-luna"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAllEnv(t)
			store := &resolverCodexStore{}
			setCodexResolverAuthSeams(t, store, nil)
			path, _ := writeResolverConfig(t, configFile{
				Provider:  "codex",
				Providers: map[string]providerEntryConfig{"codex": tc.entry},
			})

			_, err := ResolveEndpoint(path)
			if err == nil || !strings.Contains(err.Error(), "set api_key explicitly") {
				t.Fatalf("ResolveEndpoint error = %v, want explicit api_key guidance", err)
			}
			if store.loads != 0 {
				t.Errorf("credential store Load calls = %d, want 0", store.loads)
			}
		})
	}
}

func TestResolveEndpoint_CodexProtocolOverrideDisablesCodexBehaviors(t *testing.T) {
	clearAllEnv(t)
	path, _ := writeResolverConfig(t, configFile{
		Provider: "codex",
		Providers: map[string]providerEntryConfig{
			"codex": {
				APIKey: "explicit-key", Protocol: ProtocolOpenAIChatCompletions, Model: "gpt-5.6-luna",
			},
		},
	})

	ep, err := ResolveEndpoint(path)
	if err != nil {
		t.Fatalf("ResolveEndpoint: %v", err)
	}
	if ep.RequiresStreaming || ep.RejectsSamplingParams || ep.DetailErrorEnvelope {
		t.Errorf("Codex behaviors remained enabled after protocol override: %+v", ep)
	}
}

func TestResolveEndpoint_CodexModelOverrideRemainsAllowlisted(t *testing.T) {
	clearAllEnv(t)
	setTestHome(t, t.TempDir())
	if err := codexauth.Save(&codexauth.CodexAuth{
		AccessToken:  "access-token-fixture",
		RefreshToken: "refresh-token-fixture",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save Codex auth: %v", err)
	}

	_, err := ResolveEndpointWithModelOverride(codexResolverConfig(t), "gpt-5.6-sol")
	if err == nil || !strings.Contains(err.Error(), "not available for provider \"codex\"") {
		t.Errorf("ResolveEndpointWithModelOverride error = %v", err)
	}
}

func TestNewLLMClient_CarriesProviderBehaviors(t *testing.T) {
	client, ok := NewLLMClient(ResolvedEndpoint{
		URL:                   "https://example.com/v1",
		Token:                 "token",
		Model:                 "model",
		Protocol:              ProtocolOpenAIResponses,
		RequiresStreaming:     true,
		RejectsSamplingParams: true,
		DetailErrorEnvelope:   true,
	}, nil).(*OpenAIResponsesClient)
	if !ok {
		t.Fatal("NewLLMClient did not return OpenAIResponsesClient")
	}
	if !client.cfg.RequiresStreaming || !client.cfg.RejectsSamplingParams || !client.cfg.DetailErrorEnvelope {
		t.Errorf("client behaviors = %+v, want all enabled", client.cfg)
	}
}
