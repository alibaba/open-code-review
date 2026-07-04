package reviewbundle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-code-review/open-code-review/internal/gitcmd"
)

func TestResolveTargetWorkspaceFingerprintsDirtyState(t *testing.T) {
	repository := initTargetRepository(t)
	writeTargetFile(t, repository, "staged.go", "package sample\n")
	runTargetGit(t, repository, "add", "staged.go")
	writeTargetFile(t, repository, "base.go", "package sample\n\nvar changed = true\n")
	writeTargetFile(t, repository, "untracked.go", "package sample\n")

	target, state, err := ResolveTarget(
		context.Background(),
		repository,
		TargetSpec{},
		gitcmd.New(2),
	)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if target.Mode != TargetWorkspace || target.HeadSHA == "" || target.BaseSHA != target.HeadSHA {
		t.Fatalf("unexpected workspace target: %+v", target)
	}
	if state == nil || state.HeadSHA != target.HeadSHA {
		t.Fatalf("unexpected workspace state: %+v", state)
	}
	emptyHash := hashFields()
	for name, value := range map[string]string{
		"staged":    state.StagedSHA256,
		"unstaged":  state.UnstagedSHA256,
		"untracked": state.UntrackedSHA256,
	} {
		if value == "" || value == emptyHash {
			t.Errorf("%s hash was not populated: %q", name, value)
		}
	}
}

func TestResolveTargetRangeUsesMergeBaseAndResolvedHead(t *testing.T) {
	repository := initTargetRepository(t)
	base := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))
	writeTargetFile(t, repository, "range.go", "package sample\n")
	runTargetGit(t, repository, "add", "range.go")
	runTargetGit(t, repository, "commit", "-m", "range")
	head := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))

	target, state, err := ResolveTarget(
		context.Background(),
		repository,
		TargetSpec{From: base, To: head},
		gitcmd.New(2),
	)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if state != nil {
		t.Fatalf("range workspace state = %+v, want nil", state)
	}
	if target.Mode != TargetRange || target.BaseSHA != base ||
		target.MergeBaseSHA != base || target.HeadSHA != head {
		t.Fatalf("unexpected range target: %+v", target)
	}
}

func TestResolveTargetCommitUsesParentAndResolvedCommit(t *testing.T) {
	repository := initTargetRepository(t)
	parent := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))
	writeTargetFile(t, repository, "commit.go", "package sample\n")
	runTargetGit(t, repository, "add", "commit.go")
	runTargetGit(t, repository, "commit", "-m", "commit target")
	head := strings.TrimSpace(runTargetGit(t, repository, "rev-parse", "HEAD"))

	target, state, err := ResolveTarget(
		context.Background(),
		repository,
		TargetSpec{Commit: "HEAD"},
		gitcmd.New(2),
	)
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if state != nil {
		t.Fatalf("commit workspace state = %+v, want nil", state)
	}
	if target.Mode != TargetCommit || target.BaseSHA != parent || target.HeadSHA != head {
		t.Fatalf("unexpected commit target: %+v", target)
	}
}

func TestResolveTargetRejectsOptionLikeRef(t *testing.T) {
	repository := initTargetRepository(t)
	_, _, err := ResolveTarget(
		context.Background(),
		repository,
		TargetSpec{Commit: "--output=/tmp/unsafe"},
		gitcmd.New(2),
	)
	if err == nil || !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("ResolveTarget() error = %v, want option-like ref rejection", err)
	}
}

func initTargetRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runTargetGit(t, repository, "init", "-q")
	runTargetGit(t, repository, "config", "user.email", "tests@example.com")
	runTargetGit(t, repository, "config", "user.name", "OCR Tests")
	runTargetGit(t, repository, "config", "commit.gpgsign", "false")
	writeTargetFile(t, repository, "base.go", "package sample\n")
	runTargetGit(t, repository, "add", "base.go")
	runTargetGit(t, repository, "commit", "-m", "initial")
	return repository
}

func writeTargetFile(t *testing.T, repository, name, content string) {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func runTargetGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}
