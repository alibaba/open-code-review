// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alibaba/open-code-review/internal/pathutil"
)

const declarationFile = "plugins.json"
const projectLockFile = "plugins.lock.json"

type ProjectDependency struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type ProjectDeclaration struct {
	SchemaVersion int                 `json:"schema_version"`
	Plugins       []ProjectDependency `json:"plugins"`
}

type LockedPlugin struct {
	ID                  string `json:"id"`
	Version             string `json:"version"`
	Source              string `json:"source"`
	Directory           string `json:"directory"`
	ArchiveSHA256       string `json:"archive_sha256,omitempty"`
	ContentSHA256       string `json:"content_sha256"`
	ContentDigestFormat string `json:"content_digest_format"`
	Revision            string `json:"revision,omitempty"`
}

type ProjectLock struct {
	SchemaVersion     int                       `json:"schema_version"`
	DeclarationSHA256 string                    `json:"declaration_sha256"`
	Platforms         map[string][]LockedPlugin `json:"platforms"`
}

type ProjectError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ProjectError) Error() string { return e.Code + ": " + e.Message }

func projectError(code, format string, args ...any) error {
	return &ProjectError{Code: code, Message: fmt.Sprintf(format, args...)}
}

type Project struct {
	root  string
	store *Store
	home  *Store
}

func (s *Store) Project(directory string) (*Project, error) {
	root, err := canonicalProjectPath(directory)
	if err != nil {
		return nil, err
	}
	store := NewStore(filepath.Join(s.root, "projects", archiveDigest([]byte(root))))
	store.client = s.client
	return &Project{root: root, store: store, home: s}, nil
}

func projectPlatform() string { return runtime.GOOS + "/" + runtime.GOARCH }

func projectLockID(lock ProjectLock) string {
	return archiveDigest([]byte(projectPlatform() + ":" + jsonDigest(lock)))
}

func jsonDigest(value any) string {
	data, _ := json.Marshal(value)
	return archiveDigest(data)
}

func (p *Project) readJSON(name string, value any) error {
	root, err := openPackageRoot(filepath.Join(p.root, ".opencodereview"))
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := readBounded(file, maxProjectConfigBytes)
	if err != nil {
		return err
	}
	return decodeStrict(data, value)
}

func (p *Project) declaration() (ProjectDeclaration, bool, error) {
	var value ProjectDeclaration
	err := p.readJSON(declarationFile, &value)
	if errors.Is(err, os.ErrNotExist) {
		var lock ProjectLock
		if lockErr := p.readJSON(projectLockFile, &lock); !errors.Is(lockErr, os.ErrNotExist) {
			return value, true, projectError("LOCK_MISMATCH", "project lock exists without a readable declaration")
		}
		return ProjectDeclaration{SchemaVersion: 1, Plugins: []ProjectDependency{}}, false, nil
	}
	if err != nil {
		return value, true, projectError("LOCK_MISMATCH", "read project declaration: %v", err)
	}
	if value.SchemaVersion != 1 {
		return value, true, projectError("LOCK_MISMATCH", "unsupported project declaration schema")
	}
	seen := make(map[string]bool)
	for _, entry := range value.Plugins {
		if !validID(entry.ID) || seen[entry.ID] {
			return value, true, projectError("LOCK_MISMATCH", "invalid or duplicate plugin id %q", entry.ID)
		}
		seen[entry.ID] = true
		if err := validateProjectSource(entry.Source); err != nil {
			return value, true, err
		}
	}
	return value, true, nil
}

func validateProjectSource(source string) error {
	if strings.HasPrefix(source, "https:") {
		if err := validateHTTPS(source); err != nil {
			return projectError("LOCK_MISMATCH", "%v", err)
		}
		u, _ := url.Parse(source)
		if u.RawQuery != "" || u.ForceQuery {
			return projectError("LOCK_MISMATCH", "project sources must use permanent URLs without query parameters")
		}
		return nil
	}
	if !strings.HasPrefix(source, "./") || safeRelativePath(strings.TrimPrefix(source, "./")) != nil {
		return projectError("LOCK_MISMATCH", "local project sources must be normalized ./paths inside the repository")
	}
	return nil
}

