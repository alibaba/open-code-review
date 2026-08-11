// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package session

import "os"

// Unsupported targets retain session persistence but cannot distinguish a
// live process from an interrupted one. They continue to use the historical
// aborted fallback rather than failing the review altogether.
func tryLockSessionFile(file *os.File) (locked, supported bool, err error) {
	return false, false, nil
}

func unlockSessionFile(file *os.File) error {
	return nil
}
