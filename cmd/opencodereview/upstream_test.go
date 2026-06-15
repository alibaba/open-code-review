package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/diff"
	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

func TestIsUpstreamURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://github.com/o/r", true},
		{"git://example.com/r.git", true},
		{"ssh://git@example.com/r.git", true},
		{"file:///tmp/bare.git", true},
		{"git@github.com:o/r.git", true},
		{"/abs/path/bare.git", true},
		{"./rel/bare.git", true},
		{"../rel/bare.git", true},
		{"upstream", false},
		{"origin", false},
		{"my-remote", false},
	}
	for _, c := range cases {
		if got := isUpstreamURL(c.in); got != c.want {
			t.Errorf("isUpstreamURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseSymrefDefaultBranch(t *testing.T) {
	out := "ref: refs/heads/main\tHEAD\n" +
		"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0\tHEAD\n"
	branch, err := parseSymrefDefaultBranch(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("got %q, want main", branch)
	}
}

func TestParseSymrefDefaultBranchMaster(t *testing.T) {
	out := "ref: refs/heads/master\tHEAD\n0000\tHEAD\n"
	branch, err := parseSymrefDefaultBranch(out)
	if err != nil || branch != "master" {
		t.Fatalf("got %q, err %v; want master", branch, err)
	}
}

func TestParseSymrefDefaultBranchMissing(t *testing.T) {
	if _, err := parseSymrefDefaultBranch("0000\tHEAD\n"); err == nil {
		t.Fatal("expected error when no symref line present")
	}
}

// initUpstreamAndFork builds: a bare "upstream" repo (branch main, one commit),
// and a "fork" clone of it with origin -> bare and a local "feature" branch that
// modifies app.go. Returns the bare repo path and the fork working dir.
func initUpstreamAndFork(t *testing.T) (upstreamBare, fork string) {
	t.Helper()
	root := t.TempDir()
	upstreamWork := filepath.Join(root, "upstream-work")
	upstreamBare = filepath.Join(root, "upstream.git")
	fork = filepath.Join(root, "fork")

	mustGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
		}
	}

	if err := os.MkdirAll(upstreamWork, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(upstreamWork, "init", "-q", "-b", "main")
	mustGit(upstreamWork, "config", "user.email", "up@example.com")
	mustGit(upstreamWork, "config", "user.name", "Up")
	mustGit(upstreamWork, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(upstreamWork, "app.go"),
		[]byte("package app\n\nfunc A() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(upstreamWork, "add", ".")
	mustGit(upstreamWork, "commit", "-q", "-m", "upstream initial")

	mustGit(root, "clone", "-q", "--bare", upstreamWork, upstreamBare)

	mustGit(root, "clone", "-q", upstreamBare, fork)
	mustGit(fork, "config", "user.email", "me@example.com")
	mustGit(fork, "config", "user.name", "Me")
	mustGit(fork, "config", "commit.gpgsign", "false")
	mustGit(fork, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(fork, "app.go"),
		[]byte("package app\n\nfunc A() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(fork, "add", ".")
	mustGit(fork, "commit", "-q", "-m", "my change")

	return upstreamBare, fork
}

// assertReviewsAppChange builds a range provider for from..to and asserts the
// only changed file is app.go.
func assertReviewsAppChange(t *testing.T, repoDir, from, to string) {
	t.Helper()
	runner := gitcmd.New(4)
	p := diff.NewProvider(repoDir, from, to, runner)
	diffs, err := p.GetDiff(context.Background())
	if err != nil {
		t.Fatalf("GetDiff(%s..%s): %v", from, to, err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 changed file, got %d: %+v", len(diffs), diffs)
	}
}

func TestResolveUpstreamRemoteNameFetches(t *testing.T) {
	bare, fork := initUpstreamAndFork(t)
	cmd := exec.Command("git", "remote", "add", "upstream", bare)
	cmd.Dir = fork
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add upstream remote: %v\n%s", err, out)
	}

	from, to, err := resolveUpstream(fork, reviewOptions{upstream: "upstream"})
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	if from != "upstream/main" {
		t.Fatalf("from = %q, want upstream/main", from)
	}
	if to != "HEAD" {
		t.Fatalf("to = %q, want HEAD", to)
	}
	assertReviewsAppChange(t, fork, from, to)
}

func TestResolveUpstreamURLFetches(t *testing.T) {
	bare, fork := initUpstreamAndFork(t)

	from, to, err := resolveUpstream(fork, reviewOptions{upstream: "file://" + bare})
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(from) {
		t.Fatalf("from = %q, want a 40-hex SHA", from)
	}
	if to != "HEAD" {
		t.Fatalf("to = %q, want HEAD", to)
	}
	assertReviewsAppChange(t, fork, from, to)
}

func TestResolveUpstreamNoFetchUsesLocal(t *testing.T) {
	_, fork := initUpstreamAndFork(t)
	// origin/main + origin/HEAD already exist from the clone; no network needed.
	from, to, err := resolveUpstream(fork, reviewOptions{upstream: "origin", noFetch: true})
	if err != nil {
		t.Fatalf("resolveUpstream: %v", err)
	}
	if from != "origin/main" || to != "HEAD" {
		t.Fatalf("got from=%q to=%q, want origin/main HEAD", from, to)
	}
}

func TestResolveUpstreamUnknownRemote(t *testing.T) {
	_, fork := initUpstreamAndFork(t)
	_, _, err := resolveUpstream(fork, reviewOptions{upstream: "nope"})
	if err == nil || !strings.Contains(err.Error(), "no git remote named") {
		t.Fatalf("expected unknown-remote error, got: %v", err)
	}
}

func TestResolveUpstreamNoFetchRejectsURL(t *testing.T) {
	bare, fork := initUpstreamAndFork(t)
	_, _, err := resolveUpstream(fork, reviewOptions{upstream: "file://" + bare, noFetch: true})
	if err == nil || !strings.Contains(err.Error(), "--no-fetch") {
		t.Fatalf("expected --no-fetch/URL error, got: %v", err)
	}
}
