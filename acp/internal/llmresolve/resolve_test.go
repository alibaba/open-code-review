// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmresolve

import (
	"strings"
	"testing"
)

func envFunc(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolveDefaultsForEachProvider(t *testing.T) {
	cases := []struct {
		provider string
		protocol string
		url      string
		auth     string
	}{
		{ProtocolAnthropic, ProtocolAnthropic, "https://api.anthropic.com", "x-api-key"},
		{ProtocolOpenAI, ProtocolOpenAI, "https://api.openai.com/v1", ""},
	}
	for _, c := range cases {
		t.Run(c.provider, func(t *testing.T) {
			ep, err := Resolve(Options{Provider: c.provider, Model: "m", APIKey: "k"})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if ep.Protocol != c.protocol {
				t.Errorf("protocol = %q, want %q", ep.Protocol, c.protocol)
			}
			if ep.Provider != c.provider {
				t.Errorf("provider = %q, want %q", ep.Provider, c.provider)
			}
			if ep.URL != c.url {
				t.Errorf("url = %q, want %q", ep.URL, c.url)
			}
			if ep.AuthHeader != c.auth {
				t.Errorf("auth header = %q, want %q", ep.AuthHeader, c.auth)
			}
			if ep.Model != "m" || ep.Token != "k" {
				t.Errorf("model/token = %q/%q, want m/k", ep.Model, ep.Token)
			}
			if ep.Source != "startup flags" {
				t.Errorf("source = %q, want startup flags", ep.Source)
			}
		})
	}
}

func TestResolveFromEnvironment(t *testing.T) {
	ep, err := Resolve(Options{Getenv: envFunc(map[string]string{
		EnvProvider: "anthropic",
		EnvModel:    "claude-3-5-sonnet",
		EnvBaseURL:  "https://proxy.example/v1",
		EnvAPIKey:   "sekret",
	})})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ep.Source != "environment" {
		t.Errorf("source = %q, want environment", ep.Source)
	}
	if ep.URL != "https://proxy.example/v1" || ep.Model != "claude-3-5-sonnet" || ep.Token != "sekret" {
		t.Errorf("unexpected endpoint: %+v", ep)
	}
}

func TestResolveFlagsWinOverEnvironment(t *testing.T) {
	ep, err := Resolve(Options{
		Provider: "openai",
		Model:    "flag-model",
		Getenv: envFunc(map[string]string{
			EnvProvider: "anthropic",
			EnvModel:    "env-model",
			EnvAPIKey:   "env-key",
		}),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ep.Protocol != ProtocolOpenAI || ep.Model != "flag-model" {
		t.Errorf("flags did not win: %+v", ep)
	}
	if ep.Token != "env-key" {
		t.Errorf("token = %q, want env-key", ep.Token)
	}
}

func TestResolveProviderCaseInsensitive(t *testing.T) {
	ep, err := Resolve(Options{Provider: "  OpenAI  ", Model: "m", APIKey: "k"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ep.Protocol != ProtocolOpenAI {
		t.Errorf("protocol = %q, want openai", ep.Protocol)
	}
}

func TestResolveErrors(t *testing.T) {
	cases := []struct {
		name   string
		opts   Options
		getenv map[string]string
		want   string
	}{
		{"no provider", Options{Model: "m", APIKey: "k"}, nil, "no parsing provider"},
		{"unsupported provider", Options{Provider: "gemini", Model: "m", APIKey: "k"}, nil, "only"},
		{"responses unsupported", Options{Provider: "openai-responses", Model: "m", APIKey: "k"}, nil, "does not support protocol"},
		{"bedrock unsupported", Options{Provider: "anthropic-bedrock", Model: "m", APIKey: "k"}, nil, "does not support protocol"},
		{"no model", Options{Provider: "openai", APIKey: "k"}, nil, "no parsing model"},
		{"no key", Options{Provider: "openai", Model: "m"}, nil, "no parsing API key"},
		{"bad scheme", Options{Provider: "openai", Model: "m", BaseURL: "ftp://example.com", APIKey: "k"}, nil, "http://"},
		{"no host", Options{Provider: "openai", Model: "m", BaseURL: "https://", APIKey: "k"}, nil, "host"},
		{"provider from env", Options{Getenv: envFunc(map[string]string{EnvProvider: "anthropic", EnvModel: "m"})}, nil, "no parsing API key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.opts.Getenv == nil {
				c.opts.Getenv = envFunc(c.getenv)
			}
			_, err := Resolve(c.opts)
			if err == nil {
				t.Fatalf("Resolve succeeded, want error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %q, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestSummaryRedactsToken(t *testing.T) {
	ep, err := Resolve(Options{Provider: "anthropic", Model: "claude-3", APIKey: "super-secret-token"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	summary := ep.Summary()
	if strings.Contains(summary, "super-secret-token") {
		t.Fatalf("Summary leaked the token: %s", summary)
	}
	for _, want := range []string{"anthropic", "claude-3", "api.anthropic.com"} {
		if !strings.Contains(summary, want) {
			t.Errorf("Summary %q does not mention %q", summary, want)
		}
	}
}

func TestRedactURLDropsUserInfoAndQuery(t *testing.T) {
	got := redactURL("https://user:pass@example.com/v1?api_key=leak#frag")
	if strings.Contains(got, "user") || strings.Contains(got, "pass") || strings.Contains(got, "api_key") || strings.Contains(got, "frag") {
		t.Errorf("redactURL left secrets in %q", got)
	}
	if got != "https://example.com/v1" {
		t.Errorf("redactURL = %q, want https://example.com/v1", got)
	}
	if redactURL("://not a url") != "://not a url" {
		t.Errorf("redactURL should return the raw value when parsing fails")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty = %q, want b", got)
	}
	if got := firstNonEmpty("  ", ""); got != "" {
		t.Errorf("firstNonEmpty = %q, want empty", got)
	}
}
