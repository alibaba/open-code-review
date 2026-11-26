// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

type Tree struct {
	Repo      string
	Commit    string // Immutable target, or the base of a workspace review.
	Workspace bool
	Git       *gitcmd.Runner
}

func validOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

const maxEvidenceBytes = 1 << 20

func (t Tree) read(ctx context.Context, p string) ([]byte, error) {
	if !validPath(p) {
		return nil, fmt.Errorf("invalid evidence path %q", p)
	}
	if t.Workspace {
		root, err := os.OpenRoot(t.Repo)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		// Reject symlinked components, including aliases to otherwise excluded files.
		for parent := p; parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			info, err := root.Lstat(parent)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("symlink in evidence path %q", p)
			}
			if parent != p {
				continue
			}
			if !info.Mode().IsRegular() || info.Size() > maxEvidenceBytes {
				return nil, fmt.Errorf("evidence must be a regular file up to 1 MiB")
			}
		}
		f, err := root.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxEvidenceBytes+1))
		if len(data) > maxEvidenceBytes {
			return nil, fmt.Errorf("evidence exceeds 1 MiB")
		}
		return data, err
	}
	if !validOID(t.Commit) {
		return nil, fmt.Errorf("expected an immutable commit for memory evidence")
	}
	entry, err := t.Git.Output(ctx, t.Repo, "--literal-pathspecs", "ls-tree", "-lz", t.Commit, "--", p)
	if err != nil {
		return nil, err
	}
	header, _, _ := bytes.Cut(entry, []byte("\t"))
	fields := strings.Fields(string(header))
	if len(fields) != 4 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !validOID(fields[2]) {
		return nil, fmt.Errorf("regular file %q not found at %s", p, t.Commit)
	}
	n, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || n < 0 || n > maxEvidenceBytes {
		return nil, fmt.Errorf("evidence exceeds 1 MiB")
	}
	return t.Git.Output(ctx, t.Repo, "cat-file", "blob", fields[2])
}

func (t Tree) Learn(ctx context.Context, source, scope, property string) ([]Fact, error) {
	if !validSource(source) || !validPath(scope) {
		return nil, fmt.Errorf("source must be a relative .json path and scope a repository-relative path")
	}
	if !validOID(t.Commit) || t.Workspace {
		return nil, fmt.Errorf("record memory at a committed revision")
	}
	data, err := t.read(ctx, source)
	if err != nil {
		return nil, err
	}
	constraints, err := Constraints(data)
	if err != nil {
		return nil, err
	}
	var facts []Fact
	for _, c := range constraints {
		if property != "" && property != c.Property {
			continue
		}
		facts = append(facts, Fact{Source: source, Revision: t.Commit, Scope: scope,
			Property: c.Property, Keyword: c.Keyword, Value: c.Value})
	}
	if len(facts) == 0 {
		return nil, fmt.Errorf("no matching direct property const/enum constraints in %q", source)
	}
	return facts, nil
}

type Checked struct {
	Fact
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (t Tree) Decide(ctx context.Context, source, scope, review, finding, reason string) (Fact, error) {
	if t.Workspace || !validOID(t.Commit) {
		return Fact{}, fmt.Errorf("record a decision at a committed revision")
	}
	if strings.TrimSpace(review) == "" || strings.TrimSpace(finding) == "" || strings.TrimSpace(reason) == "" {
		return Fact{}, fmt.Errorf("a decision requires a review ID, finding and reason")
	}
	if len(review) > 200 || len(finding) > 1000 || len(reason) > 1000 {
		return Fact{}, fmt.Errorf("review ID or decision text is too long")
	}
	evidence, err := t.read(ctx, source)
	if err != nil {
		return Fact{}, err
	}
	code, err := t.read(ctx, scope)
	if err != nil {
		return Fact{}, err
	}
	return Fact{Source: source, Revision: t.Commit, Scope: scope, ReviewID: review, Finding: finding, Reason: reason,
		SourceHash: Digest(evidence), CodeHash: Digest(code)}, nil
}

func (t Tree) Check(ctx context.Context, facts []Fact) ([]Checked, error) {
	type assertions struct {
		values []Constraint
		err    error
	}
	schemas := map[string]assertions{}
	hashes := map[string]string{}
	checkHash := func(f Fact, path, expected, label string) Checked {
		digest, ok := hashes[path]
		if !ok {
			data, err := t.read(ctx, path)
			if err != nil {
				return Checked{f, "unverified", err.Error()}
			}
			digest = Digest(data)
			hashes[path] = digest
		}
		if digest != expected {
			return Checked{f, "changed", label + " changed"}
		}
		return Checked{f, "verified", ""}
	}
	reachable := map[string]bool{}
	result := make([]Checked, 0, len(facts))
	// Several branches can independently establish the same assertion. Use the
	// first reachable observation per stable ID, keeping one diagnostic if none is.
	positions := make(map[string]int)
	for _, f := range facts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		checked := Checked{Fact: f, Status: "unverified"}
		if !validOID(f.Revision) || !validPath(f.Source) || !validPath(f.Scope) || (f.ReviewID == "" && !validSource(f.Source)) {
			return nil, fmt.Errorf("invalid stored evidence for %s", f.ID)
		}
		if _, ok := reachable[f.Revision]; !ok {
			// A shallow checkout can lack the source commit. An unproven ancestor
			// cannot authorize a decision, including on an unrelated release branch.
			if validOID(t.Commit) {
				_, err := t.Git.Output(ctx, t.Repo, "merge-base", "--is-ancestor", f.Revision, t.Commit)
				reachable[f.Revision] = err == nil
			}
		}
		pos, seen := positions[f.ID]
		if seen && result[pos].Status != "unverified" {
			continue
		}
		if !seen {
			pos = len(result)
			positions[f.ID] = pos
			result = append(result, checked)
		}
		if !reachable[f.Revision] {
			if !seen {
				result[pos].Detail = "source revision is not a proven ancestor"
			}
			continue
		}
		if f.ReviewID != "" {
			checked = checkHash(f, f.Source, f.SourceHash, "decision evidence")
			if checked.Status == "verified" {
				checked = checkHash(f, f.Scope, f.CodeHash, "reviewed file")
			}
			result[pos] = checked
			continue
		}
		a, ok := schemas[f.Source]
		if !ok {
			data, err := t.read(ctx, f.Source)
			if err == nil {
				a.values, err = Constraints(data)
			}
			a.err = err
			schemas[f.Source] = a
		}
		if a.err != nil {
			checked.Detail = a.err.Error()
			result[pos] = checked
			continue
		}
		checked.Status = "changed"
		for _, c := range a.values {
			if f.Property == c.Property && f.Keyword == c.Keyword && bytes.Equal(f.Value, c.Value) {
				checked.Status = "verified"
				break
			}
		}
		result[pos] = checked
	}
	return result, ctx.Err()
}
