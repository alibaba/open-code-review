//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
)

// jobs maps each managed command to a Job Object created before Start.
// The process is created CREATE_SUSPENDED, assigned, then resumed so no
// descendant can be born outside the job.
var jobs sync.Map

func configureProcessGroup(cmd *exec.Cmd) error {
	job, err := createKillOnCloseJob()
	if err != nil {
		return err
	}
	attr := cmd.SysProcAttr
	if attr == nil {
		attr = &syscall.SysProcAttr{}
		cmd.SysProcAttr = attr
	}
	attr.HideWindow = true
	attr.CreationFlags |= createSuspended | syscall.CREATE_NEW_PROCESS_GROUP
	jobs.Store(cmd, job)
	return nil
}

func attachProcessGroup(cmd *exec.Cmd) error {
	job, ok := loadJob(cmd)
	if !ok || cmd.Process == nil {
		return errors.New("OCR Job Object is missing after process start")
	}
	// Leave the process suspended if assignment fails so Start's caller can
	// kill it before any descendant is born outside the job.
	if err := assignProcessToJob(job, cmd.Process.Pid); err != nil {
		return err
	}
	return resumeProcessThreads(cmd.Process.Pid)
}

func interruptProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// os.Interrupt is not implemented for Process.Signal on Windows.
	// CREATE_NEW_PROCESS_GROUP lets GenerateConsoleCtrlEvent deliver CTRL_BREAK,
	// which Go maps to os.Interrupt. Programs that do not handle console control
	// events stay alive until TerminateJobObject after the grace period.
	//
	// CTRL_BREAK is best-effort: delivery requires the caller to share a console
	// with the process group, and an agent launched without one (the common
	// editor/GUI arrangement) cannot deliver it at all. Treat that expected
	// failure as degradation so an ordinary cancellation is not reported as a
	// cleanup error; a genuine failure of the guaranteed path still surfaces
	// through killProcessGroup.
	if err := generateCtrlBreak(cmd.Process.Pid); err != nil && !ctrlBreakUndeliverable(err) {
		return err
	}
	return nil
}

func killProcessGroup(cmd *exec.Cmd) error {
	var errs []error
	if job, ok := loadJob(cmd); ok {
		if err := terminateJob(job, 1); err != nil && !ignoreAlreadyExited(err) {
			errs = append(errs, err)
		}
	}
	if cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil && !ignoreAlreadyExited(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func closeProcessGroup(cmd *exec.Cmd) {
	v, ok := jobs.LoadAndDelete(cmd)
	if !ok {
		return
	}
	job, _ := v.(syscall.Handle)
	if job != 0 {
		_ = syscall.CloseHandle(job)
	}
}

func loadJob(cmd *exec.Cmd) (syscall.Handle, bool) {
	v, ok := jobs.Load(cmd)
	if !ok {
		return 0, false
	}
	job, ok := v.(syscall.Handle)
	return job, ok && job != 0
}
