// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"io"
	"os"
	"testing"
)

// captureStderr swaps os.Stderr for a pipe around fn and returns what was written.
// Output here is tiny, so reading after the writer is closed avoids any pipe-buffer
// deadlock without a goroutine.
func captureStderr(test *testing.T, fn func()) string {
	test.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		test.Fatalf("os.Pipe: %v", err)
	}
	originalStderr := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = originalStderr }()

	fn()

	if err := writer.Close(); err != nil {
		test.Fatalf("close pipe writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		test.Fatalf("read captured stderr: %v", err)
	}
	return string(output)
}
