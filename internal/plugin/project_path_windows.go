// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func canonicalProjectPath(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	// Resolve junctions as well as symbolic links so aliases share one project
	// identity and local sources are compared against the same physical root.
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buffer)) {
		return "", windows.ERROR_INSUFFICIENT_BUFFER
	}
	path := windows.UTF16ToString(buffer[:n])
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\\` + strings.TrimPrefix(path, `\\?\UNC\`), nil
	}
	return strings.TrimPrefix(path, `\\?\`), nil
}
