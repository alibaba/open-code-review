// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"runtime"
	"strings"
	"testing"
	"time"

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
//
// RunE runs under a deadline because losing the guard does not turn this into a
// returned error — it falls through to StartServer, which serves until the
// listener fails and so never returns. Calling RunE directly would then hang
// until the suite-wide timeout panic, blaming whichever test the runner was on
// rather than this flag. ValidateOpenMode itself is covered in
// internal/viewer's TestValidateOpenMode; what this test adds is that the
// viewer command actually calls it, and calls it first.
func TestViewerCmd_RejectsInvalidOpenMode(t *testing.T) {
	prevOpen, prevAddr := viewerOpts.open, viewerOpts.addr
	t.Cleanup(func() {
		viewerOpts.open = prevOpen
		viewerOpts.addr = prevAddr
	})

	// Port 0 so a regression binds an ephemeral port instead of fighting the
	// default one with a viewer the developer may already have running.
	viewerOpts.open = "yes"
	viewerOpts.addr = "127.0.0.1:0"

	done := make(chan error, 1)
	go func() { done <- viewerCmd.RunE(viewerCmd, nil) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunE with --open=yes = nil, want a validation error")
		}
		if !strings.Contains(err.Error(), "invalid --open value") {
			t.Errorf("error = %q, want it to mention the invalid --open value", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunE did not return; --open is not validated before StartServer binds")
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
