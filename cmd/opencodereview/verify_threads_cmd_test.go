// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteVerifyThreadsRejectsBadInputBeforeAnyModelCall(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		opts verifyThreadsOptions
		want string
	}{
		{"no input", verifyThreadsOptions{outputPath: filepath.Join(dir, "out.json")}, "--input and --output are required"},
		{"no output", verifyThreadsOptions{inputPath: bad}, "--input and --output are required"},
		{"missing input file", verifyThreadsOptions{inputPath: filepath.Join(dir, "absent.json"), outputPath: filepath.Join(dir, "out.json")}, "read input"},
		{"malformed input", verifyThreadsOptions{inputPath: bad, outputPath: filepath.Join(dir, "out.json")}, "parse input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeVerifyThreads(context.Background(), tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
