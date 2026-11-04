//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"os/signal"
)

func waitForInterrupt() {
	ch := make(chan os.Signal, 1)
	// CTRL_BREAK is mapped to os.Interrupt. syscall.SIGBREAK is not defined
	// in the standard syscall package on Windows.
	signal.Notify(ch, os.Interrupt)
	<-ch
}
