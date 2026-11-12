// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPreparedInstallHoldsInspectedBytesAndPrivateMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "store")
	store := NewStore(root)
	source := testPackage(t, "example", "1")
	prepared, err := store.PrepareInstall(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preparation changed store: %v", err)
	}
	if prepared.Manifest.ID != "example" || prepared.Source != source || len(prepared.ContentSHA256) != 64 {
		t.Fatalf("prepared metadata = %+v", prepared)
	}
	// The source and the public preview can both change without changing the
	// package that was already prepared and confirmed.
	writeTestJSON(t, filepath.Join(source, ManifestFilename), testManifest("example", "2"))
	prepared.Manifest.ID = "modified"
	prepared.Manifest.Rules[0] = "other.json"
	prepared.Source, prepared.ContentSHA256 = "modified", "modified"
	installed, err := NewStore(root).InstallPrepared(ctx, prepared, true)
	if err != nil || installed.Manifest.ID != "example" || installed.Manifest.Version != "1" || installed.Manifest.Rules[0] != "rules.json" || installed.Source != source || len(installed.ContentSHA256) != 64 || !installed.Enabled {
		t.Fatalf("confirmed package = %+v, %v", installed, err)
	}
	if _, err := os.Stat(prepared.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed preparation retained files: %v", err)
	}
	_, err = store.InstallPrepared(ctx, prepared, false)
	requireError(t, err, "closed")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedInstallCloseChangeAndStoreOwnership(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	source := testPackage(t, "example", "1")
	prepared, err := store.PrepareInstall(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	_, err = NewStore(t.TempDir()).InstallPrepared(ctx, prepared, false)
	requireError(t, err, "another store")
	_, err = store.InstallPrepared(ctx, nil, false)
	requireError(t, err, "required")
	writeTestJSON(t, filepath.Join(prepared.packageDir, "rules.json"), map[string]string{"default_rule": "Modified after preview"})
	_, err = store.InstallPrepared(ctx, prepared, false)
	requireError(t, err, "changed after inspection")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.InstallPrepared(ctx, prepared, false)
	requireError(t, err, "closed")
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.PrepareInstall(cancelled, source)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation = %v", err)
	}
}

func TestPreparedInstallCloseSerializesWithCommit(t *testing.T) {
	store := NewStore(t.TempDir())
	prepared, err := store.PrepareInstall(context.Background(), testPackage(t, "example", "1"))
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if err := prepared.Close(); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer group.Done()
		if _, err := store.InstallPrepared(context.Background(), prepared, false); err != nil && !strings.Contains(err.Error(), "closed") {
			t.Error(err)
		}
	}()
	group.Wait()
	if _, err := os.Stat(prepared.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("racing cleanup retained preparation: %v", err)
	}
}

func TestSourceCannotContainTemporaryStagingDirectory(t *testing.T) {
	source := testPackage(t, "example", "1")
	store := NewStore(t.TempDir())
	t.Setenv("TMP", source)
	t.Setenv("TEMP", source)
	t.Setenv("TMPDIR", source)
	_, err := store.PrepareInstall(context.Background(), source)
	requireError(t, err, "cannot contain the plugin store or staging directory")
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 2 {
		t.Fatalf("rejected staging retained files: %v, %v", entries, err)
	}
}
