// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package vcs_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func TestRequestsCannotSilentlySelectAnotherComparison(t *testing.T) {
	// Supplying a base for commit/workspace, or omitting a range endpoint,
	// must never silently change which revisions a provider compares.
	for _, spec := range []vcs.ReviewSpec{
		{Mode: vcs.Workspace, Base: "r1"}, {Mode: vcs.Workspace, Head: "r2"},
		{Mode: vcs.Commit}, {Mode: vcs.Commit, Base: "r1", Head: "r2"},
		{Mode: vcs.MergeBase, Base: "r1"}, {Mode: vcs.MergeBase, Head: "r2"},
		{Mode: vcs.TwoRevisions, Base: "r1"}, {Mode: vcs.TwoRevisions, Head: "r2"},
	} {
		if err := spec.Validate(); err == nil {
			t.Fatalf("accepted ambiguous request %+v", spec)
		}
	}
	for _, spec := range []vcs.ReviewSpec{
		{Mode: vcs.Workspace}, {Mode: vcs.Commit, Head: "r2"},
		{Mode: vcs.MergeBase, Base: "trunk", Head: "branch"},
		{Mode: vcs.TwoRevisions, Base: "r1", Head: "r2"},
	} {
		if err := spec.Validate(); err != nil {
			t.Fatalf("rejected %+v: %v", spec, err)
		}
	}
	if err := (vcs.ReviewSpec{Mode: "index-only"}).Validate(); !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatalf("unsupported mode: %v", err)
	}
	if !(vcs.ContentRef{}).IsWorkspace() || (vcs.ContentRef{Revision: "r2"}).IsWorkspace() {
		t.Fatal("workspace and revision content were confused")
	}
}

func TestChangesPreserveProviderExclusionOrder(t *testing.T) {
	changes := vcs.Changes{
		Included:   []model.Diff{{NewPath: "a.go"}, {NewPath: "c.go"}},
		Excluded:   []model.Diff{{NewPath: "vendor/b.go"}, {NewPath: "vendor/d.go"}},
		ExcludedAt: []int{1, 3},
	}
	var paths []string
	changes.ForEachInOrder(func(d model.Diff, excluded bool) {
		if excluded != strings.HasPrefix(d.NewPath, "vendor/") {
			t.Errorf("exclusion flag lost for %q", d.NewPath)
		}
		paths = append(paths, d.NewPath)
	})
	if !slices.Equal(paths, []string{"a.go", "vendor/b.go", "c.go", "vendor/d.go"}) {
		t.Fatalf("changeset order = %v", paths)
	}
	changes.ExcludedAt = nil
	paths = nil
	changes.ForEachInOrder(func(d model.Diff, _ bool) { paths = append(paths, d.NewPath) })
	if !slices.Equal(paths, []string{"a.go", "c.go", "vendor/b.go", "vendor/d.go"}) {
		t.Fatalf("default changeset order = %v", paths)
	}
}
