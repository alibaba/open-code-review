//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package contract_test

import (
	"os/exec"
	"syscall"
)

func prepareInterruptible(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func interruptProcess(cmd *exec.Cmd) error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GenerateConsoleCtrlEvent")
	r1, _, e := proc.Call(1, uintptr(uint32(cmd.Process.Pid)))
	if r1 == 0 {
		return e
	}
	return nil
}
