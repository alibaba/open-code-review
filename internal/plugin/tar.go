// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Include headers, padding and PAX metadata in the decompression budget.
const maxTARStreamBytes = maxPackageBytes + (8 << 20)

func extractArchive(ctx context.Context, data []byte, destination string) error {
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		return extractTARGZ(ctx, data, destination)
	}
	return extractZIP(ctx, data, destination)
}

func extractTARGZ(ctx context.Context, data []byte, destination string) error {
	if len(data) > maxArchiveBytes {
		return fmt.Errorf("tar.gz exceeds %d byte limit", maxArchiveBytes)
	}
	compressed, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read plugin tar.gz: %w", err)
	}
	defer compressed.Close()
	stream := &io.LimitedReader{R: compressed, N: maxTARStreamBytes + 1}
	archive := tar.NewReader(stream)
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	seen := make(map[string]bool)
	var total int64
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, err := archive.Next()
		if stream.N == 0 {
			return fmt.Errorf("tar.gz exceeds %d decompressed stream byte limit", maxTARStreamBytes)
		}
		if err == io.EOF {
			// Reading to gzip EOF verifies its checksum; tar EOF alone does not.
			tail, err := io.ReadAll(stream)
			if err != nil {
				return fmt.Errorf("read plugin tar.gz trailer: %w", err)
			}
			if stream.N == 0 || len(bytes.Trim(tail, "\x00")) != 0 {
				return fmt.Errorf("tar.gz has excessive padding or unexpected trailing data")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read plugin tar.gz entry: %w", err)
		}
		if count >= maxPackageFiles {
			return fmt.Errorf("tar.gz exceeds %d entries", maxPackageFiles)
		}
		// GitHub archives include a global PAX record containing the commit ID.
		if entry.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeDir {
			return fmt.Errorf("tar.gz entry %q is a link or special file", entry.Name)
		}
		for key := range entry.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse.") {
				return fmt.Errorf("tar.gz entry %q is a sparse file", entry.Name)
			}
		}
		name := strings.TrimSuffix(strings.TrimPrefix(entry.Name, "./"), "/")
		if entry.Typeflag == tar.TypeDir && (entry.Name == "./" || entry.Name == ".") {
			continue
		}
		if err := safeRelativePath(name); err != nil {
			return fmt.Errorf("tar.gz entry %q: %w", entry.Name, err)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("tar.gz contains duplicate filename %q", entry.Name)
		}
		seen[key] = true
		if entry.Size < 0 || entry.Size > maxPackageBytes-total {
			return fmt.Errorf("tar.gz exceeds %d uncompressed byte limit", maxPackageBytes)
		}
		total += entry.Size
		target := filepath.FromSlash(name)
		if entry.Typeflag == tar.TypeDir {
			if err := root.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err := root.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := extractTARFile(root, archive, entry, target); err != nil {
			return err
		}
	}
}

func extractTARFile(root *os.Root, reader io.Reader, entry *tar.Header, target string) error {
	output, err := root.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|entry.FileInfo().Mode().Perm()&0111)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(output, io.LimitReader(reader, entry.Size+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != entry.Size {
		return fmt.Errorf("tar.gz entry %q size does not match its header", entry.Name)
	}
	return nil
}
