//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"bufio"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/orchestrator/process"
)

func TestStopKillsIgnoringMemberAfterLeaderExit(t *testing.T) {
	// Both processes are children of the test so their exit can be reaped and
	// verified even on containers whose PID 1 does not reap orphan zombies.
	leader := exec.Command("sh", "-c", "read line")
	input, err := leader.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := process.Start(leader); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = leader.Process.Kill(); _ = leader.Wait() }()
	child := exec.Command("sh", "-c", "trap '' INT; echo READY; exec sleep 30")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: leader.Process.Pid}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "READY\n" {
		t.Fatalf("child readiness = %q, %v", line, err)
	}
	_ = input.Close()
	waited := make(chan error, 1)
	waited <- leader.Wait()
	if err := syscall.Kill(child.Process.Pid, 0); err != nil {
		t.Fatalf("group member exited before cleanup: %v", err)
	}
	if _, err := NewRunner("").stop(leader, waited); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
			t.Fatalf("expected forced group termination, got %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = child.Process.Kill()
		<-done
		t.Fatal("group member survived cleanup after leader exit")
	}
}
