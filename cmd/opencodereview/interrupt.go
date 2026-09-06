// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// forceExit terminates the process immediately when a second signal arrives.
// It is a variable so tests can intercept the forced-exit path without
// killing the test binary.
var forceExit = func(sig os.Signal) {
	fmt.Fprintf(os.Stderr, "[ocr] second %v signal received, exiting immediately\n", sig)
	os.Exit(1)
}

// interruptContextWithForcedExit returns a child of parent that is canceled
// by the first os.Interrupt or syscall.SIGTERM, together with a stop function
// that restores default signal handling.
//
// The first signal cancels the context so executeReviewContext's defer chain
// can run the graceful shutdown: flush the report, freeze the retry report,
// persist the session manifest, close MCP clients. signal.NotifyContext
// cannot serve this command's second requirement: its watcher goroutine
// exits right after the first signal without unregistering, so for the whole
// graceful-shutdown window every further signal lands in its unread buffered
// channel and is dropped while the default kill behavior stays suppressed —
// a second Ctrl+C does nothing. Here the watcher stays alive across the
// whole shutdown window and a second signal force-exits the process at
// once: the user has explicitly abandoned the graceful path, however long
// the remaining cleanup would take.
//
// stop is idempotent. It unregisters the signal channel, releases the
// watcher goroutine and cancels the context, mirroring the stop returned by
// signal.NotifyContext.
func interruptContextWithForcedExit(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	// Buffer 2: os/signal sends non-blockingly, so with a size-one buffer two
	// back-to-back signals could drop the second before the watcher consumes
	// the first — the very failure mode this function exists to eliminate.
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	var once sync.Once
	// mu + stopped close the race between stop() and the watcher's
	// late-signal check: without it, a signal already in ch's buffer can be
	// received, pass a non-blocking done-probe, and force-exit a run whose
	// shutdown already completed — all in the window between signal.Stop
	// and close(done). Setting stopped and closing done under the same
	// mutex the watcher checks makes the decision atomic.
	var mu sync.Mutex
	stopped := false
	stop := func() {
		once.Do(func() {
			mu.Lock()
			stopped = true
			close(done)
			mu.Unlock()
			signal.Stop(ch)
			cancel()
		})
	}

	go func() {
		defer stop()
		select {
		case <-ch:
			// First signal: start the graceful shutdown.
			cancel()
			// Second signal: the user wants out now; skip the remaining
			// cleanup. stop() (graceful shutdown finished first) releases
			// the watcher instead.
			select {
			case sig := <-ch:
				// A signal queued while stop() was running must not turn
				// an already-successful run into exit 1.
				mu.Lock()
				if stopped {
					mu.Unlock()
					return
				}
				mu.Unlock()
				forceExit(sig)
			case <-done:
			}
		case <-ctx.Done():
			// The parent context or the review ended before any signal.
		case <-done:
		}
	}()

	return ctx, stop
}
