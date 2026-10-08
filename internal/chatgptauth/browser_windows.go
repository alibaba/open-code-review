//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"os/exec"
)

func openBrowserURL(ctx context.Context, target string) error {
	return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", target).Run()
}
