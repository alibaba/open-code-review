//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// inheritedPipe registers a nonblocking duplicate with Go's poller. Closing an
// inherited blocking os.Stdout cannot reliably interrupt a system write.
func inheritedPipe(source *os.File, output bool) (*os.File, error) {
	fd, err := syscall.Dup(int(source.Fd()))
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(fd)
	if err = syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "acp-stdio")
	if output {
		err = f.SetWriteDeadline(time.Time{})
	} else {
		err = f.SetReadDeadline(time.Time{})
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("ACP stdio requires a pollable pipe: %w", err)
	}
	return f, nil
}
