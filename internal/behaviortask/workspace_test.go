// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func workspaceInput(test *testing.T) Input {
	test.Helper()
	input := fixture(test)
	git(test, input.RepoDir, "reset", "--mixed", input.Resolution.ResolvedBase)
	git(test, input.RepoDir, "add", "-A")
	input.Mode = "workspace"
	input.Resolution = diff.InputResolution{ResolvedBase: input.Resolution.ResolvedBase}
	input.Diffs = nil
	for name, content := range map[string]string{
		"main.go":      "workspace value\nunchanged\n",
		"untracked.go": "workspace value from untracked\n",
		".env":         "SECRET workspace value\n",
		"excluded.go":  "EXCLUDED workspace value\n",
		".gitignore":   "ignored.go\n",
		"ignored.go":   "IGNORED workspace value\n",
	} {
		if err := os.WriteFile(filepath.Join(input.RepoDir, name), []byte(content), 0600); err != nil {
			test.Fatal(err)
		}
	}
	return input
}

func TestPrepareWorkspaceInput(test *testing.T) {
	input := workspaceInput(test)
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	if code.comparison.Mode != "workspace" || code.comparison.Head != "" || code.comparison.Old != input.Resolution.ResolvedBase {
		test.Fatalf("comparison: %+v", code.comparison)
	}
	if !strings.Contains(code.diffs["main.go"].Diff, "+workspace value") || !code.diffs["untracked.go"].IsNew || !code.diffs["deleted.go"].IsDeleted || !code.diffs["renamed.go"].IsRenamed {
		test.Fatalf("workspace inventory: %+v", code.diffs)
	}
	for _, changed := range prepared.Diffs {
		if changed.NewPath == "ignored.go" {
			test.Fatal("ignored untracked file was included")
		}
		if changed.NewPath == ".env" || changed.NewPath == "excluded.go" {
			if changed.Diff != "" || changed.NewFileContent != "" {
				test.Fatalf("excluded content retained: %+v", changed)
			}
		}
	}
	git(test, input.RepoDir, "add", "main.go")
	git(test, input.RepoDir, "commit", "-qm", "Advance HEAD after preparation")
	for side, want := range map[string]string{"old": "old value", "new": "workspace value"} {
		observation, err := code.execute(test.Context(), "file_read", map[string]any{"side": side, "file_path": "main.go"})
		if err != nil || !strings.Contains(observation.ReturnedContent, want) {
			test.Fatalf("%s read: %+v %v", side, observation, err)
		}
		if side == "new" && observation.Revision != "" {
			test.Fatalf("workspace claimed a commit: %+v", observation)
		}
		if side == "old" && observation.Revision != input.Resolution.ResolvedBase {
			test.Fatalf("old side moved with HEAD: %+v", observation)
		}
	}
}

