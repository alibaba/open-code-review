// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Installed describes an installed snapshot. Dir is an absolute path.
type Installed struct {
	Manifest            Manifest `json:"manifest"`
	Enabled             bool     `json:"enabled"`
	Source              string   `json:"source"`
	Dir                 string   `json:"dir"`
	SHA256              string   `json:"sha256,omitempty"`
	ArchiveSHA256       string   `json:"archive_sha256,omitempty"`
	ContentSHA256       string   `json:"content_sha256,omitempty"`
	ContentDigestFormat string   `json:"content_digest_format,omitempty"`
	Revision            string   `json:"revision,omitempty"`
	PluginID            string   `json:"plugin_id,omitempty"`
}

// Store owns an independent plugin registry. Constructing or reading an empty
// store does not create directories or modify application configuration.
type Store struct {
	root   string
	client *http.Client
}

type state struct {
	SchemaVersion int                  `json:"schema_version"`
	Installed     map[string]Installed `json:"installed"`
}

const maxStateBytes = 16 << 20

func NewStore(root string) *Store {
	return &Store{root: filepath.Clean(root), client: &http.Client{Timeout: 30 * time.Second}}
}

func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find plugin home: %w", err)
	}
	return NewStore(filepath.Join(home, ".opencodereview", "plugins")), nil
}

func (s *Store) readState() (state, error) {
	result := state{SchemaVersion: 1, Installed: make(map[string]Installed)}
	data, err := readFileBounded(filepath.Join(s.root, "state.json"), maxStateBytes)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read plugin state: %w", err)
	}
	var stored struct {
		SchemaVersion int                  `json:"schema_version"`
		Installed     map[string]Installed `json:"installed"`
		Markets       json.RawMessage      `json:"markets"`
	}
	if err := decodeStrict(data, &stored); err != nil {
		return result, fmt.Errorf("parse plugin state: %w", err)
	}
	result.SchemaVersion, result.Installed = stored.SchemaVersion, stored.Installed
	if result.SchemaVersion != 1 {
		return result, fmt.Errorf("unsupported plugin state schema_version %d", result.SchemaVersion)
	}
	if result.Installed == nil {
		result.Installed = make(map[string]Installed)
	}
	for id, item := range result.Installed {
		if id != item.Manifest.ID {
			return result, fmt.Errorf("plugin state id %q does not match manifest", id)
		}
		if err := item.Manifest.Validate(); err != nil {
			return result, fmt.Errorf("plugin state %q: %w", id, err)
		}
		if err := safeRelativePath(item.Dir); err != nil || filepath.ToSlash(filepath.Dir(item.Dir)) != "packages" {
			return result, fmt.Errorf("plugin state %q has invalid snapshot directory", id)
		}
	}
	migrateLegacySources(result.Installed, stored.Markets)
	return result, nil
}

func (s *Store) writeState(value state) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxStateBytes {
		return fmt.Errorf("plugin state exceeds %d byte limit", maxStateBytes)
	}
	file, err := os.CreateTemp(s.root, ".state-*")
	if err != nil {
		return fmt.Errorf("create plugin state: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(s.root, "state.json")); err != nil {
		return fmt.Errorf("commit plugin state: %w", err)
	}
	return nil
}

// lock coordinates CLI processes. A crashed process leaves a visible lock;
// timeouts report its location rather than guessing that another process is dead.
func (s *Store) lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return nil, err
	}
	lockDir := filepath.Join(s.root, ".lock")
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		err := os.Mkdir(lockDir, 0700)
		if err == nil {
			return func() { os.Remove(lockDir) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock plugin store: %w", err)
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-deadline.C:
			timer.Stop()
			return nil, fmt.Errorf("plugin store is locked at %s; remove a stale lock after verifying no plugin command is running", lockDir)
		case <-timer.C:
		}
	}
}

