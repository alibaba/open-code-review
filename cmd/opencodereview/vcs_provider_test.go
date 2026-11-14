// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/vcs/lore"
)

func TestConfiguredVCS(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{".lore", "sub"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := runConfigSet("vcs_provider", "lore"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "") // Selecting Lore must not call Git during discovery.
	sub := filepath.Join(root, "sub")
	for _, test := range []struct {
		review bool
		want   string
	}{{true, root}, {false, sub}} {
		dir, source, isGit, err := openConfiguredRepository(sub, test.review, nil)
		if _, ok := source.(*lore.Repository); !ok || err != nil || dir != test.want || isGit {
			t.Fatalf("open: %q, %T, %t, %v", dir, source, isGit, err)
		}
	}
	if err := runConfigSet("vcs_provider", "unknown"); err == nil {
		t.Fatal("accepted unknown VCS")
	}
	if _, _, _, err := openConfiguredRepository(home, true, nil); err == nil {
		t.Fatal("accepted missing Lore repository")
	}
	if err := runConfigUnset("vcs_provider"); err != nil {
		t.Fatal(err)
	}
	cfgPath, err := defaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadAppConfig(cfgPath)
	if err != nil || cfg.VCSProvider != "" {
		t.Fatalf("unset: %v %v", cfg, err)
	}
}
