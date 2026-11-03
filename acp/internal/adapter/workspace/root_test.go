// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResultRootMatchesCLIOperation(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"review walks to git root", []string{"review"}, canonical},
		{"scan stays in cwd", []string{"scan"}, filepath.Join(canonical, "sub")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResultRoot(context.Background(), sub, tc.args)
			if err != nil || got != tc.want {
				t.Fatalf("root = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	for _, args := range [][]string{nil, {"unknown"}} {
		if _, err := ResultRoot(context.Background(), root, args); err == nil {
			t.Fatalf("expected invalid root for %v", args)
		}
	}
	if _, err := ResultRoot(context.Background(), ".", []string{"scan"}); err == nil {
		t.Fatal("relative cwd accepted")
	}
	if _, err := ResultRoot(context.Background(), t.TempDir(), []string{"review"}); err == nil {
		t.Fatal("nonrepository review accepted")
	}
}
