// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestModelAlias(t *testing.T) {
	id := ToolID{
		Server: "Server With Unicode \u670d\u52a1 and a very long suffix",
		Name:   "file/read:dangerous tool name with a very long suffix",
	}
	got := ModelAlias(id)
	if len(got) > 64 {
		t.Fatalf("alias length = %d, want <= 64: %q", len(got), got)
	}
	if !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(got) {
		t.Fatalf("alias contains provider-unsafe characters: %q", got)
	}
	if got != ModelAlias(id) {
		t.Fatal("alias is not deterministic")
	}
	other := ModelAlias(ToolID{Server: id.Server + "2", Name: id.Name})
	if got == other {
		t.Fatal("different tool identities produced the same alias")
	}
}

func TestDefinitionFingerprintExcludesConnectionSecrets(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	first := MCPServerConfig{
		Type: "remote", URL: "https://example.test/mcp?token=first",
		Headers: map[string]string{"Authorization": "Bearer first-secret"},
	}
	second := MCPServerConfig{
		Type: "remote", URL: "https://example.test/mcp?token=second",
		Headers: map[string]string{"Authorization": "Bearer second-secret"},
	}
	one, err := definitionFingerprint("server", first, "search", "description", schema)
	if err != nil {
		t.Fatal(err)
	}
	two, err := definitionFingerprint("server", second, "search", "description", schema)
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Errorf("secret-only connection changes altered fingerprint: %s != %s", one, two)
	}
	changed, err := definitionFingerprint("server", second, "search", "changed description", schema)
	if err != nil {
		t.Fatal(err)
	}
	if changed == one {
		t.Error("description change did not alter fingerprint")
	}
}

func TestDefinitionFingerprintIncludesConnectionPolicyIdentity(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	base := MCPServerConfig{Type: "remote", URL: "http://127.0.0.1/mcp?scope=one"}
	one, err := definitionFingerprint("server", base, "tool", "description", schema)
	if err != nil {
		t.Fatal(err)
	}

	insecure := base
	insecure.AllowInsecureHTTP = true
	two, err := definitionFingerprint("server", insecure, "tool", "description", schema)
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("allow_insecure_http policy change did not alter the fingerprint")
	}

	repeatedQuery := base
	repeatedQuery.URL = "http://127.0.0.1/mcp?scope=one&scope=two"
	three, err := definitionFingerprint("server", repeatedQuery, "tool", "description", schema)
	if err != nil {
		t.Fatal(err)
	}
	if one == three {
		t.Fatal("query parameter multiplicity change did not alter the connection identity")
	}
}

func TestCanonicalInputSchema(t *testing.T) {
	t.Run("adds object type and sanitizes strings", func(t *testing.T) {
		schema, err := canonicalInputSchema(map[string]any{
			"description": "before\x1b[31mred\x1b[0m\x00after secret-value",
			"properties":  map[string]any{},
		}, []string{"secret-value"}, "")
		if err != nil {
			t.Fatal(err)
		}
		text := string(schema)
		if !strings.Contains(text, `"type":"object"`) {
			t.Errorf("schema lacks object type: %s", text)
		}
		for _, forbidden := range []string{"\\u001b", "[31m", "secret-value"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("schema contains unsanitized value %q: %s", forbidden, text)
			}
		}
	})

	t.Run("rejects non-object", func(t *testing.T) {
		if _, err := canonicalInputSchema([]any{"x"}, nil, ""); err == nil {
			t.Fatal("expected non-object schema to fail")
		}
	})

	t.Run("rejects non-object type", func(t *testing.T) {
		if _, err := canonicalInputSchema(map[string]any{"type": "string"}, nil, ""); err == nil {
			t.Fatal("expected string schema type to fail")
		}
	})

	t.Run("rejects excessive depth", func(t *testing.T) {
		var value any = "leaf"
		for range maxSchemaDepth + 2 {
			value = map[string]any{"child": value}
		}
		if _, err := canonicalInputSchema(map[string]any{"type": "object", "nested": value}, nil, ""); err == nil {
			t.Fatal("expected excessive schema depth to fail")
		}
	})

	t.Run("rejects excessive size", func(t *testing.T) {
		value := strings.Repeat("x", maxSchemaBytes)
		if _, err := canonicalInputSchema(map[string]any{"type": "object", "description": value}, nil, ""); err == nil {
			t.Fatal("expected oversized schema to fail")
		}
	})
}

func TestSanitizeDescription(t *testing.T) {
	got, err := sanitizeDescription("safe\x1b[2J\x00 text")
	if err != nil {
		t.Fatal(err)
	}
	if got != "safe  text" {
		t.Errorf("description = %q, want sanitized text", got)
	}
	if _, err := sanitizeDescription(strings.Repeat("x", maxDescriptionBytes+1)); err == nil {
		t.Fatal("expected oversized description to fail")
	}
}
