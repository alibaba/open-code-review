// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/vcs"
)

type memoryReviewSource struct {
	provider string
	resolves int
	changes  int
	last     vcs.ResolvedReview
	err      error
}

func TestLegacySealsPreserveWorkspaceAndIncompleteRangeInputs(t *testing.T) {
	for _, test := range []struct {
		name string
		args Args
		want vcs.ReviewSpec
	}{
		{name: "workspace", args: Args{SealedInput: &diff.InputResolution{ResolvedHead: "sealed-head"}}, want: vcs.ReviewSpec{Mode: vcs.Workspace}},
		{name: "commit", args: Args{Commit: "HEAD", SealedInput: &diff.InputResolution{ResolvedHead: "sealed-head"}}, want: vcs.ReviewSpec{Mode: vcs.Commit, Head: "sealed-head"}},
		{name: "range", args: Args{From: "main", To: "feature", SealedInput: &diff.InputResolution{ResolvedBase: "sealed-base", ResolvedHead: "sealed-head"}}, want: vcs.ReviewSpec{Mode: vcs.MergeBase, Base: "sealed-base", Head: "sealed-head"}},
		{name: "incomplete range seal", args: Args{From: "main", To: "feature", SealedInput: &diff.InputResolution{ResolvedHead: "sealed-head"}}, want: vcs.ReviewSpec{Mode: vcs.MergeBase, Base: "main", Head: "feature"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var received vcs.ReviewSpec
			test.args.Repository = &callbackReviewSource{
				resolve: func(_ context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
					received = spec
					return vcs.ResolvedReview{Provider: "example", Spec: spec}, spec.Validate()
				},
				changes: func(context.Context, vcs.ResolvedReview) (vcs.Changes, error) { return vcs.Changes{}, nil },
			}
			a := &Agent{args: test.args}
			if err := a.loadDiffs(context.Background()); err != nil || received != test.want {
				t.Fatalf("resolution request = %+v, want %+v: %v", received, test.want, err)
			}
		})
	}
}

func (s *memoryReviewSource) ResolveReview(_ context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
	s.resolves++
	return vcs.ResolvedReview{Provider: s.provider, RepositoryID: "example/repo", Spec: spec, Base: "r41", Head: "r42", Content: vcs.ContentRef{Revision: "r42"}}, s.err
}

func (s *memoryReviewSource) Changes(_ context.Context, review vcs.ResolvedReview) (vcs.Changes, error) {
	s.changes++
	s.last = review
	return vcs.Changes{Included: []model.Diff{{OldPath: "main.go", NewPath: "main.go", Diff: "@@ -1 +1 @@\n-old\n+new\n", Insertions: 1, Deletions: 1}}}, s.err
}

func TestRepositorySourceWorksWithoutGitAndReusesResolvedInput(t *testing.T) {
	source := &memoryReviewSource{provider: "example"}
	args := Args{RepoDir: t.TempDir(), Commit: "latest", Repository: source}
	sealed, err := ResolveIdentity(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if source.resolves != 1 || source.changes != 1 || sealed.Review.Head != "r42" {
		t.Fatalf("unexpected resolution: %+v, %+v", source, sealed)
	}
	args.ResolvedReview = &sealed.Review
	if got := runPathIdentity(t, args); got != sealed.Identity {
		t.Fatalf("sealed identity changed: %+v != %+v", got, sealed.Identity)
	}
	if source.resolves != 1 || source.changes != 2 {
		t.Fatalf("sealed input was resolved again: %+v", source)
	}
	preview, err := Preview(context.Background(), args)
	if err != nil || preview.ReviewableCount != 1 {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if repositoryBranch(context.Background(), args) != "" {
		t.Fatal("provider without branches must have an empty label")
	}
}

func TestRepositoryIdentityNamespacesNonGitSources(t *testing.T) {
	var previous string
	for _, kind := range []string{"git", "example", "another"} {
		source := &memoryReviewSource{provider: kind}
		sealed, err := ResolveIdentity(context.Background(), Args{Repository: source, Commit: "latest"})
		if err != nil {
			t.Fatal(err)
		}
		if sealed.Identity.RepositorySHA256 == previous {
			t.Fatal("different providers shared an identity")
		}
		previous = sealed.Identity.RepositorySHA256
		if kind != "git" && sealed.Resolution.ExactRange != "" {
			t.Fatal("non-Git revision received Git range syntax")
		}
	}
}

func TestRepositoryResolutionFailureStopsBeforeChanges(t *testing.T) {
	want := errors.New("revision unavailable")
	source := &memoryReviewSource{provider: "example", err: want}
	if _, err := ResolveIdentity(context.Background(), Args{Repository: source, Commit: "latest"}); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	if source.changes != 0 {
		t.Fatal("loaded changes after failed resolution")
	}
}

func TestResolvedSpecDrivesSessionAndManifestWithoutLegacyFlags(t *testing.T) {
	setTestHome(t, t.TempDir())
	t.Setenv("PATH", "")
	for _, spec := range []vcs.ReviewSpec{{Mode: vcs.Commit, Head: "r42"}, {Mode: vcs.TwoRevisions, Base: "r40", Head: "r42"}, {Mode: vcs.MergeBase, Base: "trunk", Head: "feature"}, {Mode: vcs.Workspace}} {
		source := &memoryReviewSource{provider: "example"}
		resolved, err := source.ResolveReview(context.Background(), spec)
		if err != nil {
			t.Fatal(err)
		}
		a := New(Args{RepoDir: t.TempDir(), Repository: source, ResolvedReview: &resolved})
		t.Cleanup(func() {
			if err := a.Session().Finalize(); err != nil {
				t.Error(err)
			}
		})
		if err := a.loadDiffs(context.Background()); err != nil {
			t.Fatal(err)
		}
		in := a.manifestInput()
		wantMode := session.InputModeRange
		if spec.Mode == vcs.Commit {
			wantMode = session.InputModeCommit
		}
		if spec.Mode == vcs.Workspace {
			wantMode = session.InputModeWorkspace
		}
		if in.Mode != wantMode || in.RequestedHead != spec.Head || in.RequestedFrom != spec.Base {
			t.Fatalf("spec %+v recorded as %+v", spec, in)
		}
		history := a.Session()
		head := history.DiffTo
		if spec.Mode == vcs.Commit {
			head = history.DiffCommit
		}
		if history.ReviewMode != wantMode || history.DiffFrom != spec.Base || head != spec.Head {
			t.Fatalf("semantic input lost from session metadata: %+v", history)
		}
	}
}
