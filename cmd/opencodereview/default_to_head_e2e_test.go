// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

// newDivergedBranchRepo builds a repository where main and feature both have
// commits after the merge-base, and the feature worktree also carries an
// uncommitted edit plus an untracked file. Range review of feature against main
// must see exactly the feature-side commit.
func newDivergedBranchRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	write("a.go", "package main\n\nfunc A() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "base")
	run("branch", "-M", "main")
	run("checkout", "-q", "-b", "feature")
	write("b.go", "package main\n\nfunc B() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "feature: add b")
	run("checkout", "-q", "main")
	write("c.go", "package main\n\nfunc C() {}\n")
	run("add", ".")
	run("commit", "-q", "-m", "main: add c")
	run("checkout", "-q", "feature")
	write("staged.go", "package main\n\nfunc Staged() {}\n")
	run("add", "staged.go")
	// Uncommitted and untracked changes must never enter a committed range.
	write("a.go", "package main\n\nfunc A() {}\n\nfunc AChanged() {}\n")
	write("untracked.go", "package main\n\nfunc Untracked() {}\n")
	return repo
}

func previewJSON(t *testing.T, repo string, extra ...string) model.Preview {
	t.Helper()
	args := append([]string{"--repo", repo, "--preview", "--format", "json"}, extra...)
	var out string
	captureStderr(t, func() {
		out = captureStdout(t, func() {
			if err := runReview(args); err != nil {
				t.Fatalf("preview failed: %v", err)
			}
		})
	})
	var preview model.Preview
	if err := json.Unmarshal([]byte(out), &preview); err != nil {
		t.Fatalf("decode preview JSON: %v\n%s", err, out)
	}
	return preview
}

// TestReviewFromWithoutToUsesHEADRangeE2E pins the CLI shorthand end to end:
// --from main must behave exactly like --from main --to HEAD, review committed
// changes only, and exclude both main-only commits and working-tree changes.
func TestReviewFromWithoutToUsesHEADRangeE2E(t *testing.T) {
	setTestHome(t, t.TempDir())
	repo := newDivergedBranchRepo(t)

	shorthand := previewJSON(t, repo, "--from", "main")
	explicit := previewJSON(t, repo, "--from", "main", "--to", "HEAD")
	if !reflect.DeepEqual(shorthand, explicit) {
		t.Fatalf("shorthand preview differs from explicit --to HEAD:\nshorthand=%+v\nexplicit=%+v", shorthand, explicit)
	}

	paths := make(map[string]bool, len(shorthand.Entries))
	for _, entry := range shorthand.Entries {
		paths[entry.Path] = true
	}
	if !paths["b.go"] {
		t.Errorf("feature-side commit b.go missing from preview: %v", paths)
	}
	for _, unexpected := range []string{"a.go", "c.go", "staged.go", "untracked.go"} {
		if paths[unexpected] {
			t.Errorf("%s must not be reviewed by a committed range: %v", unexpected, paths)
		}
	}
}

func TestDelegateFromWithoutToUsesHEADRangeE2E(t *testing.T) {
	if os.Getenv("OCR_TEST_DELEGATE_DEFAULT_HEAD") == "1" {
		os.Exit(runArgs(os.Args[3:]))
	}
	freshOCRHome(t)
	repo := newDivergedBranchRepo(t)
	runDelegate := func(subcommand string, extra ...string) []byte {
		t.Helper()
		args := []string{"-test.run=^TestDelegateFromWithoutToUsesHEADRangeE2E$", "--", "delegate", subcommand, "--repo", repo, "--format", "json"}
		args = append(args, extra...)
		cmd := exec.Command(os.Args[0], args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "OCR_TEST_DELEGATE_DEFAULT_HEAD=1", "OCR_NO_UPDATE=1", "OCR_ENABLE_TELEMETRY=0")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("delegate %s %v: %v\n%s", subcommand, extra, err, stderr.String())
		}
		return out
	}

	shorthand := runDelegate("preview", "--from", "main")
	explicit := runDelegate("preview", "--from", "main", "--to", "HEAD")
	if !bytes.Equal(shorthand, explicit) {
		t.Fatalf("shorthand preview differs from explicit target:\n%s\n%s", shorthand, explicit)
	}
	var preview delegatePreviewJSON
	if err := json.Unmarshal(shorthand, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Mode != "range" || preview.From != "main" || preview.To != "HEAD" || preview.MergeBase == "" || preview.Commit != "" {
		t.Fatalf("unexpected range metadata: %+v", preview)
	}
	if preview.TotalFiles != 1 || len(preview.ReviewableFiles) != 1 || preview.ReviewableFiles[0].Path != "b.go" {
		t.Fatalf("range must contain only the feature commit: %+v", preview)
	}

	shorthand = runDelegate("rule", "--from", "main", "b.go")
	explicit = runDelegate("rule", "--from", "main", "--to", "HEAD", "b.go")
	if !bytes.Equal(shorthand, explicit) {
		t.Fatalf("shorthand rules differ from explicit target:\n%s\n%s", shorthand, explicit)
	}
	var rules delegateRulesJSON
	if err := json.Unmarshal(shorthand, &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules.Groups) != 1 || !reflect.DeepEqual(rules.Groups[0].Files, []string{"b.go"}) || rules.Groups[0].Rule == "" {
		t.Fatalf("expected rules for the feature file: %+v", rules)
	}

	preview = delegatePreviewJSON{}
	if err := json.Unmarshal(runDelegate("preview"), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Mode != "workspace" || preview.From != "" || preview.To != "" || preview.MergeBase != "" || preview.Commit != "" {
		t.Fatalf("plain delegate preview must remain workspace mode: %+v", preview)
	}
	paths := make(map[string]bool, len(preview.ReviewableFiles))
	for _, file := range preview.ReviewableFiles {
		paths[file.Path] = true
	}
	if !reflect.DeepEqual(paths, map[string]bool{"a.go": true, "staged.go": true, "untracked.go": true}) {
		t.Fatalf("workspace must include only local changes: %v", paths)
	}
}
