// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

const defaultMaxConcurrent = 16

// Runner limits the number of concurrent git subprocesses via an internal
// semaphore. All git command invocations should go through a shared Runner
// instance so that the total system-wide subprocess count stays bounded.
// A nil *Runner runs without concurrency limits; a zero-value &Runner{} is invalid.
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
	if r == nil {
		return ctx.Err()
	}
	if r.sem == nil {
		return fmt.Errorf("gitcmd.Runner not initialized; use gitcmd.New()")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case r.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runner) release() {
	if r == nil {
		return
	}
	<-r.sem
}

// cancelledErr normalizes command errors under a cancelled context to ctx.Err()
// because TerminateProcess on Windows exits with status 1 instead of a signal.
func cancelledErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// Run executes a git command and returns the combined stdout+stderr output.
func (r *Runner) Run(ctx context.Context, repoDir string, args ...string) (string, error) {
	if err := r.acquire(ctx); err != nil {
		return "", err
	}
	defer r.release()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if cErr := cancelledErr(ctx, err); cErr != nil {
		return "", cErr
	}
	return string(out), err
}

// Output executes a git command and returns stdout only.
func (r *Runner) Output(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if cErr := cancelledErr(ctx, err); cErr != nil {
		return nil, cErr
	}
	return out, err
}

// RunSplit executes a git command and returns stdout and stderr separately.
func (r *Runner) RunSplit(ctx context.Context, repoDir string, args ...string) (string, string, error) {
	if err := r.acquire(ctx); err != nil {
		return "", "", err
	}
	defer r.release()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if cErr := cancelledErr(ctx, err); cErr != nil {
		return "", "", cErr
	}
	return stdout.String(), stderr.String(), err
}

// Stream acquires the semaphore, starts a git command, and passes its stdout
// as an io.Reader to consume. The semaphore is held for the full duration.
// consume MUST fully drain the stdout reader before returning nil;
// otherwise cmd.Wait() may block or return a broken-pipe error.
func (r *Runner) Stream(ctx context.Context, repoDir string, consume func(stdout io.Reader) error, args ...string) error {
	if err := r.acquire(ctx); err != nil {
		return err
	}
	defer r.release()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir

	err := streamCommand(cmd, consume)
	if cErr := cancelledErr(ctx, err); cErr != nil {
		return cErr
	}
	return err
}

func streamCommand(cmd *exec.Cmd, consume func(stdout io.Reader) error) error {
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	consumeErr := consume(stdoutPipe)
	if consumeErr != nil {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()

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
