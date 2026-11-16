// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package lore reads locally available Lore revisions through the Lore CLI.
package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/alibaba/open-code-review/internal/vcs"
)

type Repository struct {
	root  string
	scope string
}

func New(root, scope string) *Repository {
	if scope == "." {
		scope = ""
	}
	return &Repository{root: root, scope: scope}
}

// Lore emits newline-delimited events. Read them incrementally so directory
// dumps need only keep their paths, not all of the revision metadata.
func (r *Repository) events(ctx context.Context, visit func(string, json.RawMessage) error, args ...string) error {
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "lore", append([]string{"--offline", "--json", "--no-pager", "--non-interactive"}, args...)...)
	cmd.Dir = r.root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start lore: %w", err)
	}
	dec := json.NewDecoder(stdout)
	var reported string
	completed := false
	for {
		var ev struct {
			TagName string
			Data    json.RawMessage
		}
		err = dec.Decode(&ev)
		if errors.Is(err, io.EOF) {
			err = nil
			break
		}
		if err != nil {
			break
		}
		if ev.TagName == "complete" {
			var result struct {
				Status int
				Error  struct{ Message string }
			}
			if err = json.Unmarshal(ev.Data, &result); err != nil {
				break
			}
			if result.Status == 0 {
				completed = true
			} else {
				reported = result.Error.Message
			}
		} else if visit != nil {
			if err = visit(ev.TagName, ev.Data); err != nil {
				break
			}
		}
	}
	if err != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("decode lore %s: %w", args[0], err)
	}
	if waitErr != nil {
		if reported == "" {
			reported = strings.TrimSpace(stderr.String())
		}
		return fmt.Errorf("lore %s: %w: %s", args[0], waitErr, reported)
	}
	// revision info may emit a subsequent remote lookup failure after the
	// successful local result. The CLI exit status describes the operation.
	if !completed {
		return fmt.Errorf("lore %s did not complete: %s", args[0], reported)
	}
	return nil
}

type revision struct {
	Repository string
	Revision   string
	BranchName string
	Parent     []string
}

func (r *Repository) revision(ctx context.Context, ref string) (revision, error) {
	tag := "repositoryStatusRevision"
	args := []string{"status", "--revision-only"}
	if ref != "" {
		if strings.HasPrefix(ref, "-") {
			return revision{}, fmt.Errorf("invalid Lore revision %q", ref)
		}
		tag, args = "revisionInfo", []string{"revision", "info", ref}
	}
	var rev revision
	err := r.events(ctx, func(t string, data json.RawMessage) error {
		if t == tag {
			return json.Unmarshal(data, &rev)
		}
		return nil
	}, args...)
	if err == nil && (rev.Repository == "" || rev.Revision == "") {
		err = fmt.Errorf("lore returned no revision information for %q", ref)
	}
	return rev, err
}

const zeroRevision = "0000000000000000000000000000000000000000000000000000000000000000"

func (r *Repository) CurrentBranch(ctx context.Context) (string, error) {
	rev, err := r.revision(ctx, "")
	return rev.BranchName, err
}

func (r *Repository) ResolveReview(ctx context.Context, spec vcs.ReviewSpec) (vcs.ResolvedReview, error) {
	var out vcs.ResolvedReview
	if err := spec.Validate(); err != nil {
		return out, err
	}
	if spec.Mode == vcs.MergeBase {
		return out, fmt.Errorf("%w: Lore does not support merge-base review; use --commit or review the working tree", vcs.ErrUnsupported)
	}
	rev, err := r.revision(ctx, spec.Head)
	if err != nil {
		return out, err
	}
	out = vcs.ResolvedReview{Provider: "lore", RepositoryID: rev.Repository, Spec: spec, Base: rev.Revision}
	if spec.Mode != vcs.Workspace {
		out.Head, out.Content.Revision = rev.Revision, rev.Revision
		if spec.Mode == vcs.Commit {
			if len(rev.Parent) == 0 {
				return out, fmt.Errorf("lore revision %q has no parent metadata", rev.Revision)
			}
			out.Base = rev.Parent[0]
		} else {
			base, err := r.revision(ctx, spec.Base)
			if err != nil {
				return out, err
			}
			out.Base = base.Revision
		}
	}
	if out.Base == zeroRevision {
		out.Base, out.BaseIsEmpty = "", true
	}
	return out, nil
}
