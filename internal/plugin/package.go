// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxManifestBytes = 1 << 20
	maxPackageBytes  = 64 << 20
	maxPackageFiles  = 4096
)

// ContentDigest hashes the current package files, including executable bits.
func ContentDigest(ctx context.Context, directory string) (string, error) {
	return packageDigest(ctx, directory)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("content exceeds %d byte limit", limit)
	}
	return data, nil
}

func readFileBounded(name string, limit int64) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBounded(file, limit)
}

func openPackageRoot(name string) (*os.Root, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("plugin source must be a directory, without symlinks")
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	if !os.SameFile(info, opened) {
		root.Close()
		return nil, fmt.Errorf("plugin source changed while opening its directory")
	}
	return root, nil
}

// inspectPackage rejects symlinks throughout the package, not only in declared
// contributions. It also bounds the snapshot and prevents ambiguous filenames.
func inspectPackage(ctx context.Context, directory string) (Manifest, error) {
	root, err := openPackageRoot(directory)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	return inspectPackageRoot(ctx, root)
}

func inspectPackageRoot(ctx context.Context, root *os.Root) (Manifest, error) {

	var m Manifest
	count := 0
	var total int64
	seen := make(map[string]bool)
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		rel := name
		if err := safeRelativePath(rel); err != nil {
			return fmt.Errorf("package file %q: %w", rel, err)
		}
		key := strings.ToLower(rel)
		if seen[key] {
			return fmt.Errorf("package contains conflicting filename %q", rel)
		}
		seen[key] = true
		count++
		if count > maxPackageFiles {
			return fmt.Errorf("package exceeds %d entries", maxPackageFiles)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("package file %q is a symlink or special file", rel)
		}
		if info.Mode().IsRegular() {
			total += info.Size()
			if total > maxPackageBytes {
				return fmt.Errorf("package exceeds %d byte limit", maxPackageBytes)
			}
		}
		return nil
	})
	if err != nil {
		return m, err
	}
	file, err := root.Open(ManifestFilename)
	if err != nil {
		return m, fmt.Errorf("read %s: %w", ManifestFilename, err)
	}
	defer file.Close()
	data, err := readBounded(file, maxManifestBytes)
	if err != nil {
		return m, fmt.Errorf("read %s: %w", ManifestFilename, err)
	}
	m, err = ParseManifest(data)
	if err != nil {
		return m, err
	}
	for _, rule := range m.Rules {
		info, err := root.Lstat(filepath.FromSlash(rule))
		if err != nil || !info.Mode().IsRegular() {
			return m, fmt.Errorf("rule %q must be an existing regular file", rule)
		}
	}
	return m, nil
}

func copyPackage(ctx context.Context, source, destination string) error {
	sourceRoot, err := openPackageRoot(source)
	if err != nil {
		return err
	}
	defer sourceRoot.Close()
	return copyPackageRoot(ctx, sourceRoot, destination)
}

func copyPackageRoot(ctx context.Context, source *os.Root, destination string) error {
	targetRoot, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer targetRoot.Close()
	// Directory handles prevent ancestor replacements from escaping either tree.
	var total int64
	return fs.WalkDir(source.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if err := safeRelativePath(name); err != nil {
			return err
		}
		rel := filepath.FromSlash(name)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return targetRoot.MkdirAll(rel, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("package file %q is a symlink or special file", rel)
		}
		input, err := source.Open(rel)
		if err != nil {
			return err
		}
		defer input.Close()
		opened, err := input.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			return fmt.Errorf("package source changed while copying %q", rel)
		}
		output, err := targetRoot.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600|info.Mode().Perm()&0111)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, maxPackageBytes-total+1))
		closeErr := output.Close()
		total += n
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if total > maxPackageBytes {
			return fmt.Errorf("package exceeds %d byte limit", maxPackageBytes)
		}
		return nil
	})
}
