// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const defaultMaxConcurrent = 16

// cancelGracePeriod bounds how long the runner keeps waiting for output to drain
// after cancellation. It is a package var (not const) so tests can shrink it.
//
// exec.CommandContext SIGKILLs only the process it started. A hook, alias or
// helper that git spawns — or a grandchild of one — can inherit the stdout pipe
// and outlive that kill. The runner owns both ends of its pipes (see
// outputPipe), so on cancellation it stops waiting after this period by closing
// the read end, which unblocks the copy and releases the semaphore slot.
//
// This is deliberately not exec.Cmd.WaitDelay: WaitDelay also starts when a
// command exits normally, so a descendant still flushing just after a
// successful exit would turn the call into exec.ErrWaitDelay with truncated
// output (#1909 review).
var cancelGracePeriod = 5 * time.Second

// Runner limits the number of concurrent git subprocesses via an internal
// semaphore. All git command invocations should go through a shared Runner
// instance so that the total system-wide subprocess count stays bounded.
type Runner struct {
	sem chan struct{}
}

// New creates a Runner that allows at most maxConcurrent simultaneous git
// subprocesses. If maxConcurrent <= 0 the default (16) is used.
func New(maxConcurrent int) *Runner {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}
	return &Runner{sem: make(chan struct{}, maxConcurrent)}
}

func (r *Runner) acquire(ctx context.Context) error {
	if r.sem == nil {
		return fmt.Errorf("gitcmd.Runner not initialized; use gitcmd.New()")
	}
	select {
	case r.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runner) release() { <-r.sem }

// outputPipe is a pipe pair owned by the Runner instead of by exec.Cmd. Owning
// both ends lets the Runner decide when the read side stops waiting: os/exec
// closes its own pipe only when Wait returns, and Wait does not return while an
// inherited grandchild still holds the write end.
//
// Because both ends are *os.File, os/exec hands the descriptors straight to the
// child and creates no copying goroutines of its own — Wait therefore returns as
// soon as the direct child is reaped, regardless of who still holds the pipe.
type outputPipe struct {
	r *os.File
	w *os.File
}

func newOutputPipe() (*outputPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &outputPipe{r: r, w: w}, nil
}

// closeWrite drops the parent's copy of the write end so that EOF becomes
// visible once every child copy is closed.
func (p *outputPipe) closeWrite() {
	if p.w != nil {
		_ = p.w.Close()
		p.w = nil
	}
}

// closeRead unblocks a pending read. It is safe to call repeatedly and
// concurrently with the reader: closing the file makes Read return.
func (p *outputPipe) closeRead() {
	if p.r != nil {
		_ = p.r.Close()
	}
}

func (p *outputPipe) closeAll() {
	p.closeWrite()
	if p.r != nil {
		_ = p.r.Close()
		p.r = nil
	}
}

// startGrace returns a stop func. Once ctx is done it waits cancelGracePeriod
// and then calls closeReads, which unblocks any copy still waiting on a pipe
// held by an orphaned descendant. On the success path it does nothing.
func startGrace(ctx context.Context, closeReads func()) (stop func()) {
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-stopped:
			return
		}
		timer := time.NewTimer(cancelGracePeriod)
		defer timer.Stop()
		select {
		case <-timer.C:
			closeReads()
		case <-stopped:
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stopped) }) }
}

