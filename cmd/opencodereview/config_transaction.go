// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"crypto/sha256"
	"errors"
	"os"

	"github.com/gofrs/flock"
)

var errConfigConflict = errors.New("configuration changed in another operation; reopen settings and retry")
var errConfigBusy = errors.New("configuration is being saved by another operation; retry")

type configRevision struct {
	exists bool
	digest [sha256.Size]byte
}

func revisionOfConfig(data []byte) *configRevision {
	return &configRevision{exists: true, digest: sha256.Sum256(data)}
}

func checkConfigRevision(path string, expected *configRevision) error {
	// Fresh programmatically constructed configs are used by setup fixtures;
	// every production read-modify-write path carries a loaded revision.
	if expected == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	actual := &configRevision{}
	if err == nil {
		actual = revisionOfConfig(data)
	} else if !os.IsNotExist(err) {
		return errors.New("cannot check current configuration before saving")
	}
	if *actual != *expected {
		return errConfigConflict
	}
	return nil
}

func lockConfig(path string) (func(), error) {
	// Keep the lock inode stable across atomic config replacement. Do not delete
	// this sidecar: unlinking a locked file allows another writer to lock a new inode.
	lock := flock.New(path+".lock", flock.SetPermissions(0o600))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, errors.New("cannot lock configuration for saving")
	}
	if !locked {
		return nil, errConfigBusy
	}
	return func() { _ = lock.Close() }, nil
}
