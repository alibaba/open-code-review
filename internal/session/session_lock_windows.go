// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build windows

package session

import (
	"os"

	"golang.org/x/sys/windows"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
)

func tryLockSessionFile(file *os.File) (locked, supported bool, err error) {
	overlapped := new(windows.Overlapped)
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		lockfileFailImmediately|lockfileExclusiveLock,
		0,
		1,
		0,
		overlapped,
	)
	if err == nil {
		return true, true, nil
	}
	if err == windows.ERROR_LOCK_VIOLATION {
		return false, true, nil
	}
	return false, true, err
}

func unlockSessionFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, new(windows.Overlapped))
}
