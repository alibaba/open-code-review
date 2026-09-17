//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func processRunning(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// Linux container PID 1 may leave an orphan zombie unreaped. It cannot run
	// or keep the output pipe open, and does not mean termination failed.
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if end := strings.LastIndex(string(data), ") "); end >= 0 {
			return !strings.HasPrefix(string(data[end+2:]), "Z ")
		}
	}
	return true
}

func killPID(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
