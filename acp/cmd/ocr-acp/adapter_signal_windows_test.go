//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os/exec"
	"syscall"
)

func prepareAdapterCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func interruptAdapter(cmd *exec.Cmd) error {
	r1, _, e := procGenerateConsoleCtrlEvent.Call(1, uintptr(uint32(cmd.Process.Pid)))
	if r1 == 0 {
		return e
	}
	return nil
}
