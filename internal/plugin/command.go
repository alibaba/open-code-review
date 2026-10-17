// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"runtime"
	"strings"
)

// ExpandCommandValue replaces package/platform placeholders without expanding
// shell syntax, environment variables, or placeholders inside substituted paths.
func ExpandCommandValue(value, directory string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return strings.NewReplacer("${plugin_dir}", directory, "${os}", runtime.GOOS, "${arch}", runtime.GOARCH, "${exe}", ext).Replace(value)
}
