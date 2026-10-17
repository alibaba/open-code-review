// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"path/filepath"
)

type ProjectInspection struct {
	Present    bool
	Authorized bool
	Plugins    []Installed
}

// Inspect verifies installed snapshots without requiring or granting execution
// authorization, so management commands can explain an untrusted project.
func (p *Project) Inspect(ctx context.Context) (ProjectInspection, error) {
	_, lock, present, err := p.locked()
	result := ProjectInspection{Present: present}
	if err != nil || !present {
		return result, err
	}
	result.Plugins, err = p.load(ctx, lock, false)
	if err != nil {
		return result, err
	}
	result.Authorized = p.executionAuthorized(lock)
	return result, nil
}

func (p *Project) executionAuthorized(lock ProjectLock) bool {
	digest := projectLockID(lock)
	trusted, err := readFileBounded(filepath.Join(p.store.root, "trust-"+digest), 128)
	return err == nil && string(trusted) == digest
}
