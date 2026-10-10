//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os/exec"
	"syscall"
)

func prepareAdapterCmd(cmd *exec.Cmd) {}

func interruptAdapter(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}
