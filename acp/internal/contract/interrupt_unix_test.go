//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract_test

import (
	"os/exec"
	"syscall"
)

func prepareInterruptible(cmd *exec.Cmd) {}

func interruptProcess(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGINT)
}
