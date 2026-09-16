//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
)

func inheritedPipe(_ *os.File, _ bool) (*os.File, error) {
	return nil, fmt.Errorf("platform_unsupported: Windows cancellable stdio is not implemented")
}
