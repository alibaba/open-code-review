// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"fmt"
	"testing"
)

// A sweep matches the per-file reads used by review filtering and checkpoint
// collection. Include paths without findings, which must not scan unrelated
// comments either.
func BenchmarkCommentCollectorPathSweep(b *testing.B) {
	for _, files := range []int{20, 1000, 10000} {
		b.Run(fmt.Sprintf("files=%d", files), func(b *testing.B) {
			collector := NewCommentCollector()
			paths := make([]string, files)
			for i := range paths {
				paths[i] = fmt.Sprintf("src/package-%06d/file.go", i)
				if i%4 == 0 {
					continue
				}
				for j := range 3 {
					collector.Add(cm(paths[i], fmt.Sprintf("finding %d", j)))
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				count := 0
				for _, path := range paths {
					count += len(collector.CommentsForPath(path))
				}
				if count != files/4*9 {
					b.Fatalf("found %d comments", count)
				}
			}
		})
	}
}

func BenchmarkCommentCollectorPopulate(b *testing.B) {
	for _, files := range []int{20, 1000, 10000} {
		b.Run(fmt.Sprintf("files=%d", files), func(b *testing.B) {
			paths := make([]string, files)
			for i := range paths {
				paths[i] = fmt.Sprintf("src/package-%06d/file.go", i)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				collector := NewCommentCollector()
				for _, path := range paths {
					for range 3 {
						collector.Add(cm(path, "finding"))
					}
				}
			}
		})
	}
}

// Include collection, per-file filtering and subsequent checkpoint reads. Each
// removal still compacts the ordered store; path lookups must not make that
// mutation-heavy workflow more expensive by rebuilding every other path.
func BenchmarkCommentCollectorFilterPass(b *testing.B) {
	for _, files := range []int{20, 1000, 10000} {
		for _, removeEvery := range []int{1, 5} {
			b.Run(fmt.Sprintf("files=%d/remove_every=%d", files, removeEvery), func(b *testing.B) {
				paths := make([]string, files)
				for i := range paths {
					paths[i] = fmt.Sprintf("src/package-%06d/file.go", i)
				}
				indices := map[int]struct{}{0: {}}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					collector := NewCommentCollector()
					for _, path := range paths {
						for range 3 {
							collector.Add(cm(path, "finding"))
						}
					}
					for i, path := range paths {
						if len(collector.CommentsForPath(path)) != 3 {
							b.Fatal("unexpected filter input")
						}
						if i%removeEvery == 0 {
							collector.RemoveByPathAndIndices(path, indices)
						}
					}
					count := 0
					for _, path := range paths {
						count += len(collector.CommentsForPath(path))
					}
					if count != files*3-files/removeEvery {
						b.Fatalf("found %d comments after filtering", count)
					}
				}
			})
		}
	}
}
