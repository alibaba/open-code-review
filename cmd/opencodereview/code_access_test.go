// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/tool"
)

func TestPrepareReviewCodeTools(test *testing.T) {
	repo := initResumeRepo(test)
	commitFile(test, repo, "version.go", "package example\nconst Value = 1\n", "old")
	previous := revParse(test, repo, "HEAD")
	commitFile(test, repo, "version.go", "package example\nconst Value = 2\n", "new")
	head := revParse(test, repo, "HEAD")
	runner := gitcmd.New(2)
	for _, opts := range []reviewOptions{{from: previous, to: "HEAD"}, {commit: "HEAD"}} {
		setup, err := prepareReviewCodeTools(context.Background(), repo, opts, runner, nil)
		if err != nil {
			test.Fatal(err)
		}
		if setup.access == nil || setup.resolution.ResolvedHead != head || setup.resolution.ResolvedBase != previous {
			test.Fatalf("incorrect setup: %+v", setup)
		}
		registry := buildToolRegistryWithCodeAccess(nil, setup.reader, setup.access)
		provider, _ := registry.Get("file_read")
		for _, side := range []string{"old", "new"} {
			got, err := provider.Execute(context.Background(), map[string]any{"file_path": "version.go", "side": side})
			want := "Value = 2"
			if side == "old" {
				want = "Value = 1"
			}
			if err != nil || !strings.Contains(got, want) {
				test.Fatalf("%s: %q %v", side, got, err)
			}
		}
	}
	opts := reviewOptions{commit: "HEAD"}
	setup, err := prepareReviewCodeTools(context.Background(), repo, opts, runner, nil)
	if err != nil {
		test.Fatal(err)
	}
	commitFile(test, repo, "version.go", "package example\nconst Value = 3\n", "advance")
	got, err := setup.reader.Read(context.Background(), "version.go")
	if err != nil || !strings.Contains(got, "Value = 2") {
		test.Fatalf("new side moved with HEAD: %q %v", got, err)
	}
	for _, resumedOpts := range []reviewOptions{opts, {from: previous, to: "HEAD"}} {
		resumed, err := prepareReviewCodeTools(context.Background(), repo, resumedOpts, runner, setup.resolution)
		if err != nil {
			test.Fatal(err)
		}
		if resumed.access == nil || resumed.reader.Ref != head {
			test.Fatalf("resume lost comparison access or snapshot: %+v", resumed)
		}
	}
	workspace, err := prepareReviewCodeTools(context.Background(), repo, reviewOptions{}, runner, nil)
	if err != nil || workspace.access != nil || workspace.reader.Mode != tool.ModeWorkspace || workspace.reader.Ref != "" {
		test.Fatalf("workspace changed: %+v %v", workspace, err)
	}
	root := revParse(test, repo, "HEAD~4")
	empty, err := prepareReviewCodeTools(context.Background(), repo, reviewOptions{commit: root}, runner, nil)
	if err != nil || !empty.resolution.BaseIsEmpty || empty.access == nil {
		test.Fatalf("root comparison: %+v %v", empty, err)
	}
}

func TestPrepareReviewCodeToolsErrors(test *testing.T) {
	repo := initResumeRepo(test)
	for _, scenario := range []struct {
		opts       reviewOptions
		resolution *diff.InputResolution
	}{
		{opts: reviewOptions{commit: "missing"}},
		{opts: reviewOptions{commit: "HEAD"}, resolution: &diff.InputResolution{}},
		{opts: reviewOptions{commit: "HEAD"}, resolution: &diff.InputResolution{ResolvedHead: "HEAD"}},
	} {
		if _, err := prepareReviewCodeTools(context.Background(), repo, scenario.opts, nil, scenario.resolution); err == nil {
			test.Fatalf("accepted invalid setup: %+v", scenario)
		}
	}
}
