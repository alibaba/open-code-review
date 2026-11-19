// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func OpenRegularWorkspaceFile(repoDir, filePath string) (*os.File, error) {
	if !fs.ValidPath(filePath) || !filepath.IsLocal(filePath) || strings.ContainsAny(filePath, "\\:") || filePath == "." {
		return nil, fmt.Errorf("invalid workspace path")
	}
	return openRegularComponents(repoDir, strings.Split(filePath, "/"))
}
