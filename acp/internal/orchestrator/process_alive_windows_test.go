//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"os"
)

func processRunning(pid int) bool {
	return windowsPIDRunning(pid)
}

func killPID(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	err = p.Kill()
	if err != nil && err != os.ErrProcessDone {
		return err
	}
	return nil
}
