// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/vcs"
)

func (r *Repository) Changes(ctx context.Context, review vcs.ResolvedReview) (vcs.Changes, error) {
	var result vcs.Changes
	if err := review.Spec.Validate(); err != nil {
		return result, err
	}
	if r.scope != "" || review.Provider != "lore" || (review.Spec.Mode != vcs.Workspace && review.Spec.Mode != vcs.Commit && review.Spec.Mode != vcs.TwoRevisions) {
		return result, fmt.Errorf("%w: invalid Lore comparison", vcs.ErrUnsupported)
	}
	if (review.Base == "") != review.BaseIsEmpty || (review.Head == "") != (review.Spec.Mode == vcs.Workspace) || review.Head != review.Content.Revision {
		return result, fmt.Errorf("invalid resolved Lore endpoints")
	}
	changes := map[string]model.Diff{}
	seenOld := map[string]bool{}
	if !review.BaseIsEmpty {
		args := []string{"diff", "--source", review.Base}
		if review.Head != "" {
			args = append(args, "--target", review.Head)
		}
		err := r.events(ctx, func(tag string, data json.RawMessage) error {
			if tag != "fileDiff" {
				return nil
			}
			var e struct{ Path, Patch, Action string }
			if err := json.Unmarshal(data, &e); err != nil {
				return err
			}
			d, err := normalizeChange(e.Path, e.Patch, e.Action)
			if err != nil {
				return err
			}
			changes[e.Path] = d
			if !d.IsNew {
				seenOld[d.OldPath] = true
			}
			return nil
		}, args...)
		if err != nil {
			return result, err
		}
	}
	// Lore omits an empty-file addition or deletion from its line diff.
	// Compare membership as well, including for the all-added first revision.
	var before []string
	var err error
	if !review.BaseIsEmpty {
		before, err = r.ListFiles(ctx, vcs.ContentRef{Revision: review.Base})
		if err != nil {
			return result, err
		}
	}
	after, err := r.ListFiles(ctx, review.Content)
	if err != nil {
		return result, err
	}
	for _, name := range before {
		if !seenOld[name] {
			if _, found := slices.BinarySearch(after, name); !found {
				changes[name] = model.Diff{OldPath: name, NewPath: "/dev/null", IsDeleted: true}
			}
		}
	}
	for _, name := range after {
		if _, found := changes[name]; found {
			continue
		}
		if _, found := slices.BinarySearch(before, name); !found {
			changes[name] = model.Diff{OldPath: name, NewPath: name, IsNew: true}
		}
	}
	paths := make([]string, 0, len(changes))
	for name := range changes {
		paths = append(paths, name)
	}
	slices.Sort(paths)
	for i, name := range paths {
		d := changes[name]
		if diff.ProviderDirPrefix(name) != "" {
			result.Excluded = append(result.Excluded, d)
			result.ExcludedAt = append(result.ExcludedAt, i)
			continue
		}
		if !d.IsDeleted && !d.IsBinary {
			err := r.loadContent(ctx, review.Content, name, &d)
			if err != nil {
				return result, err
			}
		}
		if d.IsBinary {
			d.Diff = "Binary files differ"
		}
		result.Included = append(result.Included, d)
	}
	return result, nil
}

func normalizeChange(name, patch, action string) (model.Diff, error) {
	if !validPath(name) {
		return model.Diff{}, fmt.Errorf("invalid Lore change path %q", name)
	}
	d := model.Diff{NewPath: name, OldPath: name, Diff: patch}
	switch action {
	case "add":
		d.IsNew = true
	case "delete":
		d.IsDeleted = true
		d.NewPath = "/dev/null"
	case "keep":
	case "move":
		from, _, _ := strings.Cut(patch, "\n")
		old, ok := strings.CutPrefix(from, "move from ")
		if !ok || !validPath(old) {
			return d, fmt.Errorf("invalid Lore move for %q", name)
		}
		d.IsRenamed, d.OldPath = true, old
	default:
		return d, fmt.Errorf("unsupported Lore file action %q", action)
	}
	inHunk := false
	for line := range strings.SplitSeq(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
		case !inHunk && line == "Binary files differ":
			d.IsBinary = true
		case inHunk && strings.HasPrefix(line, "+"):
			d.Insertions++
		case inHunk && strings.HasPrefix(line, "-"):
			d.Deletions++
		}
	}
	d.IsBinary = d.IsBinary || strings.ContainsRune(patch, 0)
	return d, nil
}

func (r *Repository) loadContent(ctx context.Context, ref vcs.ContentRef, name string, d *model.Diff) error {
	return r.ReadFile(ctx, ref, name, func(reader io.Reader) error {
		data, err := io.ReadAll(io.LimitReader(reader, (64<<20)+1))
		if err != nil {
			return err
		}
		if len(data) > 64<<20 || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			d.IsBinary = true
			return nil
		}
		d.NewFileContent = string(data)
		if d.IsNew && d.Diff == "" && len(data) > 0 {
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			d.Insertions = int64(len(lines))
			d.Diff = fmt.Sprintf("--- /dev/null\n+++ %s\n@@ -0,0 +1,%d @@\n+%s\n", name, len(lines), strings.Join(lines, "\n+"))
			if data[len(data)-1] != '\n' {
				d.Diff += "\\ No newline at end of file\n"
			}
		}
		return nil
	})
}
