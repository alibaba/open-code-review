//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"os/exec"
	"runtime"
)

func openBrowserURL(ctx context.Context, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	// A browser may outlive the login; cancelling the callback must not close it.
	return exec.Command(cmd, target).Run()
}