func (s *Store) list(enabledOnly bool, selectedID string) ([]Installed, error) {
	value, err := s.readState()
	if err != nil {
		return nil, err
	}
	items := make([]Installed, 0, len(value.Installed))
	for _, item := range value.Installed {
		if enabledOnly && !item.Enabled && item.Manifest.ID != selectedID {
			continue
		}
		item, err = s.validateSnapshot(item)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Manifest.ID < items[j].Manifest.ID })
	return items, nil
}

func (s *Store) validateSnapshot(item Installed) (Installed, error) {
	if err := s.checkPackagesDir(); err != nil {
		return item, err
	}
	var err error
	item.Dir, err = filepath.Abs(filepath.Join(s.root, filepath.FromSlash(item.Dir)))
	if err != nil {
		return item, err
	}
	manifest, err := inspectPackage(context.Background(), item.Dir)
	if err != nil {
		return item, fmt.Errorf("installed plugin %q: %w", item.Manifest.ID, err)
	}
	stored, _ := json.Marshal(item.Manifest)
	actual, _ := json.Marshal(manifest)
	if string(stored) != string(actual) {
		return item, fmt.Errorf("installed plugin %q manifest changed; reinstall or update the plugin", item.Manifest.ID)
	}
	return item, nil
}

func (s *Store) checkPackagesDir() error {
	info, err := os.Lstat(filepath.Join(s.root, "packages"))
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("plugin packages directory cannot be a symlink or special file")
	}
	return nil
}

func (s *Store) List() ([]Installed, error)    { return s.list(false, "") }
func (s *Store) Enabled() ([]Installed, error) { return s.list(true, "") }

// EnabledWith includes one invocation-selected snapshot without enabling it in
// the store or inspecting unrelated disabled packages.
func (s *Store) EnabledWith(id string) ([]Installed, error) { return s.list(true, id) }

func (s *Store) Install(ctx context.Context, source string, enable bool) (Installed, error) {
	prepared, err := s.PrepareInstall(ctx, source, "")
	if err != nil {
		return Installed{}, err
	}
	defer prepared.Close()
	return s.InstallPrepared(ctx, prepared, enable)
}

func (s *Store) Update(ctx context.Context, id string) (Installed, error) {
	return s.UpdateWithOptions(ctx, id, UpdateOptions{})
}

func (s *Store) UpdateWithOptions(ctx context.Context, id string, options UpdateOptions) (Installed, error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return Installed{}, err
	}
	defer unlock()
	value, err := s.readState()
	if err != nil {
		return Installed{}, err
	}
	old, ok := value.Installed[id]
	if !ok {
		return Installed{}, fmt.Errorf("plugin %q is not installed", id)
	}
	source, pluginID := old.Source, old.PluginID
	if options.Source != "" {
		source, pluginID = options.Source, id
	}
	prepared, err := s.prepareInstall(ctx, source, pluginID)
	if err != nil {
		return Installed{}, err
	}
	defer prepared.Close()
	if prepared.item.Manifest.ID != id {
		return Installed{}, fmt.Errorf("updated package id %q does not match installed id %q", prepared.item.Manifest.ID, id)
	}
	if changes := executionChanges(old.Manifest, prepared.item.Manifest); len(changes) > 0 && !options.AcceptPermissions {
		return Installed{}, &ExecutionChangesError{PluginID: id, Changes: changes}
	}
	return s.installPrepared(ctx, &value, prepared, old.Enabled, id)
}

