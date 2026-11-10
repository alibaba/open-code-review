// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package allowedext

import (
	"testing"
)

func TestIsSecretPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		// .env and its per-environment variants
		{"env at root", ".env", true},
		{"env nested", "foo/.env", true},
		{"env deeply nested", "a/b/c/.env", true},
		{"env local at root", ".env.local", true},
		{"env local nested", "foo/.env.local", true},
		{"env scoped local at root", ".env.production.local", true},
		{"env scoped local nested", "foo/.env.production.local", true},
		{"env production", ".env.production", true},
		{"env staging", ".env.staging", true},
		{"env development", ".env.development", true},
		{"env production nested", "foo/.env.production", true},

		// SSH material: the directory pattern carries everything inside it,
		// including files the key-name patterns do not list (config, known_hosts).
		{"ssh dir at root", ".ssh/id_ed25519", true},
		{"ssh dir nested", "nested/.ssh/id_ed25519", true},
		{"ssh config", ".ssh/config", true},
		{"ssh known_hosts", "home/.ssh/known_hosts", true},
		{"ssh nested key dir", "a/b/c/.ssh/keys/id_rsa", true},

		// Private keys outside a .ssh directory
		{"id_rsa at root", "id_rsa", true},
		{"id_rsa nested", "foo/id_rsa", true},
		{"id_dsa", "id_dsa", true},
		{"id_ecdsa", "keys/id_ecdsa", true},
		{"id_ed25519", "id_ed25519", true},

		// Credential files for network and package tooling
		{"netrc", ".netrc", true},
		{"netrc nested", "home/.netrc", true},
		{"windows netrc", "_netrc", true},
		{"npmrc", ".npmrc", true},
		{"npmrc nested", "foo/.npmrc", true},
		{"pypirc", ".pypirc", true},
		{"dockercfg", ".dockercfg", true},

		// Templates are not credentials. They stay out of the secret list so an
		// explicit include can still bring them into review.
		{"env example", ".env.example", false},
		{"env example nested", "foo/.env.example", false},
		{"env sample", ".env.sample", false},
		{"env template", ".env.template", false},

		// Template exceptions must not bypass other secret-path rules.
		{"env example in ssh dir", ".ssh/.env.example", true},

		// Public and derived files that only look like key material
		{"public key", "id_rsa.pub", false},
		{"key backup", "foo/id_rsa.backup", false},

		// "credentials" is deliberately absent from the pattern list: it is a
		// common ordinary identifier, not a conventional credential filename.
		{"bare credentials file", "credentials", false},
		{"credentials package dir", "src/credentials/package.go", false},
		{"credentials in filename", "internal/credentials_loader.go", false},

		// Legitimate extensionless files must keep their existing behavior.
		{"dockerfile", "Dockerfile", false},
		{"makefile", "Makefile", false},
		{"nested dockerfile", "build/Dockerfile", false},

		// Files that merely carry the .env extension are outside this list;
		// the extension allowlist governs them, unchanged by #1240.
		{"env extension file", "prod.env", false},
		{"env extension nested", "config/staging.env", false},

		// Case-insensitive, matching IsExcludedPath.
		{"uppercase env", ".ENV", true},
		{"uppercase env production", ".ENV.PRODUCTION", true},
		{"uppercase env example", ".ENV.EXAMPLE", false},
		{"uppercase key", "ID_RSA", true},
		{"mixed case ssh dir", "Foo/.SSH/id_ed25519", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSecretPath(tt.path); got != tt.want {
				t.Errorf("IsSecretPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
