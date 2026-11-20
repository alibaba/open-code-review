// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
)

func TestExactPathspecsRespectArgumentBudget(test *testing.T) {
	groups := [][]string{{"parent", "parent/keep"}}
	for index := range 1000 {
		groups = append(groups, []string{fmt.Sprintf("parent/sibling-[%d]", index)})
	}
	batches, err := pathBatches(groups, exactFilePathspec)
	if err != nil || len(batches) < 2 {
		test.Fatalf("batches=%d err=%v", len(batches), err)
	}
	for _, args := range batches {
		if len(strings.Join(args, "\x00"))+1 > scopedPathBytes {
			test.Fatal("oversized Git argument list")
		}
	}
}

func TestPrepareRejectsWorkspace(test *testing.T) {
	input := Input{Mode: "workspace", Resolution: diff.InputResolution{ResolvedBase: strings.Repeat("a", 40)}}
	if _, err := PrepareInput(test.Context(), input); err == nil || !strings.Contains(err.Error(), "commit or range") {
		test.Fatalf("expected unsupported comparison, got %v", err)
	}
}

func TestPrepareFixedInput(test *testing.T) {
	input := fixture(test)
	if err := os.WriteFile(filepath.Join(input.RepoDir, "added.go"), []byte("new source\n"), 0600); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input.RepoDir, ".env"), []byte("NEW SECRET\n"), 0600); err != nil {
		test.Fatal(err)
	}
	git(test, input.RepoDir, "add", ".")
	git(test, input.RepoDir, "commit", "-qm", "Add source and secret")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(input.RepoDir, ".gitattributes"), []byte("added.go -diff\n"), 0600); err != nil {
		test.Fatal(err)
	}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	if len(prepared.Diffs) != 5 || len(code.diffs) != 4 {
		test.Fatalf("changes=%+v visible=%d", prepared.Diffs, len(code.diffs))
	}
	for _, changed := range prepared.Diffs {
		if changed.NewFileContent != "" {
			test.Fatalf("retained full contents of %s", changed.NewPath)
		}
		if changed.NewPath == ".env" && changed.Diff != "" {
			test.Fatal("secret diff was loaded")
		}
		if changed.NewPath != ".env" && changed.Diff == "" {
			test.Fatalf("missing diff: %+v", changed)
		}
	}
	if !code.diffs["added.go"].IsNew || !code.diffs["deleted.go"].IsDeleted || !code.diffs["renamed.go"].IsRenamed {
		test.Fatalf("change kinds: %+v", code.diffs)
	}
	input.Resolution.ResolvedHead = strings.Repeat("f", 40)
	if _, err := PrepareInput(test.Context(), input); err == nil {
		test.Fatal("missing revision accepted")
	}
	ctx, cancel := context.WithCancel(test.Context())
	cancel()
	if _, err := PrepareInput(ctx, prepared); err == nil {
		test.Fatal("canceled prepare succeeded")
	}
}

func TestPatchPathSelectionIsExactAcrossTreeShapes(test *testing.T) {
	repo := test.TempDir()
	git(test, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "item"), []byte("before\n"), 0600); err != nil {
		test.Fatal(err)
	}
	git(test, repo, "add", ".")
	git(test, repo, "commit", "-qm", "Flat item")
	git(test, repo, "rm", "item")
	if err := os.Mkdir(filepath.Join(repo, "item"), 0700); err != nil {
		test.Fatal(err)
	}
	for _, name := range []string{"first", "second", "[ab]", "a"} {
		if err := os.WriteFile(filepath.Join(repo, "item", name), []byte(name), 0600); err != nil {
			test.Fatal(err)
		}
	}
	git(test, repo, "add", ".")
	git(test, repo, "commit", "-qm", "Nested items")
	for _, paths := range [][]string{{"item"}, {"item", "item/first"}, {"item/[ab]"}} {
		var batch []string
		for _, name := range paths {
			batch = append(batch, exactFilePathspec(name))
		}
		args := []string{"diff", "--name-only", "--no-renames", "HEAD~1", "HEAD", "--"}
		got := git(test, repo, append(args, batch...)...)
		if got != strings.Join(paths, "\n") {
			test.Fatalf("selected %v, got %q", paths, got)
		}
	}
}

func TestPrepareRootAndEmptyRange(test *testing.T) {
	input := fixture(test)
	input.Mode = "commit"
	input.Resolution = diff.InputResolution{ResolvedHead: input.Resolution.ResolvedBase, BaseIsEmpty: true}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	for _, changed := range prepared.Diffs {
		if !changed.IsNew {
			test.Fatalf("root change: %+v", changed)
		}
	}
	input.Mode = "range"
	input.Resolution = diff.InputResolution{ResolvedBase: input.Resolution.ResolvedHead, ResolvedHead: input.Resolution.ResolvedHead}
	prepared, err = PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != 0 {
		test.Fatalf("empty diff: %+v %v", prepared, err)
	}
}

func TestPrepareRenameExclusion(test *testing.T) {
	input := fixture(test)
	input.ExcludePatterns = []string{"**/consumer.go"}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	for _, changed := range code.scope {
		if changed.Renamed && changed.ExcludedReason == "" {
			test.Fatalf("rename bypassed old exclusion: %+v", changed)
		}
	}
}

func TestTrackedCredentialScopes(test *testing.T) {
	input := fixture(test)
	names := []string{".git-credentials", "home/.Docker/config.json"}
	const marker = "scope-fixture-marker"
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(input.RepoDir, name)), 0700); err != nil {
			test.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(input.RepoDir, name), []byte(marker), 0600); err != nil {
			test.Fatal(err)
		}
		git(test, input.RepoDir, "add", "-f", name)
	}
	git(test, input.RepoDir, "commit", "-qm", "Add scope fixture")
	input.Resolution.ResolvedBase = git(test, input.RepoDir, "rev-parse", "HEAD")
	git(test, input.RepoDir, "commit", "--allow-empty", "-qm", "Keep fixture unchanged")
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != 0 {
		test.Fatalf("unchanged scope: %+v %v", prepared.Diffs, err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	for _, side := range []string{"old", "new"} {
		for _, name := range names {
			found, err := code.execute(test.Context(), "file_find", map[string]any{"side": side, "query_name": filepath.Base(name)})
			if err != nil || strings.Contains(found.ReturnedContent, name) {
				test.Fatalf("credential enumeration: %+v %v", found, err)
			}
			if _, err := code.execute(test.Context(), "file_read", map[string]any{"side": side, "file_path": name}); err == nil {
				test.Fatalf("%s side allowed a credential path", side)
			}
		}
		found, err := code.execute(test.Context(), "code_search", map[string]any{"side": side, "search_text": marker})
		if err != nil || !strings.Contains(found.ReturnedContent, "No matches found") || strings.Contains(found.ReturnedContent, marker) {
			test.Fatalf("%s search: %+v %v", side, found, err)
		}
	}
}

func TestParseInventory(test *testing.T) {
	for _, raw := range []string{"M\x00", "\x00path\x00", "R100\x00old\x00", "X\x00path\x00"} {
		if _, err := parseInventory(raw); err == nil {
			test.Fatalf("accepted %q", raw)
		}
	}
	if changes, err := parseInventory("T\x00space name\x00"); err != nil || len(changes) != 1 || changes[0].NewPath != "space name" {
		test.Fatalf("inventory: %+v %v", changes, err)
	}
}
