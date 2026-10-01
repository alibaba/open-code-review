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
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.CommandContext(ctx, cmd, target).Run()
}
