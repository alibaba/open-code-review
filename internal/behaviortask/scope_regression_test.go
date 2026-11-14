// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
)

func TestPreparePreservesInventoryAcrossRenameLimit(test *testing.T) {
	repo := test.TempDir()
	git(test, repo, "init", "-q")
	git(test, repo, "config", "diff.renameLimit", "1")
	if err := os.Mkdir(filepath.Join(repo, "vendor"), 0700); err != nil {
		test.Fatal(err)
	}
	for _, source := range []string{"alpha-old.go", "vendor/beta-old.go"} {
		var content strings.Builder
		for index := 0; index < 30; index++ {
			fmt.Fprintf(&content, "%s distinctive payload number %d\n", source, index)
		}
		if err := os.WriteFile(filepath.Join(repo, source), []byte(content.String()), 0600); err != nil {
			test.Fatal(err)
		}
	}
	git(test, repo, "add", ".")
	git(test, repo, "commit", "-qm", "Before renames")
	base := git(test, repo, "rev-parse", "HEAD")
	for oldPath, newPath := range map[string]string{"alpha-old.go": "alpha-new.go", "vendor/beta-old.go": "vendor/beta-new.go"} {
		git(test, repo, "mv", oldPath, newPath)
		content, err := os.ReadFile(filepath.Join(repo, newPath))
		if err != nil {
			test.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, newPath), append(content, []byte("One new line\n")...), 0600); err != nil {
			test.Fatal(err)
		}
	}
	git(test, repo, "add", ".")
	git(test, repo, "commit", "-qm", "After renames")
	head := git(test, repo, "rev-parse", "HEAD")
	test.Logf("global inventory: %s", git(test, repo, "diff", "--find-renames", "--name-status", base, head))
	test.Logf("scoped inventory: %s", git(test, repo, "diff", "--find-renames", "--name-status", base, head, "--", "alpha-old.go", "alpha-new.go"))
	input := Input{RepoDir: repo, Mode: "range", Resolution: diff.InputResolution{ResolvedBase: base, ResolvedHead: head}}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatalf("valid range must remain analyzable: %v", err)
	}
	for _, changed := range prepared.Diffs {
		if changed.IsRenamed || !changed.IsNew && !changed.IsDeleted {
			test.Fatalf("inventory status changed: %+v", changed)
		}
		if strings.HasPrefix(changed.NewPath, "vendor/") && changed.Diff != "" {
			test.Fatal("excluded diff was loaded")
		}
	}
}

func TestFileFindPreservesWhitespacePaths(test *testing.T) {
	for _, filePath := range []string{" leading.go", "trailing.go "} {
		test.Run(filePath, func(test *testing.T) {
			input := fixture(test)
			if err := os.WriteFile(filepath.Join(input.RepoDir, filePath), []byte("source content\n"), 0600); err != nil {
				test.Fatal(err)
			}
			git(test, input.RepoDir, "add", ".")
			git(test, input.RepoDir, "commit", "-qm", "Add whitespace path")
			input.Resolution.ResolvedHead = git(test, input.RepoDir, "rev-parse", "HEAD")
			code, err := prepareCodeTools(test.Context(), input)
			if err != nil {
				test.Fatal(err)
			}
			arguments := map[string]any{"query_name": strings.TrimSpace(filePath)}
			provider, _ := code.registry.Get("file_find")
			raw, err := provider.Execute(test.Context(), arguments)
			if err != nil {
				test.Fatal(err)
			}
			observation, err := code.execute(test.Context(), "file_find", arguments)
			if err != nil {
				test.Fatal(err)
			}
			test.Logf("authorized=%v raw=%q observed=%q", code.fileAllowed("new", filePath), raw, observation.ReturnedContent)
			if observation.ReturnedContent != filePath {
				test.Fatalf("authorized path must preserve whitespace: got %q, want %q", observation.ReturnedContent, filePath)
			}
		})
	}
}

