//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"bufio"
	"errors"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
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
	if err := configureProcessGroup(leader); err != nil {
		t.Fatal(err)
	}
	if err := leader.Start(); err != nil {
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

func TestKillProcessGroupRetriesOnlyDarwinPermissionRace(t *testing.T) {
	for _, final := range []error{nil, syscall.ESRCH, syscall.EINVAL} {
		calls := 0
		got := killUnixProcessGroup(123, func(pid int, signal syscall.Signal) error {
			if pid != -123 || signal != syscall.SIGKILL {
				t.Fatalf("kill(%d, %v)", pid, signal)
			}
			calls++
			if calls == 1 {
				return syscall.EPERM
			}
			return final
		})
		if runtime.GOOS != "darwin" {
			if got != syscall.EPERM || calls != 1 {
				t.Fatalf("non-Darwin result=%v calls=%d", got, calls)
			}
			continue
		}
		want := final
		if final == syscall.ESRCH {
			want = nil
		}
		if got != want || calls != 2 {
			t.Fatalf("result=%v want=%v calls=%d", got, want, calls)
		}
	}
}

func TestKillProcessGroupPreservesPersistentErrors(t *testing.T) {
	for _, failure := range []error{syscall.EPERM, syscall.EINVAL} {
		calls := 0
		got := killUnixProcessGroup(123, func(int, syscall.Signal) error { calls++; return failure })
		wantCalls := 1
		if runtime.GOOS == "darwin" && failure == syscall.EPERM {
			wantCalls = 11
		}
		if got != failure || calls != wantCalls {
			t.Fatalf("result=%v calls=%d want=%v calls=%d", got, calls, failure, wantCalls)
		}
	}
}
