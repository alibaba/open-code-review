// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func TestPreparedReviewPinsDiffRulesAndFileTools(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "source.m", "#import <Foundation/Foundation.h>\n", "add Objective-C source")
	cc, err := loadCommonContext(dir, "", "HEAD", 0, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	opts := reviewOptions{commit: "HEAD"}
	ctx := context.Background()
	if err := prepareRepositoryReview(ctx, cc, opts); err != nil {
		t.Fatal(err)
	}
	args := agent.Args{RepoDir: dir, Commit: opts.commit, Repository: cc.Repository, ResolvedReview: cc.Review, SystemRule: cc.Resolver, FileFilter: cc.FileFilter}
	sealed, err := agent.ResolveIdentity(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	gitCommitFile(t, dir, "source.m", "% MATLAB source\nx = 1;\n", "move HEAD")
	codeTools, err := prepareReviewCodeTools(ctx, dir, opts, cc.GitRunner, nil, cc.Repository, cc.Review)
	if err != nil {
		t.Fatal(err)
	}
	reader := codeTools.reader
	if codeTools.resolution.ExactRange != sealed.Resolution.ExactRange {
		t.Fatalf("code tools lost the sealed exact range: %+v", codeTools.resolution)
	}
	content, err := reader.Read(ctx, "source.m")
	if err != nil || !strings.HasPrefix(content, "#import") {
		t.Fatalf("file tool read moved content: %q, %v", content, err)
	}
	detail := cc.Resolver.(rules.DetailResolver).ResolveDetail("source.m")
	if detail.SniffedAs != "objc" {
		t.Fatalf("rule resolver read moved content: %+v", detail)
	}
	args.ResolvedReview = &sealed.Review
	again, err := agent.ResolveIdentity(ctx, args)
	if err != nil || again.Identity != sealed.Identity {
		t.Fatalf("review changed after HEAD moved: %+v, %v", again, err)
	}
	search, err := tool.NewCodeSearch(reader).Execute(ctx, map[string]any{"search_text": "Foundation"})
	if err != nil || !strings.Contains(search, "source.m") {
		t.Fatalf("search read moved content: %q, %v", search, err)
	}
}

func TestHistoricalReviewCancellationPreservesPreflightError(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "source.go", "package source\n", "add source")
	gitCommitFile(t, dir, "source.go", "package source\nconst value = 1\n", "update source")
	for _, opts := range []reviewOptions{{repoDir: dir, commit: "HEAD"}, {repoDir: dir, from: "HEAD~1", to: "HEAD"}} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := executeReviewContext(ctx, opts)
		if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "resolve review input:") {
			t.Fatalf("preflight failure was replaced: %v", err)
		}
	}
}

// preparedRepository serves an opaque revision without a Git executable or
// working-copy files. Consumers must preserve the provider's content reference.
type preparedRepository struct {
	resolved vcs.ResolvedReview
	requests []vcs.ReviewSpec
	err      error
	changes  int
}

func (r *preparedRepository) ResolveReview(ctx context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
	r.requests = append(r.requests, spec)
	if err := ctx.Err(); err != nil {
		return vcs.ResolvedReview{}, err
	}
	return r.resolved, r.err
}

func (r *preparedRepository) Changes(_ context.Context, resolved vcs.ResolvedReview) (vcs.Changes, error) {
	r.changes++
	if resolved != r.resolved {
		return vcs.Changes{}, fmt.Errorf("changes received a different resolved review: %+v", resolved)
	}
	return vcs.Changes{Included: []model.Diff{
		{OldPath: "source.m", NewPath: "source.m", Insertions: 1, Diff: "+#import <Foundation/Foundation.h>"},
		{NewPath: "generated.go", IsNew: true, Insertions: 2, Diff: "+package generated"},
	}}, nil
}

func (r *preparedRepository) ReadFile(_ context.Context, ref vcs.ContentRef, path string, consume func(io.Reader) error) error {
	if ref != r.resolved.Content || path != "source.m" {
		return fmt.Errorf("unexpected content request: %+v %q", ref, path)
	}
	return consume(strings.NewReader("#import <Foundation/Foundation.h>\n"))
}

func (r *preparedRepository) ListFiles(_ context.Context, ref vcs.ContentRef) ([]string, error) {
	if ref != r.resolved.Content {
		return nil, fmt.Errorf("unexpected list reference: %+v", ref)
	}
	return []string{"source.m"}, nil
}

