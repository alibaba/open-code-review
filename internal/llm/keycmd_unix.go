//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"os/exec"
)

func newKeyCmd(ctx context.Context, cmd string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", cmd)
}
