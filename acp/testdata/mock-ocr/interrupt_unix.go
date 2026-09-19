//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"syscall"
)

func interruptSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
