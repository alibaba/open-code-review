// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

//go:build windows

package codexauth

import "os/exec"

func openBrowserURL(target string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
}
