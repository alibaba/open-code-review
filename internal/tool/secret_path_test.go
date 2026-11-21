// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import "testing"

func TestIsCredentialPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".env", true},
		{"./.env", true},
		{".env.local", true},
		{"config/.env.production", true},
		{"id_rsa", true},
		{"deploy/id_rsa", true},
		{".ssh/id_ed25519", true},
		{".npmrc", true},
		{".netrc", true},
		{".env.example", false},
		{".env.template", false},
		{"pkg/util.go", false},
		{"README.md", false},
		{"", false},
		{"./pkg/util.go", false},
		{`config\.env`, true},
		{`pkg\util.go`, false},
	}
	for _, tt := range tests {
		if got := isCredentialPath(tt.path); got != tt.want {
			t.Errorf("isCredentialPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
