package reviewbundle

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func resolveScanTargetPath(repoDir, path string) (string, error) {
	root, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resolved path escapes repository")
	}
	return resolved, nil
}

func hashScanTargetFileAtPath(repoDir, path string) (string, error) {
	resolved, err := resolveScanTargetPath(repoDir, path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return hashStreamedFileContent(file, info.Size())
}

func hashStreamedFileContent(file *os.File, size int64) (string, error) {
	hasher := sha256.New()
	if err := writeLengthPrefixedStream(hasher, file, size); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func writeLengthPrefixedStream(hasher hash.Hash, reader io.Reader, size int64) error {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(size))
	if _, err := hasher.Write(length[:]); err != nil {
		return err
	}
	written, err := io.Copy(hasher, io.LimitReader(reader, size))
	if err != nil {
		return err
	}
	if written != size {
		return fmt.Errorf("file size changed while hashing")
	}
	var extra [1]byte
	extraRead, extraErr := reader.Read(extra[:])
	if extraRead > 0 {
		return fmt.Errorf("file size changed while hashing")
	}
	if extraErr != nil && extraErr != io.EOF {
		return extraErr
	}
	return nil
}
