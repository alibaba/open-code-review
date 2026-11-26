// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package behaviortask

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestFileFindPreservesSideScopeAndOrdering(test *testing.T) {
	code := &codeTools{filePaths: &[2]fileFindIndex{}, allowed: map[string]map[string]bool{
		"old": {"old/z.go": true, "old/a.go": true, "old/hidden.go": false},
		"new": {"new/z.go": true, "new/a.go": true, "new/hidden.go": false},
	}}
	for range 3 {
		for _, side := range []string{"old", "new"} {
			observation, err := code.execute(context.Background(), "file_find", map[string]any{"side": side, "query_name": ".go"})
			if err != nil {
				test.Fatal(err)
			}
			want := side + "/a.go\n" + side + "/z.go"
			if observation.ReturnedContent != want || observation.Truncated {
				test.Fatalf("side %s: got %+v, want %q", side, observation, want)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := code.execute(ctx, "file_find", map[string]any{"query_name": ".go"}); err != context.Canceled {
		test.Fatalf("canceled lookup returned %v", err)
	}
}

func TestFileFindConcurrentLookupsKeepComparisonSidesSeparate(test *testing.T) {
	code := &codeTools{filePaths: &[2]fileFindIndex{}, allowed: map[string]map[string]bool{
		"old": {"old/entry.go": true},
		"new": {"new/entry.go": true},
	}}
	var workers sync.WaitGroup
	for range 16 {
		for _, side := range []string{"old", "new"} {
			workers.Go(func() {
				observation, err := code.execute(context.Background(), "file_find", map[string]any{"side": side, "query_name": "entry.go"})
				if err != nil || observation.ReturnedContent != side+"/entry.go" {
					test.Errorf("side %s: got %+v, %v", side, observation, err)
				}
			})
		}
	}
	workers.Wait()
}

func BenchmarkBehaviorFileFindInventory(bench *testing.B) {
	for _, count := range []int{10000, 100000} {
		bench.Run(fmt.Sprintf("paths=%d", count), func(bench *testing.B) {
			files := make(map[string]bool, count)
			for index := range count {
				files[fmt.Sprintf("pkg/%06d/entry.go", index)] = true
			}
			code := &codeTools{filePaths: &[2]fileFindIndex{}, allowed: map[string]map[string]bool{"new": files}}
			args := map[string]any{"query_name": "entry.go", "case_sensitive": true}
			ctx := context.Background()
			first, err := code.execute(ctx, "file_find", args)
			if err != nil {
				bench.Fatal(err)
			}
			if !first.Truncated || len(strings.Split(first.ReturnedContent, "\n")) != 100 ||
				!strings.HasPrefix(first.ReturnedContent, "pkg/000000/entry.go\n") ||
				!strings.HasSuffix(first.ReturnedContent, "pkg/000099/entry.go") {
				bench.Fatalf("unexpected capped result: %+v", first)
			}
			bench.ReportAllocs()
			bench.ResetTimer()
			for range bench.N {
				observation, err := code.execute(ctx, "file_find", args)
				if err != nil || observation.ReturnedContent != first.ReturnedContent || !observation.Truncated {
					bench.Fatalf("file_find changed: %+v, %v", observation, err)
				}
			}
		})
	}
}
