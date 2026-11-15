// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package gitcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "hello.txt")
	run("commit", "-m", "init")
	return dir
}

func TestRunner_New(t *testing.T) {
	r := New(0)
	if r == nil {
		t.Fatal("New(0) returned nil")
	}
	if cap(r.sem) != defaultMaxConcurrent {
		t.Errorf("default capacity = %d, want %d", cap(r.sem), defaultMaxConcurrent)
	}

	r2 := New(4)
	if cap(r2.sem) != 4 {
		t.Errorf("capacity = %d, want 4", cap(r2.sem))
	}
}

func TestRunner_Run(t *testing.T) {
	dir := initRepo(t)
	r := New(2)

	out, err := r.Run(context.Background(), dir, "log", "--oneline")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !strings.Contains(out, "init") {
		t.Errorf("expected 'init' in output: %q", out)
	}
}

func TestRunner_Run_InvalidCommand(t *testing.T) {
	dir := initRepo(t)
	r := New(2)

	_, err := r.Run(context.Background(), dir, "nonexistent-subcommand")
	if err == nil {
		t.Error("expected error for invalid git subcommand")
	}
}

func TestRunner_Output(t *testing.T) {
	dir := initRepo(t)
	r := New(2)

	out, err := r.Output(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("Output error: %v", err)
	}
	hash := strings.TrimSpace(string(out))
	if len(hash) != 40 {
		t.Errorf("expected 40-char hash, got %q", hash)
	}
}

func TestRunner_RunSplit(t *testing.T) {
	dir := initRepo(t)
	r := New(2)

	stdout, stderr, err := r.RunSplit(context.Background(), dir, "status", "--short")
	if err != nil {
		t.Fatalf("RunSplit error: %v", err)
	}
	_ = stderr
	if strings.Contains(stdout, "??") {
		t.Errorf("unexpected untracked files in clean repo: %q", stdout)
	}
}

func TestRunner_Stream(t *testing.T) {
	dir := initRepo(t)
	r := New(2)

	var content string
	err := r.Stream(context.Background(), dir, func(stdout io.Reader) error {
		data, err := io.ReadAll(stdout)
		if err != nil {
			return err
		}
		content = string(data)
		return nil
	}, "show", "HEAD:hello.txt")
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	if content != "hello\n" {
		t.Errorf("Stream content = %q, want %q", content, "hello\n")
	}
}

func TestRunner_ContextCancelled(t *testing.T) {
	r := New(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Run(ctx, ".", "status")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestRunner_AcquireTimeout(t *testing.T) {
	r := New(1)
	r.sem <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := r.Run(ctx, ".", "status")
	if err == nil {
		t.Error("expected timeout error when semaphore full")
	}
}

// On Windows, cmdBody must use only cmd.exe builtins so TerminateProcess leaves
// no grandchild holding stdout or stderr pipes open.
func shimGit(t *testing.T, shBody, cmdBody string) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		normalized := strings.ReplaceAll(cmdBody, "\r\n", "\n")
		normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
		content := "@echo off\r\n" + normalized
		if !strings.HasSuffix(content, "\r\n") {
			content += "\r\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "git.cmd"), []byte(content), 0o644); err != nil {
			t.Fatalf("write git.cmd shim: %v", err)
		}
		pathExt := os.Getenv("PATHEXT")
		hasCmd := false
		for _, ext := range strings.Split(pathExt, string(os.PathListSeparator)) {
			if strings.EqualFold(strings.TrimSpace(ext), ".cmd") {
				hasCmd = true
				break
			}
		}
		if !hasCmd {
			if pathExt == "" {
				t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
			} else {
				t.Setenv("PATHEXT", pathExt+string(os.PathListSeparator)+".CMD")
			}
		}
	} else {
		if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"+shBody), 0o755); err != nil {
			t.Fatalf("write git shim: %v", err)
		}
	}

	path := os.Getenv("PATH")
	if path == "" {
		t.Setenv("PATH", dir)
	} else {
		t.Setenv("PATH", dir+string(os.PathListSeparator)+path)
	}
}

func waitForMarkerAndCancel(t *testing.T, markerPath string, cancel context.CancelFunc) {
	t.Helper()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, err := os.Stat(markerPath); err == nil {
				cancel()
				return
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
	})
}

func assertMethodsRejected(t *testing.T, r *Runner, ctx context.Context, repoDir string, iterations int, checkErr func(t *testing.T, err error)) {
	t.Helper()
	methods := []struct {
		name string
		call func(t *testing.T) error
	}{
		{
			name: "Run",
			call: func(t *testing.T) error {
				out, err := r.Run(ctx, repoDir, "status")
				if out != "" {
					t.Errorf("Run output = %q, want empty", out)
				}
				return err
			},
		},
		{
			name: "Output",
			call: func(t *testing.T) error {
				out, err := r.Output(ctx, repoDir, "status")
				if out != nil {
					t.Errorf("Output = %q, want nil", out)
				}
				return err
			},
		},
		{
			name: "RunSplit",
			call: func(t *testing.T) error {
				stdout, stderr, err := r.RunSplit(ctx, repoDir, "status")
				if stdout != "" || stderr != "" {
					t.Errorf("RunSplit outputs = (%q, %q), want empty", stdout, stderr)
				}
				return err
			},
		},
		{
			name: "Stream",
			call: func(t *testing.T) error {
				consumed := false
				err := r.Stream(ctx, repoDir, func(stdout io.Reader) error {
					consumed = true
					_, _ = io.ReadAll(stdout)
					return nil
				}, "status")
				if consumed {
					t.Error("Stream invoked consume callback unexpectedly")
				}
				return err
			},
		},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			for i := 0; i < iterations; i++ {
				checkErr(t, m.call(t))
			}
		})
	}
}

