//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

func TestRunnerCancellationReapsProcessGroupChild(t *testing.T) {
	binary := buildPhaseThreeMock(t)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := NewRunner(binary)
	events, outcomes := runner.Run(ctx, Request{
		CWD:  t.TempDir(),
		Args: []string{"review", "-scenario", "spawn-child", "-child-pid-file", pidFile},
	})
	for event := range events {
		if event.Message == "READY" {
			cancel()
			break
		}
	}
	outcome := <-outcomes
	if outcome.Kind != OutcomeCancelled {
		t.Fatalf("outcome = %+v", outcome)
	}
	contents, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read child PID: %v", err)
	}
	pid, err := strconv.Atoi(string(contents[:len(contents)-1]))
	if err != nil {
		t.Fatalf("parse child PID %q: %v", contents, err)
	}
	if err := syscall.Kill(pid, 0); err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child process %d is still alive: %v", pid, err)
	}
}
