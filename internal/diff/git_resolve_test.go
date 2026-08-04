package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOut runs a git command in dir and returns its trimmed stdout, failing the
// test on error. Used to capture the SHAs a ResolveInput result must match.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// initBareRepo creates an empty (unborn) git repository with identity configured
// but no commits, returning the repo dir.
func initBareRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitTest(t, repo, "init", "-q")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")
	runGitTest(t, repo, "config", "commit.gpgsign", "false")
	return repo
}

// writeCommit writes content to name, stages it, and commits with msg.
func writeCommit(t *testing.T, repo, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runGitTest(t, repo, "add", name)
	runGitTest(t, repo, "commit", "-q", "-m", msg)
}

func TestResolveInput_Range(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "base")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	// Capture the default branch name (master or main, git-version dependent).
	baseBranch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")
	runGitTest(t, repo, "checkout", "-q", "-b", "feature")
	writeCommit(t, repo, "a.txt", "one\ntwo\n", "feature change")
	head := gitOut(t, repo, "rev-parse", "HEAD")

	p := NewProvider(repo, baseBranch, "feature", nil)
	got := p.ResolveInput(context.Background())
	if got.ResolvedBase != base {
		t.Errorf("ResolvedBase = %q, want merge-base %q", got.ResolvedBase, base)
	}
	if got.ResolvedHead != head {
		t.Errorf("ResolvedHead = %q, want %q", got.ResolvedHead, head)
	}
	if want := base + ".." + head; got.ExactRange != want {
		t.Errorf("ExactRange = %q, want %q", got.ExactRange, want)
	}
}

func TestResolveInput_SingleParentCommit(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	parent := gitOut(t, repo, "rev-parse", "HEAD")
	writeCommit(t, repo, "a.txt", "one\ntwo\n", "second")
	head := gitOut(t, repo, "rev-parse", "HEAD")

	p := NewCommitProvider(repo, head, nil)
	got := p.ResolveInput(context.Background())
	if got.ResolvedHead != head {
		t.Errorf("ResolvedHead = %q, want %q", got.ResolvedHead, head)
	}
	if got.ResolvedBase != parent {
		t.Errorf("ResolvedBase = %q, want parent %q", got.ResolvedBase, parent)
	}
	if want := parent + ".." + head; got.ExactRange != want {
		t.Errorf("ExactRange = %q, want %q", got.ExactRange, want)
	}
}

func TestResolveInput_RootCommit(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	head := gitOut(t, repo, "rev-parse", "HEAD")

	p := NewCommitProvider(repo, head, nil)
	got := p.ResolveInput(context.Background())
	if got.ResolvedHead != head {
		t.Errorf("ResolvedHead = %q, want %q", got.ResolvedHead, head)
	}
	if got.ResolvedBase != "" {
		t.Errorf("ResolvedBase = %q, want empty for a root commit", got.ResolvedBase)
	}
	if got.ExactRange != "" {
		t.Errorf("ExactRange = %q, want empty for a root commit", got.ExactRange)
	}
}

