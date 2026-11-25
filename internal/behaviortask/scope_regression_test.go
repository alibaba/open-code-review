// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		for index := range 30 {
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
			blob := git(test, input.RepoDir, "rev-parse", "HEAD:main.go")
			entries := git(test, input.RepoDir, "ls-tree", "HEAD")
			command := exec.Command("git", "mktree")
			command.Dir = input.RepoDir
			command.Stdin = strings.NewReader(entries + fmt.Sprintf("\n100644 blob %s\t%s\n", blob, filePath))
			tree, err := command.CombinedOutput()
			if err != nil {
				test.Fatalf("tree fixture: %s: %v", tree, err)
			}
			input.Resolution.ResolvedHead = git(test, input.RepoDir, "commit-tree", strings.TrimSpace(string(tree)), "-p", input.Resolution.ResolvedHead, "-m", "Add whitespace path")
			code, err := prepareCodeTools(test.Context(), input)
			if err != nil {
				test.Fatal(err)
			}
			arguments := map[string]any{"query_name": strings.TrimSpace(filePath)}
			observation, err := code.execute(test.Context(), "file_find", arguments)
			if err != nil {
				test.Fatal(err)
			}
			if observation.ReturnedContent != filePath {
				test.Fatalf("authorized path must preserve whitespace: got %q, want %q", observation.ReturnedContent, filePath)
			}
		})
	}
}

func TestFileFindFallbackUsesAuthorizedNames(test *testing.T) {
	code := &codeTools{allowed: map[string]map[string]bool{"new": {"service/main.go": true, "service.go": false}}}
	observation, err := code.execute(test.Context(), "file_find", map[string]any{"query_name": "service"})
	if err != nil || observation.ReturnedContent != "service/main.go" || observation.Truncated {
		test.Fatalf("find: %+v %v", observation, err)
	}
}

func TestInvalidUTF8PathIsMetadataOnly(test *testing.T) {
	name := "source-" + string([]byte{0xff}) + ".go"
	if safePath(name) {
		test.Fatal("invalid UTF-8 is not an addressable evidence path")
	}
	input := fixture(test)
	blob := git(test, input.RepoDir, "rev-parse", "HEAD:main.go")
	command := exec.Command("git", "mktree", "-z")
	command.Dir = input.RepoDir
	command.Stdin = strings.NewReader("100644 blob " + blob + "\t" + name + "\x00")
	tree, err := command.Output()
	if err != nil {
		test.Fatal(err)
	}
	input.Resolution.ResolvedHead = git(test, input.RepoDir, "commit-tree", strings.TrimSpace(string(tree)), "-p", input.Resolution.ResolvedHead, "-m", "Unaddressable file name")
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	if code.allowed["new"][name] {
		test.Fatal("invalid name is in tool scope")
	}
	for _, change := range code.scope {
		if change.NewPath == name && change.ExcludedReason != "" {
			return
		}
	}
	test.Fatal("missing excluded metadata")
}
