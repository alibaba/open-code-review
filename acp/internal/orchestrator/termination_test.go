// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Deliberately hold Done delivery to prove that synchronous Err observation
// does not depend on a cancellation watcher being scheduled first.
type delayedDoneContext struct {
	context.Context
	release <-chan struct{}
}

func (c delayedDoneContext) Done() <-chan struct{} {
	<-c.release
	return c.Context.Done()
}

func TestPreterminatedRequestDoesNotStartOCR(t *testing.T) {
	for _, name := range []string{"cancelled", "context deadline", "request deadline"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "ocr")
			if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf started > started\nprintf '{\"status\":\"success\",\"comments\":[]}'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			deadline := time.Time{}
			want := OutcomeTimedOut
			switch name {
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				want = OutcomeCancelled
			case "context deadline":
				c, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Hour))
				defer cancel()
				ctx = c
			case "request deadline":
				deadline = time.Now().Add(-time.Hour)
			}
			release := make(chan struct{})
			defer close(release)
			ctx = delayedDoneContext{ctx, release}
			events, outcomes := NewRunner(binary).Run(ctx, Request{CWD: dir, Args: []string{"scan"}, Deadline: deadline})
			select {
			case outcome := <-outcomes:
				if outcome.Kind != want {
					t.Errorf("kind=%s, want %s", outcome.Kind, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("pre-start termination blocked")
			}
			for range events {
			}
			if _, open := <-outcomes; open {
				t.Fatal("more than one outcome")
			}
			if _, err := os.Stat(filepath.Join(dir, "started")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("OCR executed: marker stat=%v", err)
			}
		})
	}
}

func TestAwaitCompletedProcessObservesCancellationWithoutWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release := make(chan struct{})
	defer close(release)
	waited := make(chan error, 1)
	exitErr := errors.New("observed exit")
	waited <- exitErr
	done := make(chan struct{})
	go func() {
		defer close(done)
		kind, err, cleanup := NewRunner("unused").await(delayedDoneContext{ctx, release}, time.Time{}, &exec.Cmd{}, waited)
		if kind != OutcomeCancelled || err != exitErr || cleanup != nil {
			t.Errorf("kind=%s wait=%v cleanup=%v", kind, err, cleanup)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("completed result consumed twice or watcher required")
	}
}

func TestCompletedOutcomeIsNotRewritten(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	outcome, _ := runRequest(t, NewRunner(writeOCRScript(t)), ctx, Request{CWD: t.TempDir(), Args: []string{"scan"}})
	cancel()
	if outcome.Kind != OutcomeCompleted {
		t.Fatalf("outcome=%+v", outcome)
	}
}

type observationContext struct {
	context.Context
	observe func()
}

func (c observationContext) Err() error {
	c.observe()
	return c.Context.Err()
}

func TestAwaitRechecksAfterConsumingCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	wrapped := observationContext{Context: ctx, observe: func() {
		calls++
		// The first observation admits waiting. The second occurs only after
		// the ready completion has been consumed, and makes cancel observable.
		if calls == 2 {
			cancel()
		}
	}}
	waited := make(chan error, 1)
	exitErr := errors.New("completed result")
	waited <- exitErr
	done := make(chan struct{})
	go func() {
		defer close(done)
		kind, err, cleanup := NewRunner("unused").await(wrapped, time.Time{}, &exec.Cmd{}, waited)
		if kind != OutcomeCancelled || err != exitErr || cleanup != nil {
			t.Errorf("kind=%s err=%v cleanup=%v", kind, err, cleanup)
		}
		if len(waited) != 0 {
			t.Error("did not exercise consumed completion")
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("completion was consumed twice")
	}
}

func TestAwaitFirstObservedCauseSurvivesCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observed := make(chan struct{})
	deadline := time.Now().Add(time.Second)
	wrapped := observationContext{Context: ctx, observe: func() { close(observed) }}
	waited := make(chan error)
	result := make(chan OutcomeKind, 1)
	runner := NewRunner("unused")
	runner.GracePeriod = 3 * time.Second
	go func() { kind, _, _ := runner.await(wrapped, deadline, &exec.Cmd{}, waited); result <- kind }()
	<-observed
	// Deadline expires while cleanup waits. The earlier cancellation must win.
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	<-timer.C
	waited <- nil
	if kind := <-result; kind != OutcomeCancelled {
		t.Fatalf("kind=%s", kind)
	}
}

func TestSimultaneouslyObservableReasonsPreferTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waited := make(chan error, 1)
	waited <- nil
	kind, _, _ := NewRunner("unused").await(ctx, time.Now().Add(-time.Hour), &exec.Cmd{}, waited)
	if kind != OutcomeTimedOut {
		t.Fatalf("kind=%s", kind)
	}
}

func TestCancellationAtStartBoundaryDoesNotStartOCR(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "ocr")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf started > started\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observations := 0
	wrapped := observationContext{Context: ctx, observe: func() {
		observations++
		if observations == 2 {
			cancel()
		}
	}}
	events, outcomes := NewRunner(binary).Run(wrapped, Request{CWD: dir, Args: []string{"scan"}})
	outcome := <-outcomes
	for range events {
	}
	if outcome.Kind != OutcomeCancelled || outcome.ExitCode != -1 {
		t.Fatalf("outcome=%+v", outcome)
	}
	if _, open := <-outcomes; open {
		t.Fatal("outcomes not closed")
	}
	if _, err := os.Stat(filepath.Join(dir, "started")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OCR started: %v", err)
	}
}

func TestAwaitObservedTimeoutIsNotReplacedByCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := make(chan struct{})
	wrapped := observationContext{Context: ctx, observe: func() { close(observed) }}
	waited := make(chan error)
	result := make(chan OutcomeKind, 1)
	runner := NewRunner("unused")
	runner.GracePeriod = 3 * time.Second
	go func() {
		kind, _, _ := runner.await(wrapped, time.Now().Add(-time.Hour), &exec.Cmd{}, waited)
		result <- kind
	}()
	<-observed
	cancel()
	waited <- nil
	if kind := <-result; kind != OutcomeTimedOut {
		t.Fatalf("kind=%s", kind)
	}
}
