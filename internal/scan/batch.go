package scan

import (
	"sort"
	"strings"

	"github.com/open-code-review/open-code-review/internal/model"
)

// BatchStrategy enumerates the grouping policies for scan dispatch.
type BatchStrategy string

const (
	// BatchNone treats every file as its own batch (v1 behavior).
	BatchNone BatchStrategy = "none"
	// BatchByLanguage groups files by extension (case-insensitive).
	BatchByLanguage BatchStrategy = "by-language"
	// BatchByDirectory groups files by their first-level directory under
	// the repo root. Files directly in the root form their own batch.
	BatchByDirectory BatchStrategy = "by-directory"
)

// parseBatchStrategy normalizes a user-supplied strategy string. Unknown
// or empty values fall back to BatchNone (safe v1 behavior).
func parseBatchStrategy(s string) BatchStrategy {
	switch BatchStrategy(strings.ToLower(strings.TrimSpace(s))) {
	case BatchByLanguage:
		return BatchByLanguage
	case BatchByDirectory:
		return BatchByDirectory
	default:
		return BatchNone
	}
}

// groupBatches partitions items according to strategy, then slices each
// natural group into BatchSize-sized chunks (when size > 0). Within a batch
// the input order is preserved; batches themselves are sorted by their
// group key for determinism.
//
// Returns nil when items is empty.
func groupBatches(items []model.ScanItem, strategy BatchStrategy, size int) [][]model.ScanItem {
	if len(items) == 0 {
		return nil
	}

	// Bucket by group key.
	keyFn := batchKeyFunc(strategy)
	buckets := make(map[string][]model.ScanItem)
	for _, it := range items {
		key := keyFn(it)
		buckets[key] = append(buckets[key], it)
	}

	// Deterministic batch order via sorted keys.
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out [][]model.ScanItem
	for _, k := range keys {
		group := buckets[k]
		if size <= 0 || len(group) <= size {
			out = append(out, group)
			continue
		}
		// Chunk the natural group into BatchSize-sized slices.
		for start := 0; start < len(group); start += size {
			end := start + size
			if end > len(group) {
				end = len(group)
			}
			out = append(out, group[start:end])
		}
	}
	return out
}

// batchKeyFunc returns the grouping key extractor for a strategy.
func batchKeyFunc(strategy BatchStrategy) func(model.ScanItem) string {
	switch strategy {
	case BatchByLanguage:
		return languageKey
	case BatchByDirectory:
		return firstLevelDirKey
	default:
		// BatchNone: each file is its own batch.
		return func(it model.ScanItem) string { return it.Path }
	}
}

// languageKey returns the lowercased file extension (with the leading dot)
// or "<no-ext>" for extensionless files.
func languageKey(it model.ScanItem) string {
	base := it.Path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	dot := strings.LastIndex(base, ".")
	if dot <= 0 {
		return "<no-ext>"
	}
	return strings.ToLower(base[dot:])
}

// firstLevelDirKey returns the first path segment of a repo-relative path,
// or "<root>" for files directly in the repo root.
func firstLevelDirKey(it model.ScanItem) string {
	idx := strings.IndexByte(it.Path, '/')
	if idx < 0 {
		return "<root>"
	}
	return it.Path[:idx]
}