func (s *Store) installPrepared(ctx context.Context, value *state, prepared *PreparedInstall, enabled bool, updating string) (Installed, error) {
	manifest := prepared.item.Manifest
	if updating != "" && manifest.ID != updating {
		return Installed{}, fmt.Errorf("updated package id %q does not match installed id %q", manifest.ID, updating)
	}
	if _, exists := value.Installed[manifest.ID]; exists && updating == "" {
		return Installed{}, fmt.Errorf("plugin %q is already installed; use update", manifest.ID)
	}
	if _, err := inspectPackage(ctx, prepared.packageDir); err != nil {
		return Installed{}, err
	}
	digest, err := packageDigest(ctx, prepared.packageDir)
	if err != nil {
		return Installed{}, err
	}
	if digest != prepared.packageHash {
		return Installed{}, fmt.Errorf("prepared plugin changed after inspection; prepare it again")
	}
	packages := filepath.Join(s.root, "packages")
	if err := os.MkdirAll(packages, 0700); err != nil {
		return Installed{}, err
	}
	if err := s.checkPackagesDir(); err != nil {
		return Installed{}, err
	}
	stage, err := os.MkdirTemp(packages, ".snapshot-*")
	if err != nil {
		return Installed{}, err
	}
	committed := false
	defer func() {
		if !committed {
			os.RemoveAll(stage)
		}
	}()
	if err := copyPackage(ctx, prepared.packageDir, stage); err != nil {
		return Installed{}, err
	}
	if _, err := inspectPackage(ctx, stage); err != nil {
		return Installed{}, err
	}
	if digest, err := packageDigest(ctx, stage); err != nil {
		return Installed{}, err
	} else if digest != prepared.packageHash {
		return Installed{}, fmt.Errorf("prepared plugin changed while installing; prepare it again")
	}
	if err := ctx.Err(); err != nil {
		return Installed{}, err
	}
	final := filepath.Join(packages, manifest.ID+"-"+strings.TrimPrefix(filepath.Base(stage), ".snapshot-"))
	if err := os.Rename(stage, final); err != nil {
		return Installed{}, err
	}
	stage = final
	item := prepared.item
	item.Enabled, item.Dir = enabled, "packages/"+filepath.Base(final)
	value.Installed[manifest.ID] = item
	if err := s.writeState(*value); err != nil {
		return Installed{}, err
	}
	committed = true
	prepared.closed = true
	os.RemoveAll(prepared.directory)
	item.Dir, _ = filepath.Abs(final)
	// Retain previous snapshots: another review process may still be reading the
	// directory captured before this update.
	return item, nil
}

func (s *Store) SetEnabled(id string, enabled bool) error {
	_, err := s.SetEnabledWithResult(id, enabled)
	return err
}

// SetEnabledWithResult returns the declaration from the same locked update so
// management output describes the package that was actually enabled.
func (s *Store) SetEnabledWithResult(id string, enabled bool) (Installed, error) {
	unlock, err := s.lock(context.Background())
	if err != nil {
		return Installed{}, err
	}
	defer unlock()
	value, err := s.readState()
	if err != nil {
		return Installed{}, err
	}
	item, ok := value.Installed[id]
	if !ok {
		return Installed{}, fmt.Errorf("plugin %q is not installed", id)
	}
	if enabled {
		if err := item.Manifest.CheckCompatibility(HostVersion); err != nil {
			return Installed{}, err
		}
		if _, err := s.validateSnapshot(item); err != nil {
			return Installed{}, fmt.Errorf("enable plugin %q: %w", id, err)
		}
	}
	item.Enabled = enabled
	value.Installed[id] = item
	item.Dir, err = filepath.Abs(filepath.Join(s.root, filepath.FromSlash(item.Dir)))
	if err != nil {
		return Installed{}, err
	}
	if err := s.writeState(value); err != nil {
		return Installed{}, err
	}
	return item, nil
}

func (s *Store) Remove(id string) error {
	unlock, err := s.lock(context.Background())
	if err != nil {
		return err
	}
	defer unlock()
	value, err := s.readState()
	if err != nil {
		return err
	}
	_, ok := value.Installed[id]
	if !ok {
		return fmt.Errorf("plugin %q is not installed", id)
	}
	delete(value.Installed, id)
	if err := s.writeState(value); err != nil {
		return err
	}
	// Removal unregisters the plugin. In-flight reviews keep their snapshots.
	return nil
}
