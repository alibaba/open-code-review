// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := testutil.Cleanup(); err != nil {
		fmt.Fprintln(os.Stderr, "clean up OCR stub:", err)
		code = 1
	}
	os.Exit(code)
}