func TestPrepareWorkspaceStagedDeletionWithExistingFile(test *testing.T) {
	input := fixture(test)
	input.Mode = "workspace"
	input.Resolution = diff.InputResolution{ResolvedBase: input.Resolution.ResolvedHead}
	input.Diffs = nil
	git(test, input.RepoDir, "rm", "--cached", "main.go")
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	if len(prepared.Diffs) != 0 {
		test.Fatalf("unchanged workspace has a net diff: %+v", prepared.Diffs)
	}
	for _, change := range prepared.Diffs {
		test.Logf("old=%q new=%q added=%v deleted=%v", change.OldPath, change.NewPath, change.IsNew, change.IsDeleted)
	}
	client := fakeClient(nil)
	runner, err := NewRunner(test.Context(), Deps{LLMClient: client, Model: "fake", ArtifactDir: filepath.Join(test.TempDir(), "artifacts")}, prepared)
	if err != nil {
		test.Fatalf("valid workspace must remain analyzable: %v", err)
	}
	test.Cleanup(func() { _ = runner.history.Finalize() })
}

func TestPrepareWorkspaceNetChanges(test *testing.T) {
	for _, fixtureCase := range []struct {
		name       string
		content    string
		executable bool
		excluded   bool
		binary     bool
	}{
		{name: "modified", content: "net value\nunchanged\n"},
		{name: "mode only", content: "new value\nunchanged\n", executable: true},
		{name: "modified executable", content: "net value\nunchanged\n", executable: true},
		{name: "excluded", content: "net value\n", excluded: true},
		{name: "binary", content: "net\x00value\n", binary: true},
	} {
		test.Run(fixtureCase.name, func(test *testing.T) {
			if fixtureCase.executable && runtime.GOOS == "windows" {
				test.Skip("executable mode changes require POSIX file modes")
			}
			input := fixture(test)
			input.Mode = "workspace"
			input.Resolution = diff.InputResolution{ResolvedBase: input.Resolution.ResolvedHead}
			input.Diffs = nil
			if fixtureCase.excluded {
				input.ExcludePatterns = append(input.ExcludePatterns, "main.go")
			}
			git(test, input.RepoDir, "rm", "--cached", "main.go")
			git(test, input.RepoDir, "commit", "-qm", "Advance HEAD after fixing the comparison base")
			if err := os.WriteFile(filepath.Join(input.RepoDir, "main.go"), []byte(fixtureCase.content), 0600); err != nil {
				test.Fatal(err)
			}
			if fixtureCase.executable {
				if err := os.Chmod(filepath.Join(input.RepoDir, "main.go"), 0700); err != nil {
					test.Fatal(err)
				}
			}
			prepared, err := PrepareInput(test.Context(), input)
			if err != nil || len(prepared.Diffs) != 1 {
				test.Fatalf("net diff: %+v %v", prepared.Diffs, err)
			}
			changed := prepared.Diffs[0]
			if changed.IsNew || changed.IsDeleted || changed.NewPath != "main.go" || changed.IsBinary != fixtureCase.binary {
				test.Fatalf("net status: %+v", changed)
			}
			code, err := prepareCodeTools(test.Context(), prepared)
			if err != nil {
				test.Fatal(err)
			}
			if fixtureCase.excluded || fixtureCase.binary {
				if changed.Diff != "" || changed.NewFileContent != "" || len(code.diffs) != 0 {
					test.Fatalf("excluded source retained: %+v", changed)
				}
				return
			}
			if changed.NewFileContent != fixtureCase.content || len(code.diffs) != 1 {
				test.Fatalf("net content: %+v", changed)
			}
			if fixtureCase.executable && (!strings.Contains(changed.Diff, "old mode 100644") || !strings.Contains(changed.Diff, "new mode 100755")) {
				test.Fatalf("mode change missing: %s", changed.Diff)
			}
			if strings.Contains(fixtureCase.content, "net value") && (!strings.Contains(changed.Diff, "-new value") || !strings.Contains(changed.Diff, "+net value")) {
				test.Fatalf("fixed-base content change missing: %s", changed.Diff)
			}
		})
	}
}
