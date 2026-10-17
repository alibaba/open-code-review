// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (p *Project) Add(ctx context.Context, source, selection string) (ProjectLock, error) {
	var result ProjectLock
	err := p.withLock(ctx, func() error {
		declaration, _, err := p.declaration()
		if err != nil {
			return err
		}
		prepared, err := p.home.PrepareInstall(ctx, source, selection)
		if err != nil {
			return err
		}
		defer prepared.Close()
		portable, err := p.portableSource(prepared.Source)
		if err != nil {
			return err
		}
		if _, err := p.sourcePath(portable); err != nil {
			return err
		}
		id := prepared.item.Manifest.ID
		for _, entry := range declaration.Plugins {
			if entry.ID == id {
				return fmt.Errorf("project plugin %q already declared; edit plugins.json and run ocr plugin lock", id)
			}
		}
		declaration.Plugins = append(declaration.Plugins, ProjectDependency{ID: id, Source: portable})
		result, err = p.resolve(ctx, declaration, prepared)
		return err
	})
	return result, err
}

func (p *Project) Lock(ctx context.Context) (ProjectLock, error) {
	var result ProjectLock
	err := p.withLock(ctx, func() error {
		declaration, present, err := p.declaration()
		if err != nil {
			return err
		}
		if !present {
			return projectError("LOCK_MISMATCH", "no project declaration; use ocr plugin add SOURCE")
		}
		result, err = p.resolve(ctx, declaration, nil)
		return err
	})
	return result, err
}

func (p *Project) resolve(ctx context.Context, declaration ProjectDeclaration, added *PreparedInstall) (ProjectLock, error) {
	lock := ProjectLock{SchemaVersion: 1, DeclarationSHA256: jsonDigest(declaration), Platforms: map[string][]LockedPlugin{}}
	var previous ProjectLock
	if err := p.readJSON(projectLockFile, &previous); err == nil && previous.SchemaVersion == 1 && previous.DeclarationSHA256 == lock.DeclarationSHA256 && previous.Platforms != nil {
		lock.Platforms = previous.Platforms
	}
	entries := make([]LockedPlugin, 0, len(declaration.Plugins))
	for _, dependency := range declaration.Plugins {
		entry, err := p.resolveDependency(ctx, dependency, added)
		if err != nil {
			return lock, err
		}
		entries = append(entries, entry)
	}
	lock.Platforms[projectPlatform()] = entries
	if added != nil {
		// Publish the declaration first: an interrupted pair is rejected by its
		// digest and can be recovered with lock, without guessing an old source.
		if err := p.writeJSON(declarationFile, declaration); err != nil {
			return lock, err
		}
	}
	return lock, p.writeJSON(projectLockFile, lock)
}

func (p *Project) resolveDependency(ctx context.Context, dependency ProjectDependency, prepared *PreparedInstall) (LockedPlugin, error) {
	if prepared == nil || prepared.item.Manifest.ID != dependency.ID {
		source, err := p.sourcePath(dependency.Source)
		if err != nil {
			return LockedPlugin{}, err
		}
		prepared, err = p.home.PrepareInstall(ctx, source, dependency.ID)
		if err != nil {
			return LockedPlugin{}, err
		}
		defer prepared.Close()
	}
	source, err := p.portableSource(prepared.artifact.Source)
	if err != nil {
		return LockedPlugin{}, err
	}
	item := prepared.item
	return LockedPlugin{
		ID: item.Manifest.ID, Version: item.Manifest.Version, Source: source,
		Directory: prepared.artifact.Directory, ArchiveSHA256: item.ArchiveSHA256,
		ContentSHA256: item.ContentSHA256, ContentDigestFormat: item.ContentDigestFormat, Revision: item.Revision,
	}, nil
}

func (p *Project) Sync(ctx context.Context, offline bool) ([]Installed, error) {
	var result []Installed
	err := p.withLock(ctx, func() error {
		_, lock, present, err := p.locked()
		if err != nil {
			return err
		}
		if !present {
			return projectError("LOCK_MISMATCH", "no project declaration; use ocr plugin add SOURCE")
		}
		store := p.snapshotStore(lock)
		state, err := store.readState()
		if err != nil {
			return err
		}
		for _, entry := range lock.Platforms[projectPlatform()] {
			if cached, exists := state.Installed[entry.ID]; exists {
				if _, err := store.verifyLocked(ctx, cached, entry); err != nil {
					return err
				}
				continue
			}
			if offline && strings.HasPrefix(entry.Source, "https:") {
				return projectError("ARTIFACT_MISSING", "%q is not cached; offline sync makes no network requests", entry.ID)
			}
			prepared, err := p.prepareLocked(ctx, store, entry)
			if err != nil {
				return err
			}
			_, err = store.InstallPrepared(ctx, prepared, true)
			prepared.Close()
			if err != nil {
				return err
			}
		}
		result, err = p.load(ctx, lock, false)
		return err
	})
	return result, err
}

