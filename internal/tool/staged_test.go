// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestStagedToolsReadFilesRemovedFromWorkingTreeBeforeCapture(t *testing.T) {
	dir := setupTestRepo(t)
	files := []struct {
		name   string
		marker string
		isNew  bool
	}{
		{name: "hello.go", marker: "STAGED_EXISTING_REMOVED"},
		{name: "added.go", marker: "STAGED_ADDED_REMOVED", isNew: true},
	}
	for _, file := range files {
		writeTestFile(t, dir, file.name, "package main\n// "+file.marker+"\n")
	}
	cmd := exec.Command("git", "add", "--", files[0].name, files[1].name)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stage test files: %v: %s", err, out)
	}
	for _, file := range files {
		if err := os.Remove(filepath.Join(dir, file.name)); err != nil {
			t.Fatal(err)
		}
	}
	indexPath := filepath.Join(dir, ".git", "index")
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	snapshot, err := diff.CaptureStagedSnapshot(ctx, dir, gitcmd.New(2))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		runner *gitcmd.Runner
	}{
		{name: "direct"},
		{name: "shared runner", runner: gitcmd.New(2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diffs, err := diff.NewStagedProvider(dir, snapshot, tc.runner).GetDiff(ctx)
			if err != nil || len(diffs) != len(files) {
				t.Fatalf("staged changes disappeared after working-tree removal: %+v, %v", diffs, err)
			}
			byPath := make(map[string]model.Diff)
			for _, d := range diffs {
				byPath[d.NewPath] = d
			}
			fr := &FileReader{RepoDir: dir, Mode: ModeStaged, Ref: snapshot.Tree, Runner: tc.runner}
			for _, file := range files {
				want := "package main\n// " + file.marker + "\n"
				d, ok := byPath[file.name]
				if !ok || d.IsDeleted || d.IsNew != file.isNew || d.NewFileContent != want || !strings.Contains(d.Diff, "+// "+file.marker) {
					t.Errorf("unstaged removal changed staged diff for %q: %+v", file.name, d)
				}
				if content, err := fr.Read(ctx, file.name); err != nil || content != want {
					t.Errorf("read removed file %q: %q, %v", file.name, content, err)
				}
				if lines, total, err := fr.ReadLines(ctx, file.name, 2, 1); err != nil || total != 3 || len(lines) != 1 || lines[0] != "// "+file.marker {
					t.Errorf("read removed file lines %q: %v, total=%d, %v", file.name, lines, total, err)
				}
				if found, err := NewFileFind(fr).Execute(ctx, map[string]any{"query_name": file.name}); err != nil || found != file.name {
					t.Errorf("find removed file %q: %q, %v", file.name, found, err)
				}
				if matches, err := NewCodeSearch(fr).Execute(ctx, map[string]any{"search_text": file.marker}); err != nil || !strings.Contains(matches, file.name) || !strings.Contains(matches, file.marker) {
					t.Errorf("search removed file %q: %q, %v", file.name, matches, err)
				}
			}
		})
	}
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(dir, file.name)); !os.IsNotExist(err) {
			t.Errorf("review restored removed working-tree file %q: %v", file.name, err)
		}
	}
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil || !bytes.Equal(indexBefore, indexAfter) {
		t.Fatalf("review changed the original index: %v", err)
	}
}

func TestStagedToolsKeepFrozenTreeAndAttributes(t *testing.T) {
	dir := setupTestRepo(t)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write("staged file.go", "package main\n// SNAPSHOT_MARKER\n")
	write(".gitattributes", "*.go diff\n")
	git("add", ".")
	ctx := context.Background()
	runner := gitcmd.New(2)
	snapshot, err := diff.CaptureStagedSnapshot(ctx, dir, runner)
	if err != nil {
		t.Fatal(err)
	}
	write("staged file.go", "package main\n// LIVE_MARKER\n")
	write(".gitattributes", "*.go -diff\n")
	write("live.go", "// SNAPSHOT_MARKER\n")
	git("add", ".")
	for _, limiter := range []*gitcmd.Runner{nil, runner} {
		fr := &FileReader{RepoDir: dir, Mode: ModeStaged, Ref: snapshot.Tree, Runner: limiter}
		content, err := fr.Read(ctx, "staged file.go")
		if err != nil || !strings.Contains(content, "SNAPSHOT_MARKER") || strings.Contains(content, "LIVE_MARKER") {
			t.Fatalf("read escaped snapshot: %q, %v", content, err)
		}
		lines, _, err := fr.ReadLines(ctx, "staged file.go", 2, 1)
		if err != nil || len(lines) != 1 || lines[0] != "// SNAPSHOT_MARKER" {
			t.Fatalf("lines: %v, %v", lines, err)
		}
		if _, err := fr.Read(ctx, "live.go"); err == nil {
			t.Fatal("read unstaged context")
		}
		files, err := NewFileFind(fr).Execute(ctx, map[string]any{"query_name": ".go"})
		if err != nil || !strings.Contains(files, "staged file.go") || strings.Contains(files, "live.go") {
			t.Fatalf("file discovery escaped snapshot: %q, %v", files, err)
		}
		matches, err := NewCodeSearch(fr).Execute(ctx, map[string]any{"search_text": "SNAPSHOT_MARKER"})
		if err != nil || !strings.Contains(matches, "SNAPSHOT_MARKER") || strings.Contains(matches, "live.go") || strings.Contains(matches, "Binary file") {
			t.Fatalf("search escaped snapshot or used live attributes: %q, %v", matches, err)
		}
	}
}

