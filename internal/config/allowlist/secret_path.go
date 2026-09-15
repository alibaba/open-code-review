// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package allowedext

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// Secret paths are kept apart from default_exclude_patterns.json on purpose:
// the default exclude list holds review noise that an include rule is allowed
// to bring back, while these paths must not enter the review scope at all, so
// no include rule can admit them. Matching follows the same glob and case rules
// as IsExcludedPath; see the package comment in allowed_ext.go for the syntax.

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
func IsSecretPath(path string) bool {
	secretOnce.Do(initSecret)
	lowerPath := strings.ToLower(path)
	for _, pattern := range secretPatterns {
		if matched, _ := doublestar.Match(pattern, lowerPath); matched {
			return true
		}
	}
	return false
}
