// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"path"
	"strings"

	allowedext "github.com/alibaba/open-code-review/internal/config/allowlist"
)

// isCredentialPath reports whether a repository path names a credential file
// that the review gate keeps out of scope. The tools have to apply that gate
// themselves: the file list is filtered before dispatch, but the model still
// drives file_read and code_search, so an ungated read puts the same bytes in
// front of the provider that the gate refused to send.
func isCredentialPath(filePath string) bool {
	if filePath == "" {
		return false
	}
	// The model may spell a path with backslashes or a leading ./; normalize
	// before matching so the gate cannot be stepped around by notation.
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	normalized = path.Clean(normalized)
	normalized = strings.TrimPrefix(normalized, "/")
	return allowedext.IsSecretPath(normalized)
}
