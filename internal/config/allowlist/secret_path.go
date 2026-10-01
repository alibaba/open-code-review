// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package allowedext

import (
	_ "embed"
	"encoding/json"
	"path"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// Secret paths are kept apart from default_exclude_patterns.json on purpose:
// the default exclude list holds review noise that an include rule is allowed
// to bring back, while these paths must not enter the review scope at all, so
// no include rule can admit them. Unconditional secret paths use the same glob
// and case rules as IsExcludedPath; .env-family paths are handled directly here
// because they have explicit template exceptions.

//go:embed default_secret_patterns.json
var secretData []byte

var (
	secretPatterns []string // raw patterns from JSON, lowercased by initSecret
	secretOnce     sync.Once
)

func initSecret() {
	if err := json.Unmarshal(secretData, &secretPatterns); err != nil {
		panic("allowedext: failed to parse default_secret_patterns.json: " + err.Error())
	}
	for i, p := range secretPatterns {
		secretPatterns[i] = strings.ToLower(p)
	}
}

// IsSecretPath returns true when the given file path matches any built-in
// secret pattern — credential files such as .env, id_rsa or .netrc that should
// never be sent to a model as part of a review. The check is case-insensitive
// and purely path-based; file contents are never inspected.
//
// A path that is not a secret is not thereby reviewable: it still has to pass
// the extension allowlist and the default exclude patterns.
func IsSecretPath(filePath string) bool {
	secretOnce.Do(initSecret)
	lowerPath := strings.ToLower(filePath)

	if isSecretEnvPath(lowerPath) {
		return true
	}

	for _, pattern := range secretPatterns {
		if matched, _ := doublestar.Match(pattern, lowerPath); matched {
			return true
		}
	}
	return false
}

func isSecretEnvPath(lowerPath string) bool {
	base := path.Base(lowerPath)

	switch base {
	case ".env.example", ".env.sample", ".env.template":
		return false
	}

	return base == ".env" || strings.HasPrefix(base, ".env.")
}
