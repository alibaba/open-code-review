// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package lore

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func TestWorkspaceFileBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.go"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := New(root, ".")
	for _, name := range []string{"../outside", ".lore/state", "nested/.lore/config", "/outside", ".", `nested\..\outside`} {
		if err := repo.ReadFile(t.Context(), vcs.ContentRef{}, name, func(io.Reader) error { t.Fatal("unexpected read"); return nil }); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if err := New(root, ".lore").ReadFile(t.Context(), vcs.ContentRef{}, "state", func(io.Reader) error { return nil }); err == nil {
		t.Fatal("accepted an administrative scope")
	}
	sentinel := errors.New("stop")
	if err := repo.ReadFile(t.Context(), vcs.ContentRef{}, "ok.go", func(io.Reader) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := repo.ReadFile(ctx, vcs.ContentRef{}, "ok.go", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := repo.ResolveReview(t.Context(), vcs.ReviewSpec{Mode: vcs.MergeBase, Base: "1", Head: "2"}); !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestOfflineRepository(t *testing.T) {
	binary, err := exec.LookPath("lore")
	if err != nil {
		t.Skip("install Lore CLI to run the offline integration test")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), binary, append([]string{"--offline", "--non-interactive"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("lore %v: %v: %s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("repository", "create", "review-test")
	repo := New(root, ".")
	changes := func(spec vcs.ReviewSpec) map[string]model.Diff {
		t.Helper()
		rev, err := repo.ResolveReview(t.Context(), spec)
		if err != nil {
			t.Fatal(err)
		}
		set, err := repo.Changes(t.Context(), rev)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]model.Diff{}
		for _, d := range set.Included {
			p := d.NewPath
			if d.IsDeleted {
				p = d.OldPath
			}
			out[p] = d
		}
		return out
	}
	workspace := vcs.ReviewSpec{Mode: vcs.Workspace}
	write("main.go", "package first\n")
	if d := changes(workspace)["main.go"]; !d.IsNew || d.Insertions != 1 {
		t.Fatalf("unborn: %+v", d)
	}
	run("stage", "--scan", ".")
	run("commit", "first")
	write("main.go", "package second\n")
	write("empty.go", "")
	run("stage", "--scan", ".")
	run("commit", "second")
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	write("local.go", "package local\n")
	ws := changes(workspace)
	if !ws["main.go"].IsDeleted || !ws["local.go"].IsNew {
		t.Fatalf("workspace: %+v", ws)
	}
	for _, spec := range []vcs.ReviewSpec{{Mode: vcs.Commit, Head: "2"}, {Mode: vcs.TwoRevisions, Base: "1", Head: "2"}} {
		got := changes(spec)
		if d := got["main.go"]; d.Insertions != 1 || d.Deletions != 1 || d.NewFileContent != "package second\n" {
			t.Fatalf("%s: %+v", spec.Mode, d)
		}
		if !got["empty.go"].IsNew {
			t.Fatalf("missing empty addition: %+v", got)
		}
	}
	rootCommit := changes(vcs.ReviewSpec{Mode: vcs.Commit, Head: "1"})
	if d := rootCommit["main.go"]; !d.IsNew || !strings.Contains(d.Diff, "+package first") {
		t.Fatalf("first revision: %+v", d)
	}
	files, err := repo.ListFiles(t.Context(), vcs.ContentRef{})
	if err != nil || !slices.Contains(files, "local.go") || slices.Contains(files, "main.go") {
		t.Fatalf("workspace files: %v, %v", files, err)
	}
	if err := repo.ReadFile(t.Context(), vcs.ContentRef{Revision: "1"}, "local.go", func(io.Reader) error { return nil }); err == nil {
		t.Fatal("historical read returned a workspace-only file")
	}
}