func TestStagedToolsNeverFallBackWithoutSnapshot(t *testing.T) {
	fr := &FileReader{RepoDir: setupTestRepo(t), Mode: ModeStaged}
	ctx := context.Background()
	if _, err := fr.Read(ctx, "hello.go"); err == nil {
		t.Fatal("read live file without a snapshot")
	}
	if _, _, err := fr.ReadLines(ctx, "hello.go", 1, 1); err == nil {
		t.Fatal("read live lines without a snapshot")
	}
	if _, err := NewFileFind(fr).Execute(ctx, map[string]any{"query_name": ".go"}); err == nil {
		t.Fatal("searched live paths without a snapshot")
	}
	if _, err := NewCodeSearch(fr).Execute(ctx, map[string]any{"search_text": "package"}); err == nil {
		t.Fatal("searched live contents without a snapshot")
	}
}

func TestStagedToolsOnlyReadAndDiscoverBlobs(t *testing.T) {
	dir := setupTestRepo(t)
	git := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := git("", "rev-parse", "HEAD")
	link := git("hello.go", "hash-object", "-w", "--stdin")
	git("", "update-index", "--add", "--cacheinfo", "160000,"+commit+",submodule.go")
	git("", "update-index", "--add", "--cacheinfo", "120000,"+link+",link.go")
	git("", "update-index", "--add", "--cacheinfo", "100644,"+link+",dir/file.go")
	git("", "commit", "-m", "Add context entries")
	ctx := context.Background()
	snapshot, err := diff.CaptureStagedSnapshot(ctx, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, runner := range []*gitcmd.Runner{nil, gitcmd.New(2)} {
		fr := &FileReader{RepoDir: dir, Mode: ModeStaged, Ref: snapshot.Tree, Runner: runner}
		for _, path := range []string{"submodule.go", "dir"} {
			if content, err := fr.Read(ctx, path); err == nil || content != "" || !strings.Contains(err.Error(), "staged blob") {
				t.Fatalf("read non-blob %q: %q, %v", path, content, err)
			}
			if lines, _, err := fr.ReadLines(ctx, path, 1, 10); err == nil || len(lines) != 0 || !strings.Contains(err.Error(), "staged blob") {
				t.Fatalf("read non-blob lines %q: %v, %v", path, lines, err)
			}
		}
		if content, err := fr.Read(ctx, "link.go"); err != nil || content != "hello.go" {
			t.Fatalf("symlink blob was not preserved: %q, %v", content, err)
		}
		files, err := NewFileFind(fr).Execute(ctx, map[string]any{"query_name": ".go"})
		if err != nil || strings.Contains(files, "submodule.go") || !strings.Contains(files, "link.go") || !strings.Contains(files, "dir/file.go") {
			t.Fatalf("discovered non-blob or lost blob paths: %q, %v", files, err)
		}
	}
}

func TestStagedCodeSearchDoesNotRecurseIntoUnchangedSubmodules(t *testing.T) {
	parent := setupTestRepo(t)
	child := setupTestRepo(t)
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(child, "nested.go", "// SNAPSHOT_SCOPE_SUBMODULE_COMMITTED\n")
	git(child, "add", "nested.go")
	git(child, "commit", "-m", "Add child source")
	git(parent, "-c", "protocol.file.allow=always", "submodule", "add", child, "nested")
	git(parent, "commit", "-am", "Add initialized submodule")
	git(parent, "config", "submodule.recurse", "true")
	write(parent, "staged.go", "// SNAPSHOT_SCOPE_PARENT\n")
	git(parent, "add", "staged.go")
	ctx := context.Background()
	snapshot, err := diff.CaptureStagedSnapshot(ctx, parent, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The gitlink is unchanged and initialized, so Git's user-level recursive
	// search setting would otherwise search the child repository's commit.
	write(filepath.Join(parent, "nested"), "nested.go", "// SNAPSHOT_SCOPE_SUBMODULE_LIVE\n")
	for _, tc := range []struct {
		name   string
		runner *gitcmd.Runner
	}{
		{name: "direct"},
		{name: "shared runner", runner: gitcmd.New(2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fr := &FileReader{RepoDir: parent, Mode: ModeStaged, Ref: snapshot.Tree, Runner: tc.runner}
			search := NewCodeSearch(fr)
			matches, err := search.Execute(ctx, map[string]any{"search_text": "SNAPSHOT_SCOPE_PARENT"})
			if err != nil || !strings.Contains(matches, "SNAPSHOT_SCOPE_PARENT") {
				t.Fatalf("lost parent snapshot match: %q, %v", matches, err)
			}
			for _, marker := range []string{"SNAPSHOT_SCOPE_SUBMODULE_COMMITTED", "SNAPSHOT_SCOPE_SUBMODULE_LIVE"} {
				matches, err := search.Execute(ctx, map[string]any{"search_text": marker})
				if err != nil || matches != "No matches found" {
					t.Errorf("searched outside the parent snapshot's blobs for %q: %q, %v", marker, matches, err)
				}
			}
		})
	}
}