func (p *Project) sourcePath(source string) (string, error) {
	if err := validateProjectSource(source); err != nil {
		return "", err
	}
	if strings.HasPrefix(source, "https:") {
		return source, nil
	}
	path, err := canonicalProjectPath(filepath.Join(p.root, filepath.FromSlash(source)))
	if err != nil {
		return "", err
	}
	if !pathutil.WithinBase(p.root, path) {
		return "", projectError("LOCK_MISMATCH", "local plugin source resolves outside the repository")
	}
	return path, nil
}

func (p *Project) portableSource(source string) (string, error) {
	if strings.HasPrefix(source, "https:") {
		return source, validateProjectSource(source)
	}
	canonical, err := canonicalProjectPath(source)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(p.root, canonical)
	if err != nil {
		return "", err
	}
	source = "./" + filepath.ToSlash(rel)
	return source, validateProjectSource(source)
}

func (p *Project) locked() (ProjectDeclaration, ProjectLock, bool, error) {
	declaration, present, err := p.declaration()
	var lock ProjectLock
	if err != nil || !present {
		return declaration, lock, present, err
	}
	if err := p.readJSON(projectLockFile, &lock); err != nil {
		return declaration, lock, true, projectError("LOCK_MISMATCH", "read project lock: %v; run ocr plugin lock", err)
	}
	if lock.SchemaVersion != 1 || lock.DeclarationSHA256 != jsonDigest(declaration) {
		return declaration, lock, true, projectError("LOCK_MISMATCH", "declaration changed; run ocr plugin lock")
	}
	entries, exists := lock.Platforms[projectPlatform()]
	if !exists {
		return declaration, lock, true, projectError("PLATFORM_UNSUPPORTED", "no lock for %s; run ocr plugin lock on that platform", projectPlatform())
	}
	if len(entries) != len(declaration.Plugins) {
		return declaration, lock, true, projectError("LOCK_MISMATCH", "locked plugins do not match the declaration")
	}
	for i, entry := range entries {
		if entry.ID != declaration.Plugins[i].ID || entry.Version == "" || entry.ContentDigestFormat != "ocr-package-tree-v1" {
			return declaration, lock, true, projectError("LOCK_MISMATCH", "invalid locked plugin %q", entry.ID)
		}
		if _, err := parseDigest(entry.ContentSHA256); err != nil {
			return declaration, lock, true, projectError("LOCK_MISMATCH", "invalid content digest for %q", entry.ID)
		}
		if entry.ArchiveSHA256 != "" {
			if _, err := parseDigest(entry.ArchiveSHA256); err != nil {
				return declaration, lock, true, projectError("LOCK_MISMATCH", "invalid archive digest for %q", entry.ID)
			}
		} else if strings.HasPrefix(entry.Source, "https:") {
			return declaration, lock, true, projectError("LOCK_MISMATCH", "remote artifact %q requires an archive digest", entry.ID)
		}
		if err := validateProjectSource(entry.Source); err != nil {
			return declaration, lock, true, err
		}
		if entry.Directory != "." && safeRelativePath(entry.Directory) != nil {
			return declaration, lock, true, projectError("LOCK_MISMATCH", "invalid artifact directory for %q", entry.ID)
		}
	}
	return declaration, lock, true, nil
}

func (p *Project) writeJSON(name string, value any) error {
	projectRoot, err := os.OpenRoot(p.root)
	if err != nil {
		return err
	}
	defer projectRoot.Close()
	if err := projectRoot.Mkdir(".opencodereview", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	root, err := openPackageRoot(filepath.Join(p.root, ".opencodereview"))
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxProjectConfigBytes {
		return fmt.Errorf("project plugin configuration exceeds %d bytes", maxProjectConfigBytes)
	}
	temporary := ".plugins-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, writeErr := file.Write(append(data, '\n'))
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return root.Rename(temporary, name)
}

func (p *Project) snapshotStore(lock ProjectLock) *Store {
	cacheID := archiveDigest([]byte(p.root + "\x00" + projectLockID(lock)))
	store := NewStore(filepath.Join(p.home.root, "project-packages", cacheID))
	store.client = p.home.client
	return store
}

func (p *Project) withLock(ctx context.Context, action func() error) error {
	unlock, err := p.store.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return action()
}
