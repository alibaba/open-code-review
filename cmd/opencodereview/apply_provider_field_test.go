// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// TestApplyProviderField exercises every field branch of applyProviderField,
// including the JSON/parse error paths and the unknown-field default.
func TestApplyProviderField(t *testing.T) {
	t.Run("success branches set the entry", func(t *testing.T) {
		var e ProviderEntry
		cases := []struct {
			field, value string
			check        func(ProviderEntry) bool
		}{
			{"api_key", "sk-x", func(e ProviderEntry) bool { return e.APIKey == "sk-x" }},
			{"url", "https://x.example", func(e ProviderEntry) bool { return e.URL == "https://x.example" }},
			{"auth_mode", "env", func(e ProviderEntry) bool { return e.AuthMode == "env" }},
			{"model", "gpt-4", func(e ProviderEntry) bool { return e.Model == "gpt-4" }},
			{"models", "a,b,a", func(e ProviderEntry) bool { return len(e.Models) == 2 }},
			{"identity_token_file", " /tmp/token.jwt ", func(e ProviderEntry) bool { return e.IdentityTokenFile == "/tmp/token.jwt" }},
			{"token_exchange_url", "https://auth.example/token", func(e ProviderEntry) bool { return e.TokenExchangeURL == "https://auth.example/token" }},
			{"extra_body", `{"k":1}`, func(e ProviderEntry) bool { return e.ExtraBody["k"] != nil }},
		}
		for _, c := range cases {
			if err := applyProviderField("p", &e, c.field, "providers.p."+c.field, c.value); err != nil {
				t.Fatalf("field %q: %v", c.field, err)
			}
			if !c.check(e) {
				t.Errorf("field %q not applied: %+v", c.field, e)
			}
		}
	})

	t.Run("protocol validated and normalized", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "protocol", "providers.p.protocol", "openai"); err != nil {
			t.Fatalf("valid protocol: %v", err)
		}
		if e.Protocol == "" {
			t.Error("protocol not set")
		}
		if err := applyProviderField("p", &e, "protocol", "providers.p.protocol", "not-a-protocol"); err == nil {
			t.Error("expected error for invalid protocol")
		}
	})

	t.Run("protocol clears stale ambient auth mode", func(t *testing.T) {
		e := ProviderEntry{
			Protocol:   "anthropic-bedrock",
			AuthMode:   string(llm.AuthModeAmbient),
			AWSRegion:  "us-west-2",
			AWSProfile: "dev",
		}
		if err := applyProviderField("p", &e, "protocol", "providers.p.protocol", "openai"); err != nil {
			t.Fatalf("set protocol: %v", err)
		}
		if e.AuthMode != "" {
			t.Errorf("AuthMode = %q, want cleared", e.AuthMode)
		}
		if e.AWSRegion != "" || e.AWSProfile != "" {
			t.Errorf("AWS settings = %q/%q, want cleared", e.AWSRegion, e.AWSProfile)
		}
	})

	t.Run("auth_header normalized", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "auth_header", "providers.p.auth_header", "x-api-key"); err != nil {
			t.Fatalf("valid auth header: %v", err)
		}
		if e.AuthHeader == "" {
			t.Error("auth header not set")
		}
	})

	t.Run("auth_header rejects unsupported value", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "auth_header", "providers.p.auth_header", "cookie"); err == nil {
			t.Error("expected error for unsupported auth header")
		}
	})

	t.Run("auth_mode rejects unsupported value", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "auth_mode", "providers.p.auth_mode", "cookie"); err == nil {
			t.Error("expected error for unsupported auth mode")
		}
	})

	t.Run("ambient auth_mode requires ambient protocol", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "auth_mode", "providers.p.auth_mode", "ambient"); err == nil {
			t.Error("expected error for ambient auth mode on non-ambient provider")
		}
		if err := applyProviderField("p", &e, "protocol", "providers.p.protocol", "anthropic-bedrock"); err != nil {
			t.Fatalf("set bedrock protocol: %v", err)
		}
		if err := applyProviderField("p", &e, "auth_mode", "providers.p.auth_mode", "ambient"); err != nil {
			t.Fatalf("ambient auth mode should work for bedrock protocol: %v", err)
		}
	})

	t.Run("extra_body rejects invalid JSON", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "extra_body", "providers.p.extra_body", "{bad"); err == nil {
			t.Error("expected JSON error")
		}
	})

	t.Run("extra_headers parsed", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "extra_headers", "providers.p.extra_headers", "X-A=1"); err != nil {
			t.Fatalf("valid extra headers: %v", err)
		}
		if len(e.ExtraHeaders) == 0 {
			t.Error("extra headers not set")
		}
	})

	t.Run("unknown field returns error", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField("p", &e, "bogus", "providers.p.bogus", "x"); err == nil {
			t.Error("expected error for unknown field")
		}
	})
}