func (p *Project) prepareLocked(ctx context.Context, store *Store, entry LockedPlugin) (*PreparedInstall, error) {
	source, err := p.sourcePath(entry.Source)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "ocr-locked-plugin-*")
	if err != nil {
		return nil, err
	}
	prepared := &PreparedInstall{directory: directory, packageDir: filepath.Join(directory, filepath.FromSlash(entry.Directory))}
	success := false
	defer func() {
		if !success {
			prepared.Close()
		}
	}()
	prepared.storeRoot, err = filepath.Abs(store.root)
	if err != nil {
		return nil, err
	}
	// Only fetch the locked artifact, even if the URL now serves different data.
	archiveHash, err := store.preparePackage(ctx, source, entry.ArchiveSHA256, directory)
	if err != nil {
		return nil, projectError("ARTIFACT_MISMATCH", "prepare %q: %v", entry.ID, err)
	}
	manifest, err := inspectPackage(ctx, prepared.packageDir)
	if err != nil {
		return nil, projectError("ARTIFACT_MISMATCH", "inspect %q: %v", entry.ID, err)
	}
	if manifest.ID != entry.ID || manifest.Version != entry.Version {
		return nil, projectError("ARTIFACT_MISMATCH", "identity changed for %q", entry.ID)
	}
	if err := manifest.CheckCompatibility(HostVersion); err != nil {
		return nil, err
	}
	prepared.packageHash, err = packageDigest(ctx, prepared.packageDir)
	if err != nil || !strings.EqualFold(prepared.packageHash, entry.ContentSHA256) || !strings.EqualFold(archiveHash, entry.ArchiveSHA256) {
		return nil, projectError("ARTIFACT_MISMATCH", "content changed for %q", entry.ID)
	}
	legacyHash := archiveHash
	if legacyHash == "" {
		legacyHash = prepared.packageHash
	}
	prepared.item = Installed{Manifest: manifest, Source: entry.Source, SHA256: legacyHash, ArchiveSHA256: archiveHash,
		ContentSHA256: prepared.packageHash, ContentDigestFormat: entry.ContentDigestFormat, Revision: entry.Revision}
	success = true
	return prepared, nil
}

func (s *Store) verifyLocked(ctx context.Context, item Installed, entry LockedPlugin) (Installed, error) {
	item, err := s.validateSnapshot(item)
	if err != nil {
		return item, projectError("ARTIFACT_MISMATCH", "%q: %v; remove project cache %s and sync again", entry.ID, err, s.root)
	}
	if item.Manifest.ID != entry.ID || item.Manifest.Version != entry.Version {
		return item, projectError("ARTIFACT_MISMATCH", "identity changed for %q", entry.ID)
	}
	if err := item.Manifest.CheckCompatibility(HostVersion); err != nil {
		return item, err
	}
	digest, err := packageDigest(ctx, item.Dir)
	if err != nil || !strings.EqualFold(digest, entry.ContentSHA256) {
		return item, projectError("ARTIFACT_MISMATCH", "installed content changed for %q; remove project cache %s and sync again", entry.ID, s.root)
	}
	item.Enabled = true
	return item, nil
}

func (p *Project) Load(ctx context.Context) ([]Installed, bool, error) {
	_, lock, present, err := p.locked()
	if err != nil || !present {
		return nil, present, err
	}
	items, err := p.load(ctx, lock, true)
	return items, true, err
}

func (p *Project) load(ctx context.Context, lock ProjectLock, requireTrust bool) ([]Installed, error) {
	store := p.snapshotStore(lock)
	state, err := store.readState()
	if err != nil {
		return nil, err
	}
	items := make([]Installed, 0, len(lock.Platforms[projectPlatform()]))
	var execution []string
	for _, entry := range lock.Platforms[projectPlatform()] {
		item, exists := state.Installed[entry.ID]
		if !exists {
			return nil, projectError("ARTIFACT_MISSING", "%q is not installed; run ocr plugin sync --frozen", entry.ID)
		}
		item, err := store.verifyLocked(ctx, item, entry)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if len(item.Manifest.MCPServers) > 0 || (item.Manifest.Review != nil && !item.Manifest.Review.retired()) {
			execution = append(execution, entry.ID)
		}
	}
	if requireTrust && len(execution) > 0 && !p.executionAuthorized(lock) {
		return nil, projectError("PLUGIN_EXEC_NOT_AUTHORIZED", "project plugins %s require local authorization; inspect the packages, then run ocr plugin trust", strings.Join(execution, ", "))
	}
	return items, nil
}

func (p *Project) Trust(ctx context.Context, revoke bool) error {
	return p.withLock(ctx, func() error {
		_, lock, present, err := p.locked()
		if err != nil {
			return err
		}
		if !present {
			return projectError("LOCK_MISMATCH", "no project plugins declared")
		}
		digest := projectLockID(lock)
		path := filepath.Join(p.store.root, "trust-"+digest)
		if revoke {
			err := os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if _, err := p.load(ctx, lock, false); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(digest), 0600)
	})
}

func (p *Project) Explain() (ProjectLock, error) {
	_, lock, present, err := p.locked()
	if err == nil && !present {
		err = projectError("LOCK_MISMATCH", "no project plugins declared")
	}
	return lock, err
}

// Fingerprint identifies the verified project lock without machine-local paths.
func (p *Project) Fingerprint(ctx context.Context) (string, error) {
	lock, err := p.Explain()
	if err != nil {
		return "", err
	}
	if _, err := p.load(ctx, lock, true); err != nil {
		return "", err
	}
	return projectLockID(lock), nil
}
