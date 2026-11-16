// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"testing"
)

const testHomeIsolatedEnv = "OCR_TEST_HOME_ISOLATED"

func TestMain(m *testing.M) {
	// Helper subprocesses re-exec this binary and must keep the isolated home
	// their parent selected, so both processes read and write the same config.
	if os.Getenv(testHomeIsolatedEnv) == "1" {
		os.Exit(m.Run())
	}

	tmp, err := os.MkdirTemp("", "ocr-test-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv(testHomeIsolatedEnv, "1")
	_ = os.Setenv("HOME", tmp)
	_ = os.Setenv("USERPROFILE", tmp)

	code := m.Run()

	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
