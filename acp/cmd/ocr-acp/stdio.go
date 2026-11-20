// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// stdioHandle is a deadline-capable stdio endpoint. cleanup runs once on Close
// so a Windows copy goroutine blocked on an inherited pipe can be cancelled.
type stdioHandle struct {
	file    *os.File
	cleanup func() error
	once    sync.Once
}

func (h *stdioHandle) Read(p []byte) (int, error) { return h.file.Read(p) }

func (h *stdioHandle) Write(p []byte) (int, error) { return h.file.Write(p) }

func (h *stdioHandle) SetReadDeadline(t time.Time) error { return h.file.SetReadDeadline(t) }

func (h *stdioHandle) SetWriteDeadline(t time.Time) error { return h.file.SetWriteDeadline(t) }

func (h *stdioHandle) Close() error {
	var err error
	h.once.Do(func() {
		err = h.file.Close()
		if h.cleanup != nil {
			err = errors.Join(err, h.cleanup())
		}
	})
	return err
}

var _ io.ReadWriteCloser = (*stdioHandle)(nil)
