//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"os"
	"syscall"
	"unsafe"
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

var (
	procWaitForSingleObject   = syscall.NewLazyDLL("kernel32.dll").NewProc("WaitForSingleObject")
	procGetProcessHandleCount = syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcessHandleCount")
)

func windowsPIDRunning(pid int) bool {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	r1, _, _ := procWaitForSingleObject.Call(uintptr(h), 0)
	return r1 == uintptr(syscall.WAIT_TIMEOUT)
}

func processHandleCount() (uint32, error) {
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	var n uint32
	r1, _, e := procGetProcessHandleCount.Call(uintptr(current), uintptr(unsafe.Pointer(&n)))
	if r1 == 0 {
		return 0, e
	}
	return n, nil
}
