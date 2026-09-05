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
}

// ResolveIdentity computes the identity a review with these args would record,
// without creating anything: no session, manifest, runner or LLM call. Resume
// needs it before it may reuse a checkpoint, and a rejected resume must leave no
// trace on disk, so the identity has to be available strictly before
// session.New — which writes session_start the moment it is called.
//
// It reproduces the run's selection exactly — the same diff load followed by the
// same two filter passes — because the identity is derived from the filtered,
// sealed selected set, not from every parsed diff. See runIdentity for what
// skipping a pass would cost.
//
// Those filter passes are chatty, and the review that follows in the same
// command prints them again, so this stays silent. stdout.Quiet is safe here for
// the reason it documents: this is pre-flight work on the main goroutine, before
// any concurrent output exists.
func ResolveIdentity(ctx context.Context, args Args) (*SealedInput, error) {
	defer stdout.Quiet()()

	resolution := args.SealedInput
	var err error
	if resolution == nil {
		resolution, err = resolveInputBeforeDiff(ctx, args)
	}
	if err != nil {
		return nil, err
	}
	if resolution != nil {
		args.SealedInput = resolution
	}

	a := &Agent{args: args}
	if err := a.loadDiffs(ctx); err != nil {
		return nil, fmt.Errorf("load diffs: %w", err)
	}
	a.diffs = a.filterDiffs(a.diffs)
	a.diffs = a.filterLargeDiffs(a.diffs)
	return &SealedInput{Identity: a.runIdentity(), Resolution: a.inputResolution}, nil
}

// resolveInputBeforeDiff turns every moving head ref into an immutable commit
// before the diff used for admission is loaded. Range mode then computes its
// merge-base against that frozen head; commit mode needs only the frozen head.
func resolveInputBeforeDiff(ctx context.Context, args Args) (*diff.InputResolution, error) {
	switch {
	case args.DiffDir != "":
		ref := args.PatchRef
		if ref == "" {
			ref = "HEAD"
		}
		head, err := resolveCommitHead(ctx, args, ref)
		if err != nil {
			return nil, fmt.Errorf("resolve patch post-image ref %q: %w", ref, err)
		}
		return &diff.InputResolution{ResolvedHead: head}, nil
	case args.Commit != "":
		head, err := resolveCommitHead(ctx, args, args.Commit)
		if err != nil {
			return nil, err
		}
		return &diff.InputResolution{ResolvedHead: head}, nil
	case args.From != "" && args.To != "":
		from, err := resolveCommitHead(ctx, args, args.From)
		if err != nil {
			return nil, err
		}
		head, err := resolveCommitHead(ctx, args, args.To)
		if err != nil {
			return nil, err
		}
		resolved := diff.NewProvider(args.RepoDir, from, head, args.GitRunner).ResolveInput(ctx)
		if resolved.ResolvedBase == "" {
			return nil, fmt.Errorf("resolve merge-base between %q and %q", args.From, args.To)
		}
		return &resolved, nil
	default:
		return nil, nil
	}
}

func resolveCommitHead(ctx context.Context, args Args, ref string) (string, error) {
	head := diff.NewCommitProvider(args.RepoDir, ref, args.GitRunner).ResolveInput(ctx).ResolvedHead
	if head == "" {
		return "", fmt.Errorf("resolve commit %q", ref)
	}
	return head, nil
}

// runIdentity reads the identity off the agent's current selection.
//
// It is only meaningful once the selection is final: sourceArtifactSHA256 hashes
// whatever a.diffs holds, so calling it before both filter passes yields a digest
// no run ever records, and a resume comparing that digest against a parent
// manifest would reject work it should have reused.
func (a *Agent) runIdentity() session.RunIdentity {
	id := session.RunIdentity{
		Mode:                 a.manifestMode(),
		SourceArtifactSHA256: a.sourceArtifactSHA256(),
		RuleConfigSHA256:     a.ruleConfigSHA256(),
	}
	if raw := a.repoRemoteIdentity; raw != "" {
		sum := sha256.Sum256([]byte(raw))
		id.RepositorySHA256 = hex.EncodeToString(sum[:])
	}
	return id
}