func TestWorkspaceToolExclusions(test *testing.T) {
	input := workspaceInput(test)
	code, err := prepareCodeTools(test.Context(), input)
	if err != nil {
		test.Fatal(err)
	}
	for _, name := range []string{"late.key", "late.go"} {
		if err := os.WriteFile(filepath.Join(input.RepoDir, name), []byte("LATE SECRET workspace value\n"), 0600); err != nil {
			test.Fatal(err)
		}
	}
	for _, name := range []string{".env", "excluded.go", "ignored.go", "late.key", "late.go"} {
		if _, err := code.execute(test.Context(), "file_read", map[string]any{"file_path": name}); err == nil {
			test.Fatalf("read excluded file %q", name)
		}
	}
	observation, err := code.execute(test.Context(), "code_search", map[string]any{"search_text": "workspace value"})
	if err != nil || !strings.Contains(observation.ReturnedContent, "untracked.go") || strings.Contains(observation.ReturnedContent, "SECRET") || strings.Contains(observation.ReturnedContent, "EXCLUDED") || strings.Contains(observation.ReturnedContent, "IGNORED") {
		test.Fatalf("search scope: %+v %v", observation, err)
	}
	observation, err = code.execute(test.Context(), "file_find", map[string]any{"query_name": ".go"})
	if err != nil || !strings.Contains(observation.ReturnedContent, "untracked.go") || strings.Contains(observation.ReturnedContent, "late.go") || strings.Contains(observation.ReturnedContent, "excluded.go") {
		test.Fatalf("find scope: %+v %v", observation, err)
	}
	observation, err = code.execute(test.Context(), "code_search", map[string]any{"search_text": "LATE SECRET"})
	if err != nil || strings.Contains(observation.ReturnedContent, "SECRET") || strings.Contains(observation.ReturnedContent, "late.") {
		test.Fatalf("late files escaped the prepared scope: %+v %v", observation, err)
	}
	if err := os.Remove(filepath.Join(input.RepoDir, "main.go")); err != nil {
		test.Fatal(err)
	}
	if err := os.Symlink(".env", filepath.Join(input.RepoDir, "main.go")); err != nil {
		test.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := code.execute(test.Context(), "file_read", map[string]any{"file_path": "main.go"}); err == nil {
		test.Fatal("read a source path replaced by a symlink")
	}
	if regularWorkspaceFile(input.RepoDir, "../outside") || regularWorkspaceFile(input.RepoDir, "") || regularWorkspaceFile(input.RepoDir, "main.go/child") {
		test.Fatal("unsafe workspace path accepted")
	}
}

func TestWorkspaceSymlinkAndBinaryExclusions(test *testing.T) {
	input := workspaceInput(test)
	if err := os.Symlink(".env", filepath.Join(input.RepoDir, "link.go")); err != nil {
		test.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(input.RepoDir, "binary.go"), []byte{0, 1, 2}, 0600); err != nil {
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
	for _, changed := range prepared.Diffs {
		if changed.NewPath == "link.go" || changed.NewPath == "binary.go" {
			if changed.Diff != "" || changed.NewFileContent != "" {
				test.Fatalf("unsafe patch retained: %+v", changed)
			}
		}
	}
	if _, err := code.execute(test.Context(), "file_read", map[string]any{"file_path": "link.go"}); err == nil {
		test.Fatal("symlink read succeeded")
	}
	if _, exists := code.diffs["binary.go"]; exists {
		test.Fatal("binary patch exposed to diff tool")
	}
	if regularWorkspaceFile(input.RepoDir, "main.go/child") || regularWorkspaceFile(input.RepoDir, "missing.go") {
		test.Fatal("non-regular workspace path accepted")
	}
}

func TestUnbornAndEmptyWorkspace(test *testing.T) {
	repo := test.TempDir()
	git(test, repo, "init", "-q")
	provider := diff.NewWorkspaceProvider(repo, gitcmd.New(1))
	resolution, err := provider.ResolveWorkspaceComparison(test.Context())
	if err != nil || !resolution.BaseIsEmpty {
		test.Fatalf("unborn base: %+v %v", resolution, err)
	}
	input := Input{RepoDir: repo, Mode: "workspace", Resolution: resolution}
	prepared, err := PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != 0 {
		test.Fatalf("empty workspace: %+v %v", prepared, err)
	}
	for _, name := range []string{"staged.go", "untracked.go"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("new workspace content\n"), 0600); err != nil {
			test.Fatal(err)
		}
	}
	git(test, repo, "add", "staged.go")
	prepared, err = PrepareInput(test.Context(), input)
	if err != nil || len(prepared.Diffs) != 2 {
		test.Fatalf("unborn changes: %+v %v", prepared, err)
	}
	code, err := prepareCodeTools(test.Context(), prepared)
	if err != nil {
		test.Fatal(err)
	}
	for _, changed := range prepared.Diffs {
		if !changed.IsNew || changed.Diff == "" {
			test.Fatalf("unborn change: %+v", changed)
		}
	}
	if !code.comparison.BaseIsEmpty || code.comparison.Head != "" || code.comparison.Old != "" {
		test.Fatalf("unborn identity: %+v", code.comparison)
	}
	for _, resolution := range []diff.InputResolution{{}, {ResolvedBase: "HEAD"}, {ResolvedBase: strings.Repeat("f", 40)}, {BaseIsEmpty: true, ResolvedHead: strings.Repeat("a", 40)}, {BaseIsEmpty: true, ExactRange: "HEAD..HEAD"}} {
		input.Resolution = resolution
		if _, err := PrepareInput(test.Context(), input); err == nil {
			test.Fatalf("accepted invalid workspace input: %+v", resolution)
		}
	}
}
