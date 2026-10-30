// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build windows

package session

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive takes a non-blocking exclusive byte-range lock, the Windows
// counterpart of flock. Non-blocking matters for the same reason it does on
// unix: the viewer probes runs it does not own, and a blocking lock would hang
// the page instead of reporting a live session.
//
// A zero-length range would be rejected by LockFileEx, so one byte is claimed.
// Nothing is ever written at that offset - it exists only to be locked.
func lockExclusive(f *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &overlapped,
	)
}

func unlockFile(f *os.File) {
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped)
}

// isLockContended reports whether the failure was another owner holding the
// range. LockFileEx reports contention as ERROR_LOCK_VIOLATION; anything else
// (a share violation, an unusable filesystem) must not read as "running".
func isLockContended(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