func TestResolveInput_MergeCommit(t *testing.T) {
	repo := initBareRepo(t)
	writeCommit(t, repo, "a.txt", "one\n", "root")
	root := gitOut(t, repo, "rev-parse", "HEAD")
	// Determine the current branch name (main or master).
	mainBranch := gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	runGitTest(t, repo, "checkout", "-q", "-b", "side")
	writeCommit(t, repo, "b.txt", "side\n", "side change")

	runGitTest(t, repo, "checkout", "-q", mainBranch)
	writeCommit(t, repo, "c.txt", "mainline\n", "main change")
	firstParent := gitOut(t, repo, "rev-parse", "HEAD")
	runGitTest(t, repo, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	merge := gitOut(t, repo, "rev-parse", "HEAD")

	if merge == root {
		t.Fatal("merge commit setup failed: HEAD did not advance")
	}

	p := NewCommitProvider(repo, merge, nil)
	got := p.ResolveInput(context.Background())
	if got.ResolvedHead != merge {
		t.Errorf("ResolvedHead = %q, want %q", got.ResolvedHead, merge)
	}
	if got.ResolvedBase != firstParent {
		t.Errorf("ResolvedBase = %q, want first parent %q", got.ResolvedBase, firstParent)
	}
	if want := firstParent + ".." + merge; got.ExactRange != want {
		t.Errorf("ExactRange = %q, want %q", got.ExactRange, want)
	}
}

func TestResolveInput_Workspace(t *testing.T) {
	repo := initRepoWithChange(t) // one commit + working-tree change
	head := gitOut(t, repo, "rev-parse", "HEAD")

	p := NewWorkspaceProvider(repo, nil)
	got := p.ResolveInput(context.Background())
	if got.ResolvedBase != head {
		t.Errorf("ResolvedBase = %q, want HEAD %q", got.ResolvedBase, head)
	}
	if got.ResolvedHead != "" {
		t.Errorf("ResolvedHead = %q, want empty (workspace has no immutable head)", got.ResolvedHead)
	}
	if got.ExactRange != "" {
		t.Errorf("ExactRange = %q, want empty for workspace", got.ExactRange)
	}
}

func TestCanonicalRemote(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"https", "https://github.com/org/repo.git", "github.com/org/repo"},
		{"https creds stripped", "https://user:token@github.com/org/repo.git", "github.com/org/repo"},
		{"https query fragment", "https://github.com/org/repo.git?ref=x#frag", "github.com/org/repo"},
		{"scp", "git@github.com:org/repo.git", "github.com/org/repo"},
		{"scp no user", "github.com:org/repo", "github.com/org/repo"},
		{"host uppercased", "https://GitHub.com/Org/Repo.git", "github.com/Org/Repo"},
		{"trailing slash", "https://github.com/org/repo/", "github.com/org/repo"},
		// B1: a port distinguishes endpoints and must survive canonicalization.
		{"https port kept", "https://example.com:8443/org/repo.git", "example.com:8443/org/repo"},
		{"ssh port kept", "ssh://git@example.com:2222/org/repo.git", "example.com:2222/org/repo"},
		// B2: an "@" inside the path must not be truncated as scp userinfo.
		{"scp at in path", "git@host.com:a/b@c.git", "host.com/a/b@c"},
		// B3: local remotes have no stable network identity → omitted.
		{"local absolute", "/srv/git/repo.git", ""},
		{"local relative", "../peer/repo.git", ""},
		{"file scheme", "file:///srv/git/repo.git", ""},
		{"windows drive", `C:\repos\thing.git`, ""},
		{"unc share", `\\server\share\repo.git`, ""},
		{"empty", "", ""},
		{"whitespace", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalRemote(tc.in); got != tc.want {
				t.Errorf("canonicalRemote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRemoteIdentity(t *testing.T) {
	repo := initBareRepo(t)
	// No origin remote yet: identity is empty.
	p := NewWorkspaceProvider(repo, nil)
	if got := p.RemoteIdentity(context.Background()); got != "" {
		t.Errorf("RemoteIdentity without origin = %q, want empty", got)
	}
	// Add an origin with an embedded credential; identity must be credential-free.
	runGitTest(t, repo, "remote", "add", "origin", "https://user:secret@example.com/acme/widget.git")
	if got := p.RemoteIdentity(context.Background()); got != "example.com/acme/widget" {
		t.Errorf("RemoteIdentity = %q, want %q", got, "example.com/acme/widget")
	}
}

func TestResolveInput_UnbornWorkspace(t *testing.T) {
	repo := initBareRepo(t) // no commits: HEAD is unborn

	p := NewWorkspaceProvider(repo, nil)
	got := p.ResolveInput(context.Background())
	if got.ResolvedBase != "" {
		t.Errorf("ResolvedBase = %q, want empty for an unborn repository", got.ResolvedBase)
	}
	if got.ResolvedHead != "" {
		t.Errorf("ResolvedHead = %q, want empty", got.ResolvedHead)
	}
	if got.ExactRange != "" {
		t.Errorf("ExactRange = %q, want empty", got.ExactRange)
	}
}
