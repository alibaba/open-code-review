// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package process manages platform resources for entire child process trees.
package process

import (
	"errors"
	"os/exec"
)

// Start applies OS process-tree membership and starts cmd.
// On Windows the process is born suspended, assigned to a Job Object, then
// resumed so descendants cannot escape between CreateProcess and assignment.
func Start(cmd *exec.Cmd) error {
	if err := configureProcessGroup(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		Close(cmd)
		return err
	}
	if err := attachProcessGroup(cmd); err != nil {
		// Keep the cleanup failure instead of discarding it: if the process
		// survived the kill, its error explains why rather than leaving only
		// the attach failure behind.
		killErr := Kill(cmd)
		waitErr := cmd.Wait()
		Close(cmd)
		return errors.Join(err, killErr, waitErr)
	}
	return nil
}
