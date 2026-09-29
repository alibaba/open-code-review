//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	modntdll                     = syscall.NewLazyDLL("ntdll.dll")
	procCreateJobObjectW         = modkernel32.NewProc("CreateJobObjectW")
	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = modkernel32.NewProc("TerminateJobObject")
	procSetInformationJobObject  = modkernel32.NewProc("SetInformationJobObject")
	procOpenThread               = modkernel32.NewProc("OpenThread")
	procResumeThread             = modkernel32.NewProc("ResumeThread")
	procThread32First            = modkernel32.NewProc("Thread32First")
	procThread32Next             = modkernel32.NewProc("Thread32Next")
	procGenerateConsoleCtrlEvent = modkernel32.NewProc("GenerateConsoleCtrlEvent")
	procWaitForSingleObject      = modkernel32.NewProc("WaitForSingleObject")
	procGetProcessHandleCount    = modkernel32.NewProc("GetProcessHandleCount")
	procNtResumeProcess          = modntdll.NewProc("NtResumeProcess")
)

const (
	createSuspended              = 0x00000004
	jobObjectExtendedLimitClass  = 9
	jobObjectLimitKillOnJobClose = 0x00002000
	processSetQuota              = 0x0100
	processSuspendResume         = 0x0800
	threadSuspendResume          = 0x0002
	ctrlBreakEvent               = 1
	errorNoMoreFiles             = syscall.Errno(18)
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type threadEntry32 struct {
	Size           uint32
	Usage          uint32
	ThreadID       uint32
	OwnerProcessID uint32
	BasePri        int32
	DeltaPri       int32
	Flags          uint32
}

func createKillOnCloseJob() (syscall.Handle, error) {
	r0, _, e := procCreateJobObjectW.Call(0, 0)
	job := syscall.Handle(r0)
	if job == 0 {
		return 0, fmt.Errorf("CreateJobObject: %v", e)
	}
	var info jobObjectExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	r1, _, e := procSetInformationJobObject.Call(
		uintptr(job),
		jobObjectExtendedLimitClass,
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if r1 == 0 {
		_ = syscall.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %v", e)
	}
	return job, nil
}

func assignProcessToJob(job syscall.Handle, pid int) error {
	access := uint32(syscall.PROCESS_TERMINATE | processSetQuota | syscall.PROCESS_QUERY_INFORMATION | syscall.SYNCHRONIZE)
	process, err := syscall.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer syscall.CloseHandle(process)
	r1, _, e := procAssignProcessToJobObject.Call(uintptr(job), uintptr(process))
	if r1 == 0 {
		return fmt.Errorf("AssignProcessToJobObject: %v", e)
	}
	return nil
}

func terminateJob(job syscall.Handle, exitCode uint32) error {
	r1, _, e := procTerminateJobObject.Call(uintptr(job), uintptr(exitCode))
	if r1 == 0 {
		return e
	}
	return nil
}

func resumeProcessThreads(pid int) error {
	if err := ntResumeProcess(pid); err == nil {
		return nil
	}
	return resumeThreadsBySnapshot(pid)
}

func ntResumeProcess(pid int) error {
	access := uint32(processSuspendResume | syscall.PROCESS_QUERY_INFORMATION | syscall.SYNCHRONIZE)
	process, err := syscall.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(process)
	r0, _, e := procNtResumeProcess.Call(uintptr(process))
	if r0 != 0 {
		if e != syscall.Errno(0) {
			return e
		}
		return fmt.Errorf("NtResumeProcess status %d", r0)
	}
	return nil
}

func resumeThreadsBySnapshot(pid int) error {
	var last error
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		n, err := resumeThreadsOnce(pid)
		if n > 0 {
			// Resuming any thread runs the process, which is all that is
			// required. A non-nil err here describes threads that exited between
			// the snapshot and OpenThread, or a Thread32Next enumeration error;
			// reporting it would fail startup and kill a healthy process.
			return nil
		}
		last = err
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("no threads resumed for pid %d: %v", pid, last)
}

func resumeThreadsOnce(pid int) (int, error) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(snapshot)
	var entry threadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	r1, _, e := procThread32First.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
	if r1 == 0 {
		return 0, e
	}
	var errs error
	resumed := 0
	for {
		if entry.OwnerProcessID == uint32(pid) {
			thread, err := openThread(threadSuspendResume, entry.ThreadID)
			if err != nil {
				errs = errors.Join(errs, err)
			} else {
				if err := resumeThread(thread); err != nil {
					errs = errors.Join(errs, err)
				} else {
					resumed++
				}
				_ = syscall.CloseHandle(thread)
			}
		}
		r1, _, e = procThread32Next.Call(uintptr(snapshot), uintptr(unsafe.Pointer(&entry)))
		if r1 == 0 {
			if e == errorNoMoreFiles {
				break
			}
			errs = errors.Join(errs, e)
			break
		}
	}
	return resumed, errs
}

func openThread(access uint32, threadID uint32) (syscall.Handle, error) {
	r0, _, e := procOpenThread.Call(uintptr(access), 0, uintptr(threadID))
	h := syscall.Handle(r0)
	if h == 0 {
		return 0, fmt.Errorf("OpenThread: %v", e)
	}
	return h, nil
}

func resumeThread(thread syscall.Handle) error {
	r1, _, e := procResumeThread.Call(uintptr(thread))
	if r1 == uintptr(^uint32(0)) {
		return fmt.Errorf("ResumeThread: %v", e)
	}
	return nil
}

func generateCtrlBreak(pid int) error {
	r1, _, e := procGenerateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(uint32(pid)))
	if r1 == 0 {
		return e
	}
	return nil
}

// ctrlBreakUndeliverable reports whether err can only mean that a console
// control event reached no process, which is an expected outcome rather than a
// cleanup failure. GenerateConsoleCtrlEvent fails with ERROR_INVALID_HANDLE when
// the caller has no console to share with the target group, and with
// ERROR_INVALID_PARAMETER when the identifier is not a live process group.
func ctrlBreakUndeliverable(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch uint32(errno) {
	case 6, 87: // ERROR_INVALID_HANDLE, ERROR_INVALID_PARAMETER
		return true
	}
	return false
}

func windowsPIDRunning(pid int) bool {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	r1, _, _ := procWaitForSingleObject.Call(uintptr(h), 0)
	return r1 == uintptr(syscall.WAIT_TIMEOUT)
}

func processHandleCount() (uint32, error) {
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	var n uint32
	r1, _, e := procGetProcessHandleCount.Call(uintptr(current), uintptr(unsafe.Pointer(&n)))
	if r1 == 0 {
		return 0, e
	}
	return n, nil
}

func ignoreAlreadyExited(err error) bool {
	if err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch uint32(errno) {
		case 0, 5, 6, 87: // SUCCESS, ACCESS_DENIED, INVALID_HANDLE, INVALID_PARAMETER
			return true
		}
	}
	// The errno cases above are the locale-independent source of truth. The
	// previous English message matching was redundant with them and silently
	// changed behaviour on non-English Windows, so it is intentionally gone.
	return false
}
