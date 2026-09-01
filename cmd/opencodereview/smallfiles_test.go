// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/viewer"
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
	if viewerOpts.open != viewer.OpenAuto {
		t.Errorf("default open = %q, want %q", viewerOpts.open, viewer.OpenAuto)
	}
	// --open is the single control. --no-open never shipped, so it is not carried
	// as an alias: two flags for one decision is the complexity this avoids.
	if f := viewerCmd.Flags().Lookup("no-open"); f != nil {
		t.Error("no-open flag present; --open=never is the only way to suppress opening")
	}
}

// TestViewerCmd_RejectsInvalidOpenMode pins the validation wiring: an unknown
// value must fail before the server binds a socket.
func TestViewerCmd_RejectsInvalidOpenMode(t *testing.T) {
	prev := viewerOpts.open
	t.Cleanup(func() { viewerOpts.open = prev })

	viewerOpts.open = "yes"
	err := viewerCmd.RunE(viewerCmd, nil)
	if err == nil {
		t.Fatal("RunE with --open=yes = nil, want a validation error")
	}
	if !strings.Contains(err.Error(), "invalid --open value") {
		t.Errorf("error = %q, want it to mention the invalid --open value", err)
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
