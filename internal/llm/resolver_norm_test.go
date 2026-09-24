// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"testing"
)

// TestNormalizeAuthHeader covers every branch of NormalizeAuthHeader,
// including the empty pass-through, the two canonical forms, the "bearer"
// alias, and the unsupported-value error.
func TestNormalizeAuthHeader(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"  ", "", false},
		{"x-api-key", "x-api-key", false},
		{"X-API-KEY", "x-api-key", false},
		{"authorization", "authorization", false},
		{"Bearer", "authorization", false},
		{"  Authorization  ", "authorization", false},
		{"cookie", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeAuthHeader(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("NormalizeAuthHeader(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeAuthHeader(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestTryCCEnv covers tryCCEnv: the model-override branch, a successful resolve
// from the ANTHROPIC_* environment, and the incomplete-environment miss.
func TestTryCCEnv(t *testing.T) {
	t.Run("model override wins over env model", func(t *testing.T) {
		t.Setenv(envCCBaseURL, "https://cc.example")
		t.Setenv(envCCToken, "tok")
		t.Setenv(envCCModel, "env-model")

		ep, ok, err := tryCCEnv("override-model")
		if err != nil || !ok {
			t.Fatalf("tryCCEnv: ok=%v err=%v", ok, err)
		}
		if ep.Model != "override-model" {
			t.Errorf("model = %q, want override-model", ep.Model)
		}
		if ep.Protocol != ProtocolAnthropic || ep.AuthHeader != "authorization" {
			t.Errorf("unexpected protocol/auth: %q %q", ep.Protocol, ep.AuthHeader)
		}
	})

	t.Run("incomplete environment is a miss", func(t *testing.T) {
		t.Setenv(envCCBaseURL, "https://cc.example")
		t.Setenv(envCCToken, "")
		t.Setenv(envCCAPIKey, "")
		t.Setenv(envCCModel, "m")

		_, ok, err := tryCCEnv("")
		if err != nil || ok {
			t.Fatalf("tryCCEnv should miss on empty token: ok=%v err=%v", ok, err)
		}
	})

	t.Run("anthropic api key fallback with default baseURL", func(t *testing.T) {
		t.Setenv(envCCBaseURL, "")
		t.Setenv(envCCToken, "")
		t.Setenv(envCCAPIKey, "sk-ant-test-key")
		t.Setenv(envCCModel, "claude-test-model")

		ep, ok, err := tryCCEnv("")
		if err != nil || !ok {
			t.Fatalf("tryCCEnv: ok=%v err=%v", ok, err)
		}
		if ep.Token != "sk-ant-test-key" {
			t.Errorf("token = %q, want sk-ant-test-key", ep.Token)
		}
		if ep.AuthHeader != "x-api-key" {
			t.Errorf("authHeader = %q, want x-api-key", ep.AuthHeader)
		}
		if ep.URL != "https://api.anthropic.com/v1/messages" {
			t.Errorf("url = %q, want https://api.anthropic.com/v1/messages", ep.URL)
		}
	})

	t.Run("anthropic auth token takes precedence over api key", func(t *testing.T) {
		t.Setenv(envCCBaseURL, "https://custom.anthropic.com")
		t.Setenv(envCCToken, "token-wins")
		t.Setenv(envCCAPIKey, "key-loses")
		t.Setenv(envCCModel, "claude-test-model")

		ep, ok, err := tryCCEnv("")
		if err != nil || !ok {
			t.Fatalf("tryCCEnv: ok=%v err=%v", ok, err)
		}
		if ep.Token != "token-wins" {
			t.Errorf("token = %q, want token-wins", ep.Token)
		}
		if ep.AuthHeader != "authorization" {
			t.Errorf("authHeader = %q, want authorization", ep.AuthHeader)
		}
		if ep.URL != "https://custom.anthropic.com/v1/messages" {
			t.Errorf("url = %q, want https://custom.anthropic.com/v1/messages", ep.URL)
		}
	})
}
