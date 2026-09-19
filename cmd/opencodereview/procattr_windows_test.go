//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// encodePowerShell returns the -EncodedCommand form of script, which avoids
// any quoting through `cmd /c`.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(u)*2)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// processRunning reports whether a process with the given PID exists.
func processRunning(t *testing.T, pid int) bool {
	t.Helper()
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH", "/FO", "CSV").Output()
	if err != nil {
		t.Fatalf("tasklist: %v", err)
	}
	return strings.Contains(string(out), `"`+strconv.Itoa(pid)+`"`)
}

// TestConfigureProcessGroup_CancelKillsGrandchild verifies cancelling a setup
// script kills the process `cmd /c` spawned, not only cmd.exe itself.
func TestConfigureProcessGroup_CancelKillsGrandchild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid.txt")
	// The grandchild records its own PID, then outlives any sane test run.
	ps := "Set-Content -Path '" + pidFile + "' -Value $PID; Start-Sleep -Seconds 120"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := shellCommand(ctx, "powershell -NoProfile -NonInteractive -EncodedCommand "+encodePowerShell(ps))
	configureProcessGroup(cmd)
	// Share one buffer for both streams, as CombinedOutput does, so the
	// grandchild holds the inherited output pipe.
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var pid int
	for deadline := time.Now().Add(30 * time.Second); pid == 0; {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff")))
		}
		if pid == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("grandchild never started; output: %s", out.String())
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	t.Cleanup(func() { _ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run() })
	if !processRunning(t, pid) {
		t.Fatalf("grandchild %d is not running before cancellation", pid)
	}

	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Wait did not return after cancellation")
	}

	for deadline := time.Now().Add(10 * time.Second); processRunning(t, pid); {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still running after cancellation", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