func TestPreparedReviewUsesNonGitRepositoryAcrossConsumers(t *testing.T) {
	freshOCRHome(t)
	t.Setenv("PATH", "")
	dir := t.TempDir()
	// Conflicting local content would be classified as MATLAB if any consumer
	// silently fell back to the working copy.
	if err := os.WriteFile(filepath.Join(dir, "source.m"), []byte("% MATLAB\nx = 1;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := vcs.ReviewSpec{Mode: vcs.MergeBase, Base: "trunk", Head: "feature"}
	source := &preparedRepository{resolved: vcs.ResolvedReview{
		Provider: "memory", RepositoryID: "example/repo", Spec: spec,
		Base: "revision:41", Head: "revision:42", Content: vcs.ContentRef{Revision: "revision:42"},
	}}
	cc := &commonContext{RepoDir: dir, Repository: source}
	ctx := context.Background()
	if err := prepareRepositoryReview(ctx, cc, reviewOptions{from: spec.Base, to: spec.Head, excludes: "generated.go"}); err != nil {
		t.Fatal(err)
	}
	dc := &delegateContext{cc: cc, opts: delegateOptions{from: spec.Base, to: spec.Head}}
	preview, err := dc.preview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if preview.TotalFiles != 2 || preview.ReviewableCount != 1 || preview.ExcludedCount != 1 {
		t.Fatalf("provider selection or CLI exclusions lost: %+v", preview)
	}
	if base := dc.mergeBase(ctx); base != source.resolved.Base {
		t.Fatalf("merge base = %q, want %q", base, source.resolved.Base)
	}
	if detail := cc.Resolver.(rules.DetailResolver).ResolveDetail("source.m"); detail.SniffedAs != "objc" {
		t.Fatalf("rules did not inspect the provider's fixed revision: %+v", detail)
	}
	reader := &tool.FileReader{RepoDir: dir, Source: cc.Repository, Content: cc.Review.Content}
	content, err := reader.Read(ctx, "source.m")
	if err != nil || content != "#import <Foundation/Foundation.h>\n" {
		t.Fatalf("provider content = %q, error = %v", content, err)
	}
	paths, err := tool.NewFileFind(reader).Execute(ctx, map[string]any{"query_name": "source"})
	if err != nil || paths != "source.m" {
		t.Fatalf("provider files = %q, error = %v", paths, err)
	}
	if _, err := tool.NewCodeSearch(reader).Execute(ctx, map[string]any{"search_text": "Foundation"}); !errors.Is(err, vcs.ErrUnsupported) {
		t.Fatalf("missing optional search capability must be reported: %v", err)
	}
	if len(source.requests) != 1 || source.requests[0] != spec || source.changes != 1 {
		t.Fatalf("prepared input was resolved again: requests=%+v, changes=%d", source.requests, source.changes)
	}
}

func TestPrepareRepositoryReviewFailurePreservesPreparedState(t *testing.T) {
	for _, name := range []string{"resolve", "cancelled", "rules"} {
		t.Run(name, func(t *testing.T) {
			freshOCRHome(t)
			source := &preparedRepository{}
			cc := &commonContext{RepoDir: t.TempDir(), Repository: source}
			resolver, filter, err := rules.NewResolver(cc.RepoDir, "", rules.ResolverOptions{Files: source})
			if err != nil {
				t.Fatal(err)
			}
			prior := &vcs.ResolvedReview{Provider: "previous"}
			cc.Review, cc.Resolver, cc.FileFilter = prior, resolver, filter
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := reviewOptions{excludes: "generated.go"}
			var cause error
			wantPrefix := "resolve review input:"
			switch name {
			case "resolve":
				cause = vcs.ErrUnsupported
				source.err = fmt.Errorf("provider cannot resolve this mode: %w", cause)
			case "cancelled":
				cause = context.Canceled
				cancel()
			case "rules":
				opts.rulePath = filepath.Join(t.TempDir(), "rule.json")
				if err := os.WriteFile(opts.rulePath, []byte("{broken"), 0600); err != nil {
					t.Fatal(err)
				}
				wantPrefix = "load rules:"
			}
			err = prepareRepositoryReview(ctx, cc, opts)
			if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) || (cause != nil && !errors.Is(err, cause)) {
				t.Fatalf("preparation error = %v, want prefix %q and cause %v", err, wantPrefix, cause)
			}
			if cc.Review != prior || cc.Resolver != resolver || cc.FileFilter != filter || source.changes != 0 {
				t.Fatal("failed preparation replaced prepared state or read changes")
			}
		})
	}
}

func TestPrepareRepositoryReviewDefaultsToGit(t *testing.T) {
	freshOCRHome(t)
	dir := initTestGitRepo(t)
	gitCommitFile(t, dir, "source.go", "package source\n", "add source")
	cc := &commonContext{RepoDir: dir}
	if err := prepareRepositoryReview(context.Background(), cc, reviewOptions{commit: "HEAD"}); err != nil {
		t.Fatal(err)
	}
	if cc.Repository == nil || cc.Review.Provider != "git" || cc.Review.Head == "" || cc.Review.Content.Revision != cc.Review.Head {
		t.Fatalf("default Git provider did not pin content: %+v", cc.Review)
	}
}
