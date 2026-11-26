// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestCommentCollectorPathReadsAfterMutations(t *testing.T) {
	var collector CommentCollector // The zero value remains usable.
	check := func(want ...model.LlmComment) {
		t.Helper()
		if got := collector.Comments(); !reflect.DeepEqual(got, append([]model.LlmComment{}, want...)) {
			t.Fatalf("comments = %+v, want %+v", got, want)
		}
		for _, path := range []string{"a.go", "b.go", "c.go", "", "missing.go"} {
			var expected []model.LlmComment
			for _, comment := range want {
				if comment.Path == path {
					expected = append(expected, comment)
				}
			}
			got := collector.CommentsForPath(path)
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("path %q: %+v, want %+v", path, got, expected)
			}
			if len(got) > 0 {
				got[0].Path, got[0].Content = "changed", "changed"
				if !reflect.DeepEqual(collector.CommentsForPath(path), expected) {
					t.Fatalf("path %q does not return an independent copy", path)
				}
			}
		}
	}
	check()
	for _, comment := range []model.LlmComment{cm("a.go", "a0"), cm("b.go", "b0"), cm("a.go", "a1"), cm("", "empty")} {
		collector.Add(comment)
	}
	check(cm("a.go", "a0"), cm("b.go", "b0"), cm("a.go", "a1"), cm("", "empty"))
	collector.RemoveByPathAndIndices("a.go", map[int]struct{}{0: {}})
	check(cm("b.go", "b0"), cm("a.go", "a1"), cm("", "empty"))
	collector.ReplaceSince(2, []model.LlmComment{cm("c.go", "c0"), cm("b.go", "b1")})
	check(cm("b.go", "b0"), cm("a.go", "a1"), cm("c.go", "c0"), cm("b.go", "b1"))
	collector.ReplaceSince(99, []model.LlmComment{cm("a.go", "ignored")})
	collector.RemoveByPathAndIndices("b.go", map[int]struct{}{99: {}})
	collector.RemoveByPathAndIndices("missing.go", map[int]struct{}{0: {}})
	check(cm("b.go", "b0"), cm("a.go", "a1"), cm("c.go", "c0"), cm("b.go", "b1"))
	collector.ReplaceSince(collector.Snapshot(), []model.LlmComment{cm("a.go", "a2")})
	check(cm("b.go", "b0"), cm("a.go", "a1"), cm("c.go", "c0"), cm("b.go", "b1"), cm("a.go", "a2"))
	collector.ReplaceSince(-1, nil)
	check()
	collector.Add(cm("b.go", "after reset"))
	check(cm("b.go", "after reset"))
	collector.RemoveByPathAndIndices("b.go", map[int]struct{}{0: {}})
	check()
	collector.ReplaceSince(0, []model.LlmComment{cm("c.go", "from empty")})
	check(cm("c.go", "from empty"))
}

func TestCommentCollectorConcurrentPathReadsAndFiltering(t *testing.T) {
	collector := NewCommentCollector()
	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			path := fmt.Sprintf("worker-%d.go", worker)
			for i := range 20 {
				collector.Add(cm(path, fmt.Sprint(i)))
				got := collector.CommentsForPath(path)
				if len(got) != i+1 || got[i].Content != fmt.Sprint(i) {
					t.Errorf("path %q: unexpected findings after append: %+v", path, got)
					return
				}
			}
			collector.RemoveByPathAndIndices(path, map[int]struct{}{0: {}, 19: {}})
			got := collector.CommentsForPath(path)
			if len(got) != 18 || got[0].Content != "1" || got[17].Content != "18" {
				t.Errorf("path %q: unexpected findings after filtering: %+v", path, got)
			}
		})
	}
	workers.Wait()
	if got := collector.Snapshot(); got != 8*18 {
		t.Fatalf("snapshot = %d, want %d", got, 8*18)
	}
}
