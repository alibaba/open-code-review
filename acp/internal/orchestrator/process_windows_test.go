//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func cooperativeInterruptAvailable(test *testing.T) bool {
	test.Helper()
	var processID uint32
	getConsoleProcessList := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	count, _, err := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&processID)), 1)
	if count != 0 {
		return true
	}
	if errors.Is(err, syscall.Errno(6)) {
		return false
	}
	test.Fatalf("GetConsoleProcessList: %v", err)
	return false
}

func TestWindowsCancellationWithoutConsole(test *testing.T) {
	const childEnvironment = "OCR_TEST_CANCELLATION_WITHOUT_CONSOLE"
	if os.Getenv(childEnvironment) == "1" {
		if cooperativeInterruptAvailable(test) {
			test.Fatal("cancellation subprocess unexpectedly has a console")
		}
		TestRunnerLocalCancelOverridesExit(test)
		return
	}
	binary, err := os.Executable()
	if err != nil {
		test.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestWindowsCancellationWithoutConsole$", "-test.v")
	command.Env = append(os.Environ(), childEnvironment+"=1")
	const createNoWindow = 0x08000000
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if output, err := command.CombinedOutput(); err != nil {
		test.Fatalf("console-less cancellation: %v\n%s", err, output)
	}
}

func TestWindowsJobObjectReapsGrandchild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	binary := testutil.Install(t, dir, "ocr", &testutil.Config{
		HoldPIDFile: pidFile,
		HoldInherit: false,
		HoldMS:      30000,
		StderrLines: []string{"[ocr] finished"},
		Stdout:      `{"status":"success","comments":[]}` + "\n",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	outcome, _ := runRequest(t, NewRunner(binary), ctx, Request{CWD: dir, Args: []string{"review"}})
	if outcome.Kind != OutcomeCompleted {
		t.Fatalf("outcome=%+v", outcome)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for processRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processRunning(pid) {
		t.Fatalf("job left grandchild %d running", pid)
	}
}

func TestWindowsPathWithSpacesAndNonASCII(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "评审 dir") // allow-non-english: Windows path fixture
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := testutil.Install(t, dir, "ocr tool", nil)
	resolved, err := ResolveBinary(ResolveOptions{Explicit: binary, Version: true})
	if err != nil || resolved.Path != binary {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	outcome, _ := runRequest(t, NewRunner(binary), context.Background(), Request{CWD: dir, Args: []string{"review"}})
	if outcome.Kind != OutcomeCompleted {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestWindowsRepeatedCancelDoesNotLeakJobs(t *testing.T) {
	binary := writeOCRScript(t)
	before, err := processHandleCount()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		events, outcomes := NewRunner(binary).Run(ctx, Request{CWD: t.TempDir(), Args: []string{"review", "block"}})
		for event := range events {
			if event.Message == "READY" {
				cancel()
				break
			}
		}
		outcome := <-outcomes
		cancel()
		if outcome.Kind != OutcomeCancelled {
			t.Fatalf("iteration %d: %+v", i, outcome)
		}
	}
	after, err := processHandleCount()
	if err != nil {
		t.Fatal(err)
	}
	if after > before+32 {
		t.Fatalf("handle count grew from %d to %d", before, after)
	}
}
