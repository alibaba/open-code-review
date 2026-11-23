// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestGitGrepPreservesSuccessfulCommandDiagnostics(t *testing.T) {
	for _, binary := range []bool{false, true} {
		for _, shared := range []bool{false, true} {
			name := "text"
			if binary {
				name = "binary"
			}
			if shared {
				name += "/shared runner"
			} else {
				name += "/direct"
			}
			t.Run(name, func(t *testing.T) {
				dir := setupTestRepo(t)
				git := func(args ...string) string {
					t.Helper()
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("git %v: %v\n%s", args, err, out)
					}
					return strings.TrimSpace(string(out))
				}
				if binary {
					if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package\x00main\n"), 0600); err != nil {
						t.Fatal(err)
					}
					git("add", "hello.go")
					git("commit", "-m", "binary match")
				}
				blob := git("rev-parse", "HEAD:pkg/util.go")
				if err := os.Remove(filepath.Join(dir, ".git", "objects", blob[:2], blob[2:])); err != nil {
					t.Fatal(err)
				}
				reader := &FileReader{RepoDir: dir, Ref: getHeadCommit(t, dir), Mode: ModeCommit}
				if shared {
					reader.Runner = gitcmd.New(1)
				}
				provider := NewCodeSearch(reader)
				args := provider.buildGrepArgs("package", false, false, false, nil)
				stdout, stderr, err := provider.runGitGrep(t.Context(), args)
				if err != nil || stdout == "" || !strings.Contains(stderr, "pkg/util.go") {
					t.Fatalf("expected successful Git search with a missing-blob diagnostic: stdout=%q stderr=%q err=%v", stdout, stderr, err)
				}
				result, err := provider.gitGrep(t.Context(), "package", false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(result, "Warning: ") || !strings.Contains(result, "pkg/util.go") || !strings.Contains(result, blob) {
					t.Errorf("search hid Git's missing-blob diagnostic: %q", result)
				}
				if !binary && (!strings.Contains(result, "File: hello.go\n") || !strings.Contains(result, "1|package main\n")) {
					t.Errorf("search discarded usable text matches: %q", result)
				}
			})
		}
	}
}
