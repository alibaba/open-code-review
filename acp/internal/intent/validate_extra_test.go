// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"os/exec"
	"testing"
)

func TestGitRepoRejectsEmptyRef(t *testing.T) {
	if err := (GitRepo{}).ResolveCommit(context.Background(), ""); err == nil {
		t.Fatal("expected empty ref error")
	}
}

func TestGitRepoResolvesHead(t *testing.T) {
	if err := exec.Command("git", "rev-parse", "--verify", "HEAD^{commit}").Run(); err != nil {
		t.Skip("workspace is not a git checkout")
	}
	if err := (GitRepo{Dir: "../.."}).ResolveCommit(context.Background(), "HEAD"); err != nil {
		t.Fatal(err)
	}
}
