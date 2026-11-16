// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/vcs"
	gitvcs "github.com/alibaba/open-code-review/internal/vcs/git"
	"github.com/alibaba/open-code-review/internal/vcs/lore"
)

func validateVCSProvider(name string) error {
	switch name {
	case "", "git", "lore":
		return nil
	default:
		return fmt.Errorf("unknown vcs_provider %q (supported: git, lore)", name)
	}
}

func openConfiguredRepository(input string, requireRepo bool, runner *gitcmd.Runner) (string, vcs.Repository, bool, error) {
	path, err := defaultConfigPath()
	if err != nil {
		return "", nil, false, err
	}
	cfg, err := LoadAppConfig(path)
	if err != nil {
		return "", nil, false, err
	}
	var name string
	if cfg != nil {
		name = cfg.VCSProvider
	}
	if err := validateVCSProvider(name); err != nil {
		return "", nil, false, err
	}
	if name == "lore" {
		dir, err := filepath.Abs(input)
		if err != nil {
			return "", nil, false, err
		}
		info, err := os.Stat(dir)
		if err != nil {
			return "", nil, false, err
		}
		if !info.IsDir() {
			return "", nil, false, fmt.Errorf("%s is not a directory", dir)
		}
		for root := dir; ; root = filepath.Dir(root) {
			marker, err := os.Stat(filepath.Join(root, ".lore"))
			if err == nil && marker.IsDir() {
				if requireRepo {
					dir = root
				}
				scope, err := filepath.Rel(root, dir)
				if err != nil {
					return "", nil, false, err
				}
				return dir, lore.New(root, filepath.ToSlash(scope)), false, nil
			}
			if err != nil && !os.IsNotExist(err) {
				return "", nil, false, err
			}
			if filepath.Dir(root) == root {
				break
			}
		}
		return "", nil, false, fmt.Errorf("%s is not inside a Lore repository", dir)
	}
	dir, isGit, err := resolveWorkingDir(input, requireRepo)
	if err != nil {
		return "", nil, false, err
	}
	return dir, gitvcs.New(dir, runner), isGit, nil
}

func (cc *commonContext) validateRefs(opts reviewOptions) error {
	if cc.IsGitRepo {
		return validateReviewRefs(cc.RepoDir, opts)
	}
	// Other providers validate their own revision syntax during resolution.
	return nil
}
