//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestVersionProbeAllowsBriefInheritedOutputWait(t *testing.T) {
	// The leader exits immediately, but its helper finishes the version text
	// after the former 100ms pipe-wait limit. Wait for EOF, not partial output.
	binary := writeExecutable(t, t.TempDir(), "ocr", "#!/bin/sh\nprintf 'ocr '\n(sleep 0.3; printf '1.2.3\\n') &\nexit 0\n")
	version, err := probeVersion(binary, 5*time.Second)
	if err != nil || version != "ocr 1.2.3" {
		t.Fatalf("version=%q err=%v", version, err)
	}
}

func TestVersionProbeBoundsInheritedOutputWait(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	binary := writeExecutable(t, dir, "ocr", "#!/bin/sh\nsleep 30 &\necho $! > '"+pidFile+"'\necho ocr-test\nexit 0\n")
	result := make(chan error, 1)
	go func() {
		_, err := probeVersion(binary, 2*time.Second)
		result <- err
	}()
	readChild := func() int {
		t.Helper()
		data, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
	select {
	case err := <-result:
		pid := readChild()
		t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		if err == nil {
			t.Fatal("inherited output did not fail version probe")
		}
		deadline := time.Now().Add(time.Second)
		for versionChildRunning(pid) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if versionChildRunning(pid) {
			t.Fatal("version probe left its child running")
		}
	case <-time.After(4 * time.Second):
		// Release the inherited pipe so the failed regression leaves no blocked
		// goroutine or child behind, even against the original implementation.
		_ = syscall.Kill(readChild(), syscall.SIGKILL)
		<-result
		t.Fatal("version probe ignored its deadline while a descendant held output")
	}
}

func versionChildRunning(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// Linux container PID 1 may leave an orphan zombie unreaped. It cannot run
	// or keep the output pipe open, and does not mean termination failed.
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if end := strings.LastIndex(string(data), ") "); end >= 0 {
			return !strings.HasPrefix(string(data[end+2:]), "Z ")
		}
	}
	return true
}
