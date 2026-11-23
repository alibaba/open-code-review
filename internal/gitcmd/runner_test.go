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
	"strings"
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

func TestLocalOnlySharesSlotsAndDisablesFetching(test *testing.T) {
	runner := New(1)
	local := runner.LocalOnly()
	if err := runner.acquire(test.Context()); err != nil {
		test.Fatal(err)
	}
	defer runner.release()
	ctx, cancel := context.WithCancel(test.Context())
	cancel()
	if local.acquire(ctx) == nil {
		test.Fatal("independent Git semaphore")
	}
	env := strings.Join(local.command(test.Context(), ".", "status").Env, "\n")
	if !strings.Contains(env, "GIT_NO_LAZY_FETCH=1") || !strings.Contains(env, "GIT_ALLOW_PROTOCOL=") {
		test.Fatal("local-only Git command permits network access")
	}
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

func TestLocalRepositoryRejectsRedirectsAndDisablesFilters(t *testing.T) {
	repo, other := initRepo(t), initRepo(t)
	runner := New(1)
	if _, err := runner.Run(t.Context(), repo, "config", "filter.fixture.clean", "unavailable-fixture-executable"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("hello.txt filter=fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_CONFIG_COUNT"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, other)
			var view *UnsupportedViewError
			if _, err := runner.LocalForRepository(t.Context(), repo); !errors.As(err, &view) || view.variable != key {
				t.Fatal(err)
			}
		})
	}
	local, err := runner.LocalForRepository(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	gitDir, err := local.Output(t.Context(), repo, "rev-parse", "--absolute-git-dir")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.Stat(strings.TrimSpace(string(gitDir)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(filepath.Join(repo, ".git"))
	if err != nil || !os.SameFile(actual, want) {
		t.Fatalf("wrong Git directory: %q", gitDir)
	}
	for _, check := range []struct {
		args []string
		want string
	}{
		{[]string{"ls-files", "hello.txt"}, "hello.txt"},
		{[]string{"config", "--get", "filter.fixture.clean"}, ""},
		{[]string{"config", "--get", "filter.fixture.required"}, "true"},
	} {
		output, err := local.Output(t.Context(), repo, check.args...)
		if err != nil || strings.TrimSpace(string(output)) != check.want {
			t.Fatalf("%v: %q %v", check.args, output, err)
		}
	}
	if _, err := local.Output(t.Context(), repo, "hash-object", "hello.txt"); err == nil {
		t.Fatal("configured content filter silently skipped")
	}
	reused, err := local.LocalForRepository(t.Context(), repo)
	if err != nil || reused != local || reused.sem != runner.sem {
		t.Fatal("local runner was not shared")
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
