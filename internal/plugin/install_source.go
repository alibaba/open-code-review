// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type PreparedInstall struct {
	Manifest      Manifest
	Source        string
	ContentSHA256 string
	mu            sync.Mutex
	storeRoot     string
	item          Installed
	directory     string
	packageDir    string
	packageHash   string
	closed        bool
}

func (p *PreparedInstall) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return os.RemoveAll(p.directory)
}

func (s *Store) PrepareInstall(ctx context.Context, source string) (*PreparedInstall, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.readState(); err != nil {
		return nil, err
	}
	if source == "" || strings.Contains(source, "://") {
		return nil, fmt.Errorf("plugin source must be a local directory")
	}
	resolved, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	storeRoot, err := filepath.Abs(s.root)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "ocr-plugin-*")
	if err != nil {
		return nil, err
	}
	p := &PreparedInstall{storeRoot: storeRoot, directory: directory, packageDir: directory}
	success := false
	defer func() {
		if !success {
			p.Close()
		}
	}()
	for _, target := range []string{storeRoot, directory} {
		rel, err := filepath.Rel(resolved, target)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return nil, fmt.Errorf("plugin source directory cannot contain the plugin store or staging directory")
		}
	}
	sourceRoot, err := openPackageRoot(resolved)
	if err != nil {
		return nil, err
	}
	defer sourceRoot.Close()
	manifest, err := inspectPackageRoot(ctx, sourceRoot)
	if err != nil {
		return nil, err
	}
	if err := manifest.CheckCompatibility(HostVersion); err != nil {
		return nil, err
	}
	if err := copyPackageRoot(ctx, sourceRoot, directory); err != nil {
		return nil, err
	}
	copied, err := inspectPackage(ctx, directory)
	if err != nil {
		return nil, err
	}
	originalJSON, _ := json.Marshal(manifest)
	copiedJSON, _ := json.Marshal(copied)
	if string(originalJSON) != string(copiedJSON) {
		return nil, fmt.Errorf("plugin manifest changed while preparing")
	}
	p.packageHash, err = packageDigest(ctx, directory)
	if err != nil {
		return nil, err
	}
	p.item = Installed{Manifest: copied, Source: resolved, ContentSHA256: p.packageHash, ContentDigestFormat: "ocr-package-tree-v1"}
	p.Manifest, p.Source, p.ContentSHA256 = manifest, resolved, p.packageHash
	success = true
	return p, nil
}

func packageDigest(ctx context.Context, directory string) (string, error) {
	digest := sha256.New()
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("prepared package contains a symlink or special file")
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		data, err := readFileBounded(name, maxPackageBytes)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		fmt.Fprintf(digest, "%d:%s:%d:%d:", len(relative), relative, info.Mode().Perm()&0111, len(data))
		digest.Write(data)
		return nil
	})
	return hex.EncodeToString(digest.Sum(nil)), err
}

// InstallPrepared commits the exact validated package from PrepareInstall.
func (s *Store) InstallPrepared(ctx context.Context, prepared *PreparedInstall, enable bool) (Installed, error) {
	if prepared == nil {
		return Installed{}, fmt.Errorf("prepared plugin is required")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.closed {
		return Installed{}, fmt.Errorf("prepared plugin is closed")
	}
	storeRoot, err := filepath.Abs(s.root)
	if err != nil {
		return Installed{}, err
	}
	if storeRoot != prepared.storeRoot {
		return Installed{}, fmt.Errorf("prepared plugin belongs to another store")
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return Installed{}, err
	}
	defer unlock()
	value, err := s.readState()
	if err != nil {
		return Installed{}, err
	}
	return s.installPrepared(ctx, &value, prepared, enable)
}
