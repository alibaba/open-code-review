// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/stdout"
	"github.com/alibaba/open-code-review/internal/vcs"
	gitvcs "github.com/alibaba/open-code-review/internal/vcs/git"
)

// SealedInput is what a pre-flight resolve froze: the identity it computed, and
// the commit endpoints it computed that identity from.
//
// Resolution is the point of this type. Handing it back to the run pins the run
// to the same immutable commits, so the second diff load cannot disagree with
// the first — which is what lets the resume decision stay entirely before the
// run exists. Without it, a ref that moved after admission would be discovered
// only mid-run, once a child session and manifest were already on disk.
type SealedInput struct {
	Identity   session.RunIdentity
	Resolution diff.InputResolution
	Review     vcs.ResolvedReview
}

// ResolveIdentity computes the identity a review with these args would record,
// without creating anything: no session, manifest, runner or LLM call. Resume
// needs it before it may reuse a checkpoint, and a rejected resume must leave no
// trace on disk, so the identity has to be available strictly before
// session.New — which writes session_start the moment it is called.
//
// It reproduces the run's selection exactly — the same diff load followed by the
// same selectFiles pass — because the identity is derived from the diffs that
// selection retains, not from every parsed diff. See runIdentity for what
// skipping it would cost.
//
// The review that follows in the same command reports its own diff load and
// selection, so this stays silent. stdout.Quiet is safe here for the reason it
// documents: this is pre-flight work on the main goroutine, before any
// concurrent output exists.
func ResolveIdentity(ctx context.Context, args Args) (*SealedInput, error) {
	defer stdout.Quiet()()

	a := &Agent{args: args}
	if err := a.loadDiffs(ctx); err != nil {
		return nil, fmt.Errorf("load diffs: %w", err)
	}
	a.diffs, _ = summarizeSelection(a.selectFiles(a.diffs))
	return &SealedInput{Identity: a.runIdentity(), Resolution: a.inputResolution, Review: a.resolvedReview}, nil
}

func ResolveComparisonInput(ctx context.Context, args Args) (*diff.InputResolution, error) {
	a := &Agent{args: args}
	if review := args.ResolvedReview; review != nil {
		resolution := inputResolutionForReview(*review)
		return &resolution, nil
	}
	source := args.Repository
	if source == nil {
		source = gitvcs.New(args.RepoDir, args.GitRunner)
	}
	review, err := source.ResolveReview(ctx, a.reviewSpec())
	if err != nil {
		return nil, err
	}
	resolution := inputResolutionForReview(review)
	return &resolution, nil
}

// runIdentity reads the identity off the agent's current selection.
//
// It is only meaningful once the selection is final: sourceArtifactSHA256 hashes
// whatever a.diffs holds, so calling it before selectFiles has been applied
// yields a digest no run ever records, and a resume comparing that digest
// against a parent manifest would reject work it should have reused.
func (a *Agent) runIdentity() session.RunIdentity {
	id := session.RunIdentity{
		Mode:                 a.manifestMode(),
		ResolvedBase:         a.inputResolution.ResolvedBase,
		ResolvedHead:         a.inputResolution.ResolvedHead,
		SourceArtifactSHA256: a.sourceArtifactSHA256(),
		RuleConfigSHA256:     a.ruleConfigSHA256(),
	}
	if raw := a.repoRemoteIdentity; raw != "" {
		sum := sha256.Sum256([]byte(raw))
		id.RepositorySHA256 = hex.EncodeToString(sum[:])
	}
	return id
}
