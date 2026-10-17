// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build !windows

package plugin

import "github.com/alibaba/open-code-review/internal/pathutil"

func canonicalProjectPath(name string) (string, error) {
	return pathutil.CanonicalPath(name)
}
