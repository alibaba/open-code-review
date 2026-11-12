// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package vcs defines the repository operations used by review and scan.
// Providers interpret revisions and produce normalized changes; consumers do
// not construct version-control command lines or parse provider-specific diffs.
package vcs

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/model"
)

// ErrUnsupported means the provider cannot implement the requested semantics.
// Callers must report it rather than substitute another comparison or version.
var ErrUnsupported = errors.New("unsupported repository operation")

type ReviewMode string

const (
	Workspace    ReviewMode = "workspace"
	Commit       ReviewMode = "commit"
	MergeBase    ReviewMode = "merge-base"
	TwoRevisions ReviewMode = "two-revisions"
)

// ReviewSpec distinguishes a revision's changes, a direct comparison, and a
// comparison from a common ancestor. Revision strings are opaque to consumers.
type ReviewSpec struct {
	Mode ReviewMode
	Base string
	Head string
}

// ContentRef selects either mutable workspace content or an immutable revision.
// The zero value selects the workspace. Historical reads never fall back to it.
type ContentRef struct {
	Revision string
}

func (r ContentRef) IsWorkspace() bool { return r.Revision == "" }

// ResolvedReview pins historical input before selection or session creation.
// Base may be empty for an initial revision or an unborn workspace. A workspace
// has no immutable Head; Content then remains the zero value.
type ResolvedReview struct {
	Provider     string
	RepositoryID string
	BaseIsEmpty  bool
	Spec         ReviewSpec
	Base         string
	Head         string
	Content      ContentRef
}

// Changes includes exclusions made by the provider so preview can account for
// the complete changeset. Every entry uses the same normalized Diff model.
type Changes struct {
	Included []model.Diff
	Excluded []model.Diff
	// ExcludedAt gives each excluded entry's position in the combined changeset.
	// Positions must be ascending and refer only to Included and Excluded.
	// When omitted, included entries precede excluded entries.
	ExcludedAt []int
}

// ForEachInOrder visits included and excluded files in changeset order.
func (s Changes) ForEachInOrder(visit func(model.Diff, bool)) {
	next := 0
	for i, d := range s.Excluded {
		upTo := len(s.Included)
		if i < len(s.ExcludedAt) {
			upTo = s.ExcludedAt[i] - i
		}
		for ; next < upTo; next++ {
			visit(s.Included[next], false)
		}
		visit(d, true)
	}
	for ; next < len(s.Included); next++ {
		visit(s.Included[next], false)
	}
}

type ReviewSource interface {
	ResolveReview(context.Context, ReviewSpec) (ResolvedReview, error)
	Changes(context.Context, ResolvedReview) (Changes, error)
}

// FileSource reads repository-relative slash-separated paths at the selected
// version. ReadFile owns the stream and closes/reaps it when consume returns,
// including early returns and cancellation. Implementations must enforce path
// containment and report read/process failures. ListFiles respects the VCS's
// ignore rules and includes untracked workspace files, without OCR rule filters.
type FileSource interface {
	ReadFile(ctx context.Context, ref ContentRef, path string, consume func(io.Reader) error) error
	ListFiles(ctx context.Context, ref ContentRef) ([]string, error)
}

type SearchQuery struct {
	Text          string
	CaseSensitive bool
	PerlRegexp    bool
	Patterns      []string
	Limit         int
}

type Match struct {
	Path string
	Line int
	Text string
}

type SearchResult struct {
	Matches []Match
	// MatchedFiles counts all matching files, including those omitted by Limit.
	MatchedFiles int
	Truncated    bool
	Warning      string
}

// SearchSource is optional. Unsupported pattern syntax must return
// ErrUnsupported, never silently change the meaning of the query.
type SearchSource interface {
	Search(context.Context, ContentRef, SearchQuery) (SearchResult, error)
}

type Repository interface {
	ReviewSource
	FileSource
}

// BranchSource optionally supplies a display label for session history. A
// provider without branches can omit it; review correctness never depends on it.
type BranchSource interface {
	CurrentBranch(context.Context) (string, error)
}

// Validate rejects incomplete or contradictory requests before provider work.
func (s ReviewSpec) Validate() error {
	switch s.Mode {
	case Workspace:
		if s.Base == "" && s.Head == "" {
			return nil
		}
	case Commit:
		if s.Base == "" && s.Head != "" {
			return nil
		}
	case MergeBase, TwoRevisions:
		if s.Base != "" && s.Head != "" {
			return nil
		}
	default:
		return fmt.Errorf("%w: review mode %q", ErrUnsupported, s.Mode)
	}
	return fmt.Errorf("invalid %s review endpoints", s.Mode)
}
