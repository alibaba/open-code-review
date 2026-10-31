// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"os"
	"strings"
)

// sessionLockPath is the sidecar the JSONL writer holds open for the life of
// the run. The kernel releases the lock when that process exits, including a
// crash, so a leftover file does not mean the run is still alive. File
// modification time is not used: a long model request writes nothing.
func sessionLockPath(jsonlPath string) string {
	if strings.HasSuffix(jsonlPath, ".jsonl") {
		return strings.TrimSuffix(jsonlPath, ".jsonl") + ".lock"
	}
	return jsonlPath + ".lock"
}

// SessionLockHeld reports whether a live process holds the sidecar lock for
// the session file at jsonlPath. No lock file means not held, which is also
// the case for sessions written before locks existed.
//
// When the file is free the probe acquires the lock and releases it before
// returning, so a reader never becomes the holder.
func SessionLockHeld(jsonlPath string) bool {
	f, err := os.OpenFile(sessionLockPath(jsonlPath), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()

	acquired, err := tryLock(f)
	if err != nil || acquired {
		if acquired {
			unlock(f)
		}
		return false
	}
	return true
}
