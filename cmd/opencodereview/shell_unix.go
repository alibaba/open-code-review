//go:build !windows

package main

import (
	"context"
	"os/exec"
)

func shellCommand(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", script)
}
