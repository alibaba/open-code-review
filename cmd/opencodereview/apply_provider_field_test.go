// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"
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
			{"model", "gpt-4", func(e ProviderEntry) bool { return e.Model == "gpt-4" }},
			{"models", "a,b,a", func(e ProviderEntry) bool { return len(e.Models) == 2 }},
			{"extra_body", `{"k":1}`, func(e ProviderEntry) bool { return e.ExtraBody["k"] != nil }},
		}
		for _, c := range cases {
			if err := applyProviderField(&e, c.field, "providers.p."+c.field, c.value); err != nil {
				t.Fatalf("field %q: %v", c.field, err)
			}
			if !c.check(e) {
				t.Errorf("field %q not applied: %+v", c.field, e)
			}
		}
	})

	t.Run("protocol validated and normalized", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField(&e, "protocol", "providers.p.protocol", "openai"); err != nil {
			t.Fatalf("valid protocol: %v", err)
		}
		if e.Protocol == "" {
			t.Error("protocol not set")
		}
		if err := applyProviderField(&e, "protocol", "providers.p.protocol", "not-a-protocol"); err == nil {
			t.Error("expected error for invalid protocol")
		}
	})

	t.Run("auth_header normalized", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField(&e, "auth_header", "providers.p.auth_header", "x-api-key"); err != nil {
			t.Fatalf("valid auth header: %v", err)
		}
		if e.AuthHeader == "" {
			t.Error("auth header not set")
		}
	})

	t.Run("auth_header rejects unsupported value", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField(&e, "auth_header", "providers.p.auth_header", "cookie"); err == nil {
			t.Error("expected error for unsupported auth header")
		}
	})

	t.Run("extra_body rejects invalid JSON", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField(&e, "extra_body", "providers.p.extra_body", "{bad"); err == nil {
			t.Error("expected JSON error")
		}
	})

	t.Run("extra_headers parsed", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField(&e, "extra_headers", "providers.p.extra_headers", "X-A=1"); err != nil {
			t.Fatalf("valid extra headers: %v", err)
		}
		if len(e.ExtraHeaders) == 0 {
			t.Error("extra headers not set")
		}
	})

	t.Run("unknown field returns error", func(t *testing.T) {
		var e ProviderEntry
		if err := applyProviderField(&e, "bogus", "providers.p.bogus", "x"); err == nil {
			t.Error("expected error for unknown field")
		}
	})
}
