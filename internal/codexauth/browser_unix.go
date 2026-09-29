// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build !windows

package codexauth

import (
	"os/exec"
	"runtime"
)

func openBrowserURL(target string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	return exec.Command(command, target).Start()
}
