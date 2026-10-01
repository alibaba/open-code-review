// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRunFlushesTelemetryOnError(t *testing.T) {
	if os.Getenv("OCR_TEST_RUN_ERROR") == "1" {
		rootCmd.SetArgs([]string{"--invalid-flag"})
		os.Exit(run())
	}

	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunFlushesTelemetryOnError$")
	cmd.Env = append(os.Environ(),
		"OCR_TEST_RUN_ERROR=1",
		"OCR_ENABLE_TELEMETRY=1",
		"OTEL_EXPORTER_OTLP_ENDPOINT=",
		"HOME="+home,
		"USERPROFILE="+home,
	)
	output, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("exit error = %v, want exit code 1; output:\n%s", err, output)
	}
	if !strings.Contains(string(output), `"Resource"`) {
		t.Fatalf("telemetry was not flushed on error; output:\n%s", output)
	}
}
