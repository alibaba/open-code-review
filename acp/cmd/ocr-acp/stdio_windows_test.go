//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestInheritedPipeAcceptsGoPipes(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	out, err := inheritedPipe(w, true)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	in, err := inheritedPipe(r, false)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	writeDL, ok := out.(interface{ SetWriteDeadline(time.Time) error })
	if !ok {
		t.Fatal("output is not deadline-capable")
	}
	readDL, ok := in.(interface{ SetReadDeadline(time.Time) error })
	if !ok {
		t.Fatal("input is not deadline-capable")
	}
	if err := writeDL.SetWriteDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("write deadline: %v", err)
	}
	if err := readDL.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("read deadline: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := in.Read(buf); err == nil {
		t.Fatal("expected read deadline")
	}
}

func TestInheritedPipeCloseCancelsBlockedWrite(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out, err := inheritedPipe(w, true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		buf := bytes.Repeat([]byte("x"), 1<<20)
		for {
			if _, err := out.Write(buf); err != nil {
				done <- err
				return
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	if err := out.Close(); err != nil && !errors.Is(err, os.ErrClosed) && !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-done:
		if time.Since(start) > 3*time.Second {
			t.Fatal("write close did not unblock promptly")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked write was not cancelled")
	}
}

// A successful bridge Write only reaches its intermediate pipe. Close must
// let the copier deliver those bytes before cancelling it.
func TestOutputBridgeCloseDrainsBufferedBytes(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out, err := bridgeNonpollable(w, true)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("final response\n"), 2048)
	if _, err := out.Write(payload); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- out.Close() }()
	time.Sleep(50 * time.Millisecond)
	received := make(chan []byte, 1)
	go func() { data, _ := io.ReadAll(r); received <- data }()
	select {
	case data := <-received:
		if !bytes.Equal(data, payload) {
			t.Fatalf("drain delivered %d of %d bytes", len(data), len(payload))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("output did not drain")
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

func TestCopyRejectsThreadInitializationFailure(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	failure := errors.New("thread duplication failed")
	copier, err := startCancellableCopy(w, r, func() (syscall.Handle, error) { return 0, failure })
	if copier != nil || !errors.Is(err, failure) {
		t.Fatalf("copier=%v error=%v", copier, err)
	}
}

func TestCompletedCopyReleasesThreadBeforeAbort(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	copier, err := startCancellableCopy(dst, r, duplicateCurrentThread)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-copier.done:
	case <-time.After(3 * time.Second):
		t.Fatal("copy did not finish")
	}
	copier.mu.Lock()
	thread := copier.thread
	copier.mu.Unlock()
	if thread != 0 {
		t.Fatalf("completed copier retained thread handle %v", thread)
	}
	if err := copier.abort(); err != nil {
		t.Fatal(err)
	}
}

func TestCopyAbortCancelsIdleSynchronousRead(t *testing.T) {
	for range 20 {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		dst, err := os.CreateTemp(t.TempDir(), "output")
		if err != nil {
			t.Fatal(err)
		}
		copier, err := startCancellableCopy(dst, r, duplicateCurrentThread)
		if err != nil {
			t.Fatal(err)
		}
		// Abort immediately to exercise cancellation racing the first Read.
		if err := copier.abort(); err != nil {
			t.Fatal(err)
		}
		_ = w.Close()
	}
}

func TestOutputBridgeCloseReportsReceiverFailure(t *testing.T) {
	r, w := smallSynchronousPipe(t)
	defer r.Close()
	out, err := bridgeNonpollable(w, true)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := out.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write(bytes.Repeat([]byte("x"), 32*1024)); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- out.Close() }()
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("Close hid receiver failure")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after receiver failure")
	}
}

func TestOutputBridgeCloseReportsDrainTimeout(t *testing.T) {
	r, w := smallSynchronousPipe(t)
	defer r.Close()
	out, err := bridgeNonpollable(w, true)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := out.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write(bytes.Repeat([]byte("x"), 32*1024)); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- out.Close() }()
	select {
	case err := <-closed:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("Close = %v, want drain timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not cancel blocked output")
	}
}

// Keep the receiver buffer below the test payload so an unread write blocks.
func smallSynchronousPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	var r, w syscall.Handle
	if err := syscall.CreatePipe(&r, &w, nil, 4096); err != nil {
		t.Fatal(err)
	}
	reader, writer := os.NewFile(uintptr(r), "test-pipe-r"), os.NewFile(uintptr(w), "test-pipe-w")
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	return reader, writer
}