// capture runs a git command and returns its stdout and stderr. With combined
// set, stderr is folded into the same pipe so relative write order is preserved,
// matching exec.Cmd.CombinedOutput.
func (r *Runner) capture(ctx context.Context, repoDir string, combined bool, args ...string) ([]byte, []byte, error) {
	outPipe, err := newOutputPipe()
	if err != nil {
		return nil, nil, err
	}
	defer outPipe.closeAll()

	var errPipe *outputPipe
	if !combined {
		if errPipe, err = newOutputPipe(); err != nil {
			return nil, nil, err
		}
		defer errPipe.closeAll()
	}

	closeReads := func() {
		outPipe.closeRead()
		if errPipe != nil {
			errPipe.closeRead()
		}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	cmd.Stdout = outPipe.w
	if combined {
		cmd.Stderr = outPipe.w
	} else {
		cmd.Stderr = errPipe.w
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	// The child holds its own descriptors now; ours would hide EOF otherwise.
	outPipe.closeWrite()
	if errPipe != nil {
		errPipe.closeWrite()
	}

	var (
		stdoutBuf, stderrBuf bytes.Buffer
		wg                   sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdoutBuf, outPipe.r)
	}()
	if errPipe != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(&stderrBuf, errPipe.r)
		}()
	}

	// Arm the deadline before waiting: either Wait or the copy can be the one
	// held up by an inherited pipe.
	stopGrace := startGrace(ctx, closeReads)

	waitErr := cmd.Wait()
	wg.Wait()
	stopGrace()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), ctxErr
	}
	if waitErr != nil {
		// Mirror exec.Cmd.Output: callers read stderr off the ExitError.
		if exitErr, ok := waitErr.(*exec.ExitError); ok && !combined {
			exitErr.Stderr = stderrBuf.Bytes()
		}
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), waitErr
}

// Run executes a git command and returns the combined stdout+stderr output.
func (r *Runner) Run(ctx context.Context, repoDir string, args ...string) (string, error) {
	if err := r.acquire(ctx); err != nil {
		return "", err
	}
	defer r.release()

	out, _, err := r.capture(ctx, repoDir, true, args...)
	return string(out), err
}

// Output executes a git command and returns stdout only.
func (r *Runner) Output(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	out, _, err := r.capture(ctx, repoDir, false, args...)
	return out, err
}

// RunSplit executes a git command and returns stdout and stderr separately.
func (r *Runner) RunSplit(ctx context.Context, repoDir string, args ...string) (string, string, error) {
	if err := r.acquire(ctx); err != nil {
		return "", "", err
	}
	defer r.release()

	out, errOut, err := r.capture(ctx, repoDir, false, args...)
	return string(out), string(errOut), err
}

// Stream acquires the semaphore, starts a git command, and passes its stdout as
// an io.Reader to consume. The semaphore is held for the full duration.
//
// consume MUST fully drain the stdout reader before returning nil. Returning
// early closes the read end and may surface a broken-pipe error from git.
//
// The stdout read end is closed as soon as consume returns, so a descendant that
// inherited the pipe cannot hold the semaphore past the call.
func (r *Runner) Stream(ctx context.Context, repoDir string, consume func(stdout io.Reader) error, args ...string) error {
	if err := r.acquire(ctx); err != nil {
		return err
	}
	defer r.release()

	outPipe, err := newOutputPipe()
	if err != nil {
		return err
	}
	defer outPipe.closeAll()

	errPipe, err := newOutputPipe()
	if err != nil {
		return err
	}
	defer errPipe.closeAll()

	closeReads := func() {
		outPipe.closeRead()
		errPipe.closeRead()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	cmd.Stdout = outPipe.w
	cmd.Stderr = errPipe.w

	if err := cmd.Start(); err != nil {
		return err
	}
	outPipe.closeWrite()
	errPipe.closeWrite()

	var stderrBuf bytes.Buffer
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		_, _ = io.Copy(&stderrBuf, errPipe.r)
	}()

	// consume runs in its own goroutine so that a read blocked on an inherited
	// pipe can still be unblocked by the grace timer.
	consumeCh := make(chan error, 1)
	go func() { consumeCh <- consume(outPipe.r) }()

	stopGrace := startGrace(ctx, closeReads)

	consumeErr := <-consumeCh
	// The contract says consume drains stdout; closing here releases the child
	// if it did not and never blocks the caller on an unrelated descendant.
	outPipe.closeRead()

	if consumeErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	stderrWG.Wait()
	stopGrace()

	// A cancellation explains any read error it caused, so it takes precedence
	// instead of being masked by the transport error.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if consumeErr != nil {
		return consumeErr
	}
	if waitErr != nil {
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("%w: %s", waitErr, stderrBuf.String())
		}
		return waitErr
	}
	return nil
}
