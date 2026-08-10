//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import "os/exec"

func configureProcessGroup(cmd *exec.Cmd) {
	// On Windows, exec.CommandContext sends os.Kill which terminates the
	// direct child. Grandchild processes (e.g. from sh -c) may survive.
	// Full process-tree cleanup would need Windows Job Objects, but sh -c
	// is rare on Windows so this is an acceptable limitation.
}
