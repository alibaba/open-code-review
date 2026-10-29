//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCancelSynchronousIo      = modkernel32.NewProc("CancelSynchronousIo")
	procGetCurrentThread         = modkernel32.NewProc("GetCurrentThread")
	procGenerateConsoleCtrlEvent = modkernel32.NewProc("GenerateConsoleCtrlEvent")
	procCreateNamedPipeW         = modkernel32.NewProc("CreateNamedPipeW")
	pipeSeq                      atomic.Uint64
)

const (
	pipeAccessInbound         = 0x00000001
	fileFlagOverlapped        = 0x40000000
	fileFlagFirstPipeInstance = 0x00080000
	pipeTypeByte              = 0x00000000
	pipeRejectRemoteClients   = 0x00000008
	pipeWait                  = 0x00000000
)

func inheritedPipe(source *os.File, output bool) (io.ReadWriteCloser, error) {
	raw, err := duplicateFile(source)
	if err != nil {
		return nil, err
	}
	var probe error
	if output {
		probe = raw.SetWriteDeadline(time.Time{})
	} else {
		probe = raw.SetReadDeadline(time.Time{})
	}
	if probe == nil {
		return &stdioHandle{file: raw}, nil
	}
	bridged, err := bridgeNonpollable(raw, output)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	return bridged, nil
}

func duplicateFile(source *os.File) (*os.File, error) {
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	var dup syscall.Handle
	err = syscall.DuplicateHandle(current, syscall.Handle(source.Fd()), current, &dup, 0, false, syscall.DUPLICATE_SAME_ACCESS)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(dup), "acp-stdio"), nil
}

func bridgeNonpollable(raw *os.File, output bool) (*stdioHandle, error) {
	r, w, err := overlappedPipe()
	if err != nil {
		return nil, err
	}
	var copier *cancellableCopy
	if output {
		copier, err = startCancellableCopy(raw, r, duplicateCurrentThread)
	} else {
		copier, err = startCancellableCopy(w, raw, duplicateCurrentThread)
	}
	if err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, fmt.Errorf("initialize stdio copier: %w", err)
	}
	if output {
		return &stdioHandle{file: w, cleanup: copier.drain}, nil
	}
	return &stdioHandle{file: r, cleanup: copier.abort}, nil
}

// overlappedPipe is the Windows counterpart to a pollable os.Pipe. Anonymous
// pipes created by syscall.Pipe do not support deadlines; named pipes opened
// with FILE_FLAG_OVERLAPPED do, so Close and SetDeadline can cancel I/O.
func overlappedPipe() (r, w *os.File, err error) {
	name := fmt.Sprintf(`\\.\pipe\ocr-acp-%d-%d`, os.Getpid(), pipeSeq.Add(1))
	uname, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	server, _, e := procCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(uname)),
		uintptr(pipeAccessInbound|fileFlagOverlapped|fileFlagFirstPipeInstance),
		uintptr(pipeTypeByte|pipeWait|pipeRejectRemoteClients),
		1,
		65536,
		65536,
		0,
		0,
	)
	if server == 0 || server == uintptr(syscall.InvalidHandle) {
		return nil, nil, fmt.Errorf("CreateNamedPipe: %v", e)
	}
	client, err := syscall.CreateFile(
		uname,
		syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		_ = syscall.CloseHandle(syscall.Handle(server))
		return nil, nil, fmt.Errorf("CreateFile named pipe: %w", err)
	}
	reader := os.NewFile(server, "ocr-acp-pipe-r")
	writer := os.NewFile(uintptr(client), "ocr-acp-pipe-w")
	if err := reader.SetReadDeadline(time.Time{}); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, fmt.Errorf("ACP stdio requires a pollable pipe: %w", err)
	}
	if err := writer.SetWriteDeadline(time.Time{}); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, fmt.Errorf("ACP stdio requires a pollable pipe: %w", err)
	}
	return reader, writer, nil
}

const stdioCopyTimeout = 2 * time.Second

type cancellableCopy struct {
	dst, src  *os.File
	thread    syscall.Handle
	done      chan struct{}
	mu        sync.Mutex
	stopping  bool
	closeOnce sync.Once
	copyErr   error // Published when done closes.
}

func startCancellableCopy(dst, src *os.File, acquireThread func() (syscall.Handle, error)) (*cancellableCopy, error) {
	c := &cancellableCopy{dst: dst, src: src, done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(c.done)
		thread, err := acquireThread()
		if err != nil {
			ready <- err
			return
		}
		c.mu.Lock()
		c.thread = thread
		c.mu.Unlock()
		// Synchronize handle retirement with cancellation before this OS thread
		// can return to the runtime pool. A done-channel check alone would race.
		defer func() {
			c.mu.Lock()
			_ = syscall.CloseHandle(c.thread)
			c.thread = 0
			c.mu.Unlock()
		}()
		ready <- nil
		buf := make([]byte, 32*1024)
		for !c.stopped() {
			n, rerr := src.Read(buf)
			if n > 0 && !c.stopped() {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					c.copyErr = werr
					break
				}
			}
			if rerr != nil {
				if !errors.Is(rerr, io.EOF) {
					c.copyErr = rerr
				}
				break
			}
		}
		_ = dst.Close()
		_ = src.Close()
	}()
	if err := <-ready; err != nil {
		<-c.done
		return nil, err
	}
	return c, nil
}

func (c *cancellableCopy) stopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopping
}

// The output endpoint has already been closed, so EOF follows any queued
// bytes. Input cleanup cannot drain: its external writer may remain open.
func (c *cancellableCopy) drain() error {
	timer := time.NewTimer(stdioCopyTimeout)
	defer timer.Stop()
	select {
	case <-c.done:
		return c.copyErr
	case <-timer.C:
		return errors.Join(fmt.Errorf("stdio output did not drain within %s: %w", stdioCopyTimeout, os.ErrDeadlineExceeded), c.abort())
	}
}

func (c *cancellableCopy) abort() error {
	c.mu.Lock()
	c.stopping = true
	c.mu.Unlock()
	// Closing synchronous files may itself wait for an outstanding OS call.
	// Keep Close outside the caller and continue cancellation until it returns.
	c.closeOnce.Do(func() {
		go func() {
			_ = c.src.Close()
			_ = c.dst.Close()
		}()
	})
	timer := time.NewTimer(stdioCopyTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		c.mu.Lock()
		if c.thread != 0 {
			_ = cancelSynchronousIO(c.thread)
		}
		c.mu.Unlock()
		select {
		case <-c.done:
			return nil
		case <-timer.C:
			return fmt.Errorf("stdio copy did not stop within %s", stdioCopyTimeout)
		case <-ticker.C:
			// A one-shot cancellation can precede the syscall after stopped() was
			// checked. Retry while the copier still owns this locked OS thread.
		}
	}
}

func duplicateCurrentThread() (syscall.Handle, error) {
	current, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	pseudo, _, _ := procGetCurrentThread.Call()
	var dup syscall.Handle
	err = syscall.DuplicateHandle(current, syscall.Handle(pseudo), current, &dup, 0, false, syscall.DUPLICATE_SAME_ACCESS)
	return dup, err
}

func cancelSynchronousIO(thread syscall.Handle) error {
	r1, _, e := procCancelSynchronousIo.Call(uintptr(thread))
	if r1 == 0 {
		if errno, ok := e.(syscall.Errno); ok && (errno == 0 || uint32(errno) == 1168) {
			return nil
		}
		return e
	}
	return nil
}