func assertNormalSuccess(t *testing.T, r *Runner) {
	t.Helper()
	dir := initRepo(t)
	ctx := context.Background()

	t.Run("Run", func(t *testing.T) {
		out, err := r.Run(ctx, dir, "log", "--oneline")
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
		if !strings.Contains(out, "init") {
			t.Errorf("expected 'init' in output: %q", out)
		}
	})

	t.Run("Output", func(t *testing.T) {
		out, err := r.Output(ctx, dir, "rev-parse", "HEAD")
		if err != nil {
			t.Fatalf("Output error: %v", err)
		}
		if hash := strings.TrimSpace(string(out)); len(hash) != 40 {
			t.Errorf("expected 40-char hash, got %q", hash)
		}
	})

	t.Run("RunSplit", func(t *testing.T) {
		stdout, _, err := r.RunSplit(ctx, dir, "status", "--short")
		if err != nil {
			t.Fatalf("RunSplit error: %v", err)
		}
		if strings.Contains(stdout, "??") {
			t.Errorf("unexpected untracked files in clean repo: %q", stdout)
		}
	})

	t.Run("Stream", func(t *testing.T) {
		var content string
		err := r.Stream(ctx, dir, func(stdout io.Reader) error {
			data, err := io.ReadAll(stdout)
			if err != nil {
				return err
			}
			content = string(data)
			return nil
		}, "show", "HEAD:hello.txt")
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}
		if content != "hello\n" {
			t.Errorf("Stream content = %q, want %q", content, "hello\n")
		}
	})
}

func assertNormalFailure(t *testing.T, r *Runner) {
	t.Helper()
	setup := func(t *testing.T) string {
		t.Helper()
		shimGit(t,
			"printf 'partial-stdout\\n'\nprintf 'fatal: simulated failure\\n' >&2\nexit 128\n",
			"echo partial-stdout\r\necho fatal: simulated failure>&2\r\nexit /b 128\r\n",
		)
		return t.TempDir()
	}
	checkExit128 := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error for non-zero exit, got nil")
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err = %T (%v), want *exec.ExitError", err, err)
		}
		if exitErr.ExitCode() != 128 {
			t.Errorf("ExitCode() = %d, want 128", exitErr.ExitCode())
		}
	}

	t.Run("RunSplit", func(t *testing.T) {
		repoDir := setup(t)
		stdout, stderr, err := r.RunSplit(context.Background(), repoDir, "status")
		checkExit128(t, err)
		if strings.TrimSpace(stdout) != "partial-stdout" {
			t.Errorf("stdout = %q, want %q", stdout, "partial-stdout")
		}
		if strings.TrimSpace(stderr) != "fatal: simulated failure" {
			t.Errorf("stderr = %q, want %q", stderr, "fatal: simulated failure")
		}
	})

	t.Run("Run", func(t *testing.T) {
		repoDir := setup(t)
		_, err := r.Run(context.Background(), repoDir, "status")
		checkExit128(t, err)
	})

	t.Run("Output", func(t *testing.T) {
		repoDir := setup(t)
		_, err := r.Output(context.Background(), repoDir, "status")
		checkExit128(t, err)
	})

	t.Run("Stream", func(t *testing.T) {
		repoDir := setup(t)
		err := r.Stream(context.Background(), repoDir, func(stdout io.Reader) error {
			_, readErr := io.ReadAll(stdout)
			return readErr
		}, "status")
		checkExit128(t, err)
		if err != nil && !strings.Contains(err.Error(), "fatal: simulated failure") {
			t.Errorf("Stream error = %q, want stderr preserved", err)
		}
	})
}

func assertPreCancelled(t *testing.T, r *Runner, iterations int) {
	t.Helper()
	checkCanceled := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
		}
	}

	t.Run("does not spawn git", func(t *testing.T) {
		markerPath := filepath.Join(t.TempDir(), "invoked")
		t.Setenv("OCR_TEST_MARKER", markerPath)
		shimGit(t,
			"printf 'invoked\\n' > \"$OCR_TEST_MARKER\"\n",
			"echo invoked>\"%OCR_TEST_MARKER%\"\r\n",
		)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		assertMethodsRejected(t, r, ctx, t.TempDir(), iterations, checkCanceled)

		if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected git not to be spawned, marker stat err = %v", err)
		}
	})

	t.Run("without git on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		assertMethodsRejected(t, r, ctx, t.TempDir(), iterations, checkCanceled)
	})
}

