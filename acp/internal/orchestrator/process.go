// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"errors"
	"os/exec"
)

// startManagedProcess applies OS process-tree membership and starts cmd.
// On Windows the process is born suspended, assigned to a Job Object, then
// resumed so descendants cannot escape between CreateProcess and assignment.
func startManagedProcess(cmd *exec.Cmd) error {
	if err := configureProcessGroup(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		closeProcessGroup(cmd)
		return err
	}
	if err := attachProcessGroup(cmd); err != nil {
		// Keep the cleanup failure instead of discarding it: if the process
		// survived the kill, its error explains why rather than leaving only
		// the attach failure behind.
		killErr := killProcessGroup(cmd)
		waitErr := cmd.Wait()
		closeProcessGroup(cmd)
		return errors.Join(err, killErr, waitErr)
	}
	return nil
}
