// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseDiffText_RealGitPathHeaders(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		oldPath    string
		newPath    string
		edited     bool
		binary     bool
		wantHeader string
	}{
		{name: "plain modification", operation: "modify", oldPath: "src/file.go", newPath: "src/file.go"},
		{name: "ordinary spaces", operation: "modify", oldPath: "dir with spaces/file.go", newPath: "dir with spaces/file.go"},
		{name: "delimiter in path", operation: "modify", oldPath: "x b/y.go", newPath: "x b/y.go"},
		{name: "repeated delimiters", operation: "modify", oldPath: "x b/y b/z.go", newPath: "x b/y b/z.go"},
		{name: "quoted modification", operation: "modify", oldPath: "x\" b/y.go", newPath: "x\" b/y.go"},
		{name: "added delimiter path", operation: "add", newPath: "x b/y.go"},
		{name: "deleted delimiter path", operation: "delete", oldPath: "x b/y.go"},
		{name: "binary delimiter path", operation: "modify", oldPath: "x b/y.bin", newPath: "x b/y.bin", binary: true},
		{name: "plain copy", operation: "copy", oldPath: "old.go", newPath: "new.go"},
		{name: "copy with false equal split", operation: "copy", oldPath: "x", newPath: "y b/x b/y", wantHeader: "diff --git a/x b/y b/x b/y"},
		{name: "copy with delimiters", operation: "copy", oldPath: "x b/old.go", newPath: "x b/new.go"},
		{name: "edited copy with false equal split", operation: "copy", oldPath: "x", newPath: "y b/x b/y", edited: true, wantHeader: "diff --git a/x b/y b/x b/y"},
		{name: "quoted copy source", operation: "copy", oldPath: "x\" b/old.go", newPath: "new.go"},
		{name: "quoted copy destination", operation: "copy", oldPath: "old.go", newPath: "x\" b/new.go"},
		{name: "quoted copy operands", operation: "copy", oldPath: "x\" b/old.go", newPath: "y\" b/new.go"},
		{name: "plain rename", operation: "rename", oldPath: "old.go", newPath: "new.go"},
		{name: "rename with delimiters", operation: "rename", oldPath: "x b/old.go", newPath: "x b/new.go"},
		{name: "rename with false equal split", operation: "rename", oldPath: "x", newPath: "y b/x b/y", wantHeader: "diff --git a/x b/y b/x b/y"},
		{name: "edited rename with false equal split", operation: "rename", oldPath: "x", newPath: "y b/x b/y", edited: true, wantHeader: "diff --git a/x b/y b/x b/y"},
		{name: "quoted rename operands", operation: "rename", oldPath: "x\" b/old.go", newPath: "y\" b/new.go"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.ContainsAny(testCase.oldPath+testCase.newPath, "\"\\\t") {
				t.Skip("path contains characters unsupported in Windows filenames")
			}
			repo := t.TempDir()
			runGitTest(t, repo, "init", "-q")
			runGitTest(t, repo, "config", "user.email", "test@example.com")
			runGitTest(t, repo, "config", "user.name", "Test User")
			runGitTest(t, repo, "config", "commit.gpgsign", "false")
			runGitTest(t, repo, "config", "core.autocrlf", "false")

			writePath := func(relPath, content string) {
				t.Helper()
				path := filepath.Join(repo, filepath.FromSlash(relPath))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			beforeContent := strings.Repeat("unchanged content\n", 10) + "before\n"
			if testCase.binary {
				beforeContent = "before\x00\n"
			}
			if testCase.oldPath != "" {
				writePath(testCase.oldPath, beforeContent)
			}
			runGitTest(t, repo, "add", "-A")
			runGitTest(t, repo, "commit", "-q", "--allow-empty", "-m", "initial paths")

			newContent := beforeContent
			if testCase.operation == "modify" || testCase.edited {
				newContent = strings.Replace(beforeContent, "before", "after", 1)
			}
			if testCase.operation == "rename" || testCase.operation == "delete" {
				if err := os.Remove(filepath.Join(repo, filepath.FromSlash(testCase.oldPath))); err != nil {
					t.Fatal(err)
				}
			}
			if testCase.newPath != "" {
				writePath(testCase.newPath, newContent)
			}
			runGitTest(t, repo, "add", "-A")
			runGitTest(t, repo, "commit", "-q", "-m", "update paths")

			command := exec.Command("git", "-c", "core.quotepath=false", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--src-prefix=a/", "--dst-prefix=b/", "--find-renames", "--find-copies", "--find-copies-harder", "HEAD~1", "HEAD", "--")
			command.Dir = repo
			output, err := command.Output()
			if err != nil {
				t.Fatalf("git diff: %v", err)
			}
			diffText := string(output)
			t.Logf("real git diff:\n%s", diffText)
			if testCase.wantHeader != "" && !strings.HasPrefix(diffText, testCase.wantHeader+"\n") {
				t.Fatalf("expected ambiguous header %q, got:\n%s", testCase.wantHeader, diffText)
			}
			if testCase.operation == "copy" || testCase.operation == "rename" {
				for _, direction := range []string{"from", "to"} {
					if !strings.Contains(diffText, "\n"+testCase.operation+" "+direction+" ") {
						t.Fatalf("git did not detect %s: %s", testCase.operation, diffText)
					}
				}
				if !testCase.edited && strings.Contains(diffText, "\n@@") {
					t.Fatalf("expected metadata-only %s, got:\n%s", testCase.operation, diffText)
				}
			}

			wantOld, wantNew := testCase.oldPath, testCase.newPath
			wantContent := newContent
			var wantInsertions, wantDeletions int64
			switch testCase.operation {
			case "add":
				wantOld = wantNew
				wantInsertions = int64(strings.Count(newContent, "\n"))
			case "delete":
				wantNew = "/dev/null"
				wantContent = ""
				wantDeletions = int64(strings.Count(beforeContent, "\n"))
			case "modify":
				wantInsertions, wantDeletions = 1, 1
			}
			if testCase.edited {
				wantInsertions, wantDeletions = 1, 1
			}
			if testCase.binary {
				wantContent = ""
				wantInsertions, wantDeletions = 0, 0
			}

			for _, readMode := range []struct {
				name string
				ref  string
			}{
				{name: "workspace"},
				{name: "committed", ref: "HEAD"},
			} {
				if readMode.ref != "" && testCase.newPath != "" {
					writePath(testCase.newPath, "workspace-only content\n")
				}
				t.Run(readMode.name, func(t *testing.T) {
					diffs, err := ParseDiffText(context.Background(), diffText, repo, readMode.ref, nil)
					if err != nil {
						t.Fatal(err)
					}
					if len(diffs) != 1 {
						t.Fatalf("got %d diffs, want 1: %+v", len(diffs), diffs)
					}
					parsed := diffs[0]
					if parsed.OldPath != wantOld || parsed.NewPath != wantNew {
						t.Errorf("paths = %q / %q, want %q / %q", parsed.OldPath, parsed.NewPath, wantOld, wantNew)
					}
					if parsed.NewFileContent != wantContent {
						t.Errorf("NewFileContent = %q, want %q", parsed.NewFileContent, wantContent)
					}
					if parsed.IsNew != (testCase.operation == "add") || parsed.IsDeleted != (testCase.operation == "delete") || parsed.IsRenamed != (testCase.operation == "rename") || parsed.IsBinary != testCase.binary {
						t.Errorf("unexpected flags: new=%v deleted=%v renamed=%v binary=%v", parsed.IsNew, parsed.IsDeleted, parsed.IsRenamed, parsed.IsBinary)
					}
					if parsed.Insertions != wantInsertions || parsed.Deletions != wantDeletions {
						t.Errorf("changes = +%d/-%d, want +%d/-%d", parsed.Insertions, parsed.Deletions, wantInsertions, wantDeletions)
					}
				})
			}
		})
	}
}
