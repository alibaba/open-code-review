// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workspace

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// NativePathFromFileURL converts a local file: URL path into an OS path.
// Callers must already have rejected non-file schemes, hosts, queries and fragments.
func NativePathFromFileURL(u *url.URL) string {
	path := u.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

// FileURLString formats a local path and optional line as a file URL.
func FileURLString(path string, line *int) string {
	slash := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && len(slash) >= 2 && slash[1] == ':' && !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	if line != nil {
		u.Fragment = fmt.Sprintf("L%d", *line)
	}
	return u.String()
}
