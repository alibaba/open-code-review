// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspacePreviewAndReviewShareBaseline(test *testing.T) {
	repo := initPreviewRepo(test)
	commitIn(test, repo, "version.go", "package example\nconst Value = 1\n", "base")
	args := Args{RepoDir: repo}
	base, err := ResolveComparisonInput(context.Background(), args)
	if err != nil || base == nil {
		test.Fatalf("resolve workspace: %+v %v", base, err)
	}
	args.SealedInput = base
	commitIn(test, repo, "version.go", "package example\nconst Value = 2\n", "advance")
	if err := os.WriteFile(filepath.Join(repo, "version.go"), []byte("package example\nconst Value = 3\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	previewAgent := &Agent{args: args}
	preview, err := previewAgent.preview(context.Background())
	if err != nil {
		test.Fatal(err)
	}
	reviewAgent := &Agent{args: args}
	if err := reviewAgent.loadDiffs(context.Background()); err != nil {
		test.Fatal(err)
	}
	if preview.ReviewableCount != 1 || !reflect.DeepEqual(previewAgent.diffs, reviewAgent.diffs) || reviewAgent.inputResolution != *base {
		test.Fatalf("preview and review disagree: %+v %+v", preview, reviewAgent.inputResolution)
	}
	if !strings.Contains(reviewAgent.diffs[0].Diff, "-const Value = 1") || strings.Contains(reviewAgent.diffs[0].Diff, "Value = 2") {
		test.Fatalf("sealed workspace base ignored: %s", reviewAgent.diffs[0].Diff)
	}
	if err := os.WriteFile(filepath.Join(repo, "version.go"), []byte("package example\nconst Value = 2\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	gitIn(test, repo, "add", "version.go")
	if err := os.WriteFile(filepath.Join(repo, "version.go"), []byte("package example\nconst Value = 1\n"), 0o644); err != nil {
		test.Fatal(err)
	}
	preview, err = Preview(context.Background(), args)
	if err != nil || preview.TotalFiles != 0 {
		test.Fatalf("preview fell back to staged changes: %+v %v", preview, err)
	}
	if err := reviewAgent.loadDiffs(context.Background()); err != nil || len(reviewAgent.diffs) != 0 {
		test.Fatalf("review fell back to staged changes: %+v %v", reviewAgent.diffs, err)
	}
}

func TestWorkspacePreviewAndReviewDoNotUseIndex(test *testing.T) {
	for _, name := range []string{"committed", "unborn"} {
		test.Run(name, func(test *testing.T) {
			repo := initPreviewRepo(test)
			if name == "committed" {
				commitIn(test, repo, "version.go", "package example\nconst Value = 1\n", "base")
			}
			path := filepath.Join(repo, "version.go")
			if err := os.WriteFile(path, []byte("package example\nconst Value = 2\n"), 0o644); err != nil {
				test.Fatal(err)
			}
			gitIn(test, repo, "add", "version.go")
			if name == "committed" {
				if err := os.WriteFile(path, []byte("package example\nconst Value = 1\n"), 0o644); err != nil {
					test.Fatal(err)
				}
			} else if err := os.Remove(path); err != nil {
				test.Fatal(err)
			}
			args := Args{RepoDir: repo}
			preview, err := Preview(context.Background(), args)
			if err != nil || preview.TotalFiles != 0 {
				test.Fatalf("preview fell back to staged content: %+v %v", preview, err)
			}
			review := &Agent{args: args}
			if err := review.loadDiffs(context.Background()); err != nil || len(review.diffs) != 0 {
				test.Fatalf("review fell back to staged content: %+v %v", review.diffs, err)
			}
		})
	}
}