func assertMidExecutionTerminated(t *testing.T, r *Runner, wantErr error, setup func(t *testing.T) (context.Context, string)) {
	t.Helper()

	t.Run("RunSplit", func(t *testing.T) {
		ctx, repoDir := setup(t)
		stdout, stderr, err := r.RunSplit(ctx, repoDir, "status")
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want errors.Is(err, %v)", err, wantErr)
		}
		if stdout != "" || stderr != "" {
			t.Errorf("RunSplit outputs = (%q, %q), want empty on %v", stdout, stderr, wantErr)
		}
	})

	t.Run("Run", func(t *testing.T) {
		ctx, repoDir := setup(t)
		out, err := r.Run(ctx, repoDir, "status")
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want errors.Is(err, %v)", err, wantErr)
		}
		if out != "" {
			t.Errorf("Run output = %q, want empty on %v", out, wantErr)
		}
	})

	t.Run("Output", func(t *testing.T) {
		ctx, repoDir := setup(t)
		out, err := r.Output(ctx, repoDir, "status")
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want errors.Is(err, %v)", err, wantErr)
		}
		if out != nil {
			t.Errorf("Output = %q, want nil on %v", out, wantErr)
		}
	})

	t.Run("Stream", func(t *testing.T) {
		ctx, repoDir := setup(t)
		err := r.Stream(ctx, repoDir, func(stdout io.Reader) error {
			_, readErr := io.ReadAll(stdout)
			return readErr
		}, "status")
		if !errors.Is(err, wantErr) {
			t.Errorf("err = %v, want errors.Is(err, %v)", err, wantErr)
		}
		if err != nil && strings.Contains(err.Error(), "secret-stderr") {
			t.Errorf("Stream error leaked stderr: %v", err)
		}
	})
}

func assertMidExecutionCanceled(t *testing.T, r *Runner) {
	t.Helper()
	assertMidExecutionTerminated(t, r, context.Canceled, func(t *testing.T) (context.Context, string) {
		t.Helper()
		markerPath := filepath.Join(t.TempDir(), "ready")
		t.Setenv("OCR_TEST_MARKER", markerPath)
		shimGit(t,
			"printf 'secret-stdout\\n'\nprintf 'secret-stderr\\n' >&2\nprintf 'ready\\n' > \"$OCR_TEST_MARKER\"\nwhile :; do :; done\n",
			"echo secret-stdout\r\necho secret-stderr>&2\r\necho ready>\"%OCR_TEST_MARKER%\"\r\n:loop\r\ngoto loop\r\n",
		)
		baseCtx, baseCancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(baseCancel)
		ctx, cancel := context.WithCancel(baseCtx)
		t.Cleanup(cancel)
		waitForMarkerAndCancel(t, markerPath, cancel)
		return ctx, t.TempDir()
	})
}

func assertMidExecutionDeadlineExceeded(t *testing.T, r *Runner) {
	t.Helper()
	assertMidExecutionTerminated(t, r, context.DeadlineExceeded, func(t *testing.T) (context.Context, string) {
		t.Helper()
		shimGit(t,
			"printf 'secret-stdout\\n'\nprintf 'secret-stderr\\n' >&2\nwhile :; do :; done\n",
			"echo secret-stdout\r\necho secret-stderr>&2\r\n:loop\r\ngoto loop\r\n",
		)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		t.Cleanup(cancel)
		return ctx, t.TempDir()
	})
}

func TestRunner_Uninitialized(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "invoked")
	t.Setenv("OCR_TEST_MARKER", markerPath)
	shimGit(t,
		"printf 'invoked\\n' > \"$OCR_TEST_MARKER\"\n",
		"echo invoked>\"%OCR_TEST_MARKER%\"\r\n",
	)

	assertMethodsRejected(t, &Runner{}, context.Background(), t.TempDir(), 1, func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error from uninitialized Runner, got nil")
		}
	})

	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected git not to be invoked, marker stat err = %v", err)
	}
}

func TestRunner_NormalFailure(t *testing.T) {
	assertNormalFailure(t, New(1))
}

func TestRunner_PreCancelled(t *testing.T) {
	assertPreCancelled(t, New(1), 20)
}

func TestRunner_ContextCanceledMidExecution(t *testing.T) {
	assertMidExecutionCanceled(t, New(1))
}

func TestRunner_ContextDeadlineExceededMidExecution(t *testing.T) {
	assertMidExecutionDeadlineExceeded(t, New(1))
}

func TestRunner_NilRunner(t *testing.T) {
	var nilRunner *Runner

	t.Run("normal success", func(t *testing.T) {
		assertNormalSuccess(t, nilRunner)
	})

	t.Run("normal failure", func(t *testing.T) {
		assertNormalFailure(t, nilRunner)
	})

	t.Run("pre-cancelled", func(t *testing.T) {
		assertPreCancelled(t, nilRunner, 1)
	})

	t.Run("mid-execution canceled", func(t *testing.T) {
		assertMidExecutionCanceled(t, nilRunner)
	})

	t.Run("mid-execution deadline exceeded", func(t *testing.T) {
		assertMidExecutionDeadlineExceeded(t, nilRunner)
	})
}
