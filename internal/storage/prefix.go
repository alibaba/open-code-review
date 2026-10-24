// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"context"
	"strings"
)

// prefixedBackend scopes another backend to a key prefix, so a bucket or a
// shared directory can hold several environments side by side while callers
// keep using store-relative keys. List strips the prefix again, which keeps the
// wrapper invisible to callers.
type prefixedBackend struct {
	inner  Backend
	prefix string
}

func (b *prefixedBackend) Name() string { return b.inner.Name() }

func (b *prefixedBackend) Put(ctx context.Context, key string, data []byte) error {
	return b.inner.Put(ctx, b.prefix+key, data)
}

func (b *prefixedBackend) Get(ctx context.Context, key string) ([]byte, error) {
	return b.inner.Get(ctx, b.prefix+key)
}

func (b *prefixedBackend) Delete(ctx context.Context, key string) error {
	return b.inner.Delete(ctx, b.prefix+key)
}

func (b *prefixedBackend) List(ctx context.Context, prefix string) ([]string, error) {
	keys, err := b.inner.List(ctx, b.prefix+prefix)
	if err != nil {
		return nil, err
	}
	stripped := make([]string, 0, len(keys))
	for _, key := range keys {
		relative, ok := strings.CutPrefix(key, b.prefix)
		if !ok {
			continue
		}
		stripped = append(stripped, relative)
	}
	return stripped, nil
}
