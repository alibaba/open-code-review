// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build !windows

package session

import (
	"errors"
	"os"
	"syscall"
)

// lockExclusive takes a non-blocking exclusive flock. Non-blocking is what lets
// the viewer probe a run it does not own: a blocking lock would hang the page
// on every live session instead of reporting it.
func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(f *os.File) {
	// The descriptor is being closed by the caller regardless, which drops the
	// flock too; an error here has nowhere useful to go.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// isLockContended distinguishes "another process holds the lock" from "this
// filesystem cannot lock at all". Only the former means the run is alive.
func isLockContended(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}
