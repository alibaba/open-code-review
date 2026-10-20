// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"

	"github.com/alibaba/open-code-review/internal/chunk"
)

// DiffMap is a read-only snapshot of parsed diffs, keyed by file path.
// Safe for concurrent reads after construction via NewDiffMap.
type DiffMap struct {
	m map[string]string
}

// NewDiffMap creates a frozen, read-only DiffMap from a plain map.
func NewDiffMap(m map[string]string) DiffMap {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return DiffMap{m: cp}
}

// Get returns the diff text for path.
func (d DiffMap) Get(path string) (string, bool) {
	v, ok := d.m[path]
	return v, ok
}

// FileReadDiffProvider retrieves diff content by file path from an already-parsed diff set.
//
// Reads are bounded. The provider serves one chunk at a time by default, names
// whatever it did not serve, and cannot be asked for more than the read budget
// it was configured with, so no single call can hand the model an unbounded
// blob however the model frames the request.
type FileReadDiffProvider struct {
	diffMap DiffMap
	// store is the run's chunk manifest when one was built. Nil means this run
	// does not register its context as chunks - scan mode, and a review whose
	// context fits the budget - in which case reads are chunked on the fly for
	// this call alone and coverage is not tracked.
	store atomic.Pointer[chunk.Store]
	// readBudget is the maximum token count of one result, as an atomic because
	// concurrent groups lower it as they plan their context and reads happen
	// from every group's main loop. Zero refuses to serve: a provider that has
	// not been given a budget fails closed rather than serving the unbounded
	// read it exists to prevent.
	readBudget atomic.Int64
}

func NewFileReadDiff(dm DiffMap) *FileReadDiffProvider {
	return &FileReadDiffProvider{diffMap: dm}
}

// SetDiffMap replaces the diff snapshot. Must be called before concurrent access begins.
func (p *FileReadDiffProvider) SetDiffMap(dm DiffMap) {
	p.diffMap = dm
}

// SetContextStore attaches the run's chunk store and the maximum token count of
// one read. Both arguments are set together: a store with no budget would
// reinstate the unbounded read this provider exists to prevent.
func (p *FileReadDiffProvider) SetContextStore(store *chunk.Store, readBudget int) {
	p.store.Store(store)
	p.readBudget.Store(int64(readBudget))
	if store != nil {
		// A path the run never registered still has a diff in this provider's
		// snapshot. Wiring the snapshot as the store's fallback keeps such a
		// read bounded and chunked instead of reporting a diff that exists as
		// missing.
		store.SetFallback(func(path string) (string, bool) { return p.diffMap.Get(path) })
	}
}

// LowerReadBudget tightens the per-read ceiling to tokens when that is smaller
// than the current one, and sets it when none is in force yet. After that first
// value it only ever tightens: a read must fit the budget of the group that
// issued it, and groups are dispatched concurrently, so the smallest budget any
// group has planned is the only value every group is guaranteed to honour.
func (p *FileReadDiffProvider) LowerReadBudget(tokens int) {
	if tokens <= 0 {
		return
	}
	for {
		cur := p.readBudget.Load()
		if cur != 0 && int64(tokens) >= cur {
			return
		}
		if p.readBudget.CompareAndSwap(cur, int64(tokens)) {
			return
		}
	}
}

func (p *FileReadDiffProvider) Tool() Tool { return FileReadDiff }

// Execute serves diff context. Three shapes, in precedence order:
//
//   - chunk_id: exactly that chunk, addressed by the identity the manifest gave
//     the model;
//   - path: the chunk index of one file, which is how a model reaches the ids
//     of a file whose rows the manifest could not afford to list;
//   - path_array: the chunks of those files, in order, up to the read budget.
//
// Whatever is not served is named in the result, so a bounded read is never
// mistaken for the whole change.
func (p *FileReadDiffProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	budget := int(p.readBudget.Load())
	if id := stringArg(args, "chunk_id"); id != "" {
		store, ok := p.chunkStoreFor(nil, budget)
		if !ok {
			return "Error: unknown chunk id \"" + id + "\". Use file_read_diff {\"path_array\": [\"<path>\"]} to read diff context.", nil
		}
		return store.Fetch(id), nil
	}

	path := stringArg(args, "path")
	if path != "" && !hasPaths(args) {
		store, ok := p.chunkStoreFor([]string{path}, budget)
		if !ok {
			return "Error: diff not found for the requested path", nil
		}
		return store.ListPath(path), nil
	}

	paths := stringSliceArg(args, "path_array")
	if len(paths) == 0 {
		return "Error: no files found", nil
	}

	store, ok := p.chunkStoreFor(paths, budget)
	if !ok {
		return "Error: diff not found for the requested paths", nil
	}
	return store.FetchPaths(paths, p.requestedBudget(args, budget)), nil
}

// requestedBudget honours a model's max_tokens request only downwards: the read
// budget is derived from the run's own context budget, and a model asking for
// more than that would be asking for the unbounded read this provider refuses.
func (p *FileReadDiffProvider) requestedBudget(args map[string]any, budget int) int {
	if v, ok := numericArg(args, "max_tokens"); ok && v > 0 && int(v) < budget {
		return int(v)
	}
	return budget
}

// chunkStoreFor returns the store to serve from. With a configured store that
// is the run's own manifest, so reads are counted and coverage is tracked.
// Without one, a store is built for this call alone from the diff snapshot: the
// read is still bounded and still chunked, it simply leaves no coverage record
// because the run never published a manifest to account against.
func (p *FileReadDiffProvider) chunkStoreFor(paths []string, budget int) (*chunk.Store, bool) {
	if s := p.store.Load(); s != nil {
		return s, true
	}
	if budget <= 0 {
		return nil, false
	}
	store := chunk.NewStore(budget)
	found := false
	for _, path := range paths {
		text, ok := p.diffMap.Get(path)
		if !ok {
			continue
		}
		found = true
		store.Add("", path, chunk.Split(path, text, chunk.Options{Budget: budget}))
	}
	if !found {
		return nil, false
	}
	return store, true
}

func hasPaths(args map[string]any) bool {
	paths, _ := args["path_array"].([]any)
	return len(paths) > 0
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func stringSliceArg(args map[string]any, key string) []string {
	raw, _ := args[key].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func numericArg(args map[string]any, key string) (float64, bool) {
	switch v := args[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
