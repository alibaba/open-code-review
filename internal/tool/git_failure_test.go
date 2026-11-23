// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestFileFindDoesNotHideBrokenIndex(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "direct"
		if shared {
			name = "shared runner"
		}
		t.Run(name, func(t *testing.T) {
			dir := setupTestRepo(t)
			if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("broken index"), 0600); err != nil {
				t.Fatal(err)
			}
			reader := &FileReader{RepoDir: dir, Mode: ModeWorkspace}
			if shared {
				reader.Runner = gitcmd.New(1)
			}
			if files, err := NewFileFind(reader).listGitFiles(t.Context()); err == nil {
				t.Errorf("file_find replaced the failed Git inventory with %q", files)
			}
		})
	}
}
