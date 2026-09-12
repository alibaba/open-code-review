//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
)

// Windows process-tree reclamation requires a Job Object. Until that support
// is implemented and verified, Windows remains experimental as documented.
func configureProcessGroup(_ *exec.Cmd) error {
	return fmt.Errorf("Windows process-tree control requires Job Object support")
}

func interruptProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
