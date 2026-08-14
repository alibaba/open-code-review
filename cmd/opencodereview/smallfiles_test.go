// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestPrintVersion_Dev(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = "dev"
	GitCommit = ""
	BuildDate = ""

	got := captureStdout(t, func() {
		printVersion()
	})
	if !strings.Contains(got, "open-code-review dev") {
		t.Errorf("expected 'open-code-review dev', got %q", got)
	}
	if !strings.Contains(got, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("expected OS/ARCH, got %q", got)
	}
}

func TestPrintVersion_WithCommitAndDate(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = "1.2.3"
	GitCommit = "abc1234"
	BuildDate = "2026-01-01"

	got := captureStdout(t, func() {
		printVersion()
	})
	if !strings.Contains(got, "1.2.3") {
		t.Errorf("expected version, got %q", got)
	}
	if !strings.Contains(got, "abc1234") {
		t.Errorf("expected commit, got %q", got)
	}
	if !strings.Contains(got, "2026-01-01") {
		t.Errorf("expected build date, got %q", got)
	}
}

func TestViewerCmd_DefaultAddr(t *testing.T) {
	if viewerOpts.addr != "localhost:5483" {
		t.Errorf("default addr = %q, want localhost:5483", viewerOpts.addr)
	}
}

func TestRunLLMProviders(t *testing.T) {
	got := captureStdout(t, func() {
		runLLMProviders()
	})
	if !strings.Contains(got, "Built-in providers") {
		t.Errorf("expected provider listing, got %q", got)
	}
}

func TestRootCmd_Help(t *testing.T) {
	got := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"--help"})
		rootCmd.Execute()
	})
	if !strings.Contains(got, "OpenCodeReview") {
		t.Errorf("expected usage text, got %q", got)
	}
}
