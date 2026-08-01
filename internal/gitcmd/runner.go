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

// Run executes a git command and returns the combined stdout+stderr output.
func (r *Runner) Run(ctx context.Context, repoDir string, args ...string) (string, error) {
	if err := r.acquire(ctx); err != nil {
		return "", err
	}
	defer r.release()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
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
	return cmd.Output()
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
