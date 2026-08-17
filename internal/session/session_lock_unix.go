// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import (
	"os"
	"syscall"
)

func tryLockSessionFile(file *os.File) (locked, supported bool, err error) {
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, true, nil
	}
	if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
		return false, true, nil
	}
	return false, true, err
}

func unlockSessionFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
