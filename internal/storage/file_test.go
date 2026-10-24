// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileBackendRoundTrip(t *testing.T) {
	root := t.TempDir()
	backend := newFileBackend(root)
	ctx := context.Background()

	if err := backend.Put(ctx, "sessions/repo/id-1.jsonl", []byte("first")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, err := backend.Get(ctx, "sessions/repo/id-1.jsonl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "first" {
		t.Errorf("Get = %q, want first", data)
	}

	// A second Put replaces the object, which is how a rerun's session overwrites
	// an earlier copy at the same key.
	if err := backend.Put(ctx, "sessions/repo/id-1.jsonl", []byte("second")); err != nil {
		t.Fatalf("second Put: %v", err)
	}
	data, _ = backend.Get(ctx, "sessions/repo/id-1.jsonl")
	if string(data) != "second" {
		t.Errorf("Get after replace = %q, want second", data)
	}

	if err := backend.Delete(ctx, "sessions/repo/id-1.jsonl"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := backend.Get(ctx, "sessions/repo/id-1.jsonl"); !errors.Is(err, ErrNotExist) {
		t.Errorf("Get after Delete: err = %v, want ErrNotExist", err)
	}
	if err := backend.Delete(ctx, "sessions/repo/id-1.jsonl"); err != nil {
		t.Errorf("Delete of an absent object: %v", err)
	}
}

func TestFileBackendList(t *testing.T) {
	root := t.TempDir()
	backend := newFileBackend(root)
	ctx := context.Background()

	// An empty store lists nothing rather than failing: the viewer asks before any
	// session has been mirrored.
	keys, err := backend.List(ctx, "sessions/")
	if err != nil {
		t.Fatalf("List on an empty root: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List on an empty root = %v, want no keys", keys)
	}

	for _, key := range []string{
		"sessions/repo-b/id-1.jsonl",
		"sessions/repo-a/id-2.jsonl",
		"sessions/repo-a/id-0.jsonl",
		"other/id-3.jsonl",
	} {
		if err := backend.Put(ctx, key, []byte("x")); err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	keys, err = backend.List(ctx, "sessions/repo-a/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"sessions/repo-a/id-0.jsonl", "sessions/repo-a/id-2.jsonl"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("List = %v, want %v (filtered and sorted)", keys, want)
	}

	keys, err = backend.List(ctx, "sessions/")
	if err != nil {
		t.Fatalf("List sessions/: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("List = %v, want the three session objects", keys)
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, "sessions/") {
			t.Errorf("List returned a key outside the prefix: %q", key)
		}
	}
}

func TestFileBackendRejectsEscapingKeys(t *testing.T) {
	root := t.TempDir()
	backend := newFileBackend(root)
	ctx := context.Background()

	outside := filepath.Join(filepath.Dir(root), "escaped.jsonl")
	for _, key := range []string{"", "../escaped.jsonl", "../../etc/passwd", "/abs/path.jsonl"} {
		if err := backend.Put(ctx, key, []byte("x")); err == nil {
			t.Errorf("Put accepted key %q", key)
		}
		if _, err := backend.Get(ctx, key); err == nil {
			t.Errorf("Get accepted key %q", key)
		}
		if err := backend.Delete(ctx, key); err == nil {
			t.Errorf("Delete accepted key %q", key)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("a key escaped the configured root and wrote %s", outside)
	}
}

func TestPrefixedBackendScopesKeys(t *testing.T) {
	root := t.TempDir()
	inner := newFileBackend(root)
	backend := &prefixedBackend{inner: inner, prefix: "prod/"}
	ctx := context.Background()

	if err := backend.Put(ctx, "sessions/repo/id.jsonl", []byte("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "prod", "sessions", "repo", "id.jsonl")); err != nil {
		t.Errorf("object not written under the prefix: %v", err)
	}

	data, err := backend.Get(ctx, "sessions/repo/id.jsonl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("Get = %q", data)
	}

	// A key of another environment shares the backend but not the listing.
	if err := inner.Put(ctx, "other/sessions/repo/id.jsonl", []byte("other")); err != nil {
		t.Fatalf("Put other: %v", err)
	}
	keys, err := backend.List(ctx, "sessions/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 || keys[0] != "sessions/repo/id.jsonl" {
		t.Errorf("List = %v, want only this prefix's key with the prefix stripped", keys)
	}

	if err := backend.Delete(ctx, "sessions/repo/id.jsonl"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := inner.Get(ctx, "prod/sessions/repo/id.jsonl"); !errors.Is(err, ErrNotExist) {
		t.Errorf("Delete did not remove the prefixed object: %v", err)
	}
	if backend.Name() != inner.Name() {
		t.Errorf("Name = %q, want the wrapped backend's name", backend.Name())
	}
}
