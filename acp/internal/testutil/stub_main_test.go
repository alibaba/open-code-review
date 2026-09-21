// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package testutil

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := Cleanup(); err != nil {
		fmt.Fprintln(os.Stderr, "clean up OCR stub:", err)
		code = 1
	}
	os.Exit(code)
}
