//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os/exec"
	"strconv"
	"time"
)

// setupWaitDelay bounds how long Wait keeps draining output after the
// process tree was killed, in case a descendant escaped and still holds
// the inherited stdout/stderr handles.
const setupWaitDelay = 5 * time.Second

// taskkillTimeout bounds the taskkill call itself: WaitDelay cannot interrupt
// a Cancel callback that is still blocked inside it.
const taskkillTimeout = 10 * time.Second

func configureProcessGroup(cmd *exec.Cmd) {
	// Setup scripts run through `cmd /c`, so the real work (npm, node, ...)
	// is a grandchild. Killing only cmd.exe would leave it running, and
	// because it inherits the output pipe, Wait would block until it exits,
	// defeating the setup timeout. taskkill /T terminates the whole tree.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), taskkillTimeout)
		defer cancel()
		kill := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = setupWaitDelay
}
