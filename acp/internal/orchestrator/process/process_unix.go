//go:build !windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package process

import (
	"errors"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func attachProcessGroup(*exec.Cmd) error { return nil }

// Close releases the OS resources owned by a managed command.
func Close(*exec.Cmd) {}

// Interrupt requests cooperative termination of a managed command.
func Interrupt(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// Kill forcibly terminates the managed process tree, including descendants.
func Kill(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return killUnixProcessGroup(cmd.Process.Pid, syscall.Kill)
}

func killUnixProcessGroup(pid int, kill func(int, syscall.Signal) error) error {
	for attempt := 0; ; attempt++ {
		err := kill(-pid, syscall.SIGKILL)
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		// Darwin returns EPERM for a group containing only unreaped zombies.
		// Allow its reaper a bounded window; genuine permission failures survive.
		if runtime.GOOS != "darwin" || !errors.Is(err, syscall.EPERM) || attempt == 10 {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
