// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fileBackend stores objects under a root directory. It backs the filesystem
// storage type (a mounted volume that outlives the container) and is the
// backend the storage tests drive end to end, since it needs no server.
type fileBackend struct {
	root string
}

func newFileBackend(root string) *fileBackend {
	return &fileBackend{root: filepath.Clean(root)}
}

func (b *fileBackend) Name() string { return TypeFilesystem }

// resolve maps an object key to a path under the root, refusing keys that would
// escape it. Keys are built from repository paths, so a key that climbs out of
// the root would turn a mirror write into an arbitrary file write.
func (b *fileBackend) resolve(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: empty object key")
	}
	cleaned := filepath.Clean(filepath.FromSlash(key))
	if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("storage: invalid object key %q", key)
	}
	return filepath.Join(b.root, cleaned), nil
}

func (b *fileBackend) Put(_ context.Context, key string, data []byte) error {
	path, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("storage: create %q: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("storage: write %q: %w", key, err)
	}
	return nil
}

func (b *fileBackend) Get(_ context.Context, key string) ([]byte, error) {
	path, err := b.resolve(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotExist, key)
		}
		return nil, fmt.Errorf("storage: read %q: %w", key, err)
	}
	return data, nil
}

func (b *fileBackend) List(_ context.Context, prefix string) ([]string, error) {
	start, err := b.resolve(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		// A prefix that resolves outside the root cannot match anything.
		if prefix == "" {
			start = b.root
		} else {
			return nil, err
		}
	}
	var keys []string
	err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(b.root, path)
		if relErr != nil {
			return relErr
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: list %q: %w", prefix, err)
	}
	sort.Strings(keys)
	return keys, nil
}

func (b *fileBackend) Delete(_ context.Context, key string) error {
	path, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}
