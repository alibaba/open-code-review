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
		t.Setenv(envCCModel, "m")

		_, ok, err := tryCCEnv("")
		if err != nil || ok {
			t.Fatalf("tryCCEnv should miss on empty token: ok=%v err=%v", ok, err)
		}
	})
}
