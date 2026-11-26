// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package knowledge

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

var errIneligibleFile = errors.New("ineligible evidence file")

type Tree struct {
	Repo      string
	Commit    string // Immutable target, or the base of a workspace review.
	Workspace bool
	Git       *gitcmd.Runner
	AllowPath func(string) bool
	Paths     []string
	Now       time.Time
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
	if !t.permits(p) {
		return nil, fmt.Errorf("invalid or excluded evidence path %q", p)
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
	if !validOID(t.Commit) || t.Git == nil {
		return nil, fmt.Errorf("expected an immutable commit for memory evidence")
	}
	entry, err := t.Git.Output(ctx, t.Repo, "--literal-pathspecs", "ls-tree", "-lz", t.Commit, "--", p)
	if err != nil {
		return nil, err
	}
	header, _, _ := bytes.Cut(entry, []byte("\t"))
	fields := strings.Fields(string(header))
	if len(fields) != 4 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !validOID(fields[2]) {
		return nil, fmt.Errorf("%w: regular file %q not found at %s", errIneligibleFile, p, t.Commit)
	}
	n, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || n < 0 || n > maxEvidenceBytes {
		return nil, fmt.Errorf("%w: evidence exceeds 1 MiB", errIneligibleFile)
	}
	return t.Git.Output(ctx, t.Repo, "cat-file", "blob", fields[2])
}

func (t Tree) permits(p string) bool {
	return validPath(p) && (t.AllowPath == nil || t.AllowPath(p))
}

func (t Tree) Learn(ctx context.Context, source, scope, property string) ([]Fact, error) {
	if !validSource(source) || !validScope(scope) {
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
	Status    string   `json:"status"`
	Detail    string   `json:"detail,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
}

func (t Tree) Decide(ctx context.Context, source, scope, review, finding, reason string) (Fact, error) {
	return t.DecideWithCitation(ctx, source, scope, review, finding, reason, 0, 0)
}

func (t Tree) DecideWithCitation(ctx context.Context, source, scope, review, finding, reason string, start, end int) (Fact, error) {
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
	f := Fact{Source: source, Revision: t.Commit, Scope: scope, ReviewID: review, Finding: finding, Reason: reason,
		SourceHash: Digest(evidence), CodeHash: Digest(code), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if start != 0 || end != 0 {
		c, err := newCitationFile(source, evidence).cite(source, t.Commit, start, end)
		if err != nil {
			return Fact{}, err
		}
		f.Citations = []Citation{c}
		f.Category, f.Authority = CategoryDecision, AuthorityHistorical
	}
	return f, nil
}
