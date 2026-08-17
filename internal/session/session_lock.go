// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var errSessionAlreadyActive = errors.New("session is already active")

// sessionLock is held by the process that owns a live JSONL session. The lock
// file may remain after a crash, but the OS lock itself is released with the
// process and therefore remains the source of truth for activity.
type sessionLock struct {
	file      *os.File
	supported bool
}

func sessionLockPath(sessionPath string) string {
	if strings.HasSuffix(sessionPath, ".jsonl") {
		return strings.TrimSuffix(sessionPath, ".jsonl") + ".lock"
	}
	return sessionPath + ".lock"
}

func acquireSessionLock(sessionPath string) (*sessionLock, error) {
	lockPath := sessionLockPath(sessionPath)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open session lock: %w", err)
	}

	locked, supported, err := tryLockSessionFile(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock session: %w", err)
	}
	if supported && !locked {
		_ = file.Close()
		return nil, errSessionAlreadyActive
	}

	return &sessionLock{file: file, supported: supported}, nil
}

// IsSessionActive reports whether the process that owns sessionPath currently
// holds its lock. Missing lock files are treated as inactive for compatibility
// with sessions written before live-state detection was introduced.
func IsSessionActive(sessionPath string) (bool, error) {
	lockPath := sessionLockPath(sessionPath)
	file, err := os.OpenFile(lockPath, os.O_RDWR, 0600)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open session lock: %w", err)
	}

	locked, supported, err := tryLockSessionFile(file)
	if err != nil {
		_ = file.Close()
		return false, fmt.Errorf("probe session lock: %w", err)
	}
	if !supported {
		_ = file.Close()
		return false, nil
	}
	if !locked {
		_ = file.Close()
		return true, nil
	}

	unlockErr := unlockSessionFile(file)
	closeErr := file.Close()
	if err := errors.Join(unlockErr, closeErr); err != nil {
		return false, fmt.Errorf("release session lock probe: %w", err)
	}
	return false, nil
}

func (l *sessionLock) close() error {
	if l == nil || l.file == nil {
		return nil
	}

	var unlockErr error
	if l.supported {
		unlockErr = unlockSessionFile(l.file)
	}
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
