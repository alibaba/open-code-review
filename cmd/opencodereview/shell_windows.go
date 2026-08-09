//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os/exec"
)

func shellCommand(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd", "/c", script)
}
